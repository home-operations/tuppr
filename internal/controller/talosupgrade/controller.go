package talosupgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
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
)

// TalosClient defines the interface for Talos operations
type TalosClient interface {
	GetNodeVersion(ctx context.Context, nodeIP string) (string, error)
	CheckNodeReady(ctx context.Context, nodeIP, nodeName string) error
	GetNodeInstallImage(ctx context.Context, nodeIP string) (string, error)
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
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

type Reconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	TalosConfigSecret   string
	ControllerNamespace string
	HealthChecker       HealthCheckRunner
	TalosClient         TalosClient
	MetricsReporter     *metrics.Reporter
	Now                 Now
	ImageChecker        ImageChecker
	Notifier            notification.Notifier
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Starting reconciliation", "talosupgrade", req.Name)

	var talosUpgrade tupprv1alpha1.TalosUpgrade
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name}, &talosUpgrade); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

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
		talosClient, err := talos.NewClient(context.Background())
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
		Complete(r)
}

func (r *Reconciler) updateStatus(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, updates map[string]any) error {
	if _, ok := updates["observedGeneration"]; !ok {
		updates["observedGeneration"] = talosUpgrade.Generation
	}
	updates["lastUpdated"] = metav1.Now()

	patch := map[string]any{"status": updates}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	statusObj := &tupprv1alpha1.TalosUpgrade{ObjectMeta: metav1.ObjectMeta{Name: talosUpgrade.Name}}
	return r.Status().Patch(ctx, statusObj, client.RawPatch(types.MergePatchType, patchBytes))
}

func (r *Reconciler) setPhase(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, phase tupprv1alpha1.JobPhase, currentNode, message string) error {
	prevPhase := talosUpgrade.Status.Phase

	totalNodes, err := r.getTotalNodeCount(ctx)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to get total node count for metrics")
	}

	if err := r.updateStatus(ctx, talosUpgrade, map[string]any{
		"phase":       phase,
		"currentNode": currentNode,
		"message":     message,
	}); err != nil {
		return err
	}
	talosUpgrade.Status.Phase = phase
	r.recordPhaseTransition(talosUpgrade, prevPhase, phase)
	r.MetricsReporter.RecordTalosUpgradeNodes(
		talosUpgrade.Name,
		totalNodes,
		len(talosUpgrade.Status.CompletedNodes),
		len(talosUpgrade.Status.FailedNodes),
	)
	return nil
}

func (r *Reconciler) recordPhaseTransition(talosUpgrade *tupprv1alpha1.TalosUpgrade, fromPhase, toPhase tupprv1alpha1.JobPhase) {
	r.MetricsReporter.RecordTalosUpgradePhase(talosUpgrade.Name, string(toPhase))
	if fromPhase != toPhase {
		if fromPhase != "" {
			r.MetricsReporter.EndPhaseTiming(metrics.UpgradeTypeTalos, talosUpgrade.Name, string(fromPhase))
		}
		r.MetricsReporter.StartPhaseTiming(metrics.UpgradeTypeTalos, talosUpgrade.Name, string(toPhase))
	}
}

func (r *Reconciler) addCompletedNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) error {
	return r.updateStatus(ctx, talosUpgrade, map[string]any{
		"completedNodes": append(talosUpgrade.Status.CompletedNodes, nodeName),
	})
}

func (r *Reconciler) addFailedNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeStatus tupprv1alpha1.NodeUpgradeStatus) error {
	return r.updateStatus(ctx, talosUpgrade, map[string]any{
		"failedNodes": append(talosUpgrade.Status.FailedNodes, nodeStatus),
	})
}

func (r *Reconciler) getTotalNodeCount(ctx context.Context) (int, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return 0, err
	}
	return len(nodeList.Items), nil
}
