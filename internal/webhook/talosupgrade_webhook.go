package webhook

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	talosclientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
)

// log is for logging in this package.
var taloslog = logf.Log.WithName("talos-resource")

// TalosUpgradeValidator validates Talos resources
type TalosUpgradeValidator struct {
	Client            client.Client
	TalosConfigSecret string
}

// +kubebuilder:webhook:path=/validate-tuppr-home-operations-com-v1alpha1-talosupgrade,mutating=false,failurePolicy=fail,sideEffects=None,groups=tuppr.home-operations.com,resources=talosupgrades,verbs=create;update,versions=v1alpha1,name=vtalosupgrade.kb.io,admissionReviewVersions=v1
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades,verbs=get;list;watch

var _ webhook.CustomValidator = &TalosUpgradeValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *TalosUpgradeValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	talos := obj.(*tupprv1alpha1.TalosUpgrade)
	taloslog.Info("validate create", "name", talos.Name, "version", talos.Spec.Talos.Version, "talosConfigSecret", v.TalosConfigSecret)

	return v.validateTalos(ctx, talos)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *TalosUpgradeValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	talos := newObj.(*tupprv1alpha1.TalosUpgrade)
	oldTalos := oldObj.(*tupprv1alpha1.TalosUpgrade)
	taloslog.Info("validate update", "name", talos.Name, "version", talos.Spec.Talos.Version, "talosConfigSecret", v.TalosConfigSecret)

	// Prevent ANY spec updates if upgrade is in progress
	if oldTalos.Status.Phase == constants.PhaseInProgress {
		if !reflect.DeepEqual(talos.Spec, oldTalos.Spec) {
			return nil, fmt.Errorf("cannot update spec while upgrade is in progress (current phase: %s). Please wait for the upgrade to complete or reset it using the %s annotation",
				oldTalos.Status.Phase, constants.ResetAnnotation)
		}
	}

	return v.validateTalos(ctx, talos)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (v *TalosUpgradeValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	talos := obj.(*tupprv1alpha1.TalosUpgrade)

	// Warn about deleting an in-progress upgrade
	if talos.Status.Phase == constants.PhaseInProgress {
		return []string{
			fmt.Sprintf("Deleting TalosUpgrade '%s' while upgrade is in progress. This may leave nodes in an inconsistent state.", talos.Name),
		}, nil
	}

	return nil, nil
}

func (v *TalosUpgradeValidator) validateTalos(ctx context.Context, talos *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	var warnings admission.Warnings

	taloslog.Info("validating talos upgrade",
		"name", talos.Name,
		"version", talos.Spec.Talos.Version,
		"secretName", v.TalosConfigSecret)

	// Check for singleton constraint - only allow one TalosUpgrade per cluster
	if err := v.validateSingleton(ctx, talos); err != nil {
		return warnings, err
	}

	// Validate that the Talos config secret exists
	secret := &corev1.Secret{}
	err := v.Client.Get(ctx, types.NamespacedName{
		Name:      v.TalosConfigSecret,
		Namespace: talos.Namespace,
	}, secret)

	if err != nil {
		return warnings, fmt.Errorf("talosconfig secret '%s' not found in namespace '%s'. Please create this secret with your Talos configuration before creating TalosUpgrade resources: %w",
			v.TalosConfigSecret, talos.Namespace, err)
	}

	// Validate that the secret has the required key (use consistent key name)
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
	config, err := talosclientconfig.FromBytes(configData)
	if err != nil {
		return warnings, fmt.Errorf("talosconfig in secret '%s' is invalid and cannot be parsed: %w. Please ensure the secret contains valid Talos configuration data",
			v.TalosConfigSecret, err)
	}

	// Validate that talosconfig has required endpoints
	if len(config.Contexts) == 0 {
		return warnings, fmt.Errorf("talosconfig has no contexts defined")
	}

	// Validate spec fields
	if err := v.validateTalosSpec(talos); err != nil {
		return warnings, fmt.Errorf("spec validation failed: %w", err)
	}

	// Validate maintenance window if specified
	if mwWarnings, err := validateMaintenanceWindows(talos.Spec.MaintenanceWindow); err != nil {
		return warnings, fmt.Errorf("spec.maintenanceWindow validation failed: %w", err)
	} else {
		warnings = append(warnings, mwWarnings...)
	}

	// Add warnings for risky configurations
	warnings = append(warnings, v.generateWarnings(talos)...)

	taloslog.Info("talos plan validation successful", "name", talos.Name, "version", talos.Spec.Talos.Version)
	return warnings, nil
}

