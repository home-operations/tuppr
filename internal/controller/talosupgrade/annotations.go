package talosupgrade

import (
	"context"
	"fmt"
	"maps"
	"slices"

	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	if err := r.setPhase(ctx, talosUpgrade, constants.PhasePending, "", message); err != nil {
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

	if err := r.updateStatus(ctx, talosUpgrade, map[string]any{
		"phase":          constants.PhasePending,
		"currentNode":    "",
		"message":        "Reset requested via annotation",
		"completedNodes": []string{},
		"failedNodes":    []tupprv1alpha1.NodeUpgradeStatus{},
	}); err != nil {
		logger.Error(err, "Failed to reset status after annotation")
		return false, err
	}

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

	return true, r.updateStatus(ctx, talosUpgrade, map[string]any{
		"phase":          constants.PhasePending,
		"currentNode":    "",
		"message":        "Spec updated, restarting upgrade process",
		"completedNodes": []string{},
		"failedNodes":    []tupprv1alpha1.NodeUpgradeStatus{},
	})
}

func (r *Reconciler) findActiveJob(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (*batchv1.Job, string, error) {
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(r.ControllerNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/name": "talos-upgrade",
		}); err != nil {
		return nil, "", err
	}

	for _, job := range jobList.Items {
		nodeName := job.Labels["tuppr.home-operations.com/target-node"]

		if slices.Contains(talosUpgrade.Status.CompletedNodes, nodeName) {
			continue
		}

		if slices.ContainsFunc(talosUpgrade.Status.FailedNodes, func(n tupprv1alpha1.NodeUpgradeStatus) bool {
			return n.NodeName == nodeName
		}) {
			continue
		}

		return &job, nodeName, nil
	}

	return nil, "", nil
}
