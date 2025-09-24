package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
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
	TalosJobBackoffLimit        = 2     // Retry up to 2 times on failure
	TalosJobActiveDeadline      = 6000  // 100 minutes (3 [attempts] × 30 [talosctl timeouts] + 10 [buffer])
	TalosJobGracePeriod         = 300   // 5 minutes
	TalosJobTTLAfterFinished    = 300   // 5 minutes (fallback if the controller misses cleanup)
	TalosJobTalosHealthTimeout  = "10m" // Affects TalosJobActiveDeadline
	TalosJobTalosUpgradeTimeout = "20m" // Affects TalosJobActiveDeadline
)

// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades/finalizers,verbs=update
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
}

// Reconcile is part of the main kubernetes reconciliation loop
func (r *TalosUpgradeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting reconciliation", "talosupgrade", req.Name)

	var talosUpgrade tupprv1alpha1.TalosUpgrade
	// For cluster-scoped resources, use Name instead of NamespacedName
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name}, &talosUpgrade); err != nil {
		if errors.IsNotFound(err) {
			logger.V(1).Info("TalosUpgrade resource not found, likely deleted", "talosupgrade", req.Name)
		} else {
			logger.Error(err, "Failed to get TalosUpgrade resource", "talosupgrade", req.Name)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.V(1).Info("Retrieved TalosUpgrade",
		"name", talosUpgrade.Name,
		"phase", talosUpgrade.Status.Phase,
		"generation", talosUpgrade.Generation,
		"observedGeneration", talosUpgrade.Status.ObservedGeneration)

	// Handle deletion
	if talosUpgrade.DeletionTimestamp != nil {
		logger.Info("TalosUpgrade is being deleted, starting cleanup", "name", talosUpgrade.Name)
		return r.cleanup(ctx, &talosUpgrade)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&talosUpgrade, TalosUpgradeFinalizer) {
		logger.V(1).Info("Adding finalizer to TalosUpgrade", "name", talosUpgrade.Name, "finalizer", TalosUpgradeFinalizer)
		controllerutil.AddFinalizer(&talosUpgrade, TalosUpgradeFinalizer)
		return ctrl.Result{}, r.Update(ctx, &talosUpgrade)
	}

	// Simple state machine
	logger.V(1).Info("Processing upgrade for TalosUpgrade", "name", talosUpgrade.Name, "currentPhase", talosUpgrade.Status.Phase)
	return r.processUpgrade(ctx, &talosUpgrade)
}

// processUpgrade contains the main logic for handling the upgrade process
func (r *TalosUpgradeReconciler) processUpgrade(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"talosupgrade", talosUpgrade.Name,
		"generation", talosUpgrade.Generation,
	)

	logger.V(1).Info("Starting upgrade processing")

	// Check for suspend annotation first (before any other processing)
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

	// Skip if already completed/failed (and generation matches)
	if talosUpgrade.Status.Phase == constants.PhaseCompleted || talosUpgrade.Status.Phase == constants.PhaseFailed {
		logger.V(1).Info("Upgrade already completed or failed",
			"phase", talosUpgrade.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Check for active job FIRST - if there's a job running, handle it regardless of queue position or failed nodes
	if activeJob, activeNode, err := r.findActiveJob(ctx, talosUpgrade); err != nil {
		logger.Error(err, "Failed to find active jobs")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if activeJob != nil {
		logger.V(1).Info("Found active job, handling its status", "job", activeJob.Name, "node", activeNode)
		return r.handleJobStatus(ctx, talosUpgrade, activeNode, activeJob)
	}

	// Check upgrade queue only after confirming no jobs are running
	if canProceed, pos, err := r.acquireUpgradeLock(ctx, talosUpgrade); err != nil {
		logger.Error(err, "Failed to check upgrade lock")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if !canProceed {
		logger.Info("Waiting in upgrade queue", "position", pos)
		if err := r.setPhase(ctx, talosUpgrade, constants.PhasePending, "", fmt.Sprintf("Waiting in queue (position %d)", pos)); err != nil {
			logger.Error(err, "Failed to update phase while waiting in queue")
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Check for failed nodes only after confirming no jobs are running
	if len(talosUpgrade.Status.FailedNodes) > 0 {
		logger.Info("Upgrade plan has failed nodes, blocking further progress",
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
	if talosUpgrade.Status.ObservedGeneration == 0 || talosUpgrade.Status.ObservedGeneration >= talosUpgrade.Generation {
		return false, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Spec generation changed, resetting upgrade process",
		"generation", talosUpgrade.Generation,
		"observed", talosUpgrade.Status.ObservedGeneration,
		"newVersion", talosUpgrade.Spec.Talos.Version)

	// Reset status for new generation
	return true, r.updateStatus(ctx, talosUpgrade, map[string]any{
		"phase":          constants.PhasePending,
		"currentNode":    "",
		"message":        fmt.Sprintf("Spec updated to %s, restarting upgrade process", talosUpgrade.Spec.Talos.Version),
		"completedNodes": []string{},
		"failedNodes":    []tupprv1alpha1.NodeUpgradeStatus{},
	})
}

// processNextNode finds the next node to upgrade or completes the upgrade if done
func (r *TalosUpgradeReconciler) processNextNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Perform health checks before finding next node
	if err := r.HealthChecker.CheckHealth(ctx, talosUpgrade.Spec.HealthChecks); err != nil {
		logger.Info("Health checks failed, will retry", "error", err.Error())
		if setErr := r.setPhase(ctx, talosUpgrade, constants.PhasePending, "", fmt.Sprintf("Waiting for health checks: %s", err.Error())); setErr != nil {
			logger.Error(setErr, "Failed to update phase for health check")
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
	logger := log.FromContext(ctx)

	// Always include generation and timestamp
	updates["observedGeneration"] = talosUpgrade.Generation
	updates["lastUpdated"] = metav1.Now()

	patch := map[string]any{
		"status": updates,
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	logger.V(1).Info("Applying status patch", "patch", string(patchBytes))

	statusObj := &tupprv1alpha1.TalosUpgrade{}
	statusObj.Name = talosUpgrade.Name

	return r.Status().Patch(ctx, statusObj, client.RawPatch(types.MergePatchType, patchBytes))
}

// setPhase updates the phase, current node, and message in status
func (r *TalosUpgradeReconciler) setPhase(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, phase, currentNode, message string) error {
	return r.updateStatus(ctx, talosUpgrade, map[string]any{
		"phase":       phase,
		"currentNode": currentNode,
		"message":     message,
	})
}

// addCompletedNode adds a node to the completed list if not already present
func (r *TalosUpgradeReconciler) addCompletedNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) error {
	if containsNode(nodeName, talosUpgrade.Status.CompletedNodes) {
		return nil
	}

	newCompleted := append(talosUpgrade.Status.CompletedNodes, nodeName)
	return r.updateStatus(ctx, talosUpgrade, map[string]any{
		"completedNodes": newCompleted,
	})
}

// addFailedNode adds a node to the failed list if not already present
func (r *TalosUpgradeReconciler) addFailedNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeStatus tupprv1alpha1.NodeUpgradeStatus) error {
	if containsFailedNode(nodeStatus.NodeName, talosUpgrade.Status.FailedNodes) {
		return nil
	}

	newFailed := append(talosUpgrade.Status.FailedNodes, nodeStatus)
	return r.updateStatus(ctx, talosUpgrade, map[string]any{
		"failedNodes": newFailed,
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
	if err := r.cleanupJobForNode(ctx, talosUpgrade, nodeName); err != nil {
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
func (r *TalosUpgradeReconciler) cleanupJobForNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) error {
	logger := log.FromContext(ctx)

	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(r.ControllerNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":                "talos-upgrade",
			"app.kubernetes.io/instance":            talosUpgrade.Name,
			"tuppr.home-operations.com/target-node": nodeName,
		}); err != nil {
		return fmt.Errorf("failed to list jobs for node %s: %w", nodeName, err)
	}

	for _, job := range jobList.Items {
		// Only delete successfully completed jobs
		if job.Status.Succeeded > 0 {
			logger.Info("Deleting successful job and its pods after node upgrade completion",
				"job", job.Name, "node", nodeName)

			// Use foreground deletion to ensure pods are cleaned up
			deletePolicy := metav1.DeletePropagationForeground
			if err := r.Delete(ctx, &job, &client.DeleteOptions{
				PropagationPolicy: &deletePolicy,
			}); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "Failed to delete successful job", "job", job.Name, "node", nodeName)
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

	nodeIP, err := GetNodeInternalIP(node)
	if err != nil {
		return fmt.Errorf("failed to get node IP for %s: %w", nodeName, err)
	}

	// Get node info from Talos client
	nodeInfo, err := r.TalosClient.GetNodeInfo(ctx, nodeIP)
	if err != nil {
		return fmt.Errorf("failed to get node info from Talos for %s: %w", nodeName, err)
	}

	targetVersion := talosUpgrade.Spec.Talos.Version

	if nodeInfo.TalosVersion != targetVersion {
		return fmt.Errorf("node %s version mismatch: current=%s, target=%s",
			nodeName, nodeInfo.TalosVersion, targetVersion)
	}

	logger.Info("Node upgrade verification successful using Talos client",
		"node", nodeName,
		"version", nodeInfo.TalosVersion,
		"currentImage", nodeInfo.Image)
	return nil
}

// acquireUpgradeLock checks if this TalosUpgrade can proceed based on the queue
func (r *TalosUpgradeReconciler) acquireUpgradeLock(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (bool, int, error) {
	logger := log.FromContext(ctx)

	// Get all TalosUpgrade resources in the cluster
	upgradeList := &tupprv1alpha1.TalosUpgradeList{}
	if err := r.List(ctx, upgradeList); err != nil {
		return false, 0, err
	}

	// Sort by creation timestamp, then by name for deterministic ordering
	sort.Slice(upgradeList.Items, func(i, j int) bool {
		iTime := upgradeList.Items[i].CreationTimestamp.Time
		jTime := upgradeList.Items[j].CreationTimestamp.Time

		// If timestamps are equal, sort by name for deterministic ordering
		if iTime.Equal(jTime) {
			return upgradeList.Items[i].Name < upgradeList.Items[j].Name
		}

		return iTime.Before(jTime)
	})

	// Find our position in the queue
	var queuePosition int
	var currentUpgrade *tupprv1alpha1.TalosUpgrade

	for _, upgrade := range upgradeList.Items {
		// Skip completed/failed upgrades
		if upgrade.Status.Phase == constants.PhaseCompleted || upgrade.Status.Phase == constants.PhaseFailed {
			continue
		}

		queuePosition++
		if upgrade.Name == talosUpgrade.Name {
			currentUpgrade = &upgrade
			break
		}
	}

	if currentUpgrade == nil {
		return false, 0, fmt.Errorf("could not find current upgrade in list")
	}

	// Check if any upgrade is currently running
	for _, upgrade := range upgradeList.Items {
		// Skip ourselves and completed/failed upgrades
		if upgrade.Name == talosUpgrade.Name ||
			upgrade.Status.Phase == constants.PhaseCompleted || upgrade.Status.Phase == constants.PhaseFailed {
			continue
		}

		// If someone else is running, we need to wait
		if upgrade.Status.Phase == constants.PhaseInProgress {
			logger.V(1).Info("Another upgrade is in progress, waiting",
				"runningUpgrade", upgrade.Name,
				"queuePosition", queuePosition)
			return false, queuePosition, nil
		}
	}

	// Check if we're next in line by comparing our sorted position
	for _, upgrade := range upgradeList.Items {
		if upgrade.Status.Phase == constants.PhaseCompleted || upgrade.Status.Phase == constants.PhaseFailed {
			continue
		}

		// If someone comes before us in the sorted order and is still pending, they go first
		if upgrade.Status.Phase == constants.PhasePending {
			// Compare using the same logic as our sort function
			upgradeTime := upgrade.CreationTimestamp.Time
			currentTime := currentUpgrade.CreationTimestamp.Time

			var isEarlier bool
			if upgradeTime.Equal(currentTime) {
				isEarlier = upgrade.Name < currentUpgrade.Name
			} else {
				isEarlier = upgradeTime.Before(currentTime)
			}

			if isEarlier {
				logger.V(1).Info("Waiting for earlier upgrade to start",
					"earlierUpgrade", upgrade.Name,
					"queuePosition", queuePosition)
				return false, queuePosition, nil
			}
		}

		// If we reach ourselves, we're next
		if upgrade.Name == talosUpgrade.Name {
			break
		}
	}

	logger.V(1).Info("Acquired upgrade lock", "upgrade", talosUpgrade.Name, "queuePosition", queuePosition)
	return true, queuePosition, nil
}

// findActiveJob looks for any currently running job for this TalosUpgrade
func (r *TalosUpgradeReconciler) findActiveJob(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (*batchv1.Job, string, error) {
	logger := log.FromContext(ctx)

	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(r.ControllerNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     "talos-upgrade",
			"app.kubernetes.io/instance": talosUpgrade.Name,
		}); err != nil {
		return nil, "", err
	}

	logger.V(1).Info("Found jobs for TalosUpgrade", "count", len(jobList.Items))

	for _, job := range jobList.Items {
		nodeName := job.Labels["tuppr.home-operations.com/target-node"]

		// Skip jobs for nodes that are already completed
		if containsNode(nodeName, talosUpgrade.Status.CompletedNodes) {
			logger.V(1).Info("Skipping job for already completed node", "job", job.Name, "node", nodeName)
			continue
		}

		// Skip jobs for nodes that are already failed
		if containsFailedNode(nodeName, talosUpgrade.Status.FailedNodes) {
			logger.V(1).Info("Skipping job for already failed node", "job", job.Name, "node", nodeName)
			continue
		}

		// Return the FIRST job we find that's not completed/failed
		// This ensures only ONE job is processed at a time
		logger.V(1).Info("Found job that needs handling",
			"job", job.Name,
			"node", nodeName,
			"active", job.Status.Active,
			"succeeded", job.Status.Succeeded,
			"failed", job.Status.Failed)
		return &job, nodeName, nil
	}

	return nil, "", nil
}

// findNextNode determines the next node that requires an upgrade
func (r *TalosUpgradeReconciler) findNextNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (string, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Finding next node to upgrade", "talosupgrade", talosUpgrade.Name)

	nodes, err := r.getTargetNodes(ctx, talosUpgrade)
	if err != nil {
		logger.Error(err, "Failed to get target nodes")
		return "", err
	}

	// Sort nodes by name for consistent ordering
	slices.SortFunc(nodes, func(a, b corev1.Node) int {
		return strings.Compare(a.Name, b.Name)
	})

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

// nodeNeedsUpgrade determines if a given node requires an upgrade using Talos client
func (r *TalosUpgradeReconciler) nodeNeedsUpgrade(ctx context.Context, node *corev1.Node, targetVersion string) bool {
	logger := log.FromContext(ctx)

	nodeIP, err := GetNodeInternalIP(node)
	if err != nil {
		logger.Error(err, "Failed to get node IP, cannot determine upgrade need", "node", node.Name)
		return false // Cannot proceed without IP
	}

	nodeInfo, err := r.TalosClient.GetNodeInfo(ctx, nodeIP)
	if err != nil {
		logger.Error(err, "Failed to get node info from Talos client, cannot determine upgrade need",
			"node", node.Name, "nodeIP", nodeIP)
		return false // Cannot proceed without Talos info
	}

	needsUpgrade := nodeInfo.TalosVersion != targetVersion
	logger.V(1).Info("Node upgrade check using Talos client",
		"node", node.Name,
		"currentVersion", nodeInfo.TalosVersion,
		"targetVersion", targetVersion,
		"needsUpgrade", needsUpgrade,
		"currentImage", nodeInfo.Image)

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

	nodeIP, err := GetNodeInternalIP(targetNode)
	if err != nil {
		logger.Error(err, "Failed to get node internal IP", "node", nodeName)
		return nil, err
	}

	logger.V(1).Info("Node details", "node", nodeName, "ip", nodeIP)

	job := r.buildJob(ctx, talosUpgrade, nodeName, nodeIP)
	if err := controllerutil.SetControllerReference(talosUpgrade, job, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference", "job", job.Name)
		return nil, err
	}

	logger.V(1).Info("Creating job in cluster", "job", job.Name, "node", nodeName, "targetIP", nodeIP)

	if err := r.Create(ctx, job); err != nil {
		if errors.IsAlreadyExists(err) {
			// Check if existing job belongs to our generation
			existingJob := &batchv1.Job{}
			if getErr := r.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, existingJob); getErr != nil {
				logger.Error(getErr, "Failed to get existing job", "job", job.Name)
				return nil, getErr
			}

			existingGen := existingJob.Labels["tuppr.home-operations.com/generation"]
			currentGen := fmt.Sprintf("%d", talosUpgrade.Generation)

			if existingGen == currentGen {
				logger.V(1).Info("Job already exists for current generation, reusing", "job", job.Name)
				return existingJob, nil
			} else {
				logger.Error(fmt.Errorf("job generation mismatch"), "Existing job has different generation",
					"job", job.Name, "existing", existingGen, "current", currentGen)
				return nil, fmt.Errorf("job generation conflict: %s", job.Name)
			}
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
		"tuppr.home-operations.com/generation":  fmt.Sprintf("%d", talosUpgrade.Generation),
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
		// Default to the target version for talosctl
		talosctlTag = talosUpgrade.Spec.Talos.Version
		logger.V(1).Info("Using target version for talosctl", "node", nodeName, "version", talosctlTag)
	}

	talosctlImage := talosctlRepo + ":" + talosctlTag

	// Build target image with schematic from node
	targetImage, err := r.buildTalosUpgradeImage(ctx, talosUpgrade, nodeName)
	if err != nil {
		// This should never happen
		targetImage = fmt.Sprintf("factory.talos.dev/metal-installer:%s", talosUpgrade.Spec.Talos.Version)
		logger.Error(err, "Using fallback image", "node", nodeName, "fallbackImage", targetImage)
	}

	args := []string{
		"upgrade",
		"--nodes=" + nodeIP,
		"--image=" + targetImage,
		"--timeout=" + TalosJobTalosUpgradeTimeout,
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
			ActiveDeadlineSeconds:   ptr.To(int64(TalosJobActiveDeadline)),
			PodReplacementPolicy:    ptr.To(batchv1.Failed),
			Template: corev1.PodTemplateSpec{
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
					InitContainers: []corev1.Container{{
						Name:            "health",
						Image:           talosctlImage,
						ImagePullPolicy: pullPolicy,
						Args:            []string{"health", "--nodes=" + nodeIP, "--wait-timeout=" + TalosJobTalosHealthTimeout},
						Env: []corev1.EnvVar{
							{
								Name:  "TALOSCONFIG",
								Value: "/var/run/secrets/talos.dev/config",
							},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							ReadOnlyRootFilesystem:   ptr.To(true),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1m"),
								corev1.ResourceMemory: resource.MustParse("8Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      constants.TalosSecretName,
							MountPath: "/var/run/secrets/talos.dev",
							ReadOnly:  true,
						}},
					}},
					Containers: []corev1.Container{{
						Name:            "upgrade",
						Image:           talosctlImage,
						ImagePullPolicy: pullPolicy,
						Args:            args,
						Env: []corev1.EnvVar{
							{
								Name:  "TALOSCONFIG",
								Value: "/var/run/secrets/talos.dev/config",
							},
						},
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
								corev1.ResourceMemory: resource.MustParse("32Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
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

// getTargetNodes retrieves nodes matching the TalosUpgrade's node selector
func (r *TalosUpgradeReconciler) getTargetNodes(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) ([]corev1.Node, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	logger := log.FromContext(timeoutCtx)

	logger.V(1).Info("Getting target nodes", "talosupgrade", talosUpgrade.Name)

	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		logger.Error(err, "Failed to list all nodes", "talosupgrade", talosUpgrade.Name)
		return nil, err
	}

	filteredNodes := getMatchingNodes(nodeList.Items, talosUpgrade)

	nodeNames := make([]string, 0, len(filteredNodes))
	for _, node := range filteredNodes {
		nodeNames = append(nodeNames, node.Name)
	}

	logger.V(1).Info("Found target nodes",
		"talosupgrade", talosUpgrade.Name,
		"count", len(filteredNodes),
		"nodes", nodeNames)

	return filteredNodes, nil
}

// buildTalosUpgradeImage constructs the target image using current node image as template
func (r *TalosUpgradeReconciler) buildTalosUpgradeImage(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (string, error) {
	logger := log.FromContext(ctx)

	// Get the target node
	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	nodeIP, err := GetNodeInternalIP(node)
	if err != nil {
		return "", fmt.Errorf("failed to get node IP for %s: %w", nodeName, err)
	}

	// Get node info from Talos client
	nodeInfo, err := r.TalosClient.GetNodeInfo(ctx, nodeIP)
	if err != nil {
		return "", fmt.Errorf("failed to get node info from Talos client for %s: %w", nodeName, err)
	}

	// Use the current image as template and replace just the tag
	if nodeInfo.Image == "" {
		return "", fmt.Errorf("no current image found for node %s", nodeName)
	}

	// Parse the current image to extract repository and schematic
	parts := strings.Split(nodeInfo.Image, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid current image format for node %s: %s", nodeName, nodeInfo.Image)
	}

	// Build new image with same repository/schematic but new version
	targetImage := fmt.Sprintf("%s:%s", parts[0], talosUpgrade.Spec.Talos.Version)

	logger.Info("Built target image from current node image",
		"node", nodeName,
		"currentImage", nodeInfo.Image,
		"targetImage", targetImage)

	return targetImage, nil
}

// getMatchingNodes returns nodes that match the TalosUpgrade's node selector
func getMatchingNodes(allNodes []corev1.Node, talos *tupprv1alpha1.TalosUpgrade) []corev1.Node {
	selector := talos.Spec.NodeSelector

	// If no selector is specified, return all nodes
	if len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0 {
		return allNodes
	}

	// Filter nodes that match the selector
	var matchingNodes []corev1.Node
	for _, node := range allNodes {
		if nodeMatchesLabelSelector(&node, selector) {
			matchingNodes = append(matchingNodes, node)
		}
	}

	return matchingNodes
}

// nodeMatchesLabelSelector checks if a node matches the label selector (both matchLabels and matchExpressions)
func nodeMatchesLabelSelector(node *corev1.Node, selector tupprv1alpha1.NodeSelectorSpec) bool {
	// Check matchLabels (all must match - AND logic)
	for key, value := range selector.MatchLabels {
		nodeValue, exists := node.Labels[key]
		if !exists || nodeValue != value {
			return false
		}
	}

	// Check matchExpressions (all must match - AND logic)
	for _, expr := range selector.MatchExpressions {
		if !nodeMatchesLabelExpression(node, expr) {
			return false
		}
	}

	return true
}

// nodeMatchesLabelExpression checks if a node matches a single label selector requirement
func nodeMatchesLabelExpression(node *corev1.Node, expr metav1.LabelSelectorRequirement) bool {
	nodeValue, exists := node.Labels[expr.Key]

	switch expr.Operator {
	case metav1.LabelSelectorOpIn:
		return exists && slices.Contains(expr.Values, nodeValue)

	case metav1.LabelSelectorOpNotIn:
		return !exists || !slices.Contains(expr.Values, nodeValue)

	case metav1.LabelSelectorOpExists:
		return exists

	case metav1.LabelSelectorOpDoesNotExist:
		return !exists

	default:
		return false
	}
}

// containsNode checks if a node name is in the list of nodes
func containsNode(nodeName string, nodes []string) bool {
	return slices.Contains(nodes, nodeName)
}

// containsFailedNode checks if a node name is in the list of failed nodes
func containsFailedNode(nodeName string, failedNodes []tupprv1alpha1.NodeUpgradeStatus) bool {
	return slices.ContainsFunc(failedNodes, func(n tupprv1alpha1.NodeUpgradeStatus) bool {
		return n.NodeName == nodeName
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *TalosUpgradeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := ctrl.Log.WithName("setup")
	logger.Info("Setting up TalosUpgrade controller with manager")

	r.HealthChecker = &HealthChecker{Client: mgr.GetClient()}
	r.TalosClient = NewTalosClient(mgr.GetClient(), r.TalosConfigSecret, r.ControllerNamespace)

	return ctrl.NewControllerManagedBy(mgr).
		For(&tupprv1alpha1.TalosUpgrade{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
