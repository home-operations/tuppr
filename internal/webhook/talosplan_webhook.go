package webhook

import (
	"context"
	"fmt"
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

	upgradev1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

// log is for logging in this package.
var taloslog = logf.Log.WithName("talos-resource")

// TalosUpgradeValidator validates Talos resources
type TalosUpgradeValidator struct {
	Client            client.Client
	TalosConfigSecret string
}

//+kubebuilder:webhook:path=/validate-upgrade-home-operations-com-v1alpha1-talosupgrade,mutating=false,failurePolicy=fail,sideEffects=None,groups=tuppr.home-operations.com,resources=talosupgrades,verbs=create;update,versions=v1alpha1,name=vtalosupgrade.kb.io,admissionReviewVersions=v1

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
		if talos.Spec.Target.Image.Repository != oldTalos.Spec.Target.Image.Repository ||
			talos.Spec.Target.Image.Tag != oldTalos.Spec.Target.Image.Tag {
			return nil, fmt.Errorf("cannot update spec.image while upgrade is in progress (current phase: %s)", oldTalos.Status.Phase)
		}

		if !compareNodeSelectors(talos.Spec.Target.NodeSelector, oldTalos.Spec.Target.NodeSelector) {
			return nil, fmt.Errorf("cannot update spec.nodeSelector while upgrade is in progress (current phase: %s)", oldTalos.Status.Phase)
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

	// Check if we can find target nodes with the selector
	if nodeCount, err := v.validateNodeSelector(ctx, talos); err != nil {
		return warnings, fmt.Errorf("node selector validation failed: %w", err)
	} else if nodeCount == 0 {
		warnings = append(warnings, "No nodes match the specified nodeSelector. The upgrade plan will not target any nodes.")
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

func (v *TalosUpgradeValidator) validateNodeSelector(ctx context.Context, talos *upgradev1alpha1.TalosUpgrade) (int, error) {
	nodeList := &corev1.NodeList{}
	listOpts := []client.ListOption{}

	if talos.Spec.Target.NodeSelector != nil {
		listOpts = append(listOpts, client.MatchingLabels(talos.Spec.Target.NodeSelector))
	}

	if err := v.Client.List(ctx, nodeList, listOpts...); err != nil {
		return 0, fmt.Errorf("failed to list nodes with selector: %w", err)
	}

	return len(nodeList.Items), nil
}

func (v *TalosUpgradeValidator) validateTalosSpec(talos *upgradev1alpha1.TalosUpgrade) error {
	// Validate image repository and tag are not empty
	if talos.Spec.Target.Image.Repository == "" {
		return fmt.Errorf("spec.target.image.repository cannot be empty")
	}

	if talos.Spec.Target.Image.Tag == "" {
		return fmt.Errorf("spec.target.image.tag cannot be empty")
	}

	// Validate talosctl image if specified
	// Allow either both specified or both empty (defaults)
	talosctlRepoEmpty := talos.Spec.Talosctl.Image.Repository == ""
	talosctlTagEmpty := talos.Spec.Talosctl.Image.Tag == ""

	if talosctlRepoEmpty != talosctlTagEmpty {
		return fmt.Errorf("both spec.talosctl.image.repository and spec.talosctl.image.tag must be specified together, or both omitted for defaults")
	}

	// Validate pull policy if specified
	if talos.Spec.Talosctl.Image.PullPolicy != "" {
		validPolicies := []string{"Always", "Never", "IfNotPresent"}
		if !slices.Contains(validPolicies, string(talos.Spec.Talosctl.Image.PullPolicy)) {
			return fmt.Errorf("spec.talosctl.pullPolicy '%s' is invalid. Valid values are: %v", talos.Spec.Talosctl.Image.PullPolicy, validPolicies)
		}
	}

	// Validate reboot mode if provided
	if talos.Spec.Target.Options.RebootMode != "" {
		validModes := []string{"default", "powercycle"}
		if !slices.Contains(validModes, talos.Spec.Target.Options.RebootMode) {
			return fmt.Errorf("spec.target.options.rebootMode '%s' is invalid. Valid values are: %v",
				talos.Spec.Target.Options.RebootMode, validModes)
		}
	}

	return nil
}

func (v *TalosUpgradeValidator) generateWarnings(talos *upgradev1alpha1.TalosUpgrade) []string {
	var warnings []string

	// Warn about force upgrades
	if talos.Spec.Target.Options.Force {
		warnings = append(warnings, "Force upgrade enabled. This will skip etcd health checks and may cause data loss in unhealthy clusters.")
	}

	// Warn about powercycle reboot mode
	if talos.Spec.Target.Options.RebootMode == "powercycle" {
		warnings = append(warnings, "Powercycle reboot mode selected. This performs a hard power cycle and may cause data loss if nodes don't shutdown cleanly.")
	}

	// Warn about upgrading all nodes (no selector)
	if len(talos.Spec.Target.NodeSelector) == 0 {
		warnings = append(warnings, "No nodeSelector specified. This will upgrade ALL nodes in the cluster.")
	}

	// Add warning for debug mode
	if talos.Spec.Target.Options.Debug {
		warnings = append(warnings, "Debug mode enabled. This will produce verbose output in upgrade jobs.")
	}

	return warnings
}

func compareNodeSelectors(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}

	for k, v := range a {
		if b[k] != v {
			return false
		}
	}

	return true
}

func (v *TalosUpgradeValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&upgradev1alpha1.TalosUpgrade{}).
		WithValidator(v).
		Complete()
}
