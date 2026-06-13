package talosupgrade

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
)

const upgradingLabelValue = "true"

// addNodeUpgradingLabel adds the upgrading label to a node
func (r *Reconciler) addNodeUpgradingLabel(ctx context.Context, nodeName string) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			return err
		}

		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}

		if node.Labels[constants.NodeUpgradingLabel] == upgradingLabelValue {
			return nil
		}

		node.Labels[constants.NodeUpgradingLabel] = upgradingLabelValue
		return r.Update(ctx, node)
	})
	if err != nil {
		return fmt.Errorf("failed to add upgrading label to node %s: %w", nodeName, err)
	}
	return nil
}

// removeNodeUpgradingLabel removes the upgrading label from a node
func (r *Reconciler) removeNodeUpgradingLabel(ctx context.Context, nodeName string) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}

		if node.Labels == nil || node.Labels[constants.NodeUpgradingLabel] == "" {
			return nil
		}

		delete(node.Labels, constants.NodeUpgradingLabel)
		return r.Update(ctx, node)
	})
	if err != nil {
		return fmt.Errorf("failed to remove upgrading label from node %s: %w", nodeName, err)
	}
	return nil
}

// addNodeOutdatedTaint adds the PreferNoSchedule outdated taint to a node.
func (r *Reconciler) addNodeOutdatedTaint(ctx context.Context, nodeName string) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			return err
		}

		for i := range node.Spec.Taints {
			if node.Spec.Taints[i].Key == constants.NodeOutdatedTaint {
				return nil
			}
		}

		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    constants.NodeOutdatedTaint,
			Effect: corev1.TaintEffectPreferNoSchedule,
		})
		return r.Update(ctx, node)
	})
	if err != nil {
		return fmt.Errorf("failed to add outdated taint to node %s: %w", nodeName, err)
	}
	return nil
}

// removeNodeOutdatedTaint removes the outdated taint from a node.
func (r *Reconciler) removeNodeOutdatedTaint(ctx context.Context, nodeName string) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}

		filtered := make([]corev1.Taint, 0, len(node.Spec.Taints))
		removed := false
		for _, t := range node.Spec.Taints {
			if t.Key == constants.NodeOutdatedTaint {
				removed = true
				continue
			}
			filtered = append(filtered, t)
		}
		if !removed {
			return nil
		}

		node.Spec.Taints = filtered
		return r.Update(ctx, node)
	})
	if err != nil {
		return fmt.Errorf("failed to remove outdated taint from node %s: %w", nodeName, err)
	}
	return nil
}

// addNodeUpgradingTaint adds the PreferNoSchedule upgrading taint to a node.
func (r *Reconciler) addNodeUpgradingTaint(ctx context.Context, nodeName string) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			return err
		}

		for i := range node.Spec.Taints {
			if node.Spec.Taints[i].Key == constants.NodeUpgradingTaint {
				return nil
			}
		}

		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    constants.NodeUpgradingTaint,
			Effect: corev1.TaintEffectPreferNoSchedule,
		})
		return r.Update(ctx, node)
	})
	if err != nil {
		return fmt.Errorf("failed to add upgrading taint to node %s: %w", nodeName, err)
	}
	return nil
}

// removeNodeUpgradingTaint removes the upgrading taint from a node.
func (r *Reconciler) removeNodeUpgradingTaint(ctx context.Context, nodeName string) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}

		filtered := make([]corev1.Taint, 0, len(node.Spec.Taints))
		removed := false
		for _, t := range node.Spec.Taints {
			if t.Key == constants.NodeUpgradingTaint {
				removed = true
				continue
			}
			filtered = append(filtered, t)
		}
		if !removed {
			return nil
		}

		node.Spec.Taints = filtered
		return r.Update(ctx, node)
	})
	if err != nil {
		return fmt.Errorf("failed to remove upgrading taint from node %s: %w", nodeName, err)
	}
	return nil
}

// clearOutdatedTaints removes the outdated and upgrading taints from every selected node.
func (r *Reconciler) clearOutdatedTaints(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) {
	logger := log.FromContext(ctx)

	nodes, err := r.getSortedNodes(ctx, talosUpgrade.Spec.NodeSelector)
	if err != nil {
		logger.Error(err, "Failed to list nodes to clear outdated taints")
		return
	}
	for i := range nodes {
		if err := r.removeNodeOutdatedTaint(ctx, nodes[i].Name); err != nil {
			logger.Error(err, "Failed to remove outdated taint during cleanup", "node", nodes[i].Name)
		}
		if err := r.removeNodeUpgradingTaint(ctx, nodes[i].Name); err != nil {
			logger.Error(err, "Failed to remove upgrading taint during cleanup", "node", nodes[i].Name)
		}
	}
}
