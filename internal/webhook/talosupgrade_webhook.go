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

var taloslog = logf.Log.WithName("talos-resource")

// TalosUpgradeValidator validates Talos resources
type TalosUpgradeValidator struct {
	Client            client.Client
	TalosConfigSecret string
	Namespace         string
}

// +kubebuilder:webhook:path=/validate-tuppr-home-operations-com-v1alpha1-talosupgrade,mutating=false,failurePolicy=fail,sideEffects=None,groups=tuppr.home-operations.com,resources=talosupgrades,verbs=create;update,versions=v1alpha1,name=vtalosupgrade.kb.io,admissionReviewVersions=v1
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades,verbs=get;list;watch

var _ admission.Validator[*tupprv1alpha1.TalosUpgrade] = &TalosUpgradeValidator{}

// ValidateCreate implements admission.Validator so a webhook will be registered for the type
func (v *TalosUpgradeValidator) ValidateCreate(ctx context.Context, t *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	taloslog.Info("validate create", "name", t.Name, "version", t.Spec.Talos.Version, "talosConfigSecret", v.TalosConfigSecret)
	return v.validate(ctx, t)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *TalosUpgradeValidator) ValidateUpdate(ctx context.Context, old, t *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	taloslog.Info("validate update", "name", t.Name)

	if err := ValidateUpdateInProgress(old.Status.Phase, old.Spec, t.Spec); err != nil {
		return nil, err
	}
	return v.validate(ctx, t)
}

func (v *TalosUpgradeValidator) ValidateDelete(ctx context.Context, t *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	if t.Status.Phase == constants.PhaseInProgress {
		return admission.Warnings{
			fmt.Sprintf("Deleting TalosUpgrade '%s' while upgrade is in progress. This may leave nodes in an inconsistent state.", t.Name),
		}, nil
	}
	return nil, nil
}

func (v *TalosUpgradeValidator) validate(ctx context.Context, t *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	var warnings admission.Warnings

	list := &tupprv1alpha1.TalosUpgradeList{}
	if err := ValidateSingleton(ctx, v.Client, "TalosUpgrade", t.Name, list); err != nil {
		return warnings, err
	}

	if _, err := ValidateTalosConfigSecret(ctx, v.Client, v.TalosConfigSecret, v.Namespace); err != nil {
		return warnings, err
	}

	if err := ValidateVersionFormat(t.Spec.Talos.Version); err != nil {
		return warnings, fmt.Errorf("invalid talos version: %w", err)
	}

	if err := ValidateHealthChecks(t.Spec.HealthChecks); err != nil {
		return warnings, err
	}

	if err := ValidateTalosctlSpec(t.Spec.Talosctl); err != nil {
		return warnings, err
	}

	// Validate Policy
	if t.Spec.Policy.RebootMode != "" && t.Spec.Policy.RebootMode != "default" && t.Spec.Policy.RebootMode != "powercycle" {
		return warnings, fmt.Errorf("invalid rebootMode '%s'", t.Spec.Policy.RebootMode)
	}
	if t.Spec.Policy.Placement != "" && t.Spec.Policy.Placement != "hard" && t.Spec.Policy.Placement != "soft" {
		return warnings, fmt.Errorf("invalid placement '%s'", t.Spec.Policy.Placement)
	}

	warnings = append(warnings, GenerateCommonWarnings(
		t.Spec.Talos.Version,
		t.Spec.HealthChecks,
		t.Spec.Talosctl.Image.Tag,
	)...)

	if t.Spec.Policy.Force {
		warnings = append(warnings, "Force upgrade enabled.")
	}
	if t.Spec.Policy.RebootMode == "powercycle" {
		warnings = append(warnings, "Powercycle reboot mode selected.")
	}
	if t.Spec.Policy.Debug {
		warnings = append(warnings, "Debug mode enabled.")
	}
	if t.Spec.Policy.Placement == "soft" {
		warnings = append(warnings, "Soft placement preset used.")
	}

	// Validate maintenance window if specified
	if mwWarnings, err := validateMaintenanceWindows(t.Spec.Maintenance); err != nil {
		return warnings, fmt.Errorf("spec.maintenanceWindow validation failed: %w", err)
	} else {
		warnings = append(warnings, mwWarnings...)
	}

	taloslog.Info("talos plan validation successful", "name", t.Name, "version", t.Spec.Talos.Version)
	return warnings, nil
}

func (v *TalosUpgradeValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &tupprv1alpha1.TalosUpgrade{}).
		WithValidator(v).
		Complete()
}
