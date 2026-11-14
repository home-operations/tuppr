package talosupgrade

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
	"github.com/home-operations/tuppr/internal/webhook/validation"
)

const (
	rebootModeDefault = "default"
	controlPlaneLabel = "node-role.kubernetes.io/control-plane"
)

// Validator validates Talos resources
type Validator struct {
	Client            client.Client
	TalosConfigSecret string
	Namespace         string
	// AlertmanagerConfigured reflects whether the operator has an Alertmanager
	// connection, so spec.silences without one warns at apply time instead of
	// being silently inert.
	AlertmanagerConfigured bool
	// APIReader reads the talosconfig Secret uncached. A cached read would
	// start a Secret informer, which needs list/watch on Secrets in every
	// namespace.
	APIReader client.Reader
}

// +kubebuilder:webhook:path=/validate-tuppr-home-operations-com-v1alpha1-talosupgrade,mutating=false,failurePolicy=fail,sideEffects=None,groups=tuppr.home-operations.com,resources=talosupgrades,verbs=create;update,versions=v1alpha1,name=vtalosupgrade.kb.io,admissionReviewVersions=v1
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades,verbs=get;list;watch
// +kubebuilder:rbac:groups="",namespace=tuppr-system,resources=secrets,verbs=get

var _ admission.Validator[*tupprv1alpha1.TalosUpgrade] = &Validator{}

// ValidateCreate implements admission.Validator so a webhook will be registered for the type
func (v *Validator) ValidateCreate(ctx context.Context, t *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	log.FromContext(ctx).V(1).Info("Validating create", "targetVersion", t.Spec.Talos.Version)
	return v.validate(ctx, t, true)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *Validator) ValidateUpdate(ctx context.Context, old, t *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Validating update", "targetVersion", t.Spec.Talos.Version)

	if err := validation.ValidateUpdateInProgress(old.Status.Conditions, old.Status.Phase, old.Spec, t.Spec); err != nil {
		logger.Info("Rejected TalosUpgrade", "targetVersion", t.Spec.Talos.Version, "reason", err.Error())
		return nil, err
	}
	// Only a changed target or selector re-checks the upgrade path, so a
	// metadata-only update (such as removing the finalizer) is never refused
	// because the nodes have moved on since the plan was admitted.
	checkUpgradePath := old.Spec.Talos.Version != t.Spec.Talos.Version ||
		!sameNodeSelector(old.Spec.NodeSelector, t.Spec.NodeSelector)
	return v.validate(ctx, t, checkUpgradePath)
}

// sameNodeSelector reports whether two nodeSelectors select the same nodes, so
// a nil and an empty selector (both every node) compare equal.
func sameNodeSelector(a, b *metav1.LabelSelector) bool {
	selA, errA := nodeutil.ParseNodeSelector(a)
	selB, errB := nodeutil.ParseNodeSelector(b)
	if errA != nil || errB != nil {
		return reflect.DeepEqual(a, b)
	}
	return selA.String() == selB.String()
}

func (v *Validator) ValidateDelete(ctx context.Context, t *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	if t.Status.Phase.IsActive() {
		return admission.Warnings{
			fmt.Sprintf("Deleting TalosUpgrade '%s' while upgrade is in progress. This may leave nodes in an inconsistent state.", t.Name),
		}, nil
	}
	return nil, nil
}

