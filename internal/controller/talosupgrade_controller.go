package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
)

const (
	// Finalizer for TalosUpgrade resources
	TalosUpgradeFinalizer = "tuppr.home-operations.com/talos-finalizer"

	// Job constants
	TalosJobBackoffLimit               = 2   // Retry up to 2 times on failure
	TalosJobGracePeriod                = 300 // 5 minutes
	TalosJobTTLAfterFinished           = 300 // 5 minutes (fallback if the controller misses cleanup)
	TalosJobActiveDeadlineBuffer       = 600 // 10 minutes buffer for job overhead
	TalosJobTalosUpgradeDefaultTimeout = 30 * time.Minute
)

// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades/finalizers,verbs=update
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=kubernetesupgrades,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// TalosUpgradeReconciler reconciles a TalosUpgrade object
type TalosUpgradeReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	TalosConfigSecret   string
	ControllerNamespace string
	HealthChecker       *HealthChecker
	TalosClient         *TalosClient
	MetricsReporter     *MetricsReporter
}

// Reconcile is part of the main kubernetes reconciliation loop
func (r *TalosUpgradeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting reconciliation", "talosupgrade", req.Name)

	var talosUpgrade tupprv1alpha1.TalosUpgrade
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name}, &talosUpgrade); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if talosUpgrade.DeletionTimestamp != nil {
		return r.cleanup(ctx, &talosUpgrade)
	}

	// Add finalizer if needed
	if !controllerutil.ContainsFinalizer(&talosUpgrade, TalosUpgradeFinalizer) {
		controllerutil.AddFinalizer(&talosUpgrade, TalosUpgradeFinalizer)
		return ctrl.Result{}, r.Update(ctx, &talosUpgrade)
	}

	return r.processUpgrade(ctx, &talosUpgrade)
}

