package webhook

import (
	"context"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	talosclientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"

	upgradev1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

// log is for logging in this package.
var taloslog = logf.Log.WithName("talos-resource")

// TalosUpgradeValidator validates Talos resources
type TalosUpgradeValidator struct {
	Client            client.Client
	TalosConfigSecret string
}

// +kubebuilder:webhook:path=/validate-tuppr-home-operations-com-v1alpha1-talosupgrade,mutating=false,failurePolicy=fail,sideEffects=None,groups=tuppr.home-operations.com,resources=talosupgrades,verbs=create;update,versions=v1alpha1,name=vtalosupgrade.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &TalosUpgradeValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *TalosUpgradeValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	talos := obj.(*upgradev1alpha1.TalosUpgrade)
	taloslog.Info("validate create", "name", talos.Name, "talosConfigSecret", v.TalosConfigSecret)

	return v.validateTalos(ctx, talos)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *TalosUpgradeValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	talos := newObj.(*upgradev1alpha1.TalosUpgrade)
	oldTalos := oldObj.(*upgradev1alpha1.TalosUpgrade)
	taloslog.Info("validate update", "name", talos.Name, "talosConfigSecret", v.TalosConfigSecret)

	// Prevent updates to certain fields if upgrade is in progress
	if oldTalos.Status.Phase == "InProgress" {
		if talos.Spec.Image.Repository != oldTalos.Spec.Image.Repository ||
			talos.Spec.Image.Tag != oldTalos.Spec.Image.Tag {
			return nil, fmt.Errorf("cannot update spec.image while upgrade is in progress (current phase: %s)", oldTalos.Status.Phase)
		}

		// Check if node label selector has changed
		if !nodeLabelSelectorsEqual(talos.Spec.NodeLabelSelector, oldTalos.Spec.NodeLabelSelector) {
			return nil, fmt.Errorf("cannot update spec.nodeLabelSelector while upgrade is in progress (current phase: %s)", oldTalos.Status.Phase)
		}
	}

	return v.validateTalos(ctx, talos)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (v *TalosUpgradeValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	talos := obj.(*upgradev1alpha1.TalosUpgrade)

	// Warn about deleting an in-progress upgrade
	if talos.Status.Phase == "InProgress" {
		return []string{
			fmt.Sprintf("Deleting TalosUpgrade '%s' while upgrade is in progress. This may leave nodes in an inconsistent state.", talos.Name),
		}, nil
	}

	return nil, nil
}

func (v *TalosUpgradeValidator) validateTalos(ctx context.Context, talos *upgradev1alpha1.TalosUpgrade) (admission.Warnings, error) {
	var warnings admission.Warnings

	taloslog.Info("validating talos plan",
		"name", talos.Name,
		"namespace", talos.Namespace,
		"secretName", v.TalosConfigSecret)

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

	// Add warnings for risky configurations
	warnings = append(warnings, v.generateWarnings(talos)...)

	taloslog.Info("talos plan validation successful", "name", talos.Name)
	return warnings, nil
}

func (v *TalosUpgradeValidator) validateTalosSpec(talos *upgradev1alpha1.TalosUpgrade) error {
	// Validate tag is not empty (repository is optional with default)
	if talos.Spec.Image.Tag == "" {
		return fmt.Errorf("spec.image.tag cannot be empty")
	}

	// Validate node label selector
	if err := v.validateNodeLabelSelector(talos.Spec.NodeLabelSelector); err != nil {
		return fmt.Errorf("spec.nodeLabelSelector validation failed: %w", err)
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
	if talos.Spec.UpgradePolicy.RebootMode != "" {
		validModes := []string{"default", "powercycle"}
		if !slices.Contains(validModes, talos.Spec.UpgradePolicy.RebootMode) {
			return fmt.Errorf("spec.upgradePolicy.rebootMode '%s' is invalid. Valid values are: %v",
				talos.Spec.UpgradePolicy.RebootMode, validModes)
		}
	}

	// Validate placement preset if provided
	if talos.Spec.UpgradePolicy.PlacementPreset != "" {
		validPresets := []string{"hard", "soft"}
		if !slices.Contains(validPresets, talos.Spec.UpgradePolicy.PlacementPreset) {
			return fmt.Errorf("spec.upgradePolicy.placementPreset '%s' is invalid. Valid values are: %v",
				talos.Spec.UpgradePolicy.PlacementPreset, validPresets)
		}
	}

	return nil
}

func (v *TalosUpgradeValidator) validateNodeLabelSelector(selector upgradev1alpha1.NodeLabelSelector) error {
	// Validate matchLabels
	for key, value := range selector.MatchLabels {
		if key == "" {
			return fmt.Errorf("matchLabels key cannot be empty")
		}
		if value == "" {
			return fmt.Errorf("matchLabels value for key '%s' cannot be empty", key)
		}
	}

	// Validate matchExpressions
	for i, expr := range selector.MatchExpressions {
		if expr.Key == "" {
			return fmt.Errorf("matchExpressions[%d].key cannot be empty", i)
		}

		validOps := []metav1.LabelSelectorOperator{
			metav1.LabelSelectorOpIn,
			metav1.LabelSelectorOpNotIn,
			metav1.LabelSelectorOpExists,
			metav1.LabelSelectorOpDoesNotExist,
		}

		// Validate operator is valid
		if !slices.Contains(validOps, expr.Operator) {
			return fmt.Errorf("matchExpressions[%d].operator '%s' is invalid", i, expr.Operator)
		}

		// Validate that value-requiring operators have values
		if (expr.Operator == metav1.LabelSelectorOpIn ||
			expr.Operator == metav1.LabelSelectorOpNotIn) && len(expr.Values) == 0 {
			return fmt.Errorf("matchExpressions[%d] with operator '%s' requires at least one value", i, expr.Operator)
		}

		// Validate that non-value operators don't have values
		if (expr.Operator == metav1.LabelSelectorOpExists ||
			expr.Operator == metav1.LabelSelectorOpDoesNotExist) && len(expr.Values) > 0 {
			return fmt.Errorf("matchExpressions[%d] with operator '%s' must not have values", i, expr.Operator)
		}
	}

	return nil
}

func (v *TalosUpgradeValidator) validateHealthCheck(check upgradev1alpha1.HealthCheckExpr) error {
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

func nodeLabelSelectorsEqual(a, b upgradev1alpha1.NodeLabelSelector) bool {
	// Compare matchLabels
	if len(a.MatchLabels) != len(b.MatchLabels) {
		return false
	}
	for k, v := range a.MatchLabels {
		if b.MatchLabels[k] != v {
			return false
		}
	}

	// Compare matchExpressions
	if len(a.MatchExpressions) != len(b.MatchExpressions) {
		return false
	}

	// Convert to comparable format and sort for consistent comparison
	aStr := make([]string, len(a.MatchExpressions))
	bStr := make([]string, len(b.MatchExpressions))

	for i, expr := range a.MatchExpressions {
		values := make([]string, len(expr.Values))
		copy(values, expr.Values)
		slices.Sort(values)
		aStr[i] = fmt.Sprintf("%s:%s:%v", expr.Key, expr.Operator, values)
	}

	for i, expr := range b.MatchExpressions {
		values := make([]string, len(expr.Values))
		copy(values, expr.Values)
		slices.Sort(values)
		bStr[i] = fmt.Sprintf("%s:%s:%v", expr.Key, expr.Operator, values)
	}

	slices.Sort(aStr)
	slices.Sort(bStr)

	return slices.Equal(aStr, bStr)
}

func (v *TalosUpgradeValidator) generateWarnings(talos *upgradev1alpha1.TalosUpgrade) []string {
	var warnings []string

	// Warn about force upgrades
	if talos.Spec.UpgradePolicy.Force {
		warnings = append(warnings, "Force upgrade enabled. This will skip etcd health checks and may cause data loss in unhealthy clusters.")
	}

	// Warn about powercycle reboot mode
	if talos.Spec.UpgradePolicy.RebootMode == "powercycle" {
		warnings = append(warnings, "Powercycle reboot mode selected. This performs a hard power cycle and may cause data loss if nodes don't shutdown cleanly.")
	}

	// Warn about upgrading all nodes (no selector)
	if len(talos.Spec.NodeLabelSelector.MatchLabels) == 0 && len(talos.Spec.NodeLabelSelector.MatchExpressions) == 0 {
		warnings = append(warnings, "No node selector specified. This will upgrade ALL nodes in the cluster.")
	}

	// Add warning for debug mode
	if talos.Spec.UpgradePolicy.Debug {
		warnings = append(warnings, "Debug mode enabled. This will produce verbose output in upgrade jobs.")
	}

	// Warn about placement preset implications
	if talos.Spec.UpgradePolicy.PlacementPreset == "soft" {
		warnings = append(warnings, "Soft placement preset allows upgrade jobs to run on the target node if no other nodes are available. This may cause upgrade failures if the target node becomes unavailable during upgrade.")
	}

	// Warn about health checks without timeouts
	for i, check := range talos.Spec.HealthChecks {
		if check.Timeout == nil {
			warnings = append(warnings, fmt.Sprintf("Health check %d has no timeout specified, will use default 10 minutes", i))
		}
	}

	return warnings
}

func (v *TalosUpgradeValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&upgradev1alpha1.TalosUpgrade{}).
		WithValidator(v).
		Complete()
}
