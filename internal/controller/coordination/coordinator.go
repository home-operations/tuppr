package coordination

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
)

func IsAnotherUpgradeActive(ctx context.Context, c client.Client, currentUpgradeName string, currentUpgradeType string) (bool, string, error) {
	if currentUpgradeType == "talos" {
		kubernetesUpgrades := &v1alpha1.KubernetesUpgradeList{}
		if err := c.List(ctx, kubernetesUpgrades); err != nil {
			return false, "", fmt.Errorf("failed to list KubernetesUpgrade resources: %w", err)
		}

		for _, upgrade := range kubernetesUpgrades.Items {
			if upgrade.Status.Phase == constants.PhaseInProgress {
				return true, fmt.Sprintf("Waiting for KubernetesUpgrade '%s' to complete", upgrade.Name), nil
			}
		}
	} else {
		talosUpgrades := &v1alpha1.TalosUpgradeList{}
		if err := c.List(ctx, talosUpgrades); err != nil {
			return false, "", fmt.Errorf("failed to list TalosUpgrade resources: %w", err)
		}

		for _, upgrade := range talosUpgrades.Items {
			if upgrade.Status.Phase == constants.PhaseInProgress || upgrade.Status.Phase == constants.PhasePending {
				return true, fmt.Sprintf("Waiting for TalosUpgrade '%s' to complete", upgrade.Name), nil
			}
		}
	}
	if currentUpgradeType == "talos" {
		talosUpgrades := &v1alpha1.TalosUpgradeList{}
		if err := c.List(ctx, talosUpgrades); err != nil {
			return false, "", fmt.Errorf("failed to list TalosUpgrade resources: %w", err)
		}

		for _, upgrade := range talosUpgrades.Items {
			if upgrade.Name == currentUpgradeName {
				continue
			}

			if upgrade.Status.Phase == constants.PhaseInProgress {
				return true, fmt.Sprintf("Waiting for another TalosUpgrade plan '%s' to complete", upgrade.Name), nil
			}
		}
	}

	return false, "", nil
}
