package talosupgrade

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/alerting"
	"github.com/home-operations/tuppr/internal/controller/jobs"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
	"github.com/home-operations/tuppr/internal/controller/upgradeaudit"
	"github.com/home-operations/tuppr/internal/healthcheck"
	"github.com/home-operations/tuppr/internal/image"
	"github.com/home-operations/tuppr/internal/metrics"
	"github.com/home-operations/tuppr/internal/notification"
	"github.com/home-operations/tuppr/internal/talos"
)

const (
	TalosUpgradeFinalizer        = "tuppr.home-operations.com/talos-finalizer"
	TalosJobBackoffLimit         = 2
	TalosJobGracePeriod          = 300
	TalosJobTTLAfterFinished     = 300
	TalosJobActiveDeadlineBuffer = 600
	TalosJobDefaultTimeout       = 30 * time.Minute
	PlacementSoft                = "soft"
	PlacementHard                = "hard"
)

const (
	appLabelKey              = jobs.AppLabelKey
	appInstanceLabelKey      = jobs.AppInstanceLabelKey
	appPartOfLabelKey        = jobs.AppPartOfLabelKey
	appPartOfTuppr           = jobs.AppPartOfTuppr
	targetNodeLabelKey       = jobs.TargetNodeLabelKey
	talosUpgradeAppName      = "talos-upgrade"
	statusAlertSilenceIDs    = "alertSilenceIDs"
	statusAlertSilencesSince = "alertSilencesSince"
	statusCompletedNodes     = "completedNodes"
	statusFailedNodes        = "failedNodes"
	statusRebootingNodes     = "rebootingNodes"
	statusPreHookFailed      = "preHookFailed"
	statusPreHookIndex       = "preHookIndex"
	statusPostHookIndex      = "postHookIndex"
)

// TalosClient defines the interface for Talos operations
type TalosClient interface {
	GetNodeVersion(ctx context.Context, nodeIP string) (string, error)
	CheckNodeReady(ctx context.Context, nodeIP, nodeName string) error
	GetNodeInstallImage(ctx context.Context, nodeIP string) (string, error)
	GetNodeExtensions(ctx context.Context, nodeIP string) (talos.ExtensionInfo, error)
	PatchNodeInstallImage(ctx context.Context, nodeIP, newImage string) error
}

// ImageChecker defines the interface for checking image availability
type ImageChecker interface {
	Check(ctx context.Context, imageRef string) error
}

// HealthCheckRunner defines the interface for health checking
type HealthCheckRunner interface {
	CheckHealth(ctx context.Context, healthChecks []tupprv1alpha1.HealthCheckSpec) error
}

// Now defines the interface for time operations
type Now interface {
	Now() time.Time
}

// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades/finalizers,verbs=update
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=kubernetesupgrades,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=volumeattachments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

type Reconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	TalosConfigSecret   string
	ControllerNamespace string
	ControllerNodeName  string
	ControllerPodName   string
	HealthChecker       HealthCheckRunner
	TalosClient         TalosClient
	MetricsReporter     *metrics.Reporter
	Now                 Now
	ImageChecker        ImageChecker
	Notifier            notification.Notifier
	Renderer            *notification.Renderer
	Silencer            alerting.Silencer
	Recorder            record.EventRecorder

	// silenceWarnings latches once-per-CR silence Warning events (see
	// silenceWarnOnce) so steady-state conditions don't spam the apiserver.
	silenceWarnings sync.Map
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Starting reconciliation", "talosupgrade", req.Name)

	var talosUpgrade tupprv1alpha1.TalosUpgrade
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name}, &talosUpgrade); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	r.MetricsReporter.Initialize(talosUpgrade.Name, metrics.UpgradeTypeTalos)
	r.syncMetricsFromStatus(&talosUpgrade)

	if talosUpgrade.DeletionTimestamp != nil {
		return r.cleanup(ctx, &talosUpgrade)
	}

	if !controllerutil.ContainsFinalizer(&talosUpgrade, TalosUpgradeFinalizer) {
		controllerutil.AddFinalizer(&talosUpgrade, TalosUpgradeFinalizer)
		return ctrl.Result{}, r.Update(ctx, &talosUpgrade)
	}

	return r.processUpgrade(ctx, &talosUpgrade)
}

