package kubernetesupgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
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
	"github.com/home-operations/tuppr/internal/controller/jobs"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
	"github.com/home-operations/tuppr/internal/controller/upgradeaudit"
	"github.com/home-operations/tuppr/internal/healthcheck"
	"github.com/home-operations/tuppr/internal/metrics"
	"github.com/home-operations/tuppr/internal/talos"
)

const (
	KubernetesUpgradeFinalizer    = "tuppr.home-operations.com/kubernetes-finalizer"
	KubernetesJobBackoffLimit     = 5
	KubernetesJobActiveDeadline   = 3600
	KubernetesJobGracePeriod      = 300
	KubernetesJobTTLAfterFinished = 300
)

const (
	appLabelKey               = jobs.AppLabelKey
	kubernetesUpgradeAppName  = "kubernetes-upgrade"
	statusFieldJobName        = "jobName"
	statusFieldCurrentVersion = "currentVersion"
	statusFieldTargetVersion  = "targetVersion"
	statusFieldLastError      = "lastError"
	targetNodeLabelKey        = jobs.TargetNodeLabelKey
	upgradeK8sCommand         = "upgrade-k8s"

	// Fallback when Reconciler.DefaultEndpoint is unset (tests).
	defaultKubernetesAPIEndpoint = "https://kubernetes.default.svc.cluster.local:443"
)

// TalosClient defines the interface for Talos operations
type TalosClient interface {
	GetNodeVersion(ctx context.Context, nodeIP string) (string, error)
}

// HealthCheckRunner defines the interface for health checking
type HealthCheckRunner interface {
	CheckHealth(ctx context.Context, healthChecks []tupprv1alpha1.HealthCheckSpec) error
}

// VersionGetter defines the interface for getting Kubernetes version
type VersionGetter interface {
	GetCurrentKubernetesVersion(ctx context.Context) (string, error)
}

// Now defines the interface for time operations
type Now interface {
	Now() time.Time
}

// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=kubernetesupgrades,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=kubernetesupgrades/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=kubernetesupgrades/finalizers,verbs=update
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

type Reconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	TalosConfigSecret   string
	ControllerNamespace string
	ControllerNodeName  string
	DefaultEndpoint     string
	HealthChecker       HealthCheckRunner
	TalosClient         TalosClient
	MetricsReporter     *metrics.Reporter
	VersionGetter       VersionGetter
	Now                 Now
	Recorder            record.EventRecorder
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Starting KubernetesUpgrade reconciliation", "kubernetesupgrade", req.Name)

	var kubernetesUpgrade tupprv1alpha1.KubernetesUpgrade
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name}, &kubernetesUpgrade); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	r.MetricsReporter.Initialize(kubernetesUpgrade.Name, metrics.UpgradeTypeKubernetes)
	r.syncMetricsFromStatus(&kubernetesUpgrade)

	if kubernetesUpgrade.DeletionTimestamp != nil {
		return r.cleanup(ctx, &kubernetesUpgrade)
	}

	if !controllerutil.ContainsFinalizer(&kubernetesUpgrade, KubernetesUpgradeFinalizer) {
		controllerutil.AddFinalizer(&kubernetesUpgrade, KubernetesUpgradeFinalizer)
		return ctrl.Result{}, r.Update(ctx, &kubernetesUpgrade)
	}

	return r.processUpgrade(ctx, &kubernetesUpgrade)
}

func (r *Reconciler) cleanup(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Cleaning up KubernetesUpgrade", "name", kubernetesUpgrade.Name)

	logger.V(1).Info("Removing finalizer", "name", kubernetesUpgrade.Name, "finalizer", KubernetesUpgradeFinalizer)
	controllerutil.RemoveFinalizer(kubernetesUpgrade, KubernetesUpgradeFinalizer)

	if err := r.Update(ctx, kubernetesUpgrade); err != nil {
		logger.Error(err, "Failed to remove finalizer", "name", kubernetesUpgrade.Name)
		return ctrl.Result{}, err
	}

	r.MetricsReporter.CleanupUpgradeMetrics(metrics.UpgradeTypeKubernetes, kubernetesUpgrade.Name)
	logger.V(1).Info("Successfully cleaned up KubernetesUpgrade", "name", kubernetesUpgrade.Name)
	return ctrl.Result{}, nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := ctrl.Log.WithName("setup")
	logger.V(1).Info("Setting up KubernetesUpgrade controller with manager")

	if r.MetricsReporter == nil {
		r.MetricsReporter = metrics.NewReporter()
	}
	if r.HealthChecker == nil {
		r.HealthChecker = healthcheck.NewChecker(mgr.GetClient(), r.MetricsReporter)
	}
	if r.VersionGetter == nil {
		discoveryClient, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
		if err != nil {
			return fmt.Errorf("failed to create discovery client: %w", err)
		}
		r.VersionGetter = &DiscoveryVersionGetter{client: discoveryClient}
	}
	if r.TalosClient == nil {
		talosClient, err := talos.NewClient(context.Background())
		if err != nil {
			return fmt.Errorf("failed to create talos client: %w", err)
		}
		r.TalosClient = talosClient
	}
	if r.Now == nil {
		r.Now = &nodeutil.Clock{}
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&tupprv1alpha1.KubernetesUpgrade{}).
		Owns(&batchv1.Job{}).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(r.nodeToKubernetesUpgrades),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc:  func(event.CreateEvent) bool { return true },
				UpdateFunc:  func(event.UpdateEvent) bool { return false },
				DeleteFunc:  func(event.DeleteEvent) bool { return false },
				GenericFunc: func(event.GenericEvent) bool { return false },
			}),
		).
		Complete(r)
}

