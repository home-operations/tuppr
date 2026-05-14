package talosupgrade

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

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