func (r *Reconciler) cleanup(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Cleaning up TalosUpgrade", "name", talosUpgrade.Name)

	r.clearOutdatedTaints(ctx, talosUpgrade)

	// Best-effort: a deleted mid-run CR shouldn't leave its silences open for
	// the full TTL. On failure the leases lapse on their own.
	if r.Silencer != nil {
		amCtx, cancel := context.WithTimeout(ctx, silenceSyncTimeout)
		for _, id := range talosUpgrade.Status.AlertSilenceIDs {
			r.expireSilence(amCtx, talosUpgrade, id)
		}
		cancel()
	}
	r.silenceWarnings.Delete(silenceWarningKey(talosUpgrade, "SilenceMaxDurationReached"))
	r.silenceWarnings.Delete(silenceWarningKey(talosUpgrade, "SilencesNotConfigured"))

	logger.V(1).Info("Removing finalizer", "name", talosUpgrade.Name, "finalizer", TalosUpgradeFinalizer)
	controllerutil.RemoveFinalizer(talosUpgrade, TalosUpgradeFinalizer)

	if err := r.Update(ctx, talosUpgrade); err != nil {
		logger.Error(err, "Failed to remove finalizer", "name", talosUpgrade.Name)
		return ctrl.Result{}, err
	}

	r.MetricsReporter.CleanupUpgradeMetrics(metrics.UpgradeTypeTalos, talosUpgrade.Name)
	logger.V(1).Info("Successfully cleaned up TalosUpgrade", "name", talosUpgrade.Name)
	return ctrl.Result{}, nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := ctrl.Log.WithName("setup")
	logger.V(1).Info("Setting up TalosUpgrade controller with manager")

	if r.MetricsReporter == nil {
		r.MetricsReporter = metrics.NewReporter()
	}
	if r.HealthChecker == nil {
		r.HealthChecker = healthcheck.NewChecker(mgr.GetClient(), r.MetricsReporter)
	}
	if r.TalosClient == nil {
		// Pin Talos endpoints to control-plane IPs so the client survives a CoreDNS
		// drain mid-upgrade. Uncached reader: the manager cache isn't synced yet here.
		apiReader := mgr.GetAPIReader()
		talosClient, err := talos.NewClient(context.Background(),
			talos.WithEndpointResolver(func(ctx context.Context) []string {
				return nodeutil.ControlPlaneEndpointIPs(ctx, apiReader, controlPlaneLabel)
			}))
		if err != nil {
			return fmt.Errorf("failed to create talos client: %w", err)
		}
		r.TalosClient = talosClient
	}
	if r.Now == nil {
		r.Now = &nodeutil.Clock{}
	}
	if r.ImageChecker == nil {
		r.ImageChecker = image.NewChecker()
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&tupprv1alpha1.TalosUpgrade{}).
		Owns(&batchv1.Job{}).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(r.nodeToTalosUpgrades),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc:  func(event.CreateEvent) bool { return true },
				UpdateFunc:  func(event.UpdateEvent) bool { return false },
				DeleteFunc:  func(event.DeleteEvent) bool { return false },
				GenericFunc: func(event.GenericEvent) bool { return false },
			}),
		).
		Complete(r)
}

func (r *Reconciler) nodeToTalosUpgrades(ctx context.Context, _ client.Object) []reconcile.Request {
	var list tupprv1alpha1.TalosUpgradeList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(list.Items))
	for _, tu := range list.Items {
		if tu.Status.Phase == tupprv1alpha1.JobPhaseCompleted {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: tu.Name},
			})
		}
	}
	return requests
}

func (r *Reconciler) updateStatus(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, updates map[string]any) error {
	statusObj := &tupprv1alpha1.TalosUpgrade{ObjectMeta: metav1.ObjectMeta{Name: talosUpgrade.Name}}
	if err := upgradeaudit.PatchStatus(ctx, r.Client, statusObj, talosUpgrade.Generation, updates); err != nil {
		return err
	}
	// Carry the bumped resourceVersion onto the in-memory object so a later
	// full-object Update in the same reconcile (e.g. annotation removal)
	// doesn't 409 against our own status patch.
	talosUpgrade.ResourceVersion = statusObj.ResourceVersion
	return nil
}

func (r *Reconciler) setPhase(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, phase tupprv1alpha1.JobPhase, message string) error {
	return r.setPhaseWithNodes(ctx, talosUpgrade, phase, nil, message)
}

func (r *Reconciler) setPhaseWithNodes(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, phase tupprv1alpha1.JobPhase, currentNodes []string, message string) error {
	return r.setPhaseWithUpdates(ctx, talosUpgrade, phase, "", currentNodes, message, nil)
}

// setPendingWithReason pins the Progressing condition's Reason and message.
func (r *Reconciler) setPendingWithReason(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, reason, message string) error {
	return r.setPhaseWithUpdates(ctx, talosUpgrade, tupprv1alpha1.JobPhasePending, reason, nil, message, nil)
}

