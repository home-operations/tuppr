package talosupgrade

import (
	"context"
	"fmt"
	"maps"

	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
)

func (r *Reconciler) handleSuspendAnnotation(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (bool, error) {
	if talosUpgrade.Annotations == nil {
		return false, nil
	}

	suspendValue, isSuspended := talosUpgrade.Annotations[constants.SuspendAnnotation]
	if !isSuspended {
		return false, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Suspend annotation found, controller is suspended",
		"suspendValue", suspendValue,
		"talosupgrade", talosUpgrade.Name)

	message := fmt.Sprintf("Controller suspended via annotation (value: %s) - remove annotation to resume", suspendValue)
	if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhasePending, "", message); err != nil {
		logger.Error(err, "Failed to update phase for suspension")
		return true, err
	}

	logger.V(1).Info("Controller suspended, no further processing will occur",
		"talosupgrade", talosUpgrade.Name,
		"suspendValue", suspendValue)

	return true, nil
}

func (r *Reconciler) handleResetAnnotation(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (bool, error) {
	if talosUpgrade.Annotations == nil {
		return false, nil
	}

	resetValue, hasReset := talosUpgrade.Annotations[constants.ResetAnnotation]
	if !hasReset {
		return false, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Reset annotation found, clearing upgrade state", "resetValue", resetValue)

	newAnnotations := maps.Clone(talosUpgrade.Annotations)
	maps.DeleteFunc(newAnnotations, func(k, v string) bool {
		return k == constants.ResetAnnotation
	})

	talosUpgrade.Annotations = newAnnotations
	if err := r.Update(ctx, talosUpgrade); err != nil {
		logger.Error(err, "Failed to remove reset annotation")
		return false, err
	}

	prevPhase := talosUpgrade.Status.Phase
	totalNodes, err := r.getTotalNodeCount(ctx)
	if err != nil {
		logger.Error(err, "Failed to get total node count for metrics")
	}
	if err := r.updateStatus(ctx, talosUpgrade, map[string]any{
		"phase":          tupprv1alpha1.JobPhasePending,
		"currentNode":    "",
		"message":        "Reset requested via annotation",
		"completedNodes": []string{},
		"failedNodes":    []tupprv1alpha1.NodeUpgradeStatus{},
	}); err != nil {
		logger.Error(err, "Failed to reset status after annotation")
		return false, err
	}
	talosUpgrade.Status.Phase = tupprv1alpha1.JobPhasePending
	r.recordPhaseTransition(talosUpgrade, prevPhase, tupprv1alpha1.JobPhasePending)
	r.MetricsReporter.RecordTalosUpgradeNodes(talosUpgrade.Name, totalNodes, 0, 0)

	return true, nil
}

func (r *Reconciler) handleGenerationChange(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (bool, error) {
	if talosUpgrade.Status.ObservedGeneration >= talosUpgrade.Generation {
		return false, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Spec changed, resetting upgrade process",
		"generation", talosUpgrade.Generation,
		"observed", talosUpgrade.Status.ObservedGeneration)

	prevPhase := talosUpgrade.Status.Phase
	totalNodes, err := r.getTotalNodeCount(ctx)
	if err != nil {
		logger.Error(err, "Failed to get total node count for metrics")
	}
	if err := r.updateStatus(ctx, talosUpgrade, map[string]any{
		"phase":          tupprv1alpha1.JobPhasePending,
		"currentNode":    "",
		"message":        "Spec updated, restarting upgrade process",
		"completedNodes": []string{},
		"failedNodes":    []tupprv1alpha1.NodeUpgradeStatus{},
	}); err != nil {
		return false, err
	}
	talosUpgrade.Status.Phase = tupprv1alpha1.JobPhasePending
	r.recordPhaseTransition(talosUpgrade, prevPhase, tupprv1alpha1.JobPhasePending)
	r.MetricsReporter.RecordTalosUpgradeNodes(talosUpgrade.Name, totalNodes, 0, 0)
	return true, nil
}
