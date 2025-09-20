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
	nodeselection "github.com/home-operations/tuppr/internal/nodeselection"
)

// log is for logging in this package.
var taloslog = logf.Log.WithName("talos-resource")

// TalosUpgradeValidator validates Talos resources
type TalosUpgradeValidator struct {
	Client            client.Client
	TalosConfigSecret string
	NodeMatcher       *nodeselection.Matcher
}

// +kubebuilder:webhook:path=/validate-upgrade-home-operations-com-v1alpha1-talosupgrade,mutating=false,failurePolicy=fail,sideEffects=None,groups=tuppr.home-operations.com,resources=talosupgrades,verbs=create;update,versions=v1alpha1,name=vtalosupgrade.kb.io,admissionReviewVersions=v1

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

		if !compareNodeSelectorExprs(talos.Spec.Target.NodeSelectorExprs, oldTalos.Spec.Target.NodeSelectorExprs) {
			return nil, fmt.Errorf("cannot update spec.NodeSelectorExprs while upgrade is in progress (current phase: %s)", oldTalos.Status.Phase)
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
		warnings = append(warnings, "No nodes match the specified node selector. The upgrade plan will not target any nodes.")
	}

	// Check for node overlap with other TalosUpgrades
	if overlaps, err := v.checkNodeOverlap(ctx, talos); err != nil {
		return warnings, fmt.Errorf("overlap validation failed: %w", err)
	} else if len(overlaps) > 0 {
		return warnings, fmt.Errorf("node selection overlaps with active TalosUpgrade resources: %v. Please wait for them to complete or adjust node selectors to avoid conflicts", overlaps)
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

	if err := v.Client.List(ctx, nodeList); err != nil {
		return 0, fmt.Errorf("failed to list nodes: %w", err)
	}

	matchingNodes := v.getMatchingNodes(nodeList.Items, talos)
	return len(matchingNodes), nil
}

func (v *TalosUpgradeValidator) checkNodeOverlap(ctx context.Context, currentTalos *upgradev1alpha1.TalosUpgrade) ([]string, error) {
	// Get all TalosUpgrade resources
	upgradeList := &upgradev1alpha1.TalosUpgradeList{}
	if err := v.Client.List(ctx, upgradeList); err != nil {
		return nil, fmt.Errorf("failed to list TalosUpgrade resources: %w", err)
	}

	// Get all nodes
	nodeList := &corev1.NodeList{}
	if err := v.Client.List(ctx, nodeList); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	// Get nodes that would match the current TalosUpgrade
	currentNodes := v.getMatchingNodes(nodeList.Items, currentTalos)
	currentNodeSet := make(map[string]bool)
	for _, node := range currentNodes {
		currentNodeSet[node.Name] = true
	}

	var overlappingUpgrades []string

	for _, upgrade := range upgradeList.Items {
		// Skip self and completed/failed upgrades
		if upgrade.Name == currentTalos.Name ||
			upgrade.Status.Phase == "Completed" ||
			upgrade.Status.Phase == "Failed" {
			continue
		}

		// Get nodes that match this upgrade
		upgradeNodes := v.getMatchingNodes(nodeList.Items, &upgrade)

		// Check for overlap
		for _, node := range upgradeNodes {
			if currentNodeSet[node.Name] {
				overlappingUpgrades = append(overlappingUpgrades, upgrade.Name)
				break
			}
		}
	}

	slices.Sort(overlappingUpgrades)
	return slices.Compact(overlappingUpgrades), nil
}

func (v *TalosUpgradeValidator) getMatchingNodes(allNodes []corev1.Node, talos *upgradev1alpha1.TalosUpgrade) []corev1.Node {
	return v.NodeMatcher.GetMatchingNodes(allNodes, talos)
}

