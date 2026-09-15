package kubernetesupgrade

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/webhook/validation"
)

type Validator struct {
	Client            client.Client
	TalosConfigSecret string
	Namespace         string
}

// +kubebuilder:webhook:path=/validate-tuppr-home-operations-com-v1alpha1-kubernetesupgrade,mutating=false,failurePolicy=fail,sideEffects=None,groups=tuppr.home-operations.com,resources=kubernetesupgrades,verbs=create;update,versions=v1alpha1,name=vkubernetesupgrade.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*tupprv1alpha1.KubernetesUpgrade] = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *Validator) ValidateCreate(ctx context.Context, k *tupprv1alpha1.KubernetesUpgrade) (admission.Warnings, error) {
	log.FromContext(ctx).V(1).Info("Validating create", "targetVersion", k.Spec.Kubernetes.Version)
	return v.validate(ctx, k)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *Validator) ValidateUpdate(ctx context.Context, old, k *tupprv1alpha1.KubernetesUpgrade) (admission.Warnings, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Validating update", "targetVersion", k.Spec.Kubernetes.Version)

	if err := validation.ValidateUpdateInProgress(old.Status.Conditions, old.Status.Phase, old.Spec, k.Spec); err != nil {
		logger.Info("Rejected KubernetesUpgrade", "targetVersion", k.Spec.Kubernetes.Version, "reason", err.Error())
		return nil, err
	}
	return v.validate(ctx, k)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (v *Validator) ValidateDelete(ctx context.Context, k *tupprv1alpha1.KubernetesUpgrade) (admission.Warnings, error) {
	if k.Status.Phase.IsActive() {
		return admission.Warnings{
			fmt.Sprintf("Deleting KubernetesUpgrade '%s' while upgrade is in progress. This may leave the cluster in an inconsistent state.", k.Name),
		}, nil
	}
	return nil, nil
}

// validate logs the admission outcome itself: the apiserver only relays a
// rejection to the caller, so it is the one decision worth an Info line.
func (v *Validator) validate(ctx context.Context, k *tupprv1alpha1.KubernetesUpgrade) (warnings admission.Warnings, err error) {
	logger := log.FromContext(ctx)
	defer func() {
		if err != nil {
			logger.Info("Rejected KubernetesUpgrade", "targetVersion", k.Spec.Kubernetes.Version, "reason", err.Error())
			return
		}
		logger.V(1).Info("KubernetesUpgrade validation successful", "targetVersion", k.Spec.Kubernetes.Version, "warningCount", len(warnings))
	}()

	list := &tupprv1alpha1.KubernetesUpgradeList{}
	if err := validation.ValidateSingleton(ctx, v.Client, "KubernetesUpgrade", k.Name, list); err != nil {
		return warnings, err
	}

	if _, err := validation.ValidateTalosConfigSecret(ctx, v.Client, v.TalosConfigSecret, v.Namespace); err != nil {
		return warnings, err
	}

	if err := validation.ValidateVersionFormat(k.Spec.Kubernetes.Version); err != nil {
		return warnings, fmt.Errorf("invalid kubernetes version: %w", err)
	}

	if err := validation.ValidateHealthChecks(k.Spec.HealthChecks); err != nil {
		return warnings, err
	}

	if err := validation.ValidateTalosctlSpec(k.Spec.Talosctl); err != nil {
		return warnings, err
	}

	// Validate maintenance window if specified
	if mwWarnings, err := validation.ValidateMaintenanceWindows(k.Spec.Maintenance); err != nil {
		return warnings, fmt.Errorf("spec.maintenanceWindow validation failed: %w", err)
	} else {
		warnings = append(warnings, mwWarnings...)
	}

	// Add warnings for risky configurations
	warnings = append(warnings, validation.GenerateCommonWarnings(
		k.Spec.Kubernetes.Version,
		k.Spec.HealthChecks,
		k.Spec.Talosctl.Image.Tag,
	)...)

	return warnings, nil
}

func (v *Validator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &tupprv1alpha1.KubernetesUpgrade{}).
		WithValidator(v).
		Complete()
}
