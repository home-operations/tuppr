package kubernetesupgrade

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/coordination"
	"github.com/home-operations/tuppr/internal/controller/maintenance"
	"github.com/home-operations/tuppr/internal/metrics"
)

func (r *Reconciler) processUpgrade(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"kubernetesupgrade", kubernetesUpgrade.Name,
		"generation", kubernetesUpgrade.Generation,
	)

	logger.V(1).Info("Starting Kubernetes upgrade processing")
	now := r.Now.Now()

	if suspended, err := r.handleSuspendAnnotation(ctx, kubernetesUpgrade); err != nil || suspended {
		return ctrl.Result{RequeueAfter: time.Minute * 30}, err
	}

	if resetRequested, err := r.handleResetAnnotation(ctx, kubernetesUpgrade); err != nil || resetRequested {
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	if reset, err := r.handleGenerationChange(ctx, kubernetesUpgrade); err != nil || reset {
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	if kubernetesUpgrade.Status.Phase == tupprv1alpha1.JobPhaseCompleted {
		logger.V(1).Info("Kubernetes upgrade completed, skipping", "phase", kubernetesUpgrade.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	if kubernetesUpgrade.Status.Phase == tupprv1alpha1.JobPhaseFailed {
		logger.V(1).Info("Kubernetes upgrade failed, skipping", "phase", kubernetesUpgrade.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	if !kubernetesUpgrade.Status.Phase.IsActive() {
		maintenanceRes, err := maintenance.CheckWindow(kubernetesUpgrade.Spec.Maintenance, now)
		if err != nil {
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		}
		if !maintenanceRes.Allowed {
			requeueAfter := time.Until(*maintenanceRes.NextWindowStart)
			if requeueAfter > 5*time.Minute {
				requeueAfter = 5 * time.Minute
			}
			nextTimestamp := maintenanceRes.NextWindowStart.Unix()
			r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeKubernetes, kubernetesUpgrade.Name, false, &nextTimestamp)
			if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
				"phase":                 tupprv1alpha1.JobPhasePending,
				"controllerNode":        "",
				"message":               fmt.Sprintf("Waiting for maintenance window (next: %s)", maintenanceRes.NextWindowStart.Format(time.RFC3339)),
				"nextMaintenanceWindow": metav1.NewTime(*maintenanceRes.NextWindowStart),
			}); err != nil {
				logger.Error(err, "Failed to update status for maintenance window")
			}
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeKubernetes, kubernetesUpgrade.Name, true, nil)
	}

	if !kubernetesUpgrade.Status.Phase.IsActive() {
		if blocked, message, err := coordination.IsAnotherUpgradeActive(ctx, r.Client, kubernetesUpgrade.Name, coordination.UpgradeTypeKubernetes); err != nil {
			logger.Error(err, "Failed to check for other active upgrades")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		} else if blocked {
			logger.Info("Blocked by another upgrade", "reason", message)
			if err := r.setPhase(ctx, kubernetesUpgrade, tupprv1alpha1.JobPhasePending, "", message); err != nil {
				logger.Error(err, "Failed to update phase for coordination wait")
			}
			return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
		}
	}

	if activeJob, err := r.findActiveJob(ctx); err != nil {
		logger.Error(err, "Failed to find active jobs")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if activeJob != nil {
		logger.V(1).Info("Found active job, handling its status", "job", activeJob.Name)
		return r.handleJobStatus(ctx, kubernetesUpgrade, activeJob)
	}
	targetVersion := kubernetesUpgrade.Spec.Kubernetes.Version

	currentVersion, err := r.VersionGetter.GetCurrentKubernetesVersion(ctx)
	if err == nil {
		if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
			"currentVersion": currentVersion,
			"targetVersion":  targetVersion,
		}); err != nil {
			logger.Error(err, "Failed to update version status")
		}
	}

	allUpgraded, err := r.areAllControlPlaneNodesUpgraded(ctx, targetVersion)
	if err != nil {
		logger.Error(err, "Failed to verify control plane node versions")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if allUpgraded {
		logger.Info("All control plane nodes verified at target version", "version", targetVersion)

		if !strings.HasPrefix(currentVersion, "v") {
			currentVersion = "v" + currentVersion
		}
		if currentVersion != targetVersion {
			logger.Info("Nodes are updated but API server (VIP) still reports old version. Waiting for cache/LB update.")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		if err := r.setPhase(ctx, kubernetesUpgrade, tupprv1alpha1.JobPhaseCompleted, "", fmt.Sprintf("Kubernetes successfully upgraded to %s", targetVersion)); err != nil {
			logger.Error(err, "Failed to update completion phase")
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}
		return ctrl.Result{RequeueAfter: time.Hour}, nil
	}

	ctx = context.WithValue(ctx, metrics.ContextKeyUpgradeType, metrics.UpgradeTypeKubernetes)
	ctx = context.WithValue(ctx, metrics.ContextKeyUpgradeName, kubernetesUpgrade.Name)

	logger.Info("Kubernetes upgrade needed", "current", currentVersion, "target", targetVersion)

	if err := r.setPhase(ctx, kubernetesUpgrade, tupprv1alpha1.JobPhaseHealthChecking, "", "Running health checks"); err != nil {
		logger.Error(err, "Failed to update phase for health check")
	}
	if err := r.HealthChecker.CheckHealth(ctx, kubernetesUpgrade.Spec.HealthChecks); err != nil {
		logger.Info("Health checks failed, will retry", "error", err.Error())
		if err := r.setPhase(ctx, kubernetesUpgrade, tupprv1alpha1.JobPhaseHealthChecking, "", fmt.Sprintf("Waiting for health checks: %s", err.Error())); err != nil {
			logger.Error(err, "Failed to update phase for health check")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return r.startUpgrade(ctx, kubernetesUpgrade)
}

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

	return true, r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":          tupprv1alpha1.JobPhasePending,
		"controllerNode": "",
		"message":        fmt.Sprintf("Spec updated to %s, restarting upgrade process", kubernetesUpgrade.Spec.Kubernetes.Version),
		"jobName":        "",
		"retries":        0,
		"lastError":      "",
	})
}

func (r *Reconciler) areAllControlPlaneNodesUpgraded(ctx context.Context, targetVersion string) (bool, error) {
	nodeList := &corev1.NodeList{}
	opts := []client.ListOption{
		client.MatchingLabels{"node-role.kubernetes.io/control-plane": ""},
	}

	if err := r.List(ctx, nodeList, opts...); err != nil {
		return false, fmt.Errorf("failed to list control plane nodes: %w", err)
	}

	if len(nodeList.Items) == 0 {
		return false, fmt.Errorf("no control plane nodes found")
	}

	for _, node := range nodeList.Items {
		if node.Status.NodeInfo.KubeletVersion != targetVersion {
			log.FromContext(ctx).Info("Control plane node not yet upgraded",
				"node", node.Name,
				"current", node.Status.NodeInfo.KubeletVersion,
				"target", targetVersion)
			return false, nil
		}
	}

	return true, nil
}
