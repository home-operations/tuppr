package webhook

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
)

var kuberneteslog = logf.Log.WithName("kubernetes-resource")

type KubernetesUpgradeValidator struct {
	Client            client.Client
	TalosConfigSecret string
	Namespace         string
}

// +kubebuilder:webhook:path=/validate-tuppr-home-operations-com-v1alpha1-kubernetesupgrade,mutating=false,failurePolicy=fail,sideEffects=None,groups=tuppr.home-operations.com,resources=kubernetesupgrades,verbs=create;update,versions=v1alpha1,name=vkubernetesupgrade.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*tupprv1alpha1.KubernetesUpgrade] = &KubernetesUpgradeValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *KubernetesUpgradeValidator) ValidateCreate(ctx context.Context, k *tupprv1alpha1.KubernetesUpgrade) (admission.Warnings, error) {
	kuberneteslog.Info("validate create", "name", k.Name, "namespace", k.Namespace, "version", k.Spec.Kubernetes.Version)
	return v.validate(ctx, k)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type

func (v *KubernetesUpgradeValidator) ValidateUpdate(ctx context.Context, old, k *tupprv1alpha1.KubernetesUpgrade) (admission.Warnings, error) {
	kuberneteslog.Info("validate update", "name", k.Name)

	if err := ValidateUpdateInProgress(old.Status.Phase, old.Spec, k.Spec); err != nil {
		return nil, err
	}
	return v.validate(ctx, k)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (v *KubernetesUpgradeValidator) ValidateDelete(ctx context.Context, k *tupprv1alpha1.KubernetesUpgrade) (admission.Warnings, error) {
	if k.Status.Phase == constants.PhaseInProgress {
		return admission.Warnings{
			fmt.Sprintf("Deleting KubernetesUpgrade '%s' while upgrade is in progress. This may leave the cluster in an inconsistent state.", k.Name),
		}, nil
	}
	return nil, nil
}

func (v *KubernetesUpgradeValidator) validate(ctx context.Context, k *tupprv1alpha1.KubernetesUpgrade) (admission.Warnings, error) {
	var warnings admission.Warnings

	list := &tupprv1alpha1.KubernetesUpgradeList{}
	if err := ValidateSingleton(ctx, v.Client, "KubernetesUpgrade", k.Name, list); err != nil {
		return warnings, err
	}

	if _, err := ValidateTalosConfigSecret(ctx, v.Client, v.TalosConfigSecret, v.Namespace); err != nil {
		return warnings, err
	}

	if err := ValidateVersionFormat(k.Spec.Kubernetes.Version); err != nil {
		return warnings, fmt.Errorf("invalid kubernetes version: %w", err)
	}

	if err := ValidateHealthChecks(k.Spec.HealthChecks); err != nil {
		return warnings, err
	}

	if err := ValidateTalosctlSpec(k.Spec.Talosctl); err != nil {
		return warnings, err
	}

	// Validate maintenance window if specified
	if mwWarnings, err := validateMaintenanceWindows(k.Spec.Maintenance); err != nil {
		return warnings, fmt.Errorf("spec.maintenanceWindow validation failed: %w", err)
	} else {
		warnings = append(warnings, mwWarnings...)
	}

	// Add warnings for risky configurations
	warnings = append(warnings, GenerateCommonWarnings(
		k.Spec.Kubernetes.Version,
		k.Spec.HealthChecks,
		k.Spec.Talosctl.Image.Tag,
	)...)

	kuberneteslog.Info("kubernetes upgrade validation successful", "name", k.Name, "version", k.Spec.Kubernetes.Version)
	return warnings, nil
}

func (v *KubernetesUpgradeValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &tupprv1alpha1.KubernetesUpgrade{}).
		WithValidator(v).
		Complete()
}
