package talosupgrade

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabel "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/coordination"
	"github.com/home-operations/tuppr/internal/controller/drain"
	"github.com/home-operations/tuppr/internal/controller/maintenance"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
	"github.com/home-operations/tuppr/internal/controller/upgradeaudit"
	"github.com/home-operations/tuppr/internal/metrics"
	"github.com/home-operations/tuppr/internal/talos"
)

const (
	// volumeDetachTimeout bounds how long drain waits for a node's CSI volumes to
	// detach before rebooting. Best-effort: on timeout the upgrade proceeds anyway.
	volumeDetachTimeout = 2 * time.Minute
	// volumeDetachPollInterval is how often the detach wait polls VolumeAttachments.
	volumeDetachPollInterval = 5 * time.Second
)

func (r *Reconciler) processUpgrade(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"talosupgrade", talosUpgrade.Name,
		"generation", talosUpgrade.Generation,
	)

	logger.V(1).Info("Starting upgrade processing")

	if suspended, err := r.handleSuspendAnnotation(ctx, talosUpgrade); err != nil || suspended {
		return ctrl.Result{RequeueAfter: time.Minute * 30}, err
	}

	if resetRequested, err := r.handleResetAnnotation(ctx, talosUpgrade); err != nil || resetRequested {
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	if reset, err := r.handleGenerationChange(ctx, talosUpgrade); err != nil || reset {
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	if talosUpgrade.Status.Phase.IsTerminal() {
		if talosUpgrade.Status.Phase == tupprv1alpha1.JobPhaseFailed {
			logger.V(1).Info("Talos upgrade in terminal state, skipping", "phase", talosUpgrade.Status.Phase)
			return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
		}
		nextNodes, err := r.findNextNodes(ctx, talosUpgrade, 1)
		if err != nil {
			return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonFindNextNodes, "re-check nodes for restart", time.Minute*5, err), nil
		}
		if len(nextNodes) == 0 {
			return ctrl.Result{RequeueAfter: time.Hour}, nil
		}
		targetVersion := talosUpgrade.Spec.Talos.Version
		cycles := completionCyclesForVersion(talosUpgrade.Status.History, targetVersion)
		if cycles >= upgradeaudit.MaxCompletionCycles {
			message := fmt.Sprintf(
				"Node(s) never converged to %s after %d completion cycles; add the %s annotation or bump the spec to retry",
				targetVersion, cycles, constants.ResetAnnotation,
			)
			logger.Info("Completion cycles exhausted, marking upgrade Failed", "target", targetVersion, "cycles", cycles)
			if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseFailed, message); err != nil {
				logger.Error(err, "Failed to set Failed phase after exhausting completion cycles")
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
			return ctrl.Result{RequeueAfter: time.Hour}, nil
		}
		logger.Info("Node detected requiring upgrade after completion, restarting campaign", "node", nextNodes[0], "cycle", cycles+1)
		if err := r.setPhaseWithUpdates(ctx, talosUpgrade, tupprv1alpha1.JobPhasePending, "", nil, "New node detected, restarting upgrade", map[string]any{
			statusCompletedNodes: []string{},
			statusFailedNodes:    []tupprv1alpha1.NodeUpgradeStatus{},
			statusPreHookIndex:   0,
			statusPostHookIndex:  0,
			statusPreHookFailed:  false,
		}); err != nil {
			logger.Error(err, "Failed to re-enter Pending after completion")
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}
		resetHookProgress(&talosUpgrade.Status)
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	switch talosUpgrade.Status.Phase {
	case tupprv1alpha1.JobPhasePreHook:
		result, done, err := r.processHookPhase(ctx, talosUpgrade, hookPhasePre)
		if err != nil {
			return result, err
		}
		if !done {
			return result, nil
		}
		if talosUpgrade.Status.PreHookFailed {
			logger.Info("Pre-hook failed; running post-hooks then failing")
			return r.transitionToFinalize(ctx, talosUpgrade)
		}
		if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhasePending, "Pre-hooks complete; starting upgrade"); err != nil {
			logger.Error(err, "Failed to advance phase past pre-hooks")
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	case tupprv1alpha1.JobPhasePostHook:
		result, done, err := r.processHookPhase(ctx, talosUpgrade, hookPhasePost)
		if err != nil {
			return result, err
		}
		if !done {
			return result, nil
		}
		return r.completeUpgrade(ctx, talosUpgrade)
	}

	if !talosUpgrade.Status.Phase.IsActive() {
		if result, done, err := r.checkMaintenanceWindow(ctx, talosUpgrade); done {
			return result, err
		}
		if result, done := r.checkCoordination(ctx, talosUpgrade); done {
			return result, nil
		}
	}

	if activeJobs, activeNodes, err := r.findActiveJobs(ctx, talosUpgrade); err != nil {
		return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonFindActiveJobs, "list active upgrade jobs", time.Minute, err), nil
	} else if len(activeJobs) > 0 {
		logger.V(1).Info("Found active jobs, handling batch status", "count", len(activeJobs), "nodes", activeNodes)
		return r.handleBatchJobStatus(ctx, talosUpgrade, activeJobs, activeNodes)
	}

	if len(talosUpgrade.Status.FailedNodes) > 0 {
		logger.Info("Upgrade stopped due to failed nodes",
			"failedNodes", len(talosUpgrade.Status.FailedNodes))
		return r.transitionToFinalize(ctx, talosUpgrade)
	}

	return r.processNextBatch(ctx, talosUpgrade)
}