// processUpgrade contains the main logic for handling the upgrade process
func (r *TalosUpgradeReconciler) processUpgrade(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"talosupgrade", talosUpgrade.Name,
		"generation", talosUpgrade.Generation,
	)

	logger.V(1).Info("Starting upgrade processing")

	// Check for suspend annotation first
	if suspended, err := r.handleSuspendAnnotation(ctx, talosUpgrade); err != nil || suspended {
		return ctrl.Result{RequeueAfter: time.Minute * 30}, err
	}

	// Check for reset annotation
	if resetRequested, err := r.handleResetAnnotation(ctx, talosUpgrade); err != nil || resetRequested {
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// Handle generation changes
	if reset, err := r.handleGenerationChange(ctx, talosUpgrade); err != nil || reset {
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// Skip if already completed/failed
	if talosUpgrade.Status.Phase == constants.PhaseCompleted || talosUpgrade.Status.Phase == constants.PhaseFailed {
		logger.V(1).Info("Upgrade already completed or failed", "phase", talosUpgrade.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Coordination check - only when not already InProgress (first-applied wins)
	if talosUpgrade.Status.Phase != constants.PhaseInProgress {
		if blocked, message, err := IsAnotherUpgradeActive(ctx, r.Client, "talos"); err != nil {
			logger.Error(err, "Failed to check for other active upgrades")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		} else if blocked {
			logger.Info("Blocked by another upgrade", "reason", message)
			if err := r.setPhase(ctx, talosUpgrade, constants.PhasePending, "", message); err != nil {
				logger.Error(err, "Failed to update phase for coordination wait")
			}
			return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
		}
	}

	// Continue with existing upgrade logic...
	if activeJob, activeNode, err := r.findActiveJob(ctx, talosUpgrade); err != nil {
		logger.Error(err, "Failed to find active jobs")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if activeJob != nil {
		logger.V(1).Info("Found active job, handling its status", "job", activeJob.Name, "node", activeNode)
		return r.handleJobStatus(ctx, talosUpgrade, activeNode, activeJob)
	}

	// Check for failed nodes
	if len(talosUpgrade.Status.FailedNodes) > 0 {
		logger.Info("Upgrade has failed nodes, blocking further progress",
			"failedNodes", len(talosUpgrade.Status.FailedNodes))
		message := fmt.Sprintf("Upgrade stopped due to %d failed nodes", len(talosUpgrade.Status.FailedNodes))
		if err := r.setPhase(ctx, talosUpgrade, constants.PhaseFailed, "", message); err != nil {
			logger.Error(err, "Failed to update phase for failed nodes")
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Find next node or complete
	return r.processNextNode(ctx, talosUpgrade)
}

// handleResetAnnotation checks for the reset annotation and resets the upgrade if found
func (r *TalosUpgradeReconciler) handleResetAnnotation(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (bool, error) {
	if talosUpgrade.Annotations == nil {
		return false, nil
	}

	resetValue, hasReset := talosUpgrade.Annotations[constants.ResetAnnotation]
	if !hasReset {
		return false, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Reset annotation found, clearing upgrade state", "resetValue", resetValue)

	// Remove the reset annotation and reset status
	newAnnotations := maps.Clone(talosUpgrade.Annotations)
	maps.DeleteFunc(newAnnotations, func(k, v string) bool {
		return k == constants.ResetAnnotation
	})

	// Update both annotations and status
	talosUpgrade.Annotations = newAnnotations
	if err := r.Update(ctx, talosUpgrade); err != nil {
		logger.Error(err, "Failed to remove reset annotation")
		return false, err
	}

	// Reset the status
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

// handleSuspendAnnotation checks for the suspend annotation and pauses the controller if found
func (r *TalosUpgradeReconciler) handleSuspendAnnotation(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (bool, error) {
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

	// Update status to indicate suspension
	message := fmt.Sprintf("Controller suspended via annotation (value: %s) - remove annotation to resume", suspendValue)
	if err := r.setPhase(ctx, talosUpgrade, constants.PhasePending, "", message); err != nil {
		logger.Error(err, "Failed to update phase for suspension")
		return true, err
	}

	logger.V(1).Info("Controller suspended, no further processing will occur",
		"talosupgrade", talosUpgrade.Name,
		"suspendValue", suspendValue)

	return true, nil // Return true to indicate we should stop processing
}

// handleGenerationChange resets the upgrade if the spec generation has changed
func (r *TalosUpgradeReconciler) handleGenerationChange(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (bool, error) {
	if talosUpgrade.Status.ObservedGeneration >= talosUpgrade.Generation {
		return false, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Spec changed, resetting upgrade process",
		"generation", talosUpgrade.Generation,
		"observed", talosUpgrade.Status.ObservedGeneration)

	// Simple reset - no need for complex state management
	return true, r.updateStatus(ctx, talosUpgrade, map[string]any{
		"phase":          constants.PhasePending,
		"currentNode":    "",
		"message":        "Spec updated, restarting upgrade process",
		"completedNodes": []string{},
		"failedNodes":    []tupprv1alpha1.NodeUpgradeStatus{},
	})
}

// processNextNode finds the next node to upgrade or completes the upgrade if done
func (r *TalosUpgradeReconciler) processNextNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Add upgrade info to context for health check metrics
	ctx = context.WithValue(ctx, ContextKeyUpgradeType, UpgradeTypeTalos)
	ctx = context.WithValue(ctx, ContextKeyUpgradeName, talosUpgrade.Name)

	// Perform health checks before finding next node
	if err := r.HealthChecker.CheckHealth(ctx, talosUpgrade.Spec.HealthChecks); err != nil {
		logger.Info("Health checks failed, will retry", "error", err.Error())
		if err := r.setPhase(ctx, talosUpgrade, constants.PhasePending, "", fmt.Sprintf("Waiting for health checks: %s", err.Error())); err != nil {
			logger.Error(err, "Failed to update phase for health check")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil // Retry health checks
	}

	nextNode, err := r.findNextNode(ctx, talosUpgrade)
	if err != nil {
		logger.Error(err, "Failed to find next node to upgrade")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if nextNode == "" {
		return r.completeUpgrade(ctx, talosUpgrade)
	}

	logger.Info("Found next node to upgrade", "node", nextNode)

	if _, err := r.createJob(ctx, talosUpgrade, nextNode); err != nil {
		logger.Error(err, "Failed to create upgrade job", "node", nextNode)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	logger.Info("Successfully created upgrade job", "node", nextNode)
	if err := r.setPhase(ctx, talosUpgrade, constants.PhaseInProgress, nextNode, fmt.Sprintf("Upgrading node %s", nextNode)); err != nil {
		logger.Error(err, "Failed to update phase for node upgrade", "node", nextNode)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// completeUpgrade finalizes the upgrade process
func (r *TalosUpgradeReconciler) completeUpgrade(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	completedCount := len(talosUpgrade.Status.CompletedNodes)
	failedCount := len(talosUpgrade.Status.FailedNodes)

	var phase, message string
	if failedCount > 0 {
		phase = constants.PhaseFailed
		message = fmt.Sprintf("Completed with failures: %d successful, %d failed", completedCount, failedCount)
		logger.Info("Upgrade completed with failures", "completed", completedCount, "failed", failedCount)
	} else {
		phase = constants.PhaseCompleted
		message = fmt.Sprintf("Successfully upgraded %d nodes", completedCount)
		logger.Info("Upgrade completed successfully", "nodes", completedCount)
	}

	if err := r.setPhase(ctx, talosUpgrade, phase, "", message); err != nil {
		logger.Error(err, "Failed to update completion phase")
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}
	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

// updateStatus applies a partial update to the status subresource
func (r *TalosUpgradeReconciler) updateStatus(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, updates map[string]any) error {
	// Always update generation and timestamp
	updates["observedGeneration"] = talosUpgrade.Generation
	updates["lastUpdated"] = metav1.Now()

	patch := map[string]any{"status": updates}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	statusObj := &tupprv1alpha1.TalosUpgrade{ObjectMeta: metav1.ObjectMeta{Name: talosUpgrade.Name}}
	return r.Status().Patch(ctx, statusObj, client.RawPatch(types.MergePatchType, patchBytes))
}

// setPhase updates the phase, current node, and message in status
func (r *TalosUpgradeReconciler) setPhase(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, phase, currentNode, message string) error {
	// Record phase change metrics
	r.MetricsReporter.RecordTalosUpgradePhase(talosUpgrade.Name, phase)

	// Record node counts
	totalNodes, _ := r.getTotalNodeCount(ctx)
	r.MetricsReporter.RecordTalosUpgradeNodes(
		talosUpgrade.Name,
		totalNodes,
		len(talosUpgrade.Status.CompletedNodes),
		len(talosUpgrade.Status.FailedNodes),
	)

	return r.updateStatus(ctx, talosUpgrade, map[string]any{
		"phase":       phase,
		"currentNode": currentNode,
		"message":     message,
	})
}

// addCompletedNode adds a node to the completed list if not already present
func (r *TalosUpgradeReconciler) addCompletedNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) error {
	// Simple check since we know state is managed by single controller
	if slices.Contains(talosUpgrade.Status.CompletedNodes, nodeName) {
		return nil
	}

	return r.updateStatus(ctx, talosUpgrade, map[string]any{
		"completedNodes": append(talosUpgrade.Status.CompletedNodes, nodeName),
	})
}

// addFailedNode adds a node to the failed list if not already present
func (r *TalosUpgradeReconciler) addFailedNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeStatus tupprv1alpha1.NodeUpgradeStatus) error {
	// Simple check since we know state is managed by single controller
	if slices.ContainsFunc(talosUpgrade.Status.FailedNodes, func(n tupprv1alpha1.NodeUpgradeStatus) bool {
		return n.NodeName == nodeStatus.NodeName
	}) {
		return nil
	}

	return r.updateStatus(ctx, talosUpgrade, map[string]any{
		"failedNodes": append(talosUpgrade.Status.FailedNodes, nodeStatus),
	})
}

// handleJobStatus processes the status of an active job
func (r *TalosUpgradeReconciler) handleJobStatus(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.V(1).Info("Handling job status",
		"job", job.Name,
		"node", nodeName,
		"active", job.Status.Active,
		"succeeded", job.Status.Succeeded,
		"failed", job.Status.Failed,
		"backoffLimit", *job.Spec.BackoffLimit)

	// Job still running
	if job.Status.Succeeded == 0 && (job.Status.Failed == 0 || job.Status.Failed < *job.Spec.BackoffLimit) {
		message := fmt.Sprintf("Upgrading node %s (job: %s)", nodeName, job.Name)
		if err := r.setPhase(ctx, talosUpgrade, constants.PhaseInProgress, nodeName, message); err != nil {
			logger.Error(err, "Failed to update phase for active job", "job", job.Name, "node", nodeName)
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		}
		logger.V(1).Info("Job is still active", "job", job.Name, "node", nodeName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	// Job succeeded - verify upgrade
	if job.Status.Succeeded > 0 {
		return r.handleJobSuccess(ctx, talosUpgrade, nodeName)
	}

	// Job failed permanently
	return r.handleJobFailure(ctx, talosUpgrade, nodeName)
}

// handleJobSuccess verifies the upgrade and marks the node as completed
func (r *TalosUpgradeReconciler) handleJobSuccess(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Job completed successfully", "node", nodeName)

	// Verify the upgrade actually worked
	if err := r.verifyNodeUpgrade(ctx, talosUpgrade, nodeName); err != nil {
		logger.Error(err, "Node upgrade verification failed", "node", nodeName)
		return r.handleJobFailure(ctx, talosUpgrade, nodeName)
	}

	// Clean up the successful job immediately
	if err := r.cleanupJobForNode(ctx, nodeName); err != nil {
		logger.Error(err, "Failed to cleanup job, but continuing", "node", nodeName)
		// Don't fail the entire process if cleanup fails
	}

	if err := r.addCompletedNode(ctx, talosUpgrade, nodeName); err != nil {
		logger.Error(err, "Failed to add completed node", "node", nodeName)
		return ctrl.Result{}, err
	}

	completedCount := len(talosUpgrade.Status.CompletedNodes) + 1
	message := fmt.Sprintf("Node %s upgraded successfully, health checks passed (%d completed)", nodeName, completedCount)

	if err := r.setPhase(ctx, talosUpgrade, constants.PhasePending, "", message); err != nil {
		logger.Error(err, "Failed to update phase", "node", nodeName)
		return ctrl.Result{}, err
	}

	logger.Info("Successfully completed node upgrade and cleaned up job", "node", nodeName)
	return ctrl.Result{RequeueAfter: time.Second * 5}, nil
}

// cleanupJobForNode removes the job for a specific node after successful completion
func (r *TalosUpgradeReconciler) cleanupJobForNode(ctx context.Context, nodeName string) error {
	logger := log.FromContext(ctx)

	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(r.ControllerNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":                "talos-upgrade",
			"tuppr.home-operations.com/target-node": nodeName,
		}); err != nil {
		return fmt.Errorf("failed to list jobs for node %s: %w", nodeName, err)
	}

	// Since there's only one TalosUpgrade CR, all jobs are ours
	for _, job := range jobList.Items {
		if job.Status.Succeeded > 0 {
			logger.Info("Deleting successful job", "job", job.Name, "node", nodeName)

			deletePolicy := metav1.DeletePropagationForeground
			if err := r.Delete(ctx, &job, &client.DeleteOptions{
				PropagationPolicy: &deletePolicy,
			}); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

// handleJobFailure marks the node as failed and stops the upgrade process
func (r *TalosUpgradeReconciler) handleJobFailure(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Job failed permanently", "node", nodeName)

	nodeStatus := tupprv1alpha1.NodeUpgradeStatus{
		NodeName:  nodeName,
		LastError: "Job failed permanently",
	}

	if err := r.addFailedNode(ctx, talosUpgrade, nodeStatus); err != nil {
		logger.Error(err, "Failed to add failed node", "node", nodeName)
		return ctrl.Result{}, err
	}

	failedCount := len(talosUpgrade.Status.FailedNodes) + 1
	message := fmt.Sprintf("Node %s upgrade failed (%d failed) - stopping", nodeName, failedCount)

	if err := r.setPhase(ctx, talosUpgrade, constants.PhaseFailed, "", message); err != nil {
		logger.Error(err, "Failed to update phase", "node", nodeName)
		return ctrl.Result{}, err
	}

	logger.Info("Successfully marked node as failed", "node", nodeName)
	return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}

// verifyNodeUpgrade checks if the node has been upgraded to the target version using Talos client
func (r *TalosUpgradeReconciler) verifyNodeUpgrade(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Verifying node upgrade using Talos client", "node", nodeName)

	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return fmt.Errorf("failed to get node: %w", err)
	}

	nodeIP, err := GetNodeIP(node)
	if err != nil {
		return fmt.Errorf("failed to get node IP for %s: %w", nodeName, err)
	}

	targetVersion := talosUpgrade.Spec.Talos.Version

	// Wait for the Talos node to be ready after upgrade
	if err := r.TalosClient.WaitForNodeReady(ctx, nodeIP, nodeName); err != nil {
		return fmt.Errorf("failed waiting for Talos node to be ready for %s: %w", nodeName, err)
	}

	currentVersion, err := r.TalosClient.GetNodeVersion(ctx, nodeIP)
	if err != nil {
		return fmt.Errorf("failed to get current version from Talos for %s: %w", nodeName, err)
	}

	if currentVersion != targetVersion {
		return fmt.Errorf("node %s version mismatch: current=%s, target=%s",
			nodeName, currentVersion, targetVersion)
	}

	logger.Info("Node upgrade verification successful using Talos client",
		"node", nodeName,
		"version", currentVersion)
	return nil
}

// findActiveJob looks for any currently running job for this TalosUpgrade
func (r *TalosUpgradeReconciler) findActiveJob(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (*batchv1.Job, string, error) {
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(r.ControllerNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/name": "talos-upgrade",
		}); err != nil {
		return nil, "", err
	}

	// Since there's only one TalosUpgrade CR, any job we find is ours
	for _, job := range jobList.Items {
		nodeName := job.Labels["tuppr.home-operations.com/target-node"]

		// Skip jobs for nodes that are already completed
		if slices.Contains(talosUpgrade.Status.CompletedNodes, nodeName) {
			continue
		}

		// Skip jobs for nodes that are already failed
		if slices.ContainsFunc(talosUpgrade.Status.FailedNodes, func(n tupprv1alpha1.NodeUpgradeStatus) bool {
			return n.NodeName == nodeName
		}) {
			continue
		}

		return &job, nodeName, nil
	}

	return nil, "", nil
}

// findNextNode determines the next node that requires an upgrade
func (r *TalosUpgradeReconciler) findNextNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (string, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Finding next node to upgrade", "talosupgrade", talosUpgrade.Name)

	nodes, err := r.getSortedNodes(ctx)
	if err != nil {
		logger.Error(err, "Failed to get nodes")
		return "", err
	}

	targetVersion := talosUpgrade.Spec.Talos.Version

	// Use slices.IndexFunc to find first node needing upgrade
	idx := slices.IndexFunc(nodes, func(node corev1.Node) bool {
		// Skip completed nodes
		if slices.Contains(talosUpgrade.Status.CompletedNodes, node.Name) {
			return false
		}

		// Skip failed nodes
		if slices.ContainsFunc(talosUpgrade.Status.FailedNodes, func(fn tupprv1alpha1.NodeUpgradeStatus) bool {
			return fn.NodeName == node.Name
		}) {
			return false
		}

		// Check if needs upgrade
		return r.nodeNeedsUpgrade(ctx, &node, targetVersion)
	})

	if idx == -1 {
		logger.V(1).Info("No nodes need upgrade")
		return "", nil
	}

	nodeName := nodes[idx].Name
	logger.Info("Node needs upgrade", "node", nodeName)
	return nodeName, nil
}

// getSortedNodes retrieves all nodes in the cluster sorted by name for consistent ordering
func (r *TalosUpgradeReconciler) getSortedNodes(ctx context.Context) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	nodes := nodeList.Items
	slices.SortFunc(nodes, func(a, b corev1.Node) int {
		return strings.Compare(a.Name, b.Name)
	})

	return nodes, nil
}

// nodeNeedsUpgrade determines if a given node requires an upgrade using Talos client
func (r *TalosUpgradeReconciler) nodeNeedsUpgrade(ctx context.Context, node *corev1.Node, targetVersion string) bool {
	logger := log.FromContext(ctx)

	nodeIP, err := GetNodeIP(node)
	if err != nil {
		logger.Error(err, "Failed to get node IP, cannot determine upgrade need", "node", node.Name)
		return false // Cannot proceed without IP
	}

	currentVersion, err := r.TalosClient.GetNodeVersion(ctx, nodeIP)
	if err != nil {
		logger.Error(err, "Failed to get current version from Talos client, cannot determine upgrade need",
			"node", node.Name, "nodeIP", nodeIP)
		return false // Cannot proceed without Talos info
	}

	needsUpgrade := currentVersion != targetVersion
	logger.V(1).Info("Node upgrade check using Talos client",
		"node", node.Name,
		"currentVersion", currentVersion,
		"targetVersion", targetVersion,
		"needsUpgrade", needsUpgrade)

	return needsUpgrade
}

// createJob creates a Kubernetes Job to perform the Talos upgrade on the specified node
func (r *TalosUpgradeReconciler) createJob(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Creating upgrade job", "node", nodeName)

	targetNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, targetNode); err != nil {
		logger.Error(err, "Failed to get target node", "node", nodeName)
		return nil, err
	}

	nodeIP, err := GetNodeIP(targetNode)
	if err != nil {
		logger.Error(err, "Failed to get node internal IP", "node", nodeName)
		return nil, err
	}

	job := r.buildJob(ctx, talosUpgrade, nodeName, nodeIP)
	if err := controllerutil.SetControllerReference(talosUpgrade, job, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference", "job", job.Name)
		return nil, err
	}

	if err := r.Create(ctx, job); err != nil {
		if errors.IsAlreadyExists(err) {
			// Job already exists, just return it
			existingJob := &batchv1.Job{}
			if getErr := r.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, existingJob); getErr != nil {
				logger.Error(getErr, "Failed to get existing job", "job", job.Name)
				return nil, getErr
			}
			logger.V(1).Info("Job already exists, reusing", "job", job.Name)
			return existingJob, nil
		}
		logger.Error(err, "Failed to create job", "job", job.Name, "node", nodeName)
		return nil, err
	}

	logger.Info("Successfully created upgrade job", "job", job.Name, "node", nodeName)
	return job, nil
}

// buildJob constructs a Kubernetes Job object for upgrading a specific node
func (r *TalosUpgradeReconciler) buildJob(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName, nodeIP string) *batchv1.Job {
	logger := log.FromContext(ctx)

	jobName := GenerateSafeJobName(talosUpgrade.Name, nodeName)

	labels := map[string]string{
		"app.kubernetes.io/name":                "talos-upgrade",
		"app.kubernetes.io/instance":            talosUpgrade.Name,
		"app.kubernetes.io/part-of":             "tuppr",
		"tuppr.home-operations.com/target-node": nodeName,
	}

	// Configure node affinity based on PlacementPreset
	placement := talosUpgrade.Spec.Policy.Placement
	if placement == "" {
		placement = "soft" // Default value from kubebuilder annotation
	}

	nodeSelector := corev1.NodeSelectorRequirement{
		Key:      "kubernetes.io/hostname",
		Operator: corev1.NodeSelectorOpNotIn,
		Values:   []string{nodeName},
	}

	var nodeAffinity *corev1.NodeAffinity
	if placement == "hard" {
		// Required avoidance - job will fail if it can't avoid the target node
		nodeAffinity = &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{nodeSelector},
				}},
			},
		}
		logger.V(1).Info("Using hard placement preset - required node avoidance", "node", nodeName)
	} else {
		// Preferred avoidance - job prefers to avoid but can run on target node if needed
		nodeAffinity = &corev1.NodeAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
				Weight: 100,
				Preference: corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{nodeSelector},
				},
			}},
		}
		if placement != "soft" {
			logger.V(1).Info("Unknown placement preset, using soft placement as fallback", "preset", placement, "node", nodeName)
		} else {
			logger.V(1).Info("Using soft placement preset - preferred node avoidance", "node", nodeName)
		}
	}

	// Get talosctl image with defaults
	talosctlRepo := constants.DefaultTalosctlImage
	if talosUpgrade.Spec.Talosctl.Image.Repository != "" {
		talosctlRepo = talosUpgrade.Spec.Talosctl.Image.Repository
	}

	talosctlTag := talosUpgrade.Spec.Talosctl.Image.Tag
	if talosctlTag == "" {
		// Try to detect the current Talos version for talosctl compatibility
		if currentVersion, err := r.TalosClient.GetNodeVersion(ctx, nodeIP); err == nil && currentVersion != "" {
			talosctlTag = currentVersion
			logger.V(1).Info("Using current node version for talosctl compatibility",
				"node", nodeName, "currentVersion", currentVersion)
		} else {
			// This should never happen but lets fallback to the target version
			talosctlTag = talosUpgrade.Spec.Talos.Version
			logger.V(1).Info("Could not detect current version, using target version for talosctl",
				"node", nodeName, "version", talosctlTag, "error", err)
		}
	}

	talosctlImage := talosctlRepo + ":" + talosctlTag

	// Build target image with schematic from node
	targetImage, err := r.buildTalosUpgradeImage(ctx, talosUpgrade, nodeName)
	if err != nil {
		// This should never happen
		targetImage = fmt.Sprintf("factory.talos.dev/metal-installer:%s", talosUpgrade.Spec.Talos.Version)
		logger.Error(err, "Using fallback image", "node", nodeName, "fallbackImage", targetImage)
	}

	// Get timeout with default
	timeout := TalosJobTalosUpgradeDefaultTimeout
	if talosUpgrade.Spec.Policy.Timeout != nil {
		timeout = talosUpgrade.Spec.Policy.Timeout.Duration
	}

	args := []string{
		"upgrade",
		"--nodes=" + nodeIP,
		"--image=" + targetImage,
		"--timeout=" + timeout.String(),
		"--wait=true",
	}

	if talosUpgrade.Spec.Policy.Debug {
		args = append(args, "--debug=true")
		logger.V(1).Info("Debug upgrade enabled", "node", nodeName)
	}

	if talosUpgrade.Spec.Policy.Force {
		args = append(args, "--force=true")
		logger.V(1).Info("Force upgrade enabled", "node", nodeName)
	}

	if talosUpgrade.Spec.Policy.RebootMode == "powercycle" {
		args = append(args, "--reboot-mode=powercycle")
		logger.V(1).Info("Powercycle reboot mode enabled", "node", nodeName)
	}

	if talosUpgrade.Spec.Policy.Stage {
		args = append(args, "--stage")
		logger.V(1).Info("Stage upgrade enabled", "node", nodeName)
	}

	pullPolicy := corev1.PullIfNotPresent
	if talosUpgrade.Spec.Talosctl.Image.PullPolicy != "" {
		pullPolicy = talosUpgrade.Spec.Talosctl.Image.PullPolicy
	}

	logger.V(1).Info("Building job specification",
		"node", nodeName,
		"talosctlImage", talosctlImage,
		"targetImage", targetImage,
		"pullPolicy", pullPolicy,
		"args", args)

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: r.ControllerNamespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To(int32(TalosJobBackoffLimit)),
			Completions:             ptr.To(int32(1)),
			TTLSecondsAfterFinished: ptr.To(int32(TalosJobTTLAfterFinished)),
			Parallelism:             ptr.To(int32(1)),
			ActiveDeadlineSeconds:   ptr.To(getActiveDeadlineSeconds(timeout)),
			PodReplacementPolicy:    ptr.To(batchv1.Failed),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy:                 corev1.RestartPolicyNever,
					TerminationGracePeriodSeconds: ptr.To(int64(TalosJobGracePeriod)),
					PriorityClassName:             "system-node-critical",
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(int64(65532)),
						RunAsGroup:   ptr.To(int64(65532)),
						FSGroup:      ptr.To(int64(65532)),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Affinity: &corev1.Affinity{
						NodeAffinity: nodeAffinity,
					},
					Tolerations: []corev1.Toleration{
						{
							Operator: corev1.TolerationOpExists,
						},
					},
					Containers: []corev1.Container{{
						Name:            "upgrade",
						Image:           talosctlImage,
						ImagePullPolicy: pullPolicy,
						Args:            args,
						Env: []corev1.EnvVar{{
							Name:  "TALOSCONFIG",
							Value: "/var/run/secrets/talos.dev/talosconfig",
						}},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							ReadOnlyRootFilesystem:   ptr.To(true),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      constants.TalosSecretName,
							MountPath: "/var/run/secrets/talos.dev",
							ReadOnly:  true,
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: constants.TalosSecretName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName:  r.TalosConfigSecret,
								DefaultMode: ptr.To(int32(0420)),
							},
						},
					}},
				},
			},
		},
	}
}