// reportReconcileError logs the failure, writes a Pending status with the
// given reason, and returns the requeue. The caller should `return result, nil`.
// op is the failed operation (e.g. "find next nodes") for the log and message.
func (r *Reconciler) reportReconcileError(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, reason, op string, requeue time.Duration, err error) ctrl.Result {
	logger := log.FromContext(ctx)
	logger.Error(err, "Failed to "+op, "reason", reason)
	if setErr := r.setPendingWithReason(ctx, talosUpgrade, reason, fmt.Sprintf("Cannot %s: %s", op, err.Error())); setErr != nil {
		logger.Error(setErr, "Failed to update status", "op", op)
	}
	return ctrl.Result{RequeueAfter: requeue}
}

// setPhaseWithUpdates writes phase plus any additional status fields atomically
// and runs the shared audit/event/metric bookkeeping. All phase transitions
// must go through this function (directly or via setPhase/setPhaseWithNodes).
// No-ops when the resulting status would be identical to the current one.
func (r *Reconciler) setPhaseWithUpdates(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, phase tupprv1alpha1.JobPhase, reason string, currentNodes []string, message string, extra map[string]any) error {
	prevPhase := talosUpgrade.Status.Phase

	currentNode := ""
	if len(currentNodes) > 0 {
		currentNode = currentNodes[0]
	}

	conditions := upgradeaudit.ApplyConditions(talosUpgrade.Status.Conditions, phase, reason, message, talosUpgrade.Generation)

	if len(extra) == 0 &&
		prevPhase == phase &&
		talosUpgrade.Status.Message == message &&
		slices.Equal(talosUpgrade.Status.CurrentNodes, currentNodes) &&
		upgradeaudit.ConditionsEqual(talosUpgrade.Status.Conditions, conditions) {
		return nil
	}

	totalNodes, err := r.getTotalNodeCount(ctx)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to get total node count for metrics")
	}

	updates := map[string]any{
		"phase":        phase,
		"currentNode":  currentNode,
		"currentNodes": currentNodes,
		"message":      message,
		"conditions":   conditions,
	}
	maps.Copy(updates, extra)
	applyPhaseAuditFields(&talosUpgrade.Status, updates, phase, metav1.Now(), talosUpgrade.Spec.Talos.Version)

	if err := r.updateStatus(ctx, talosUpgrade, updates); err != nil {
		return err
	}
	talosUpgrade.Status.Phase = phase
	talosUpgrade.Status.CurrentNodes = currentNodes
	talosUpgrade.Status.Message = message
	talosUpgrade.Status.Conditions = conditions
	syncLocalAuditFields(&talosUpgrade.Status, updates)
	r.recordPhaseTransition(talosUpgrade, prevPhase, phase)
	r.emitPhaseEvent(talosUpgrade, prevPhase, phase, message)
	if prog := meta.FindStatusCondition(conditions, tupprv1alpha1.ConditionTypeProgressing); prog != nil {
		r.MetricsReporter.RecordProgressing(metrics.UpgradeTypeTalos, talosUpgrade.Name, prog.Reason, prog.Status == metav1.ConditionTrue)
	}
	r.MetricsReporter.RecordTalosUpgradeNodes(
		talosUpgrade.Name,
		totalNodes,
		len(talosUpgrade.Status.CompletedNodes),
		len(talosUpgrade.Status.FailedNodes),
	)
	return nil
}

// syncMetricsFromStatus re-emits gauges from CR status so an operator pod that
// starts after a terminal transition reflects the right state. The completion
// counter is intentionally not touched — only recordPhaseTransition increments it.
func (r *Reconciler) syncMetricsFromStatus(talosUpgrade *tupprv1alpha1.TalosUpgrade) {
	phase := talosUpgrade.Status.Phase
	if phase == "" {
		return
	}

	currentNode := ""
	if len(talosUpgrade.Status.CurrentNodes) > 0 {
		currentNode = talosUpgrade.Status.CurrentNodes[0]
	}
	r.MetricsReporter.RecordTalosUpgradePhase(talosUpgrade.Name, string(phase), currentNode)

	if phase.IsTerminal() {
		completed := len(talosUpgrade.Status.CompletedNodes)
		failed := len(talosUpgrade.Status.FailedNodes)
		r.MetricsReporter.RecordTalosUpgradeNodes(talosUpgrade.Name, completed+failed, completed, failed)
		if talosUpgrade.Status.CompletedAt != nil {
			r.MetricsReporter.RecordLastCompletionTimestamp(
				metrics.UpgradeTypeTalos,
				talosUpgrade.Name,
				metrics.TerminalResult(phase),
				talosUpgrade.Status.CompletedAt.Time,
			)
		}
	}
}

