package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/home-operations/talup/api/v1alpha1"
)

// KubernetesReconciler reconciles a Kubernetes object
type KubernetesPlanReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	TalosConfigSecret string
}

// +kubebuilder:rbac:groups=talup.home-operations.com,resources=kubernetes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=talup.home-operations.com,resources=kubernetes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=talup.home-operations.com,resources=kubernetes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Kubernetes object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *KubernetesPlanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubernetesPlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&upgradev1alpha1.KubernetesPlan{}).
		Named("kubernetes").
		Complete(r)
}
