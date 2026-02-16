package coordination

import (
	"context"
	"errors"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/home-operations/tuppr/api/v1alpha1"
)

const (
	UpgradeTypeTalos      = "talos"
	UpgradeTypeKubernetes = "kubernetes"
)

var ErrOtherUpgradeInProgress = errors.New("another upgrade is in progress")

func IsAnotherUpgradeActive(ctx context.Context, c client.Client, currentUpgradeName string, currentUpgradeType string) (bool, string, error) {
	switch currentUpgradeType {
	case UpgradeTypeTalos:
		return isTalosUpgradeBlocking(ctx, c, currentUpgradeName)
	case UpgradeTypeKubernetes:
		return isKubernetesUpgradeBlocking(ctx, c)
	default:
		return false, "", nil
	}
}

// isTalosUpgradeBlocking checks if a Talos upgrade can proceed.
// Rules:
// 1. Block if ANY Kubernetes upgrade is InProgress.
// 2. Block if OTHER Talos upgrades are InProgress (but allow self).
func isTalosUpgradeBlocking(ctx context.Context, c client.Client, currentUpgradeName string) (bool, string, error) {
	kubernetesUpgrades := &v1alpha1.KubernetesUpgradeList{}
	if err := c.List(ctx, kubernetesUpgrades); err != nil {
		return false, "", fmt.Errorf("failed to list KubernetesUpgrade resources: %w", err)
	}

	for _, upgrade := range kubernetesUpgrades.Items {
		if upgrade.Status.Phase.IsActive() {
			return true, fmt.Sprintf("Waiting for KubernetesUpgrade '%s' to complete", upgrade.Name), nil
		}
	}

	talosUpgrades := &v1alpha1.TalosUpgradeList{}
	if err := c.List(ctx, talosUpgrades); err != nil {
		return false, "", fmt.Errorf("failed to list TalosUpgrade resources: %w", err)
	}

	for _, upgrade := range talosUpgrades.Items {
		if upgrade.Name == currentUpgradeName {
			continue
		}

		if upgrade.Status.Phase.IsActive() {
			return true, fmt.Sprintf("Waiting for another TalosUpgrade plan '%s' to complete", upgrade.Name), nil
		}
	}

	return false, "", nil
}

// isKubernetesUpgradeBlocking checks if a Kubernetes upgrade can proceed.
// Rules:
//  1. Block if ANY Talos upgrade is InProgress OR Pending.
//     (Kubernetes upgrades are critical; we don't want to start if a Talos upgrade is even queued).
func isKubernetesUpgradeBlocking(ctx context.Context, c client.Client) (bool, string, error) {
	talosUpgrades := &v1alpha1.TalosUpgradeList{}
	if err := c.List(ctx, talosUpgrades); err != nil {
		return false, "", fmt.Errorf("failed to list TalosUpgrade resources: %w", err)
	}

	for _, upgrade := range talosUpgrades.Items {
		if upgrade.Status.Phase.IsActive() || upgrade.Status.Phase == v1alpha1.JobPhasePending {
			return true, fmt.Sprintf("Waiting for TalosUpgrade '%s' to complete", upgrade.Name), nil
		}
	}

	return false, "", nil
}
