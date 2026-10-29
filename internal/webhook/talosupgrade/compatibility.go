package talosupgrade

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/compatibility"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
)

// maxListedNodes caps how many node names a single message lists.
const maxListedNodes = 3

// validateUpgradePath checks the version each selected node would be upgraded
// to against the Talos and Kubernetes versions it runs now, using the
// compatibility tables Talos itself ships in machinery. Talos's installer
// refuses an unsupported Talos version path, but only once the node has been
// cordoned and drained. Talos checks Kubernetes support only when it validates
// the machine config, after the node has rebooted into the new version, so
// without this check every node in the plan would be upgraded before the
// mismatch surfaced.
//
// Nodes whose osImage or kubeletVersion is not reported are skipped, and an
// API error fails open, like the other cluster-state checks here. Node changes
// after admission (annotations, labels, new nodes) are not re-checked.
func (v *Validator) validateUpgradePath(ctx context.Context, t *tupprv1alpha1.TalosUpgrade) (admission.Warnings, error) {
	nodes, err := v.listSelectedNodes(ctx, t.Spec.NodeSelector)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to list nodes for upgrade path validation")
		return nil, nil
	}

	var (
		warnings   admission.Warnings
		problems   = map[string][]string{}
		downgrades = map[string][]string{}
		unchecked  []string
	)
	for i := range nodes {
		node := &nodes[i]
		target := nodeutil.TargetTalosVersion(node, t.Spec.Talos.Version)
		current := nodeutil.TalosVersionFromOSImage(node.Status.NodeInfo.OSImage)
		if current == "" {
			continue
		}
		// The same tolerant parse machinery uses, so "1.13.0" and "v1.13.0"
		// are the same version.
		targetSemver, err := semver.ParseTolerant(target)
		if err != nil {
			continue
		}
		currentSemver, err := semver.ParseTolerant(current)
		if err != nil || currentSemver.EQ(targetSemver) {
			continue
		}

		name := node.Name
		if node.Annotations[constants.VersionAnnotation] != "" {
			name += " (pinned by " + constants.VersionAnnotation + ")"
		}

		targetVersion, err := compatibility.ParseTalosVersion(&machine.VersionInfo{Tag: target})
		if err != nil {
			continue
		}
		// A release in the tables can always be reinstalled over itself, so
		// failing this means target is outside the tables tuppr was built with.
		if targetVersion.UpgradeableFrom(targetVersion) != nil {
			if !slices.Contains(unchecked, target) {
				unchecked = append(unchecked, target)
			}
			continue
		}

		hostVersion, err := compatibility.ParseTalosVersion(&machine.VersionInfo{Tag: current})
		if err != nil {
			continue
		}
		if err := targetVersion.UpgradeableFrom(hostVersion); err != nil {
			problems[err.Error()] = append(problems[err.Error()], name)
			continue
		}
		if currentSemver.GT(targetSemver) {
			key := current + " to " + target
			downgrades[key] = append(downgrades[key], name)
		}

		if kubelet := node.Status.NodeInfo.KubeletVersion; kubelet != "" {
			k8sVersion, err := compatibility.ParseKubernetesVersion(kubelet)
			if err != nil {
				continue
			}
			if err := k8sVersion.SupportedWith(targetVersion); err != nil {
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

func listNodes(names []string) string {
	if len(names) > maxListedNodes {
		return fmt.Sprintf("%s and %d more", strings.Join(names[:maxListedNodes], ", "), len(names)-maxListedNodes)
	}
	return strings.Join(names, ", ")
}
