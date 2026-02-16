package talosupgrade

import (
	"context"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/webhook/validation"
)

var taloslog = logf.Log.WithName("talos-resource")

// Validator validates Talos resources
type Validator struct {
	Client            client.Client
	TalosConfigSecret string
	Namespace         string
}

// +kubebuilder:webhook:path=/validate-tuppr-home-operations-com-v1alpha1-talosupgrade,mutating=false,failurePolicy=fail,sideEffects=None,groups=tuppr.home-operations.com,resources=talosupgrades,verbs=create;update,versions=v1alpha1,name=vtalosupgrade.kb.io,admissionReviewVersions=v1
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades,verbs=get;list;watch

var _ admission.Validator[*tupprv1alpha1.TalosUpgrade] = &Validator{}

// ValidateCreate implements admission.Validator so a webhook will be registered for the type
func (v *Validator) ValidateCreate(ctx context.Context, t *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	taloslog.Info("validate create", "name", t.Name, "version", t.Spec.Talos.Version, "talosConfigSecret", v.TalosConfigSecret)
	return v.validate(ctx, t)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *Validator) ValidateUpdate(ctx context.Context, old, t *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	taloslog.Info("validate update", "name", t.Name)

	if err := validation.ValidateUpdateInProgress(old.Status.Phase, old.Spec, t.Spec); err != nil {
		return nil, err
	}
	return v.validate(ctx, t)
}

func (v *Validator) ValidateDelete(ctx context.Context, t *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	if t.Status.Phase.IsActive() {
		return admission.Warnings{
			fmt.Sprintf("Deleting TalosUpgrade '%s' while upgrade is in progress. This may leave nodes in an inconsistent state.", t.Name),
		}, nil
	}
	return nil, nil
}

func (v *Validator) validate(ctx context.Context, t *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	var warnings admission.Warnings

	overlapWarnings, err := v.validateOverlaps(ctx, t)
	if err != nil {
		// We fail open if we can't check overlaps (e.g. API error), but log it
		taloslog.Error(err, "failed to check for overlaps")
	} else {
		warnings = append(warnings, overlapWarnings...)
	}

	if _, err := validation.ValidateTalosConfigSecret(ctx, v.Client, v.TalosConfigSecret, v.Namespace); err != nil {
		return warnings, err
	}

	if err := validation.ValidateVersionFormat(t.Spec.Talos.Version); err != nil {
		return warnings, fmt.Errorf("invalid talos version: %w", err)
	}

	if err := validation.ValidateHealthChecks(t.Spec.HealthChecks); err != nil {
		return warnings, err
	}

	if err := validation.ValidateTalosctlSpec(t.Spec.Talosctl); err != nil {
		return warnings, err
	}

	// Validate Policy
	if t.Spec.Policy.RebootMode != "" && t.Spec.Policy.RebootMode != "default" && t.Spec.Policy.RebootMode != "powercycle" {
		return warnings, fmt.Errorf("invalid rebootMode '%s'", t.Spec.Policy.RebootMode)
	}
	if t.Spec.Policy.Placement != "" && t.Spec.Policy.Placement != "hard" && t.Spec.Policy.Placement != "soft" {
		return warnings, fmt.Errorf("invalid placement '%s'", t.Spec.Policy.Placement)
	}

	warnings = append(warnings, validation.GenerateCommonWarnings(
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
	if mwWarnings, err := validation.ValidateMaintenanceWindows(t.Spec.Maintenance); err != nil {
		return warnings, fmt.Errorf("spec.maintenanceWindow validation failed: %w", err)
	} else {
		warnings = append(warnings, mwWarnings...)
	}

	taloslog.Info("talos plan validation successful", "name", t.Name, "version", t.Spec.Talos.Version)
	return warnings, nil
}

// validateOverlaps checks if the new/updated TalosUpgrade targets nodes that are already
// targeted by other existing TalosUpgrade resources.
func (v *Validator) validateOverlaps(ctx context.Context, current *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	var warnings admission.Warnings

	currentNodes, err := v.getMatchingNodes(ctx, current.Spec.NodeSelector)
	if err != nil {
		return nil, err
	}

	if len(currentNodes) == 0 {
		return nil, nil
	}

	existingList := &tupprv1alpha1.TalosUpgradeList{}
	if err := v.Client.List(ctx, existingList); err != nil {
		return nil, err
	}

	for _, existing := range existingList.Items {
		if existing.Name == current.Name {
			continue
		}

		otherNodes, err := v.getMatchingNodes(ctx, existing.Spec.NodeSelector)
		if err != nil {
			return nil, err
		}

		intersection := findNodeIntersection(currentNodes, otherNodes)

		if len(intersection) > 0 {
			shownNodes := intersection
			if len(shownNodes) > 3 {
				shownNodes = append(shownNodes[:3], "...")
			}

			warnings = append(warnings, fmt.Sprintf(
				"Detected node overlap with existing plan '%s'. The following nodes match both plans: %v. "+
					"This may cause conflicting upgrades/downgrades if both plans are active.",
				existing.Name, shownNodes))
		}
	}

	return warnings, nil
}

// getMatchingNodes returns a set of node names that match the given selector
func (v *Validator) getMatchingNodes(ctx context.Context, labelSelector *metav1.LabelSelector) (map[string]bool, error) {
	var selector labels.Selector
	var err error

	if labelSelector != nil {
		selector, err = metav1.LabelSelectorAsSelector(labelSelector)
		if err != nil {
			return nil, fmt.Errorf("invalid nodeSelector: %w", err)
		}
	} else {
		selector = labels.Everything()
	}

	nodeList := &corev1.NodeList{}
	if err := v.Client.List(ctx, nodeList, &client.ListOptions{LabelSelector: selector}); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	nodeSet := make(map[string]bool)
	for _, node := range nodeList.Items {
		nodeSet[node.Name] = true
	}
	return nodeSet, nil
}

func findNodeIntersection(setA, setB map[string]bool) []string {
	var intersection []string
	for name := range setA {
		if setB[name] {
			intersection = append(intersection, name)
		}
	}
	slices.Sort(intersection)
	return intersection
}

func (v *Validator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &tupprv1alpha1.TalosUpgrade{}).
		WithValidator(v).
		Complete()
}
