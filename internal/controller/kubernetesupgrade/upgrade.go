package kubernetesupgrade

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/controller/coordination"
	"github.com/home-operations/tuppr/internal/controller/maintenance"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
	"github.com/home-operations/tuppr/internal/metrics"
)

func (r *Reconciler) processUpgrade(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"kubernetesupgrade", kubernetesUpgrade.Name,
		"generation", kubernetesUpgrade.Generation,
	)

	logger.V(1).Info("Starting Kubernetes upgrade processing")

	if suspended, err := r.handleSuspendAnnotation(ctx, kubernetesUpgrade); err != nil || suspended {
		return ctrl.Result{RequeueAfter: time.Minute * 30}, err
	}

	if resetRequested, err := r.handleResetAnnotation(ctx, kubernetesUpgrade); err != nil || resetRequested {
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	if reset, err := r.handleGenerationChange(ctx, kubernetesUpgrade); err != nil || reset {
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	if kubernetesUpgrade.Status.Phase.IsTerminal() {
		logger.V(1).Info("Kubernetes upgrade in terminal state, skipping", "phase", kubernetesUpgrade.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	if !kubernetesUpgrade.Status.Phase.IsActive() {
		if result, done, err := r.checkMaintenanceWindow(ctx, kubernetesUpgrade); done {
			return result, err
		}
		if result, done := r.checkCoordination(ctx, kubernetesUpgrade); done {
			return result, nil
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
			statusFieldCurrentVersion: currentVersion,
			statusFieldTargetVersion:  targetVersion,
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
		logger.V(1).Info("All control plane nodes verified at target version", "version", targetVersion)

		if !strings.HasPrefix(currentVersion, "v") {
			currentVersion = "v" + currentVersion
		}
		if currentVersion != targetVersion {
			logger.V(1).Info("Nodes are updated but API server still reports old version, waiting for propagation")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		if err := r.setPhaseWithUpdates(ctx, kubernetesUpgrade, tupprv1alpha1.JobPhaseCompleted, "", fmt.Sprintf("Kubernetes successfully upgraded to %s", targetVersion), map[string]any{
			statusFieldCurrentVersion: targetVersion,
			statusFieldTargetVersion:  targetVersion,
		}); err != nil {
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
		logger.Info("Waiting for health checks to pass", "error", err.Error())
		if err := r.setPhase(ctx, kubernetesUpgrade, tupprv1alpha1.JobPhaseHealthChecking, "", fmt.Sprintf("Waiting for health checks: %s", err.Error())); err != nil {
			logger.Error(err, "Failed to update phase for health check")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return r.startUpgrade(ctx, kubernetesUpgrade)
}

func (r *Reconciler) checkMaintenanceWindow(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (ctrl.Result, bool, error) {
	logger := log.FromContext(ctx)

	maintenanceRes, err := maintenance.CheckWindow(kubernetesUpgrade.Spec.Maintenance, r.Now.Now())
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Second * 30}, true, err
	}
	if !maintenanceRes.Allowed {
		requeueAfter := time.Until(*maintenanceRes.NextWindowStart)
		if requeueAfter > 5*time.Minute {
			requeueAfter = 5 * time.Minute
		}
		nextTimestamp := maintenanceRes.NextWindowStart.Unix()
		r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeKubernetes, kubernetesUpgrade.Name, false, &nextTimestamp)
		message := fmt.Sprintf("Waiting for maintenance window (next: %s)", maintenanceRes.NextWindowStart.Format(time.RFC3339))
		if err := r.setPhaseWithUpdates(ctx, kubernetesUpgrade, tupprv1alpha1.JobPhaseMaintenanceWindow, "", message, map[string]any{
			"nextMaintenanceWindow": metav1.NewTime(*maintenanceRes.NextWindowStart),
		}); err != nil {
			logger.Error(err, "Failed to update status for maintenance window")
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, true, nil
	}
	r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeKubernetes, kubernetesUpgrade.Name, true, nil)
	return ctrl.Result{}, false, nil
}

func (r *Reconciler) checkCoordination(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (ctrl.Result, bool) {
	logger := log.FromContext(ctx)

	blocked, message, err := coordination.IsAnotherUpgradeActive(ctx, r.Client, kubernetesUpgrade.Name, coordination.UpgradeTypeKubernetes)
	if err != nil {
		logger.Error(err, "Failed to check for other active upgrades")
		return ctrl.Result{RequeueAfter: time.Minute}, true
	}
	if blocked {
		logger.Info("Waiting for another upgrade to complete", "reason", message)
		if err := r.setPhase(ctx, kubernetesUpgrade, tupprv1alpha1.JobPhasePending, "", message); err != nil {
			logger.Error(err, "Failed to update phase for coordination wait")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 2}, true
	}
	return ctrl.Result{}, false
}

func (r *Reconciler) startUpgrade(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	targetVersion := kubernetesUpgrade.Spec.Kubernetes.Version
	controllerNode, controllerIP, err := r.findControllerNode(ctx, targetVersion)
	if err != nil {
		logger.Error(err, "Failed to find controller node")
		if err := r.setPhase(ctx, kubernetesUpgrade, tupprv1alpha1.JobPhaseFailed, "", fmt.Sprintf("Failed to find controller node: %s", err.Error())); err != nil {
			logger.Error(err, "Failed to update phase for controller node failure")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	logger.Info("Starting Kubernetes upgrade", "controllerNode", controllerNode, "controllerIP", controllerIP)

	job, err := r.createJob(ctx, kubernetesUpgrade, controllerNode, controllerIP)
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes upgrade job")
		if err := r.setPhase(ctx, kubernetesUpgrade, tupprv1alpha1.JobPhaseFailed, controllerNode, fmt.Sprintf("Failed to create job: %s", err.Error())); err != nil {
			logger.Error(err, "Failed to update phase for job creation failure")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	logger.Info("Successfully created Kubernetes upgrade job", "job", job.Name, "controllerNode", controllerNode)

	message := fmt.Sprintf("Upgrading Kubernetes to %s on controller node %s", kubernetesUpgrade.Spec.Kubernetes.Version, controllerNode)
	if err := r.setPhaseWithUpdates(ctx, kubernetesUpgrade, tupprv1alpha1.JobPhaseUpgrading, controllerNode, message, map[string]any{
		statusFieldJobName: job.Name,
	}); err != nil {
		logger.Error(err, "Failed to update status for job creation")
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

func (r *Reconciler) findControllerNode(ctx context.Context, targetVersion string) (string, string, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return "", "", fmt.Errorf("failed to list nodes: %w", err)
	}

	for _, node := range nodeList.Items {
		if _, isController := node.Labels["node-role.kubernetes.io/control-plane"]; isController {
			if node.Status.NodeInfo.KubeletVersion != targetVersion {
				nodeIP, err := nodeutil.GetNodeIP(&node)
				if err != nil {
					continue
				}
				return node.Name, nodeIP, nil
			}
		}
	}

	return "", "", fmt.Errorf("no controller node found with node-role.kubernetes.io/control-plane label")
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
			log.FromContext(ctx).V(1).Info("Control plane node not yet upgraded",
				"node", node.Name,
				"current", node.Status.NodeInfo.KubeletVersion,
				"target", targetVersion)
			return false, nil
		}
	}

	return true, nil
}