func (r *Reconciler) nodeToKubernetesUpgrades(ctx context.Context, _ client.Object) []reconcile.Request {
	var list tupprv1alpha1.KubernetesUpgradeList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(list.Items))
	for _, ku := range list.Items {
		if ku.Status.Phase == tupprv1alpha1.JobPhaseCompleted {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: ku.Name},
			})
		}
	}
	return requests
}

func (r *Reconciler) updateStatus(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, updates map[string]any) error {
	if _, ok := updates["observedGeneration"]; !ok {
		updates["observedGeneration"] = kubernetesUpgrade.Generation
	}
	updates["lastUpdated"] = metav1.Now()

	patch := map[string]any{
		"status": updates,
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	statusObj := &tupprv1alpha1.KubernetesUpgrade{}
	statusObj.Name = kubernetesUpgrade.Name

	return r.Status().Patch(ctx, statusObj, client.RawPatch(types.MergePatchType, patchBytes))
}

func (r *Reconciler) setPhase(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, phase tupprv1alpha1.JobPhase, controllerNode, message string) error {
	return r.setPhaseWithUpdates(ctx, kubernetesUpgrade, phase, "", controllerNode, message, nil)
}

// setPhaseWithReason is like setPhase but pins the Progressing condition's Reason.
func (r *Reconciler) setPhaseWithReason(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, phase tupprv1alpha1.JobPhase, reason, controllerNode, message string) error {
	return r.setPhaseWithUpdates(ctx, kubernetesUpgrade, phase, reason, controllerNode, message, nil)
}

// setPhaseWithUpdates writes phase plus any additional status fields atomically
// and runs the shared audit/event/metric bookkeeping. All phase transitions
// must go through this function (directly or via setPhase).
// No-ops when the resulting status would be identical to the current one.
func (r *Reconciler) setPhaseWithUpdates(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, phase tupprv1alpha1.JobPhase, reason, controllerNode, message string, extra map[string]any) error {
	prevPhase := kubernetesUpgrade.Status.Phase

	conditions := upgradeaudit.ApplyConditions(kubernetesUpgrade.Status.Conditions, phase, reason, message, kubernetesUpgrade.Generation)

	if len(extra) == 0 &&
		prevPhase == phase &&
		kubernetesUpgrade.Status.Message == message &&
		kubernetesUpgrade.Status.ControllerNode == controllerNode &&
		upgradeaudit.ConditionsEqual(kubernetesUpgrade.Status.Conditions, conditions) {
		return nil
	}

	updates := map[string]any{
		"phase":          phase,
		"controllerNode": controllerNode,
		"message":        message,
		"conditions":     conditions,
	}
	maps.Copy(updates, extra)
	applyPhaseAuditFields(&kubernetesUpgrade.Status, updates, phase, metav1.Now(), message)

	if err := r.updateStatus(ctx, kubernetesUpgrade, updates); err != nil {
		return err
	}
	kubernetesUpgrade.Status.Phase = phase
	kubernetesUpgrade.Status.ControllerNode = controllerNode
	kubernetesUpgrade.Status.Message = message
	kubernetesUpgrade.Status.Conditions = conditions
	syncLocalAuditFields(&kubernetesUpgrade.Status, updates)
	r.recordPhaseTransition(kubernetesUpgrade, prevPhase, phase)
	r.emitPhaseEvent(kubernetesUpgrade, prevPhase, phase, message)
	if prog := meta.FindStatusCondition(conditions, tupprv1alpha1.ConditionTypeProgressing); prog != nil {
		r.MetricsReporter.RecordProgressing(metrics.UpgradeTypeKubernetes, kubernetesUpgrade.Name, prog.Reason, prog.Status == metav1.ConditionTrue)
	}
	return nil
}

func (r *Reconciler) syncMetricsFromStatus(kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) {
	phase := kubernetesUpgrade.Status.Phase
	if phase == "" {
		return
	}

	r.MetricsReporter.RecordKubernetesUpgradePhase(kubernetesUpgrade.Name, string(phase))

	if phase.IsTerminal() && kubernetesUpgrade.Status.CompletedAt != nil {
		r.MetricsReporter.RecordLastCompletionTimestamp(
			metrics.UpgradeTypeKubernetes,
			kubernetesUpgrade.Name,
			metrics.TerminalResult(phase),
			kubernetesUpgrade.Status.CompletedAt.Time,
		)
	}
}

func (r *Reconciler) recordPhaseTransition(kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, fromPhase, toPhase tupprv1alpha1.JobPhase) {
	r.MetricsReporter.RecordKubernetesUpgradePhase(kubernetesUpgrade.Name, string(toPhase))
	if fromPhase != toPhase {
		if fromPhase != "" {
			r.MetricsReporter.EndPhaseTiming(metrics.UpgradeTypeKubernetes, kubernetesUpgrade.Name, string(fromPhase))
		}
		r.MetricsReporter.StartPhaseTiming(metrics.UpgradeTypeKubernetes, kubernetesUpgrade.Name, string(toPhase))
		if toPhase.IsTerminal() {
			r.MetricsReporter.RecordUpgradeCompleted(metrics.UpgradeTypeKubernetes, kubernetesUpgrade.Name, metrics.TerminalResult(toPhase))
		}
	}
}

type DiscoveryVersionGetter struct {
	client discovery.DiscoveryInterface
}

func (d *DiscoveryVersionGetter) GetCurrentKubernetesVersion(ctx context.Context) (string, error) {
	serverVersion, err := d.client.ServerVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get server version: %w", err)
	}
	return serverVersion.GitVersion, nil
}