// validate logs the admission outcome itself: the apiserver only relays a
// rejection to the caller, so it is the one decision worth an Info line.
func (v *Validator) validate(ctx context.Context, t *tupprv1alpha1.TalosUpgrade, checkUpgradePath bool) (warnings admission.Warnings, err error) {
	logger := log.FromContext(ctx)
	defer func() {
		if err != nil {
			logger.Info("Rejected TalosUpgrade", "targetVersion", t.Spec.Talos.Version, "reason", err.Error())
			return
		}
		logger.V(1).Info("TalosUpgrade validation successful", "targetVersion", t.Spec.Talos.Version, "warningCount", len(warnings))
	}()

	// Listed once for every node check below. Each check fails open, like
	// the other cluster-state checks here, when nodes is nil.
	nodes, listErr := nodeutil.ListSelectedNodes(ctx, v.Client, t.Spec.NodeSelector)
	if listErr != nil {
		logger.Error(listErr, "Failed to list selected nodes")
	}

	overlapWarnings, err := v.validateOverlaps(ctx, t, nodes)
	if err != nil {
		// We fail open if we can't check overlaps (e.g. API error), but log it
		logger.Error(err, "Failed to check for overlaps")
	} else {
		warnings = append(warnings, overlapWarnings...)
	}

	if _, err := validation.ValidateTalosConfigSecret(ctx, v.APIReader, v.TalosConfigSecret, v.Namespace); err != nil {
		return warnings, err
	}

	if err := validation.ValidateVersionFormat(t.Spec.Talos.Version); err != nil {
		return warnings, fmt.Errorf("invalid talos version: %w", err)
	}

	if checkUpgradePath {
		if errors.Is(listErr, nodeutil.ErrInvalidNodeSelector) {
			return warnings, listErr
		}
		pathWarnings, err := v.validateUpgradePath(ctx, t, nodes)
		warnings = append(warnings, pathWarnings...)
		if err != nil {
			return warnings, err
		}
	}

	if err := validation.ValidateHealthChecks(t.Spec.HealthChecks); err != nil {
		return warnings, err
	}

	if err := validation.ValidateTalosctlSpec(t.Spec.Talosctl); err != nil {
		return warnings, err
	}

	// Validate Policy
	if t.Spec.Policy.RebootMode != "" && t.Spec.Policy.RebootMode != rebootModeDefault && t.Spec.Policy.RebootMode != "powercycle" {
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
		warnings = append(warnings, "Force upgrade enabled: tuppr skips the etcd check of its cluster health gate. Talos skips its own etcd health and membership checks only on Talos's legacy upgrade API, which talosctl uses when the node or talosctl is older than Talos 1.13. Control-plane nodes are still upgraded one at a time.")
	}
	if t.Spec.Policy.Stage {
		warnings = append(warnings, "Stage upgrade enabled: it only takes effect on Talos's legacy upgrade API, which talosctl uses when the node or talosctl is older than Talos 1.13.")
	}
	warnings = append(warnings, v.controlPlaneWarnings(ctx)...)
	if t.Spec.Policy.RebootMode == "powercycle" {
		warnings = append(warnings, "Powercycle reboot mode selected.")
	}
	if t.Spec.Policy.Debug {
		warnings = append(warnings, "Debug mode enabled.")
	}
	if t.Spec.Policy.Placement == "soft" {
		warnings = append(warnings, "Soft placement preset used.")
	}
	if len(t.Spec.Silences) > 0 && !v.AlertmanagerConfigured {
		warnings = append(warnings, "spec.silences is configured but the operator has no Alertmanager connection (set silences.* in the Helm chart); no silences will be created.")
	}

	// Validate maintenance window if specified
	if mwWarnings, err := validation.ValidateMaintenanceWindows(t.Spec.Maintenance); err != nil {
		return warnings, fmt.Errorf("spec.maintenanceWindow validation failed: %w", err)
	} else {
		warnings = append(warnings, mwWarnings...)
	}

	// Validate parallelism
	if pWarnings, err := validateParallelism(t, nodes); err != nil {
		return warnings, err
	} else {
		warnings = append(warnings, pWarnings...)
	}

	if err := validateHooks(t.Spec.Hooks); err != nil {
		return warnings, err
	}

	return warnings, nil
}

// validateOverlaps checks if the new/updated TalosUpgrade targets nodes that are already
// targeted by other existing TalosUpgrade resources.
func (v *Validator) validateOverlaps(ctx context.Context, current *tupprv1alpha1.TalosUpgrade, nodes []corev1.Node) (admission.Warnings, error) {
	var warnings admission.Warnings

	if len(nodes) == 0 {
		return nil, nil
	}
	currentNodes := nodeNameSet(nodes)

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
	nodes, err := nodeutil.ListSelectedNodes(ctx, v.Client, labelSelector)
	if err != nil {
		return nil, err
	}
	return nodeNameSet(nodes), nil
}

func nodeNameSet(nodes []corev1.Node) map[string]bool {
	nodeSet := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		nodeSet[node.Name] = true
	}
	return nodeSet
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