func (v *TalosUpgradeValidator) validateTalosSpec(talos *tupprv1alpha1.TalosUpgrade) error {
	// Validate version is not empty and follows semantic versioning pattern
	if talos.Spec.Talos.Version == "" {
		return fmt.Errorf("spec.version cannot be empty")
	}

	// Validate version format (should match the kubebuilder validation pattern)
	versionPattern := `^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9\-\.]+)?$`
	matched, err := regexp.MatchString(versionPattern, talos.Spec.Talos.Version)
	if err != nil {
		return fmt.Errorf("error validating version pattern: %w", err)
	}
	if !matched {
		return fmt.Errorf("spec.version '%s' does not match required pattern. Must be in format 'vX.Y.Z' or 'vX.Y.Z-suffix' (e.g., 'v1.11.0', 'v1.11.0-alpha.1')", talos.Spec.Talos.Version)
	}

	// Validate health checks
	for i, check := range talos.Spec.HealthChecks {
		if err := v.validateHealthCheck(check); err != nil {
			return fmt.Errorf("spec.healthChecks[%d] validation failed: %w", i, err)
		}
	}

	// Validate talosctl image if specified
	talosctlRepoEmpty := talos.Spec.Talosctl.Image.Repository == ""
	talosctlTagEmpty := talos.Spec.Talosctl.Image.Tag == ""

	if talosctlRepoEmpty != talosctlTagEmpty {
		return fmt.Errorf("both spec.talosctl.image.repository and spec.talosctl.image.tag must be specified together, or both omitted for defaults")
	}

	// Validate pull policy if specified
	if talos.Spec.Talosctl.Image.PullPolicy != "" {
		validPolicies := []corev1.PullPolicy{corev1.PullAlways, corev1.PullNever, corev1.PullIfNotPresent}
		if !slices.Contains(validPolicies, talos.Spec.Talosctl.Image.PullPolicy) {
			return fmt.Errorf("spec.talosctl.image.pullPolicy '%s' is invalid. Valid values are: %v", talos.Spec.Talosctl.Image.PullPolicy, validPolicies)
		}
	}

	// Validate reboot mode if provided
	if talos.Spec.Policy.RebootMode != "" {
		validModes := []string{"default", "powercycle"}
		if !slices.Contains(validModes, talos.Spec.Policy.RebootMode) {
			return fmt.Errorf("spec.policy.rebootMode '%s' is invalid. Valid values are: %v",
				talos.Spec.Policy.RebootMode, validModes)
		}
	}

	// Validate placement preset if provided
	if talos.Spec.Policy.Placement != "" {
		validPresets := []string{"hard", "soft"}
		if !slices.Contains(validPresets, talos.Spec.Policy.Placement) {
			return fmt.Errorf("spec.policy.placement '%s' is invalid. Valid values are: %v",
				talos.Spec.Policy.Placement, validPresets)
		}
	}

	return nil
}

func (v *TalosUpgradeValidator) validateHealthCheck(check tupprv1alpha1.HealthCheckSpec) error {
	if check.APIVersion == "" {
		return fmt.Errorf("apiVersion cannot be empty")
	}
	if check.Kind == "" {
		return fmt.Errorf("kind cannot be empty")
	}
	if check.Expr == "" {
		return fmt.Errorf("expr cannot be empty")
	}

	// Validate timeout if specified
	if check.Timeout != nil && check.Timeout.Duration <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	return nil
}

func (v *TalosUpgradeValidator) validateSingleton(ctx context.Context, talos *tupprv1alpha1.TalosUpgrade) error {
	// List all existing TalosUpgrade resources
	existingList := &tupprv1alpha1.TalosUpgradeList{}
	if err := v.Client.List(ctx, existingList); err != nil {
		return fmt.Errorf("failed to check for existing TalosUpgrade resources: %w", err)
	}

	// Filter out the current resource being validated (for updates)
	for _, existing := range existingList.Items {
		if existing.Name == talos.Name {
			// This is the same resource, skip it
			continue
		}

		// Found another TalosUpgrade resource
		taloslog.Info("rejecting creation/update due to existing TalosUpgrade",
			"existing", existing.Name,
			"attempted", talos.Name)

		return fmt.Errorf("only one TalosUpgrade resource is allowed per cluster. Found existing resource '%s'. Please delete the existing resource or update it instead of creating a new one",
			existing.Name)
	}

	return nil
}

func (v *TalosUpgradeValidator) generateWarnings(talos *tupprv1alpha1.TalosUpgrade) []string {
	var warnings []string

	// Warn about force upgrades
	if talos.Spec.Policy.Force {
		warnings = append(warnings, "Force upgrade enabled. This will skip etcd health checks and may cause data loss in unhealthy clusters.")
	}

	// Warn about powercycle reboot mode
	if talos.Spec.Policy.RebootMode == "powercycle" {
		warnings = append(warnings, "Powercycle reboot mode selected. This performs a hard power cycle and may cause data loss if nodes don't shutdown cleanly.")
	}

	// Add warning for debug mode
	if talos.Spec.Policy.Debug {
		warnings = append(warnings, "Debug mode enabled. This will produce verbose output in upgrade jobs.")
	}

	// Warn about placement preset implications
	if talos.Spec.Policy.Placement == "soft" {
		warnings = append(warnings, "Soft placement preset allows upgrade jobs to run on the target node if no other nodes are available. This may cause upgrade failures if the target node becomes unavailable during upgrade.")
	}

	// Warn about health checks without timeouts
	for i, check := range talos.Spec.HealthChecks {
		if check.Timeout == nil {
			warnings = append(warnings, fmt.Sprintf("Health check %d has no timeout specified, will use default 10 minutes", i))
		}
	}

	// Warn about pre-release versions
	if matched, _ := regexp.MatchString(`-[a-zA-Z]`, talos.Spec.Talos.Version); matched {
		warnings = append(warnings, fmt.Sprintf("Target version '%s' appears to be a pre-release. Ensure this version is stable and tested in your environment.", talos.Spec.Talos.Version))
	}

	// Warn if talosctl version is not specified (will default to target version)
	if talos.Spec.Talosctl.Image.Tag == "" {
		warnings = append(warnings, fmt.Sprintf("No talosctl version specified, will default to target version '%s'. Ensure talosctl version compatibility with your cluster.", talos.Spec.Talos.Version))
	}

	return warnings
}

func (v *TalosUpgradeValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&tupprv1alpha1.TalosUpgrade{}).
		WithValidator(v).
		Complete()
}
