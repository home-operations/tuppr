package talosupgrade

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/compatibility"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
	"github.com/home-operations/tuppr/internal/webhook/validation"
)

// maxListedNodes caps how many node names a single message lists.
const maxListedNodes = 3

// validateUpgradePath checks the version each selected node would be upgraded
// to against the Talos and Kubernetes versions it runs, using the
// compatibility tables Talos itself ships in machinery. Talos's installer
// refuses an unsupported Talos version path, but only once the node has been
// cordoned and drained. Talos checks Kubernetes support only when it validates
// the machine config, after the node has rebooted into the new version, so
// without this check every node in the plan would be upgraded before the
// mismatch surfaced.
//
// Nodes without a Talos osImage are skipped entirely. While a
// KubernetesUpgrade is active, the target must also support its version: the
// controller holds this plan until that upgrade finishes, but starts it anyway
// if that upgrade fails, so the running kubelet must stay supported too. Node
// changes after admission (annotations, labels, new nodes) are not re-checked.
func (v *Validator) validateUpgradePath(ctx context.Context, t *tupprv1alpha1.TalosUpgrade, nodes []corev1.Node) (admission.Warnings, error) {
	var (
		warnings   admission.Warnings
		kubeTarget string
		targets    = map[string]talosTarget{}
		problems   = map[string][]string{}
		downgrades = map[string][]string{}
		unchecked  []string
	)
	if ku := v.activeKubernetesUpgrade(ctx); ku != nil {
		kubeTarget = ku.Spec.Kubernetes.Version
		warnings = append(warnings, fmt.Sprintf(
			"KubernetesUpgrade '%s' is in progress; Kubernetes support was checked against its target %s as well as the running kubelets.",
			ku.Name, kubeTarget))
	}

	for i := range nodes {
		node := &nodes[i]
		current := nodeutil.TalosVersionFromOSImage(node.Status.NodeInfo.OSImage)
		target := nodeutil.TargetTalosVersion(node, t.Spec.Talos.Version)
		// A raw comparison, like the controller's: it treats "1.13.0" and
		// "v1.13.0" as different versions, so the former is checked below.
		if current == "" || current == target {
			continue
		}

		name := node.Name
		if node.Annotations[constants.VersionAnnotation] != "" {
			name += " (pinned by " + constants.VersionAnnotation + ")"
		}

		tgt, ok := targets[target]
		if !ok {
			tgt = parseTalosTarget(target)
			targets[target] = tgt
		}
		if tgt.err != nil {
			reason := fmt.Sprintf("invalid target version %q: %v", target, tgt.err)
			problems[reason] = append(problems[reason], name)
			continue
		}
		hostSemver, host, err := parseTalosVersion(current)
		if err != nil {
			continue
		}

		var pathErr error
		if tgt.inTables {
			pathErr = tgt.version.UpgradeableFrom(host)
		} else if !slices.Contains(unchecked, target) {
			unchecked = append(unchecked, target)
		}
		if pathErr != nil {
			problems[pathErr.Error()] = append(problems[pathErr.Error()], name)
		} else if hostSemver.GreaterThan(tgt.semver) {
			key := current + " to " + target
			downgrades[key] = append(downgrades[key], name)
		}

		if !tgt.inTables {
			continue
		}
		kubes := []string{node.Status.NodeInfo.KubeletVersion}
		if kubeTarget != "" && kubeTarget != kubes[0] {
			kubes = append(kubes, kubeTarget)
		}
		for _, kube := range kubes {
			if kube == "" {
				continue
			}
			kubeVersion, err := compatibility.ParseKubernetesVersion(kube)
			if err != nil {
				continue
			}
			if err := kubeVersion.SupportedWith(tgt.version); err != nil {
				problems[err.Error()] = append(problems[err.Error()], name)
			}
		}
	}

	for _, target := range unchecked {
		warnings = append(warnings, fmt.Sprintf(
			"Talos %s is not in the compatibility data this tuppr release was built with; the upgrade path and Kubernetes support were not checked.",
			target))
	}
	for _, key := range slices.Sorted(maps.Keys(downgrades)) {
		warnings = append(warnings, fmt.Sprintf(
			"Node(s) %s will be downgraded from Talos %s.", listNodes(downgrades[key]), key))
	}
	if len(problems) == 0 {
		return warnings, nil
	}

	reasons := make([]string, 0, len(problems))
	for _, reason := range slices.Sorted(maps.Keys(problems)) {
		reasons = append(reasons, fmt.Sprintf("%s (node(s) %s)", reason, listNodes(problems[reason])))
	}
	return warnings, fmt.Errorf("unsupported upgrade: %s", strings.Join(reasons, "; "))
}

// activeKubernetesUpgrade returns the KubernetesUpgrade in progress, if any.
// A list error skips checking its target, like the other cluster checks here.
func (v *Validator) activeKubernetesUpgrade(ctx context.Context) *tupprv1alpha1.KubernetesUpgrade {
	list := &tupprv1alpha1.KubernetesUpgradeList{}
	if err := v.Client.List(ctx, list); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list KubernetesUpgrades for upgrade path validation")
		return nil
	}
	for i := range list.Items {
		if list.Items[i].Status.Phase.IsActive() {
			return &list.Items[i]
		}
	}
	return nil
}

// talosTarget is a node's target Talos version, parsed once per distinct value.
type talosTarget struct {
	semver   *semver.Version
	version  *compatibility.TalosVersion
	inTables bool
	err      error
}

func parseTalosTarget(target string) talosTarget {
	// Nothing else validates an annotation, and the controller uses it
	// verbatim as the installer image tag, so it must match the format
	// spec.talos.version is held to.
	if err := validation.ValidateVersionFormat(target); err != nil {
		return talosTarget{err: err}
	}
	sv, version, err := parseTalosVersion(target)
	if err != nil {
		return talosTarget{err: err}
	}
	// Every table accepts a host on the target's own minor, so a failed probe
	// means the target is outside the tables tuppr was built with. The
	// pre-release keeps the probe from matching DeniedHostUpgradeVersions.
	_, probe, err := parseTalosVersion(fmt.Sprintf("v%d.%d.0-tuppr.probe", sv.Major(), sv.Minor()))
	return talosTarget{
		semver:   sv,
		version:  version,
		inTables: err == nil && version.UpgradeableFrom(probe) == nil,
	}
}

// parseTalosVersion parses v strictly. Machinery's tolerant parser reads
// "v1.014.0" as 1.14.0, but the controller uses a target verbatim as the
// installer image tag.
func parseTalosVersion(v string) (*semver.Version, *compatibility.TalosVersion, error) {
	sv, err := semver.StrictNewVersion(strings.TrimPrefix(v, "v"))
	if err != nil {
		return nil, nil, err
	}
	version, err := compatibility.ParseTalosVersion(&machine.VersionInfo{Tag: v})
	return sv, version, err
}

func listNodes(names []string) string {
	slices.Sort(names)
	if len(names) > maxListedNodes {
		return fmt.Sprintf("%s and %d more", strings.Join(names[:maxListedNodes], ", "), len(names)-maxListedNodes)
	}
	return strings.Join(names, ", ")
}
