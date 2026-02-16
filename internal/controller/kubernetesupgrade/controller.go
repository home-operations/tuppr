package kubernetesupgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
	"github.com/home-operations/tuppr/internal/healthcheck"
	"github.com/home-operations/tuppr/internal/metrics"
	"github.com/home-operations/tuppr/internal/talos"
)

const (
	KubernetesUpgradeFinalizer    = "tuppr.home-operations.com/kubernetes-finalizer"
	KubernetesJobBackoffLimit     = 2
	KubernetesJobActiveDeadline   = 3600
	KubernetesJobGracePeriod      = 300
	KubernetesJobTTLAfterFinished = 300
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
	HealthChecker       HealthCheckRunner
	TalosClient         TalosClient
	MetricsReporter     *metrics.Reporter
	VersionGetter       VersionGetter
	Now                 Now
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Starting KubernetesUpgrade reconciliation", "kubernetesupgrade", req.Name)

	var kubernetesUpgrade tupprv1alpha1.KubernetesUpgrade
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name}, &kubernetesUpgrade); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

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
		r.VersionGetter = &DiscoveryVersionGetter{}
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
		Complete(r)
}

func (r *Reconciler) updateStatus(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, updates map[string]any) error {
	updates["observedGeneration"] = kubernetesUpgrade.Generation
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
	r.MetricsReporter.RecordKubernetesUpgradePhase(kubernetesUpgrade.Name, string(phase))

	return r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":          phase,
		"controllerNode": controllerNode,
		"message":        message,
	})
}

type DiscoveryVersionGetter struct{}

func (d *DiscoveryVersionGetter) GetCurrentKubernetesVersion(ctx context.Context) (string, error) {
	config, err := ctrl.GetConfig()
	if err != nil {
		return "", fmt.Errorf("failed to get REST config: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create discovery client: %w", err)
	}

	serverVersion, err := discoveryClient.ServerVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get server version: %w", err)
	}

	return serverVersion.GitVersion, nil
}