// validateParallelism checks that parallelism is within valid bounds. No
// nodes (none selected, or the nodes couldn't be listed) skips the checks
// against the selection.
func validateParallelism(t *tupprv1alpha1.TalosUpgrade, nodes []corev1.Node) (admission.Warnings, error) {
	if t.Spec.Parallelism == nil {
		return nil, nil
	}

	p := *t.Spec.Parallelism
	if p < 1 {
		return nil, fmt.Errorf("spec.parallelism must be >= 1, got %d", p)
	}

	if len(nodes) > 0 && int(p) > len(nodes) {
		return nil, fmt.Errorf("spec.parallelism (%d) exceeds number of matching nodes (%d)", p, len(nodes))
	}

	if p <= 1 {
		return nil, nil
	}
	warnings := admission.Warnings{
		fmt.Sprintf("Parallelism set to %d: up to %d worker nodes will be upgraded concurrently per batch; control-plane nodes are always upgraded one at a time.", p, p),
	}
	if len(nodes) == 0 {
		return warnings, nil
	}
	// Only worker nodes can share a batch.
	var workers int32
	for i := range nodes {
		if _, isCP := nodes[i].Labels[controlPlaneLabel]; !isCP {
			workers++
		}
	}
	if p > workers {
		warnings = append(warnings, fmt.Sprintf(
			"spec.parallelism (%d) exceeds the %d selected worker node(s); batches never hold more than %d node(s).",
			p, workers, max(workers, 1)))
	}
	return warnings, nil
}

// controlPlaneWarnings warns about a cluster with exactly two control-plane
// nodes: etcd needs both for quorum, so rebooting either loses it until the
// node is back. Talos older than 1.13 refuses that upgrade; newer Talos does
// not. Fails open: a listing error only skips the warning.
func (v *Validator) controlPlaneWarnings(ctx context.Context) admission.Warnings {
	nodeList := &corev1.NodeList{}
	if err := v.Client.List(ctx, nodeList, client.HasLabels{controlPlaneLabel}); err != nil {
		log.FromContext(ctx).Error(err, "Failed to count control-plane nodes")
		return nil
	}
	if len(nodeList.Items) != 2 {
		return nil
	}
	return admission.Warnings{
		"The cluster has exactly two control-plane nodes: etcd loses quorum while either one reboots, so the API is unavailable during each control-plane upgrade. Talos 1.13 and newer do not refuse it. Run three or more control-plane nodes.",
	}
}

// validateHooks rejects empty images and duplicate names within each hook
// list, a pre-batch hook setting the env var tuppr sets, and a pre-batch retry
// interval under a second.
func validateHooks(hooks *tupprv1alpha1.HooksSpec) error {
	if hooks == nil {
		return nil
	}
	if err := validateHookList("spec.hooks.pre", hooks.Pre); err != nil {
		return err
	}
	if err := validateHookList("spec.hooks.preBatch", hooks.PreBatch); err != nil {
		return err
	}
	for i, h := range hooks.PreBatch {
		for _, e := range h.Env {
			if e.Name == constants.PreBatchNodesEnv {
				return fmt.Errorf("spec.hooks.preBatch[%d].env: %s is set by tuppr to the batch's nodes", i, constants.PreBatchNodesEnv)
			}
		}
	}
	// A zero interval would re-create a failing hook Job in a tight loop.
	if r := hooks.PreBatchRetry; r != nil && r.Interval != nil && r.Interval.Duration < time.Second {
		return fmt.Errorf("spec.hooks.preBatchRetry.interval must be at least 1s, got %s", r.Interval.Duration)
	}
	return validateHookList("spec.hooks.post", hooks.Post)
}

func validateHookList(path string, list []tupprv1alpha1.HookSpec) error {
	seen := make(map[string]struct{}, len(list))
	for i, h := range list {
		if h.Name == "" {
			return fmt.Errorf("%s[%d].name is required", path, i)
		}
		if h.Image == "" {
			return fmt.Errorf("%s[%d].image is required", path, i)
		}
		if _, dup := seen[h.Name]; dup {
			return fmt.Errorf("%s: duplicate hook name %q", path, h.Name)
		}
		seen[h.Name] = struct{}{}
	}
	return nil
}

func (v *Validator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &tupprv1alpha1.TalosUpgrade{}).
		WithValidator(v).
		Complete()
}