// maybeEnterPreHook transitions to PreHook phase if pre-hooks are configured
// and have not yet been attempted (or completed). Returns entered=true when
// the caller should return the result instead of continuing.
func (r *Reconciler) maybeEnterPreHook(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, bool, error) {
	if !hasPreHooks(talosUpgrade) ||
		talosUpgrade.Status.PreHookFailed ||
		talosUpgrade.Status.PreHookIndex >= len(talosUpgrade.Spec.Hooks.Pre) {
		return ctrl.Result{}, false, nil
	}
	if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhasePreHook, "Running pre-upgrade hooks"); err != nil {
		log.FromContext(ctx).Error(err, "Failed to enter PreHook phase")
		return ctrl.Result{RequeueAfter: time.Minute}, true, err
	}
	return ctrl.Result{RequeueAfter: time.Second * 5}, true, nil
}

// transitionToFinalize routes to PostHook (if configured and not yet run) or
// directly to the terminal phase via completeUpgrade.
func (r *Reconciler) transitionToFinalize(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if hasPostHooks(talosUpgrade) && talosUpgrade.Status.PostHookIndex < len(talosUpgrade.Spec.Hooks.Post) {
		if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhasePostHook, "Running post-hooks"); err != nil {
			logger.Error(err, "Failed to transition to PostHook")
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		}
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}
	return r.completeUpgrade(ctx, talosUpgrade)
}

func hasPreHooks(tu *tupprv1alpha1.TalosUpgrade) bool {
	return tu.Spec.Hooks != nil && len(tu.Spec.Hooks.Pre) > 0
}

func hasPostHooks(tu *tupprv1alpha1.TalosUpgrade) bool {
	return tu.Spec.Hooks != nil && len(tu.Spec.Hooks.Post) > 0
}