// cleanup handles finalization logic when a TalosUpgrade is deleted
func (r *TalosUpgradeReconciler) cleanup(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Cleaning up TalosUpgrade", "name", talosUpgrade.Name)

	// Jobs will be cleaned up automatically via owner references
	logger.V(1).Info("Removing finalizer", "name", talosUpgrade.Name, "finalizer", TalosUpgradeFinalizer)
	controllerutil.RemoveFinalizer(talosUpgrade, TalosUpgradeFinalizer)

	if err := r.Update(ctx, talosUpgrade); err != nil {
		logger.Error(err, "Failed to remove finalizer", "name", talosUpgrade.Name)
		return ctrl.Result{}, err
	}

	logger.Info("Successfully cleaned up TalosUpgrade", "name", talosUpgrade.Name)
	return ctrl.Result{}, nil
}

// buildTalosUpgradeImage constructs the target image using current node image as template
func (r *TalosUpgradeReconciler) buildTalosUpgradeImage(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (string, error) {
	logger := log.FromContext(ctx)

	// Get the target node
	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	nodeIP, err := GetNodeIP(node)
	if err != nil {
		return "", fmt.Errorf("failed to get node IP for %s: %w", nodeName, err)
	}

	// Get install image from Talos client using the new method
	currentImage, err := r.TalosClient.GetNodeInstallImage(ctx, nodeIP)
	if err != nil {
		return "", fmt.Errorf("failed to get install image from Talos client for %s: %w", nodeName, err)
	}

	// Parse the current image to extract repository and schematic
	parts := strings.Split(currentImage, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid current image format for node %s: %s", nodeName, currentImage)
	}

	// Build new image with same repository/schematic but new version
	targetImage := fmt.Sprintf("%s:%s", parts[0], talosUpgrade.Spec.Talos.Version)

	logger.Info("Built target image from current node image",
		"node", nodeName,
		"currentImage", currentImage,
		"targetImage", targetImage)

	return targetImage, nil
}

// getTotalNodeCount returns the total number of nodes in the cluster
func (r *TalosUpgradeReconciler) getTotalNodeCount(ctx context.Context) (int, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return 0, err
	}
	return len(nodeList.Items), nil
}

func (r *TalosUpgradeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := ctrl.Log.WithName("setup")
	logger.Info("Setting up TalosUpgrade controller with manager")

	r.HealthChecker = &HealthChecker{Client: mgr.GetClient()}
	r.MetricsReporter = NewMetricsReporter()

	talosClient, err := NewTalosClient(context.Background())
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	r.TalosClient = talosClient

	return ctrl.NewControllerManagedBy(mgr).
		For(&tupprv1alpha1.TalosUpgrade{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

// getActiveDeadlineSeconds computes the job active deadline from the upgrade timeout.
// Formula: (BackoffLimit + 1) × timeout + buffer
func getActiveDeadlineSeconds(timeout time.Duration) int64 {
	attempts := int64(TalosJobBackoffLimit + 1)
	timeoutSeconds := int64(timeout.Seconds())
	return attempts*timeoutSeconds + TalosJobActiveDeadlineBuffer
}
