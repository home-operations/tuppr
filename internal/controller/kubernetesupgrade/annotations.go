package kubernetesupgrade

import (
	"context"
	"fmt"
	"maps"

	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
)

func (r *Reconciler) handleSuspendAnnotation(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (bool, error) {
	if kubernetesUpgrade.Annotations == nil {
		return false, nil
	}

	suspendValue, isSuspended := kubernetesUpgrade.Annotations[constants.SuspendAnnotation]
	if !isSuspended {
		return false, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Suspend annotation found, controller is suspended",
		"suspendValue", suspendValue,
		"kubernetesupgrade", kubernetesUpgrade.Name)

	message := fmt.Sprintf("Controller suspended via annotation (value: %s) - remove annotation to resume", suspendValue)
	if err := r.setPhase(ctx, kubernetesUpgrade, tupprv1alpha1.JobPhasePending, "", message); err != nil {
		logger.Error(err, "Failed to update phase for suspension")
		return true, err
	}

	logger.V(1).Info("Controller suspended, no further processing will occur",
		"kubernetesupgrade", kubernetesUpgrade.Name,
		"suspendValue", suspendValue)

	return true, nil
}

func (r *Reconciler) handleResetAnnotation(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (bool, error) {
	if kubernetesUpgrade.Annotations == nil {
		return false, nil
	}

	resetValue, hasReset := kubernetesUpgrade.Annotations[constants.ResetAnnotation]
	if !hasReset {
		return false, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Reset annotation found, clearing Kubernetes upgrade state", "resetValue", resetValue)

	newAnnotations := maps.Clone(kubernetesUpgrade.Annotations)
	maps.DeleteFunc(newAnnotations, func(k, v string) bool {
		return k == constants.ResetAnnotation
	})

	kubernetesUpgrade.Annotations = newAnnotations
	if err := r.Update(ctx, kubernetesUpgrade); err != nil {
		logger.Error(err, "Failed to remove reset annotation")
		return false, err
	}

	prevPhase := kubernetesUpgrade.Status.Phase
	if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":          tupprv1alpha1.JobPhasePending,
		"controllerNode": "",
		"message":        "Reset requested via annotation",
		"jobName":        "",
		"retries":        0,
		"lastError":      "",
	}); err != nil {
		logger.Error(err, "Failed to reset status after annotation")
		return false, err
	}
	kubernetesUpgrade.Status.Phase = tupprv1alpha1.JobPhasePending
	r.recordPhaseTransition(kubernetesUpgrade, prevPhase, tupprv1alpha1.JobPhasePending)

	return true, nil
}

func (r *Reconciler) handleGenerationChange(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (bool, error) {
	if kubernetesUpgrade.Status.ObservedGeneration >= kubernetesUpgrade.Generation {
		return false, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Spec generation changed, resetting Kubernetes upgrade process",
		"generation", kubernetesUpgrade.Generation,
		"observed", kubernetesUpgrade.Status.ObservedGeneration,
		"newVersion", kubernetesUpgrade.Spec.Kubernetes.Version)

	prevPhase := kubernetesUpgrade.Status.Phase
	if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":          tupprv1alpha1.JobPhasePending,
		"controllerNode": "",
		"message":        fmt.Sprintf("Spec updated to %s, restarting upgrade process", kubernetesUpgrade.Spec.Kubernetes.Version),
		"jobName":        "",
		"retries":        0,
		"lastError":      "",
	}); err != nil {
		return false, err
	}
	kubernetesUpgrade.Status.Phase = tupprv1alpha1.JobPhasePending
	r.recordPhaseTransition(kubernetesUpgrade, prevPhase, tupprv1alpha1.JobPhasePending)
	return true, nil
}