func compareNodeSelectorExprs(a, b []corev1.NodeSelectorRequirement) bool {
	if len(a) != len(b) {
		return false
	}

	// Create maps for easier comparison
	aMap := make(map[string]corev1.NodeSelectorRequirement)
	bMap := make(map[string]corev1.NodeSelectorRequirement)

	// Build map for slice a
	for _, expr := range a {
		// Create a unique key for each expression
		key := fmt.Sprintf("%s-%s", expr.Key, expr.Operator)
		aMap[key] = expr
	}

	// Build map for slice b
	for _, expr := range b {
		key := fmt.Sprintf("%s-%s", expr.Key, expr.Operator)
		bMap[key] = expr
	}

	// Compare maps
	if len(aMap) != len(bMap) {
		return false
	}

	for key, exprA := range aMap {
		exprB, exists := bMap[key]
		if !exists {
			return false
		}

		// Compare the expressions
		if !compareNodeSelectorRequirement(exprA, exprB) {
			return false
		}
	}

	return true
}

// compareNodeSelectorRequirement compares two NodeSelectorRequirement for equality
func compareNodeSelectorRequirement(a, b corev1.NodeSelectorRequirement) bool {
	if a.Key != b.Key || a.Operator != b.Operator {
		return false
	}

	// Compare values - they must be in the same order and have the same content
	if len(a.Values) != len(b.Values) {
		return false
	}

	// Sort both slices to ensure consistent comparison
	aSorted := make([]string, len(a.Values))
	bSorted := make([]string, len(b.Values))
	copy(aSorted, a.Values)
	copy(bSorted, b.Values)

	slices.Sort(aSorted)
	slices.Sort(bSorted)

	for i := range aSorted {
		if aSorted[i] != bSorted[i] {
			return false
		}
	}

	return true
}

func (v *TalosUpgradeValidator) validateTalosSpec(talos *upgradev1alpha1.TalosUpgrade) error {
	// Validate image repository and tag are not empty
	if talos.Spec.Target.Image.Repository == "" {
		return fmt.Errorf("spec.target.image.repository cannot be empty")
	}

	if talos.Spec.Target.Image.Tag == "" {
		return fmt.Errorf("spec.target.image.tag cannot be empty")
	}

	// Validate node selector expressions
	for i, expr := range talos.Spec.Target.NodeSelectorExprs {
		if expr.Key == "" {
			return fmt.Errorf("spec.target.nodeSelectorExprs[%d].key cannot be empty", i)
		}

		validOps := []corev1.NodeSelectorOperator{
			corev1.NodeSelectorOpIn,
			corev1.NodeSelectorOpNotIn,
			corev1.NodeSelectorOpExists,
			corev1.NodeSelectorOpDoesNotExist,
			corev1.NodeSelectorOpGt,
			corev1.NodeSelectorOpLt,
		}

		validOp := false
		for _, op := range validOps {
			if expr.Operator == op {
				validOp = true
				break
			}
		}

		if !validOp {
			return fmt.Errorf("spec.target.nodeSelectorExprs[%d].operator '%s' is invalid", i, expr.Operator)
		}

		// Validate that value-requiring operators have values
		if (expr.Operator == corev1.NodeSelectorOpIn ||
			expr.Operator == corev1.NodeSelectorOpNotIn ||
			expr.Operator == corev1.NodeSelectorOpGt ||
			expr.Operator == corev1.NodeSelectorOpLt) && len(expr.Values) == 0 {
			return fmt.Errorf("spec.target.nodeSelectorExprs[%d] with operator '%s' requires at least one value", i, expr.Operator)
		}

		// Validate that non-value operators don't have values
		if (expr.Operator == corev1.NodeSelectorOpExists ||
			expr.Operator == corev1.NodeSelectorOpDoesNotExist) && len(expr.Values) > 0 {
			return fmt.Errorf("spec.target.nodeSelectorExprs[%d] with operator '%s' must not have values", i, expr.Operator)
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
	if len(talos.Spec.Target.NodeSelectorExprs) == 0 {
		warnings = append(warnings, "No node selector specified. This will upgrade ALL nodes in the cluster.")
	}

	// Add warning for debug mode
	if talos.Spec.Target.Options.Debug {
		warnings = append(warnings, "Debug mode enabled. This will produce verbose output in upgrade jobs.")
	}

	return warnings
}

func (v *TalosUpgradeValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&upgradev1alpha1.TalosUpgrade{}).
		WithValidator(v).
		Complete()
}