func (r *Reconciler) recordPhaseTransition(talosUpgrade *tupprv1alpha1.TalosUpgrade, fromPhase, toPhase tupprv1alpha1.JobPhase) {
	currentNode := ""
	if len(talosUpgrade.Status.CurrentNodes) > 0 {
		currentNode = talosUpgrade.Status.CurrentNodes[0]
	}
	r.MetricsReporter.RecordTalosUpgradePhase(talosUpgrade.Name, string(toPhase), currentNode)
	if fromPhase != toPhase {
		if fromPhase != "" {
			r.MetricsReporter.EndPhaseTiming(metrics.UpgradeTypeTalos, talosUpgrade.Name, string(fromPhase))
		}
		r.MetricsReporter.StartPhaseTiming(metrics.UpgradeTypeTalos, talosUpgrade.Name, string(toPhase))
		if toPhase.IsTerminal() {
			r.MetricsReporter.RecordUpgradeCompleted(metrics.UpgradeTypeTalos, talosUpgrade.Name, metrics.TerminalResult(toPhase))
		}
	}
}

func (r *Reconciler) addCompletedNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) error {
	talosUpgrade.Status.CompletedNodes = append(talosUpgrade.Status.CompletedNodes, nodeName)
	return r.updateStatus(ctx, talosUpgrade, map[string]any{
		statusCompletedNodes: talosUpgrade.Status.CompletedNodes,
	})
}

// getParallelism returns the effective parallelism value, defaulting to 1.
func getParallelism(spec tupprv1alpha1.TalosUpgradeSpec) int {
	if spec.Parallelism != nil && *spec.Parallelism > 0 {
		return int(*spec.Parallelism)
	}
	return 1
}

func (r *Reconciler) addFailedNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeStatus tupprv1alpha1.NodeUpgradeStatus) error {
	talosUpgrade.Status.FailedNodes = append(talosUpgrade.Status.FailedNodes, nodeStatus)
	return r.updateStatus(ctx, talosUpgrade, map[string]any{
		statusFailedNodes: talosUpgrade.Status.FailedNodes,
	})
}

// trackRebootingNodes ensures each named node has a reboot-wait entry with a
// deadline of now + the per-node upgrade timeout. Existing entries keep their
// original deadline so requeues don't push the deadline out.
func (r *Reconciler) trackRebootingNodes(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeNames []string) error {
	deadline := metav1.NewTime(r.Now.Now().Add(nodeUpgradeTimeout(talosUpgrade)))
	entries := talosUpgrade.Status.RebootingNodes
	changed := false
	for _, nodeName := range nodeNames {
		if slices.ContainsFunc(entries, func(e tupprv1alpha1.NodeRebootStatus) bool {
			return e.NodeName == nodeName
		}) {
			continue
		}
		entries = append(entries, tupprv1alpha1.NodeRebootStatus{NodeName: nodeName, Deadline: deadline})
		changed = true
	}
	if !changed {
		return nil
	}
	talosUpgrade.Status.RebootingNodes = entries
	return r.updateStatus(ctx, talosUpgrade, map[string]any{statusRebootingNodes: entries})
}

// clearRebootTracking drops the reboot-wait entry for the node, if present.
func (r *Reconciler) clearRebootTracking(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) error {
	idx := slices.IndexFunc(talosUpgrade.Status.RebootingNodes, func(e tupprv1alpha1.NodeRebootStatus) bool {
		return e.NodeName == nodeName
	})
	if idx < 0 {
		return nil
	}
	entries := slices.Delete(slices.Clone(talosUpgrade.Status.RebootingNodes), idx, idx+1)
	talosUpgrade.Status.RebootingNodes = entries
	return r.updateStatus(ctx, talosUpgrade, map[string]any{statusRebootingNodes: entries})
}

// rebootDeadlineExpired reports whether the node has a tracked reboot deadline
// in the past.
func (r *Reconciler) rebootDeadlineExpired(talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) bool {
	for _, e := range talosUpgrade.Status.RebootingNodes {
		if e.NodeName == nodeName {
			return r.Now.Now().After(e.Deadline.Time)
		}
	}
	return false
}

func (r *Reconciler) getTotalNodeCount(ctx context.Context) (int, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return 0, err
	}
	return len(nodeList.Items), nil
}