func (r *Reconciler) checkMaintenanceWindow(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, bool, error) {
	logger := log.FromContext(ctx)

	maintenanceRes, err := maintenance.CheckWindow(talosUpgrade.Spec.Maintenance, r.Now.Now())
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Second * 30}, true, err
	}
	if !maintenanceRes.Allowed {
		requeueAfter := maintenanceRes.RequeueAfter(r.Now.Now())
		nextTimestamp := maintenanceRes.NextWindowStart.Unix()
		r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeTalos, talosUpgrade.Name, false, &nextTimestamp)
		message := fmt.Sprintf("Waiting for maintenance window (next: %s)", maintenanceRes.NextWindowStart.Format(time.RFC3339))
		if err := r.setPhaseWithUpdates(ctx, talosUpgrade, tupprv1alpha1.JobPhaseMaintenanceWindow, "", nil, message, map[string]any{
			"nextMaintenanceWindow": metav1.NewTime(*maintenanceRes.NextWindowStart),
		}); err != nil {
			logger.Error(err, "Failed to update status for maintenance window")
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, true, nil
	}
	r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeTalos, talosUpgrade.Name, true, nil)
	return ctrl.Result{}, false, nil
}

func (r *Reconciler) checkCoordination(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, bool) {
	logger := log.FromContext(ctx)

	blocked, message, err := coordination.IsAnotherUpgradeActive(ctx, r.Client, talosUpgrade.Name, coordination.UpgradeTypeTalos)
	if err != nil {
		return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonCheckCoordination, "check for other active upgrades", time.Minute, err), true
	}
	if blocked {
		logger.Info("Waiting for another upgrade to complete", "reason", message)
		if err := r.setPendingWithReason(ctx, talosUpgrade, upgradeaudit.ReasonWaitingForOtherUpgrade, message); err != nil {
			logger.Error(err, "Failed to update phase for coordination wait")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 2}, true
	}
	return ctrl.Result{}, false
}

