package webhook

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	talosclientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"

	upgradev1alpha1 "github.com/home-operations/talup/api/v1alpha1"
)

// log is for logging in this package.
var kuberneteslog = logf.Log.WithName("kubernetes-resource")

// KubernetesPlanValidator validates Kubernetes resources
type KubernetesPlanValidator struct {
	Client            client.Client
	TalosConfigSecret string
}

//+kubebuilder:webhook:path=/validate-upgrade-home-operations-com-v1alpha1-kubernetes,mutating=false,failurePolicy=fail,sideEffects=None,groups=upgrade.home-operations.com,resources=kuberneteses,verbs=create;update,versions=v1alpha1,name=vkubernetes.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &KubernetesPlanValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *KubernetesPlanValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	kubernetes := obj.(*upgradev1alpha1.KubernetesPlan)
	kuberneteslog.Info("validate create", "name", kubernetes.Name, "talosConfigSecret", v.TalosConfigSecret)

	return v.validateKubernetes(ctx, kubernetes)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *KubernetesPlanValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	kubernetes := newObj.(*upgradev1alpha1.KubernetesPlan)
	kuberneteslog.Info("validate update", "name", kubernetes.Name, "talosConfigSecret", v.TalosConfigSecret)

	return v.validateKubernetes(ctx, kubernetes)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (v *KubernetesPlanValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// No validation needed for delete
	return nil, nil
}

func (v *KubernetesPlanValidator) validateKubernetes(ctx context.Context, kubernetes *upgradev1alpha1.KubernetesPlan) (admission.Warnings, error) {
	var warnings admission.Warnings

	kuberneteslog.Info("validating kubernetes plan",
		"name", kubernetes.Name,
		"namespace", kubernetes.Namespace,
		"secretName", v.TalosConfigSecret)

	// Validate that the Talos config secret exists (needed for Kubernetes upgrades too)
	secret := &corev1.Secret{}
	err := v.Client.Get(ctx, types.NamespacedName{
		Name:      v.TalosConfigSecret,
		Namespace: kubernetes.Namespace,
	}, secret)

	if err != nil {
		return warnings, fmt.Errorf("talosconfig secret '%s' not found in namespace '%s'. Please create this secret with your Talos configuration before creating KubernetesPlan resources: %w",
			v.TalosConfigSecret, kubernetes.Namespace, err)
	}

	// Validate that the secret has the required key
	configData, exists := secret.Data["config"]
	if !exists {
		return warnings, fmt.Errorf("talosconfig secret '%s' missing required key '%s'. The secret must contain your talosconfig data under this key",
			v.TalosConfigSecret, "config")
	}

	// Add warning if secret data is empty
	if len(configData) == 0 {
		return warnings, fmt.Errorf("talosconfig secret data is empty. Please ensure the secret contains valid Talos configuration data")
	}

	// Validate that the talosconfig can be parsed
	_, err = talosclientconfig.FromBytes(configData)
	if err != nil {
		return warnings, fmt.Errorf("talosconfig in secret '%s' is invalid and cannot be parsed: %w. Please ensure the secret contains valid Talos configuration data",
			v.TalosConfigSecret, err)
	}

	// Validate spec fields
	if err := v.validateKubernetesSpec(kubernetes); err != nil {
		return warnings, fmt.Errorf("spec validation failed: %w", err)
	}

	kuberneteslog.Info("kubernetes plan validation successful", "name", kubernetes.Name)
	return warnings, nil
}

func (v *KubernetesPlanValidator) validateKubernetesSpec(kubernetes *upgradev1alpha1.KubernetesPlan) error {
	// Validate version is not empty
	if kubernetes.Spec.Version == "" {
		return fmt.Errorf("spec.version cannot be empty")
	}

	// Add other KubernetesPlan-specific validations here

	return nil
}

func (v *KubernetesPlanValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&upgradev1alpha1.KubernetesPlan{}).
		WithValidator(v).
		Complete()
}
