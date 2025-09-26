package controller

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/home-operations/tuppr/internal/constants"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

// GetNodeInternalIP retrieves the InternalIP of a Kubernetes node
func GetNodeInternalIP(node *corev1.Node) (string, error) {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address, nil
		}
	}
	return "", fmt.Errorf("no InternalIP found for node %q", node.Name)
}

// GenerateSafeJobName generates a safe Kubernetes Job name
// It ensures the name is within Kubernetes' 63 character limit
// by truncating the identifier if necessary and appending a unique suffix.
func GenerateSafeJobName(prefix, identifier string) string {
	jobID := uuid.New().String()[:8]
	const maxLength = 57

	prefixLen := len(prefix) + len(jobID) + 2
	available := maxLength - prefixLen

	if available <= 0 {
		return fmt.Sprintf("%s-%s", prefix, jobID)
	}

	identifierLen := min(len(identifier), available)
	if identifierLen > 0 {
		return fmt.Sprintf("%s-%s-%s", prefix, identifier[:identifierLen], jobID)
	}

	return fmt.Sprintf("%s-%s", prefix, jobID)
}

// IsAnotherUpgradeActive checks if any other type of upgrade is currently active
func IsAnotherUpgradeActive(ctx context.Context, c client.Client, currentUpgradeType string) (bool, string, error) {
	if currentUpgradeType == "talos" {
		// Check if any KubernetesUpgrade is InProgress
		kubernetesUpgrades := &tupprv1alpha1.KubernetesUpgradeList{}
		if err := c.List(ctx, kubernetesUpgrades); err != nil {
			return false, "", fmt.Errorf("failed to list KubernetesUpgrade resources: %w", err)
		}

		for _, upgrade := range kubernetesUpgrades.Items {
			if upgrade.Status.Phase == constants.PhaseInProgress {
				return true, fmt.Sprintf("Waiting for KubernetesUpgrade '%s' to complete", upgrade.Name), nil
			}
		}
	} else {
		// Check if any TalosUpgrade is InProgress
		talosUpgrades := &tupprv1alpha1.TalosUpgradeList{}
		if err := c.List(ctx, talosUpgrades); err != nil {
			return false, "", fmt.Errorf("failed to list TalosUpgrade resources: %w", err)
		}

		for _, upgrade := range talosUpgrades.Items {
			if upgrade.Status.Phase == constants.PhaseInProgress {
				return true, fmt.Sprintf("Waiting for TalosUpgrade '%s' to complete", upgrade.Name), nil
			}
		}
	}

	return false, "", nil
}