func (r *Reconciler) completeUpgrade(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	completedCount := len(talosUpgrade.Status.CompletedNodes)
	failedCount := len(talosUpgrade.Status.FailedNodes)

	var phase tupprv1alpha1.JobPhase
	var message string
	switch {
	case talosUpgrade.Status.PreHookFailed:
		phase = tupprv1alpha1.JobPhaseFailed
		message = "Pre-upgrade hook failed; upgrade did not run"
		logger.Info("Upgrade marked failed due to pre-hook failure")
	case failedCount > 0:
		phase = tupprv1alpha1.JobPhaseFailed
		message = fmt.Sprintf("Completed with failures: %d successful, %d failed", completedCount, failedCount)
		logger.Info("Upgrade completed with failures", "completed", completedCount, "failed", failedCount)
	default:
		phase = tupprv1alpha1.JobPhaseCompleted
		message = fmt.Sprintf("Successfully upgraded %d nodes", completedCount)
		logger.Info("Upgrade completed successfully", "nodes", completedCount)
	}

	if err := r.setPhase(ctx, talosUpgrade, phase, message); err != nil {
		logger.Error(err, "Failed to update completion phase")
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}
	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

func (r *Reconciler) processNextBatch(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	maintenanceRes, err := maintenance.CheckWindow(talosUpgrade.Spec.Maintenance, r.Now.Now())
	if err != nil {
		return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonCheckMaintenanceWindow, "evaluate maintenance window", 30*time.Second, err), err
	}
	if !maintenanceRes.Allowed {
		requeueAfter := maintenanceRes.RequeueAfter(r.Now.Now())
		nextTimestamp := maintenanceRes.NextWindowStart.Unix()
		r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeTalos, talosUpgrade.Name, false, &nextTimestamp)
		message := fmt.Sprintf("Maintenance window closed between nodes, waiting (next: %s)", maintenanceRes.NextWindowStart.Format(time.RFC3339))
		if err := r.setPhaseWithUpdates(ctx, talosUpgrade, tupprv1alpha1.JobPhaseMaintenanceWindow, "", nil, message, map[string]any{
			"nextMaintenanceWindow": metav1.NewTime(*maintenanceRes.NextWindowStart),
		}); err != nil {
			logger.Error(err, "Failed to update status for maintenance window")
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeTalos, talosUpgrade.Name, true, nil)

	ctx = context.WithValue(ctx, metrics.ContextKeyUpgradeType, metrics.UpgradeTypeTalos)
	ctx = context.WithValue(ctx, metrics.ContextKeyUpgradeName, talosUpgrade.Name)

	parallelism := getParallelism(talosUpgrade.Spec)
	nextNodes, err := r.findNextNodes(ctx, talosUpgrade, parallelism)
	if err != nil {
		return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonFindNextNodes, "find candidate nodes", time.Minute, err), nil
	}

	if len(nextNodes) == 0 {
		if err := r.recordOutOfBandCompletedNodes(ctx, talosUpgrade); err != nil {
			logger.Error(err, "Failed to record out-of-band upgraded nodes")
		}
		return r.transitionToFinalize(ctx, talosUpgrade)
	}

	// Image availability is checked before HealthChecking: a stuck external
	// registry must not flip the phase on every reconcile.
	type nodeImage struct {
		nodeName string
		image    string
	}
	var batch []nodeImage

	for _, nodeName := range nextNodes {
		targetImage, err := r.buildTalosUpgradeImage(ctx, talosUpgrade, nodeName)
		if err != nil {
			return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonBuildTargetImage, fmt.Sprintf("build target image for node %s", nodeName), time.Minute, err), nil
		}

		logger.V(1).Info("Verifying target image availability", "node", nodeName, "image", targetImage)
		if err := r.ImageChecker.Check(ctx, targetImage); err != nil {
			logger.Info("Waiting for target image to become available", "node", nodeName, "image", targetImage, "error", err.Error())
			message := fmt.Sprintf("Waiting for image availability for node %s: %s", nodeName, err.Error())
			if err := r.setPendingWithReason(ctx, talosUpgrade, upgradeaudit.ReasonWaitingForImage, message); err != nil {
				logger.Error(err, "Failed to update phase while waiting for image")
			}
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}

		batch = append(batch, nodeImage{nodeName: nodeName, image: targetImage})
	}

	// Skip the inter-batch health check re-run when pre-hooks are configured and
	// have already run: pre-hooks own cluster state for the rest of the upgrade
	// window, so re-checking would fail (e.g. Ceph reports HEALTH_WARN while
	// `noout` is set). The initial check (first batch, no completed nodes yet)
	// still runs.
	skipHealthCheck := hasPreHooks(talosUpgrade) &&
		talosUpgrade.Status.PreHookIndex >= len(talosUpgrade.Spec.Hooks.Pre) &&
		len(talosUpgrade.Status.CompletedNodes) > 0

	if !skipHealthCheck {
		checkErr := r.HealthChecker.CheckHealth(ctx, talosUpgrade.Spec.HealthChecks)
		message := "Running health checks"
		if checkErr != nil {
			message = fmt.Sprintf("Waiting for health checks: %s", checkErr.Error())
		}
		if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseHealthChecking, message); err != nil {
			logger.Error(err, "Failed to update phase for health check")
		}
		if checkErr != nil {
			logger.Info("Waiting for health checks to pass", "error", checkErr.Error())
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
	}

	if result, entered, err := r.maybeEnterPreHook(ctx, talosUpgrade); entered {
		return result, err
	}

	logger.Info("Starting batch upgrade", "nodes", nextNodes, "batchSize", len(batch))

	// Drain all nodes in batch before creating any jobs
	if r.tupprOwnsDrain(ctx, talosUpgrade) {
		if err := r.setPhaseWithNodes(ctx, talosUpgrade, tupprv1alpha1.JobPhaseDraining, nextNodes, fmt.Sprintf("Draining %d nodes", len(nextNodes))); err != nil {
			logger.Error(err, "Failed to update phase for draining")
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		}
		var drainedNodes []string
		for _, ni := range batch {
			logger.Info("Draining node before upgrade", "node", ni.nodeName)
			if err := r.drainNode(ctx, ni.nodeName, talosUpgrade.Spec.Drain); err != nil {
				logger.Error(err, "Failed to drain node, rolling back already-drained nodes in batch", "node", ni.nodeName)
				drainer := drain.NewDrainer(r.Client)
				// Include the failing node: cordon may have landed before the
				// drain error. UncordonNode is a no-op on already-schedulable nodes.
				rollback := append(drainedNodes, ni.nodeName)
				for _, n := range rollback {
					if uncordonErr := drainer.UncordonNode(ctx, n); uncordonErr != nil {
						logger.Error(uncordonErr, "Failed to uncordon node during rollback", "node", n)
					} else {
						logger.Info("Rolled back drain for node", "node", n)
					}
				}
				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
			drainedNodes = append(drainedNodes, ni.nodeName)
			logger.V(1).Info("Node drained successfully", "node", ni.nodeName)
		}
	}

	// Create jobs for all nodes in batch
	for _, ni := range batch {
		if _, err := r.createJob(ctx, talosUpgrade, ni.nodeName, ni.image); err != nil {
			return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonCreateJob, fmt.Sprintf("create upgrade job for node %s", ni.nodeName), time.Minute, err), nil
		}

		if err := r.addNodeUpgradingLabel(ctx, ni.nodeName); err != nil {
			logger.Error(err, "Failed to add upgrading label to node", "node", ni.nodeName)
		}
	}

	if err := r.setPhaseWithNodes(ctx, talosUpgrade, tupprv1alpha1.JobPhaseUpgrading, nextNodes, fmt.Sprintf("Upgrading %d nodes", len(nextNodes))); err != nil {
		logger.Error(err, "Failed to update phase for batch upgrade")
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

func (r *Reconciler) recordOutOfBandCompletedNodes(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) error {
	logger := log.FromContext(ctx)

	nodes, err := r.getSortedNodes(ctx, talosUpgrade.Spec.NodeSelector)
	if err != nil {
		return err
	}

	crdTargetVersion := talosUpgrade.Spec.Talos.Version
	var added []string

	for i := range nodes {
		node := &nodes[i]
		if slices.Contains(talosUpgrade.Status.CompletedNodes, node.Name) {
			continue
		}
		if slices.ContainsFunc(talosUpgrade.Status.FailedNodes, func(fn tupprv1alpha1.NodeUpgradeStatus) bool {
			return fn.NodeName == node.Name
		}) {
			continue
		}

		needsUpgrade, err := r.nodeNeedsUpgrade(ctx, node, crdTargetVersion)
		if err != nil {
			return fmt.Errorf("check node %s: %w", node.Name, err)
		}
		if needsUpgrade {
			continue
		}
		// Job may have been GC'd before the controller reconciled post-reboot; the
		// node is at target but may still be cordoned.
		r.ensureNodeUncordoned(ctx, talosUpgrade, node.Name)
		added = append(added, node.Name)
	}

	if len(added) == 0 {
		return nil
	}

	logger.Info("Recording nodes upgraded out of band", "nodes", added)
	if r.Recorder != nil {
		r.Recorder.Eventf(talosUpgrade, corev1.EventTypeNormal, "OutOfBandUpgrade",
			"Recorded %d node(s) already at target version %s: %v",
			len(added), crdTargetVersion, added)
	}
	talosUpgrade.Status.CompletedNodes = append(talosUpgrade.Status.CompletedNodes, added...)
	return r.updateStatus(ctx, talosUpgrade, map[string]any{
		statusCompletedNodes: talosUpgrade.Status.CompletedNodes,
	})
}

// findNextNodes returns up to `count` node names that need upgrading, sorted alphabetically.
func (r *Reconciler) findNextNodes(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, count int) ([]string, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Finding next nodes to upgrade", "talosupgrade", talosUpgrade.Name, "count", count)

	nodes, err := r.getSortedNodes(ctx, talosUpgrade.Spec.NodeSelector)
	if err != nil {
		logger.Error(err, "Failed to get nodes")
		return nil, err
	}

	crdTargetVersion := talosUpgrade.Spec.Talos.Version
	var result []string

	// Reconcile the outdated taint across the whole selected set; no early break.
	for i := range nodes {
		node := &nodes[i]

		if slices.Contains(talosUpgrade.Status.CompletedNodes, node.Name) {
			if err := r.removeNodeOutdatedTaint(ctx, node.Name); err != nil {
				logger.Error(err, "Failed to remove outdated taint from completed node", "node", node.Name)
			}
			continue
		}

		if slices.ContainsFunc(talosUpgrade.Status.FailedNodes, func(fn tupprv1alpha1.NodeUpgradeStatus) bool {
			return fn.NodeName == node.Name
		}) {
			continue
		}

		needsUpgrade, err := r.nodeNeedsUpgrade(ctx, node, crdTargetVersion)
		if err != nil {
			logger.Error(err, "Failed to check if node needs upgrade", "node", node.Name)
			return nil, fmt.Errorf("failed to check node %s: %w", node.Name, err)
		}

		if needsUpgrade {
			logger.V(1).Info("Node needs upgrade", "node", node.Name)
			if err := r.addNodeOutdatedTaint(ctx, node.Name); err != nil {
				logger.Error(err, "Failed to add outdated taint", "node", node.Name)
			}
			if len(result) < count {
				result = append(result, node.Name)
			}
		} else if err := r.removeNodeOutdatedTaint(ctx, node.Name); err != nil {
			logger.Error(err, "Failed to remove outdated taint", "node", node.Name)
		}
	}

	if len(result) == 0 {
		logger.V(1).Info("All nodes are up to date")
	}
	return result, nil
}

func (r *Reconciler) getSortedNodes(ctx context.Context, nodeSelector *metav1.LabelSelector) ([]corev1.Node, error) {
	var selector k8slabel.Selector
	var err error
	nodeList := &corev1.NodeList{}
	if nodeSelector != nil {
		selector, err = metav1.LabelSelectorAsSelector(nodeSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to parse nodeSelector: %w", err)
		}
	} else {
		selector = k8slabel.Everything()
	}

	listOpts := &client.ListOptions{LabelSelector: selector}
	if err := r.List(ctx, nodeList, listOpts); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	nodes := nodeList.Items
	controllerNode := r.ControllerNodeName
	slices.SortFunc(nodes, func(a, b corev1.Node) int {
		// if present, always upgrade controller node first
		if controllerNode != "" {
			if a.Name == controllerNode {
				return -1
			}
			if b.Name == controllerNode {
				return 1
			}
		}
		return strings.Compare(a.Name, b.Name)
	})

	return nodes, nil
}

func (r *Reconciler) nodeNeedsUpgrade(ctx context.Context, node *corev1.Node, crdTargetVersion string) (bool, error) {
	logger := log.FromContext(ctx)

	nodeIP, err := nodeutil.GetNodeIP(node)
	if err != nil {
		return false, fmt.Errorf("failed to get node IP for %s: %w", node.Name, err)
	}

	currentVersion, err := r.TalosClient.GetNodeVersion(ctx, nodeIP)
	if err != nil {
		return false, fmt.Errorf("failed to get current version for node %s (%s): %w", node.Name, nodeIP, err)
	}

	targetVersion := r.getTargetVersion(node, crdTargetVersion)

	if currentVersion != targetVersion {
		logger.V(1).Info("Node version mismatch detected",
			"node", node.Name,
			"current", currentVersion,
			"target", targetVersion)
		return true, nil
	}

	return false, nil
}

// isSelfHostedUpgrade reports whether the cluster has a single node, in which case
// the upgrade pod can only run on the node being upgraded and is killed by the
// reboot — so the upgrade must be issued without --wait.
func (r *Reconciler) isSelfHostedUpgrade(ctx context.Context) bool {
	count, err := r.getTotalNodeCount(ctx)
	if err != nil {
		log.FromContext(ctx).V(1).Info("Failed to count nodes; assuming multi-node", "error", err)
		return false
	}
	return count == 1
}

// tupprOwnsDrain reports whether tuppr should drain the node itself before the
// upgrade job (rather than delegating to Talos). True when the deprecated Drain
// spec is enabled, or when Policy.WaitForVolumeDetach is set on a multi-node
// cluster. Single-node clusters never drain (pods have nowhere to go).
func (r *Reconciler) tupprOwnsDrain(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) bool {
	if talosUpgrade.Spec.DrainEnabled() {
		return true
	}
	return talosUpgrade.Spec.Policy.WaitForVolumeDetach && !r.isSelfHostedUpgrade(ctx)
}

func (r *Reconciler) drainNode(ctx context.Context, nodeName string, drainSpec *tupprv1alpha1.DrainSpec) error {
	logger := log.FromContext(ctx)

	// Never evict the controller's own pod: on a single node it runs on the node
	// being drained.
	drainer := drain.NewDrainer(r.Client).SkipPod(r.ControllerNamespace, r.ControllerPodName)

	// Cordon the node first
	if err := drainer.CordonNode(ctx, nodeName); err != nil {
		return fmt.Errorf("failed to cordon node %s: %w", nodeName, err)
	}

	// Capture volumes before eviction so we can wait for them to detach post-drain.
	volumePVs, err := drainer.EvictableVolumePVs(ctx, nodeName)
	if err != nil {
		logger.Error(err, "Failed to resolve volumes for node, skipping detach wait", "node", nodeName)
	}

	// drainSpec is nil when draining is driven by Policy.WaitForVolumeDetach rather
	// than the deprecated Drain spec; respect PDBs by default in that case.
	respectPDBs := true
	if drainSpec != nil && drainSpec.DisableEviction != nil {
		respectPDBs = !*drainSpec.DisableEviction
	}

	opts := drain.DrainOptions{
		RespectPDBs: respectPDBs,
		Timeout:     10 * time.Minute,
		GracePeriod: nil,
	}

	// Drain the node
	if err := drainer.DrainNode(ctx, nodeName, opts); err != nil {
		return fmt.Errorf("failed to drain node %s: %w", nodeName, err)
	}

	// Drain only waits for pods to terminate, not for CSI teardown; wait for that too.
	if len(volumePVs) > 0 {
		logger.Info("Waiting for volumes to detach before reboot", "node", nodeName, "count", len(volumePVs))
		if err := drainer.WaitForVolumeDetach(ctx, nodeName, volumePVs, volumeDetachTimeout, volumeDetachPollInterval); err != nil {
			// Best-effort: a slow detach shouldn't block the upgrade indefinitely.
			logger.Error(err, "Volumes did not detach before timeout, proceeding with upgrade", "node", nodeName)
		}
	}

	return nil
}

// Matches the canonical generic installer and any mirror that keeps the "/siderolabs/installer" suffix.
func looksLikeGenericInstaller(repo string) bool {
	return repo == constants.GenericInstallerRepo || strings.HasSuffix(repo, "/siderolabs/installer")
}

func (r *Reconciler) buildTalosUpgradeImage(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (string, error) {
	logger := log.FromContext(ctx)

	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	nodeIP, err := nodeutil.GetNodeIP(node)
	if err != nil {
		return "", fmt.Errorf("failed to get node IP for %s: %w", nodeName, err)
	}

	currentImage, err := r.TalosClient.GetNodeInstallImage(ctx, nodeIP)
	if err != nil {
		return "", fmt.Errorf("failed to get install image from Talos client for %s: %w", nodeName, err)
	}

	targetVersion := r.getTargetVersion(node, talosUpgrade.Spec.Talos.Version)

	if base := node.Annotations[constants.FactoryURLAnnotation]; base != "" {
		ext, err := r.TalosClient.GetNodeExtensions(ctx, nodeIP)
		if err != nil {
			return "", fmt.Errorf("failed to read extensions for node %s: %w", nodeName, err)
		}
		// An explicit schematic annotation overrides the runtime-reported one.
		schematic := node.Annotations[constants.SchematicAnnotation]
		if schematic == "" {
			schematic = ext.Schematic
		}
		if schematic == "" {
			return "", fmt.Errorf(
				"node %s: annotation %s is set but no schematic is available — runtime reports none and annotation %s is unset",
				nodeName, constants.FactoryURLAnnotation, constants.SchematicAnnotation)
		}
		targetImage := fmt.Sprintf("%s/%s:%s", strings.TrimRight(base, "/"), schematic, targetVersion)
		logger.V(1).Info("Built target image from FactoryURL override",
			"node", nodeName, "targetImage", targetImage, "schematic", schematic)
		return targetImage, nil
	}

	repo, _, ok := strings.Cut(currentImage, ":")
	if !ok || repo == "" {
		return "", fmt.Errorf("invalid current image format for node %s: %s", nodeName, currentImage)
	}

	ext, err := r.TalosClient.GetNodeExtensions(ctx, nodeIP)
	if err != nil {
		return "", fmt.Errorf("failed to read extensions for node %s: %w", nodeName, err)
	}
	if ext.Schematic != "" && !strings.HasSuffix(repo, "/"+ext.Schematic) {
		return "", fmt.Errorf(
			"node %s: install image %q does not embed the runtime schematic %s; reinstalling would wipe extensions. Fix .machine.install.image to a factory image, or set annotation %s",
			nodeName, currentImage, ext.Schematic, constants.FactoryURLAnnotation)
	}
	if ext.Schematic == "" && len(ext.Extensions) > 0 && looksLikeGenericInstaller(repo) {
		return "", fmt.Errorf(
			"node %s: install image %q has no schematic but the node has extensions=%v; reinstalling would wipe them. Set annotation %s with %s to upgrade to a factory image",
			nodeName, currentImage, ext.Extensions, constants.FactoryURLAnnotation, constants.SchematicAnnotation)
	}

	targetImage := fmt.Sprintf("%s:%s", repo, targetVersion)
	logger.V(1).Info("Built target image", "node", nodeName, "targetImage", targetImage, "version", targetVersion)
	return targetImage, nil
}

func (r *Reconciler) getTargetVersion(node *corev1.Node, crdTargetVersion string) string {
	if v, ok := node.Annotations[constants.VersionAnnotation]; ok && v != "" {
		return v
	}
	return crdTargetVersion
}

func (r *Reconciler) verifyNodeUpgrade(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (bool, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Verifying node upgrade using Talos client", "node", nodeName)

	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return false, fmt.Errorf("failed to get node: %w", err)
	}

	nodeIP, err := nodeutil.GetNodeIP(node)
	if err != nil {
		return false, fmt.Errorf("failed to get node IP for %s: %w", nodeName, err)
	}

	targetVersion := r.getTargetVersion(node, talosUpgrade.Spec.Talos.Version)

	if err := r.TalosClient.CheckNodeReady(ctx, nodeIP, nodeName); err != nil {
		if talos.IsTransientError(err) {
			logger.V(1).Info("Node not ready yet, will retry", "node", nodeName, "error", err)
			return false, nil
		}
		return false, err
	}

	currentVersion, err := r.TalosClient.GetNodeVersion(ctx, nodeIP)
	if err != nil {
		if talos.IsTransientError(err) {
			logger.V(1).Info("Node not ready yet, will retry", "node", nodeName, "error", err)
			return false, nil
		}
		return false, fmt.Errorf("failed to get current version from Talos for %s: %w", nodeName, err)
	}

	if currentVersion != targetVersion {
		return false, fmt.Errorf("node %s version mismatch: current=%s, target=%s",
			nodeName, currentVersion, targetVersion)
	}

	logger.V(1).Info("Node upgrade verification successful",
		"node", nodeName,
		"version", currentVersion)
	return true, nil
}

// isNodeReady returns true if the node has a Ready condition set to True.
func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
