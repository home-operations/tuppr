package talosupgrade

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/platforms"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/coordination"
	"github.com/home-operations/tuppr/internal/controller/drain"
	"github.com/home-operations/tuppr/internal/controller/jobs"
	"github.com/home-operations/tuppr/internal/controller/maintenance"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
	"github.com/home-operations/tuppr/internal/controller/upgradeaudit"
	"github.com/home-operations/tuppr/internal/healthcheck"
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
	logger := log.FromContext(ctx).WithValues("generation", talosUpgrade.Generation)
	ctx = log.IntoContext(ctx, logger)

	logger.V(1).Info("Starting upgrade processing")

	suspended, suspendErr := r.handleSuspendAnnotation(ctx, talosUpgrade)
	// The silence leases ride every reconcile pass: extended while the run is
	// active, expired when it parks, suspends, or finishes.
	r.syncAlertSilences(ctx, talosUpgrade, suspended)
	if suspendErr != nil || suspended {
		return ctrl.Result{RequeueAfter: time.Minute * 30}, suspendErr
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
		// Unreachable nodes are ignored here: a completed upgrade shouldn't
		// restart (or error) because a node went offline afterwards.
		nextNodes, _, err := r.findNextNodes(ctx, talosUpgrade, 1)
		if err != nil {
			return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonFindNextNodes, "re-check nodes for restart", time.Minute*5, err), nil
		}
		if len(nextNodes) == 0 {
			return ctrl.Result{RequeueAfter: time.Hour}, nil
		}
		targetVersion := talosUpgrade.Spec.Talos.Version
		cycles := talosUpgrade.Status.CompletionCycles
		if cycles >= upgradeaudit.MaxCompletionCycles {
			message := fmt.Sprintf(
				"Node(s) never converged to %s after %d completion cycles; add the %s annotation or bump the spec to retry",
				targetVersion, cycles, constants.ResetAnnotation,
			)
			logger.Info("Completion cycles exhausted, marking upgrade Failed", "targetVersion", targetVersion, "cycles", cycles)
			if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseFailed, message); err != nil {
				return ctrl.Result{}, fmt.Errorf("set Failed phase after exhausting completion cycles: %w", err)
			}
			return ctrl.Result{RequeueAfter: time.Hour}, nil
		}
		logger.Info("Node detected requiring upgrade after completion, restarting campaign", "node", nextNodes[0], "cycle", cycles+1)
		if err := r.setPhaseWithUpdates(ctx, talosUpgrade, tupprv1alpha1.JobPhasePending, "", nil, "New node detected, restarting upgrade", map[string]any{
			statusCompletedNodes:   []string{},
			statusFailedNodes:      []tupprv1alpha1.NodeUpgradeStatus{},
			statusRebootingNodes:   []tupprv1alpha1.NodeRebootStatus{},
			statusPreHookIndex:     0,
			statusPostHookIndex:    0,
			statusPreHookFailed:    false,
			statusPreBatchHooks:    nil,
			statusPrePulledNodes:   []tupprv1alpha1.PrePulledNode{},
			statusPrePullFailure:   nil,
			statusCompletionCycles: cycles + 1,
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("re-enter Pending after completion: %w", err)
		}
		talosUpgrade.Status.CompletionCycles = cycles + 1
		resetRunProgress(&talosUpgrade.Status)
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	if result, handled, err := r.processHookPhases(ctx, talosUpgrade); handled {
		return result, err
	}

	if !talosUpgrade.Status.Phase.IsActive() {
		if result, done := r.checkMaintenanceWindow(ctx, talosUpgrade); done {
			return result, nil
		}
		if result, done := r.checkCoordination(ctx, talosUpgrade); done {
			return result, nil
		}
	}

	if activeJobs, activeNodes, err := r.findActiveJobs(ctx, talosUpgrade); err != nil {
		return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonFindActiveJobs, "list active upgrade jobs", time.Minute, err), nil
	} else if len(activeJobs) > 0 {
		logger.V(1).Info("Found active jobs, handling batch status", "nodeCount", len(activeJobs), "nodes", activeNodes)
		return r.handleBatchJobStatus(ctx, talosUpgrade, activeJobs, activeNodes)
	}

	// The upgrade Job may have been garbage collected while a node was still
	// rebooting; resume the reboot wait from status before anything else.
	if result, done := r.processRebootingNodes(ctx, talosUpgrade); done {
		return result, nil
	}

	if len(talosUpgrade.Status.FailedNodes) > 0 {
		logger.Info("Upgrade stopped due to failed nodes",
			"failed", len(talosUpgrade.Status.FailedNodes))
		return r.transitionToFinalize(ctx, talosUpgrade)
	}

	return r.processNextBatch(ctx, talosUpgrade)
}

// processHookPhases drives the run-level pre- and post-hook phases, and
// finalizes a run whose pre-batch hook has failed. handled=false means the
// caller carries on with the batch path.
func (r *Reconciler) processHookPhases(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, bool, error) {
	// Checked outside the PreHook phase too: a suspend can land between
	// recording the failure and finalizing.
	if state := talosUpgrade.Status.PreBatchHooks; state != nil && state.Failed && talosUpgrade.Status.Phase != tupprv1alpha1.JobPhasePostHook {
		result, err := r.transitionToFinalize(ctx, talosUpgrade)
		return result, true, err
	}

	if result, observed, err := r.observeRunningPreBatchHook(ctx, talosUpgrade); observed {
		return result, true, err
	}

	// Ahead of the batch gates, which would otherwise hold the run (and its
	// post-hooks) past the hook's timeout.
	if deadline, ok := preBatchDeadline(talosUpgrade); ok && talosUpgrade.Status.Phase != tupprv1alpha1.JobPhasePostHook && !r.Now.Now().Before(deadline) {
		result, err := r.failPreBatchHooks(ctx, talosUpgrade, talosUpgrade.Status.PreBatchHooks.DeepCopy())
		return result, true, err
	}

	switch talosUpgrade.Status.Phase {
	case tupprv1alpha1.JobPhasePreHook:
		// Pre-batch hooks run in PreHook too, driven from processNextBatch.
		if talosUpgrade.Status.PreBatchHooks != nil {
			break
		}
		result, done, err := r.processHookPhase(ctx, talosUpgrade, hookPhasePre)
		if err != nil || !done {
			return result, true, err
		}
		if talosUpgrade.Status.PreHookFailed {
			log.FromContext(ctx).Info("Pre-hook failed; running post-hooks then failing")
			result, err := r.transitionToFinalize(ctx, talosUpgrade)
			return result, true, err
		}
		if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhasePending, "Pre-hooks complete; starting upgrade"); err != nil {
			log.FromContext(ctx).Error(err, "Failed to advance phase past pre-hooks")
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, true, nil
	case tupprv1alpha1.JobPhasePostHook:
		result, done, err := r.processHookPhase(ctx, talosUpgrade, hookPhasePost)
		if err != nil || !done {
			return result, true, err
		}
		result, err = r.completeUpgrade(ctx, talosUpgrade)
		return result, true, err
	}
	return ctrl.Result{}, false, nil
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
		return ctrl.Result{}, true, fmt.Errorf("enter PreHook phase: %w", err)
	}
	return ctrl.Result{RequeueAfter: time.Second * 5}, true, nil
}

// transitionToFinalize routes to PostHook (if configured and not yet run) or
// directly to the terminal phase via completeUpgrade.
func (r *Reconciler) transitionToFinalize(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	if hasPostHooks(talosUpgrade) && talosUpgrade.Status.PostHookIndex < len(talosUpgrade.Spec.Hooks.Post) {
		if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhasePostHook, "Running post-hooks"); err != nil {
			return ctrl.Result{}, fmt.Errorf("transition to PostHook: %w", err)
		}
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}
	return r.completeUpgrade(ctx, talosUpgrade)
}

func hasPreHooks(tu *tupprv1alpha1.TalosUpgrade) bool {
	return tu.Spec.Hooks != nil && len(tu.Spec.Hooks.Pre) > 0
}

// interBatchHealthChecks returns the health checks to run before the next
// batch. Once the pre-hooks have run (the run comes back through here before
// its first batch, not only between batches), checks marked skipAfterPreHooks
// are dropped: a pre-hook can invalidate them for the rest of the upgrade
// window (e.g. Ceph reports HEALTH_WARN while `noout` is set), so re-checking
// would stall the run. skip reports that nothing is left to run, which also
// covers a hooked run with no health checks at all.
func interBatchHealthChecks(tu *tupprv1alpha1.TalosUpgrade) (checks []tupprv1alpha1.HealthCheckSpec, skip bool) {
	afterPreHooks := hasPreHooks(tu) && tu.Status.PreHookIndex >= len(tu.Spec.Hooks.Pre)
	if !afterPreHooks {
		return tu.Spec.HealthChecks, false
	}
	for _, hc := range tu.Spec.HealthChecks {
		if !hc.SkipAfterPreHooks {
			checks = append(checks, hc)
		}
	}
	return checks, len(checks) == 0
}

// waitForHealthChecks evaluates checks once and reports waiting=true while
// they fail. The start of the failure streak, kept in status, spaces out the
// re-evaluations and times how long the checks took to pass.
func (r *Reconciler) waitForHealthChecks(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, checks []tupprv1alpha1.HealthCheckSpec) (ctrl.Result, bool) {
	logger := log.FromContext(ctx)
	now := r.Now.Now()
	since := talosUpgrade.Status.HealthChecksFailingSince
	checkErr := r.runHealthChecks(ctx, talosUpgrade, checks)

	if checkErr != nil {
		var extra map[string]any
		if since == nil {
			since = &metav1.Time{Time: now}
			extra = map[string]any{statusHealthChecksFailingSince: since}
		}
		if err := r.setPhaseWithUpdates(ctx, talosUpgrade, tupprv1alpha1.JobPhaseHealthChecking, "", nil, healthCheckWaitMessage(checkErr), extra); err != nil {
			logger.Error(err, "Failed to update phase for health check")
		} else {
			talosUpgrade.Status.HealthChecksFailingSince = since
		}
		logger.V(1).Info("Waiting for health checks to pass", "error", checkErr.Error())
		return ctrl.Result{RequeueAfter: healthcheck.RetryAfter(now.Sub(since.Time))}, true
	}

	var waited time.Duration
	var extra map[string]any
	if since != nil {
		waited = now.Sub(since.Time)
		extra = map[string]any{statusHealthChecksFailingSince: nil}
	}
	if len(checks) > 0 {
		r.MetricsReporter.RecordHealthCheckDuration(metrics.UpgradeTypeTalos, talosUpgrade.Name, waited.Seconds())
	}
	if err := r.setPhaseWithUpdates(ctx, talosUpgrade, tupprv1alpha1.JobPhaseHealthChecking, "", nil, "Running health checks", extra); err != nil {
		logger.Error(err, "Failed to update phase for health check")
	} else {
		talosUpgrade.Status.HealthChecksFailingSince = nil
	}
	return ctrl.Result{}, false
}

func hasPostHooks(tu *tupprv1alpha1.TalosUpgrade) bool {
	return tu.Spec.Hooks != nil && len(tu.Spec.Hooks.Post) > 0
}

func (r *Reconciler) checkMaintenanceWindow(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, bool) {
	logger := log.FromContext(ctx)

	maintenanceRes, err := maintenance.CheckWindow(talosUpgrade.Spec.Maintenance, r.Now.Now())
	if err != nil {
		return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonCheckMaintenanceWindow, "evaluate maintenance window", 30*time.Second, err), true
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
		return ctrl.Result{RequeueAfter: requeueAfter}, true
	}
	r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeTalos, talosUpgrade.Name, true, nil)
	return ctrl.Result{}, false
}

func (r *Reconciler) checkCoordination(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, bool) {
	logger := log.FromContext(ctx)

	blocked, message, err := coordination.IsAnotherUpgradeActive(ctx, r.Client, talosUpgrade.Name, coordination.UpgradeTypeTalos)
	if err != nil {
		return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonCheckCoordination, "check for other active upgrades", time.Minute, err), true
	}
	if blocked {
		logger.V(1).Info("Waiting for another upgrade to complete", "reason", message)
		if err := r.setPendingWithReason(ctx, talosUpgrade, upgradeaudit.ReasonWaitingForOtherUpgrade, message); err != nil {
			logger.Error(err, "Failed to update phase for coordination wait")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 2}, true
	}
	return ctrl.Result{}, false
}

func (r *Reconciler) completeUpgrade(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	completedCount := len(talosUpgrade.Status.CompletedNodes)
	failedCount := len(talosUpgrade.Status.FailedNodes)

	var phase tupprv1alpha1.JobPhase
	var message string
	switch {
	case talosUpgrade.Status.PreHookFailed:
		phase = tupprv1alpha1.JobPhaseFailed
		message = "Pre-upgrade hook failed; upgrade did not run"
	case talosUpgrade.Status.PreBatchHooks != nil && talosUpgrade.Status.PreBatchHooks.Failed:
		phase = tupprv1alpha1.JobPhaseFailed
		message = fmt.Sprintf("Pre-batch hook failed for node(s) %s; stopped after %d successful, %d failed",
			strings.Join(talosUpgrade.Status.PreBatchHooks.Nodes, ", "), completedCount, failedCount)
	case failedCount > 0:
		phase = tupprv1alpha1.JobPhaseFailed
		message = fmt.Sprintf("Completed with failures: %d successful, %d failed", completedCount, failedCount)
	default:
		phase = tupprv1alpha1.JobPhaseCompleted
		message = fmt.Sprintf("Successfully upgraded %d nodes", completedCount)
	}

	if err := r.setPhase(ctx, talosUpgrade, phase, message); err != nil {
		return ctrl.Result{}, fmt.Errorf("update completion phase: %w", err)
	}
	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

func (r *Reconciler) processNextBatch(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	maintenanceRes, err := maintenance.CheckWindow(talosUpgrade.Spec.Maintenance, r.Now.Now())
	if err != nil {
		return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonCheckMaintenanceWindow, "evaluate maintenance window", 30*time.Second, err), nil
	}
	if !maintenanceRes.Allowed {
		return r.parkForClosedWindow(ctx, talosUpgrade, maintenanceRes), nil
	}
	r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeTalos, talosUpgrade.Name, true, nil)

	ctx = context.WithValue(ctx, metrics.ContextKeyUpgradeType, metrics.UpgradeTypeTalos)
	ctx = context.WithValue(ctx, metrics.ContextKeyUpgradeName, talosUpgrade.Name)

	// All pending nodes are collected (not just this batch's) so the run-start
	// pre-pull can warm the whole fleet before the first node is cordoned.
	pendingNodes, unreachableNodes, err := r.findNextNodes(ctx, talosUpgrade, math.MaxInt)
	if err != nil {
		return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonFindNextNodes, "find candidate nodes", time.Minute, err), nil
	}

	// An unreachable node pauses the upgrade rather than failing the reconcile:
	// rebooting more nodes while one is already down risks quorum loss, and a
	// hard error would log a stack trace on every requeue.
	if len(unreachableNodes) > 0 {
		logger.Info("Waiting for unreachable node(s) before continuing upgrade", "nodes", unreachableNodes)
		message := fmt.Sprintf("Waiting for unreachable node(s): %s", strings.Join(unreachableNodes, ", "))
		if err := r.setPendingWithReason(ctx, talosUpgrade, upgradeaudit.ReasonNodeUnreachable, message); err != nil {
			logger.Error(err, "Failed to update phase while waiting for unreachable nodes")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if len(pendingNodes) == 0 {
		return r.finishRun(ctx, talosUpgrade)
	}

	if result, done := r.prePullInstallerImages(ctx, talosUpgrade, pendingNodes); done {
		return result, nil
	}

	nextNodes, err := r.nextBatch(ctx, pendingNodes, getParallelism(talosUpgrade.Spec))
	if err != nil {
		return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonFindNextNodes, "select next batch", time.Minute, err), nil
	}
	nextNodes = resumeStartedBatch(talosUpgrade, pendingNodes, nextNodes)

	if result, done := r.waitForControlPlaneSlot(ctx, talosUpgrade, nextNodes, pendingNodes); done {
		return result, nil
	}

	batch, result, done := r.buildBatchImages(ctx, talosUpgrade, nextNodes)
	if done {
		return result, nil
	}

	if result, done := r.waitForClusterHealth(ctx, talosUpgrade); done {
		return result, nil
	}

	now := r.Now.Now()
	preBatch, rerunReason := preBatchStageFor(talosUpgrade, nextNodes, now, func() bool {
		return r.preBatchNodesTouched(ctx, talosUpgrade)
	})

	checks, skipHealthCheck := interBatchHealthChecks(talosUpgrade)
	if preBatch.skipsHealthChecks() {
		skipHealthCheck = true
	}

	if !skipHealthCheck {
		if result, waiting := r.waitForHealthChecks(ctx, talosUpgrade, checks); waiting {
			return result, nil
		}
	}

	if result, entered, err := r.maybeEnterPreHook(ctx, talosUpgrade); entered {
		return result, err
	}

	if result, proceed, err := r.gatePreBatch(ctx, talosUpgrade, nextNodes, preBatch, rerunReason, now); !proceed {
		return result, err
	}

	logger.Info("Starting batch upgrade", "nodes", nextNodes, "batchSize", len(batch))

	// Drain all nodes in batch before creating any jobs
	if r.tupprOwnsDrain(ctx, talosUpgrade) {
		if err := r.setPhaseWithNodes(ctx, talosUpgrade, tupprv1alpha1.JobPhaseDraining, nextNodes, fmt.Sprintf("Draining %d nodes", len(nextNodes))); err != nil {
			return ctrl.Result{}, fmt.Errorf("update phase for draining: %w", err)
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
				r.resetPreBatchHooks(ctx, talosUpgrade)
				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
			drainedNodes = append(drainedNodes, ni.nodeName)
			logger.V(1).Info("Node drained successfully", "node", ni.nodeName)
		}
	}

	// Create jobs for all nodes in batch
	for _, ni := range batch {
		if _, err := r.createJob(ctx, talosUpgrade, ni.nodeName, ni.image); err != nil {
			return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonCreateJob, "create upgrade job", time.Minute, err, "node", ni.nodeName), nil
		}

		if err := r.addNodeUpgradingLabel(ctx, ni.nodeName); err != nil {
			logger.Error(err, "Failed to add upgrading label to node", "node", ni.nodeName)
		}
	}

	// The next batch runs its own pre-batch hooks.
	var extra map[string]any
	if talosUpgrade.Status.PreBatchHooks != nil {
		extra = map[string]any{statusPreBatchHooks: nil}
	}
	if err := r.setPhaseWithUpdates(ctx, talosUpgrade, tupprv1alpha1.JobPhaseUpgrading, "", nextNodes, fmt.Sprintf("Upgrading %d nodes", len(nextNodes)), extra); err != nil {
		return ctrl.Result{}, fmt.Errorf("update phase for batch upgrade: %w", err)
	}
	talosUpgrade.Status.PreBatchHooks = nil
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// nextBatch picks the next batch from pending, keeping its order: the first
// pending node alone when it is a control-plane node, otherwise up to
// parallelism pending workers, skipping control-plane nodes until they reach
// the front. Each control-plane reboot takes an etcd member offline, and
// Talos 1.13+ (LifecycleService upgrades) does not stop two at once; only the
// legacy pre-1.13 upgrade API took an etcd lock.
func (r *Reconciler) nextBatch(ctx context.Context, pending []string, parallelism int) ([]string, error) {
	var batch []string
	for _, name := range pending {
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: name}, node); err != nil {
			return nil, fmt.Errorf("failed to get node %s: %w", name, err)
		}
		_, isCP := node.Labels[controlPlaneLabel]
		if isCP {
			if len(batch) == 0 {
				return []string{name}, nil
			}
			continue
		}
		batch = append(batch, name)
		if len(batch) == parallelism {
			break
		}
	}
	return batch, nil
}

// controlPlaneUpgradeInFlight returns a control-plane node outside batch whose
// upgrade is still in flight, from any TalosUpgrade, when batch holds a
// control-plane node: an unfinished upgrade Job targets it, or it still
// carries the upgrading label (set at Job creation, removed once the outcome
// is recorded). Plans coordinate only through their phases, and a running plan
// drops to Pending between batches and on errors, so two plans selecting
// different control-plane nodes could otherwise overlap. Reads bypass the
// cache so a Job another plan created a moment ago is seen.
//
// A label on a node this run selects but no longer has pending does not hold
// the batch: the caller runs with no Job or reboot of its own outstanding, so
// the node is at the target and the label is left from a finished upgrade
// (the Job expired before the outcome was recorded, or removing the label
// failed). The cluster health gate still waits for the node to rejoin. The
// label stays: it is the record that makes finishRun wait for health, and it
// may belong to an overlapping plan.
func (r *Reconciler) controlPlaneUpgradeInFlight(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, batch, pending []string) (string, error) {
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}

	cpNodes := &corev1.NodeList{}
	if err := reader.List(ctx, cpNodes, client.HasLabels{controlPlaneLabel}); err != nil {
		return "", fmt.Errorf("failed to list control-plane nodes: %w", err)
	}
	isCP := make(map[string]bool, len(cpNodes.Items))
	for i := range cpNodes.Items {
		isCP[cpNodes.Items[i].Name] = true
	}
	if !slices.ContainsFunc(batch, func(name string) bool { return isCP[name] }) {
		return "", nil
	}
	// nextBatch read the roles from the cache. A control-plane node sharing a
	// batch means the cache is behind; the next pass rebuilds the batch.
	if len(batch) > 1 {
		return "", fmt.Errorf("batch %s holds a control-plane node with other nodes; node roles changed since it was selected", strings.Join(batch, ", "))
	}

	jobList := &batchv1.JobList{}
	if err := reader.List(ctx, jobList,
		client.InNamespace(r.ControllerNamespace),
		client.MatchingLabels{appLabelKey: talosUpgradeAppName},
	); err != nil {
		return "", fmt.Errorf("failed to list upgrade jobs: %w", err)
	}
	for i := range jobList.Items {
		job := &jobList.Items[i]
		target := job.Labels[targetNodeLabelKey]
		if jobs.IsTerminal(job) || !isCP[target] || slices.Contains(batch, target) {
			continue
		}
		return target, nil
	}

	var selected map[string]bool
	for i := range cpNodes.Items {
		node := &cpNodes.Items[i]
		if node.Labels[constants.NodeUpgradingLabel] == "" || slices.Contains(batch, node.Name) {
			continue
		}
		if selected == nil {
			nodes, err := nodeutil.ListSelectedNodes(ctx, r.Client, talosUpgrade.Spec.NodeSelector)
			if err != nil {
				return "", fmt.Errorf("failed to list selected nodes: %w", err)
			}
			selected = make(map[string]bool, len(nodes))
			for j := range nodes {
				selected[nodes[j].Name] = true
			}
		}
		if selected[node.Name] && !slices.Contains(pending, node.Name) {
			continue
		}
		return node.Name, nil
	}
	return "", nil
}

// waitForControlPlaneSlot parks the run in Pending while
// controlPlaneUpgradeInFlight reports another control-plane upgrade. Returns
// done=true when the caller should return the result.
func (r *Reconciler) waitForControlPlaneSlot(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, batch, pending []string) (ctrl.Result, bool) {
	logger := log.FromContext(ctx)

	busy, err := r.controlPlaneUpgradeInFlight(ctx, talosUpgrade, batch, pending)
	if err != nil {
		return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonCheckCoordination, "check control-plane upgrades in flight", time.Minute, err), true
	}
	if busy == "" {
		return ctrl.Result{}, false
	}
	message := fmt.Sprintf("Waiting for control-plane node %s to finish upgrading before upgrading %s", busy, strings.Join(batch, ", "))
	logger.Info("Waiting for another control-plane upgrade", "busyNode", busy, "nodes", batch)
	if err := r.setPendingWithReason(ctx, talosUpgrade, upgradeaudit.ReasonWaitingForOtherUpgrade, message); err != nil {
		logger.Error(err, "Failed to update phase while waiting for control-plane upgrade")
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, true
}

// finishRun records nodes already at the target version and moves the run to
// its post-hooks or terminal phase once no node is pending.
func (r *Reconciler) finishRun(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Read before recordOutOfBandCompletedNodes, which also lists nodes
	// this run never touched.
	upgradedNodes := len(talosUpgrade.Status.CompletedNodes) > 0
	resumed, err := r.recordOutOfBandCompletedNodes(ctx, talosUpgrade)
	if err != nil {
		logger.Error(err, "Failed to record out-of-band upgraded nodes")
		// Unknown whether this run upgraded anything; waiting is the safe side.
		resumed = true
	}
	// The last batch has no next batch to hold it back; without this the
	// run completes (unblocking a queued KubernetesUpgrade and running
	// post-hooks) while that batch is still rejoining.
	if upgradedNodes || resumed {
		if result, done := r.waitForClusterHealth(ctx, talosUpgrade); done {
			return result, nil
		}
	}
	return r.transitionToFinalize(ctx, talosUpgrade)
}

type nodeImage struct {
	nodeName string
	image    string
}

// buildBatchImages resolves each batch node's target image and verifies the
// registry serves it, before HealthChecking: a stuck external registry must
// not flip the phase on every reconcile. Returns done=true when the caller
// should return the result.
func (r *Reconciler) buildBatchImages(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nextNodes []string) ([]nodeImage, ctrl.Result, bool) {
	logger := log.FromContext(ctx)

	var batch []nodeImage
	for _, nodeName := range nextNodes {
		targetImage, err := r.buildTalosUpgradeImage(ctx, talosUpgrade, nodeName)
		if err != nil {
			return nil, r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonBuildTargetImage, "build target image", time.Minute, err, "node", nodeName), true
		}

		logger.V(1).Info("Verifying target image availability", "node", nodeName, "targetImage", targetImage)
		if err := r.ImageChecker.Check(ctx, targetImage); err != nil {
			logger.V(1).Info("Waiting for target image to become available", "node", nodeName, "targetImage", targetImage, "error", err.Error())
			message := fmt.Sprintf("Waiting for image availability for node %s: %s", nodeName, err.Error())
			if err := r.setPendingWithReason(ctx, talosUpgrade, upgradeaudit.ReasonWaitingForImage, message); err != nil {
				logger.Error(err, "Failed to update phase while waiting for image")
			}
			return nil, ctrl.Result{RequeueAfter: 1 * time.Minute}, true
		}

		batch = append(batch, nodeImage{nodeName: nodeName, image: targetImage})
	}
	return batch, ctrl.Result{}, false
}

// processRebootingNodes resumes the post-upgrade reboot wait for nodes whose
// upgrade Job no longer exists (e.g. garbage collected by its TTL while the
// node was still down). Each tracked node is verified against the Talos API:
// ready nodes are completed, nodes past their deadline are marked failed, and
// the rest keep waiting. A verification error only counts against the
// deadline, as in processTerminalJob: mid-reboot the Talos API answers with
// errors that are not transient by gRPC code (1.14's install flow has a window
// where apid returns PermissionDenied) and with the pre-upgrade version.
// Returns done=true when the caller should return the result.
func (r *Reconciler) processRebootingNodes(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, bool) {
	logger := log.FromContext(ctx)

	var waiting []string
	for _, entry := range slices.Clone(talosUpgrade.Status.RebootingNodes) {
		nodeName := entry.NodeName

		// Drop entries already resolved (e.g. a crash between recording the
		// outcome and clearing the tracking entry).
		if slices.Contains(talosUpgrade.Status.CompletedNodes, nodeName) ||
			slices.ContainsFunc(talosUpgrade.Status.FailedNodes, func(fn tupprv1alpha1.NodeUpgradeStatus) bool {
				return fn.NodeName == nodeName
			}) {
			if err := r.clearRebootTracking(ctx, talosUpgrade, nodeName); err != nil {
				logger.Error(err, "Failed to clear stale reboot tracking", "node", nodeName)
			}
			continue
		}

		isReady, verifyErr := r.verifyNodeUpgrade(ctx, talosUpgrade, nodeName)
		if verifyErr != nil {
			logger.V(1).Info("Could not verify node yet, waiting for it to settle",
				"node", nodeName, "error", verifyErr.Error())
		}
		switch {
		case isReady:
			if err := r.completeNodeUpgrade(ctx, talosUpgrade, nodeName); err != nil {
				logger.Error(err, "Failed to record completed node", "node", nodeName)
			}
		case r.rebootDeadlineExpired(talosUpgrade, nodeName):
			message := fmt.Sprintf("Node did not become ready within %s after upgrade", nodeUpgradeTimeout(talosUpgrade))
			if verifyErr != nil {
				message += ": " + verifyErr.Error()
			}
			if err := r.processSingleJobFailure(ctx, talosUpgrade, nodeName, message); err != nil {
				logger.Error(err, "Failed to record failed node", "node", nodeName)
			}
		default:
			waiting = append(waiting, nodeName)
		}
	}

	if len(waiting) == 0 {
		return ctrl.Result{}, false
	}

	message := fmt.Sprintf("Waiting for %d node(s) to finish rebooting", len(waiting))
	if err := r.setPhaseWithNodes(ctx, talosUpgrade, tupprv1alpha1.JobPhaseRebooting, waiting, message); err != nil {
		logger.Error(err, "Failed to update phase for rebooting")
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, true
}

// parkForClosedWindow parks a run whose maintenance window closed between
// batches. MaintenanceWindow does not block a KubernetesUpgrade, so a run that
// already upgraded nodes stays in HealthChecking until they recover.
func (r *Reconciler) parkForClosedWindow(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, maintenanceRes *maintenance.WindowResult) ctrl.Result {
	if len(talosUpgrade.Status.CompletedNodes) > 0 {
		if result, done := r.waitForClusterHealth(ctx, talosUpgrade); done {
			return result
		}
	}
	nextTimestamp := maintenanceRes.NextWindowStart.Unix()
	r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeTalos, talosUpgrade.Name, false, &nextTimestamp)
	message := fmt.Sprintf("Maintenance window closed between nodes, waiting (next: %s)", maintenanceRes.NextWindowStart.Format(time.RFC3339))
	if err := r.setPhaseWithUpdates(ctx, talosUpgrade, tupprv1alpha1.JobPhaseMaintenanceWindow, "", nil, message, map[string]any{
		"nextMaintenanceWindow": metav1.NewTime(*maintenanceRes.NextWindowStart),
	}); err != nil {
		log.FromContext(ctx).Error(err, "Failed to update status for maintenance window")
	}
	return ctrl.Result{RequeueAfter: maintenanceRes.RequeueAfter(r.Now.Now())}
}

// recordOutOfBandCompletedNodes adds selected nodes already at the target
// version to CompletedNodes. resumed reports whether any of them still carried
// the upgrading label: tuppr started that node's upgrade but its Job was
// garbage collected before the outcome was recorded (e.g. the controller was
// down), so the node counts as upgraded by this run, not out of band.
func (r *Reconciler) recordOutOfBandCompletedNodes(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (resumed bool, err error) {
	logger := log.FromContext(ctx)

	nodes, err := r.getSortedNodes(ctx, talosUpgrade.Spec.NodeSelector)
	if err != nil {
		return false, err
	}

	crdTargetVersion := talosUpgrade.Spec.Talos.Version
	var added, labeled []string

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
			if talos.IsTransientError(err) {
				logger.V(1).Info("Node unreachable, skipping out-of-band check", "node", node.Name, "error", err.Error())
				continue
			}
			return resumed, fmt.Errorf("check node %s: %w", node.Name, err)
		}
		if needsUpgrade {
			continue
		}
		// Job may have been GC'd before the controller reconciled post-reboot; the
		// node is at target but may still be cordoned and labeled upgrading.
		r.ensureNodeUncordoned(ctx, talosUpgrade, node.Name)
		if node.Labels[constants.NodeUpgradingLabel] != "" {
			resumed = true
			labeled = append(labeled, node.Name)
		}
		added = append(added, node.Name)
	}

	if len(added) == 0 {
		return resumed, nil
	}

	logger.Info("Recording nodes upgraded out of band", "nodes", added)
	if r.Recorder != nil {
		r.Recorder.Eventf(talosUpgrade, corev1.EventTypeNormal, "OutOfBandUpgrade",
			"Recorded %d node(s) already at target version %s: %v",
			len(added), crdTargetVersion, added)
	}
	talosUpgrade.Status.CompletedNodes = append(talosUpgrade.Status.CompletedNodes, added...)
	if err := r.updateStatus(ctx, talosUpgrade, map[string]any{
		statusCompletedNodes: talosUpgrade.Status.CompletedNodes,
	}); err != nil {
		return resumed, err
	}
	// Until CompletedNodes is saved, the label is the only record that this
	// run upgraded the node, which is what makes completion wait for health.
	for _, name := range labeled {
		if err := r.removeNodeUpgradingLabel(ctx, name); err != nil {
			logger.Error(err, "Failed to remove upgrading label from node", "node", name)
		}
	}
	return resumed, nil
}

// findNextNodes returns up to `count` node names that need upgrading, sorted
// alphabetically, plus the nodes whose version could not be checked because
// the Talos API is unreachable. Unreachable nodes are an expected condition
// (e.g. a node stuck mid-reboot), not a reconcile error.
func (r *Reconciler) findNextNodes(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, count int) ([]string, []string, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Finding next nodes to upgrade", "count", count)

	nodes, err := r.getSortedNodes(ctx, talosUpgrade.Spec.NodeSelector)
	if err != nil {
		return nil, nil, err
	}

	crdTargetVersion := talosUpgrade.Spec.Talos.Version
	var result []string
	var unreachable []string

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
			if talos.IsTransientError(err) {
				logger.V(1).Info("Node unreachable, cannot check version", "node", node.Name, "error", err.Error())
				unreachable = append(unreachable, node.Name)
				continue
			}
			return nil, nil, fmt.Errorf("failed to check node %s: %w", node.Name, err)
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

	if len(result) == 0 && len(unreachable) == 0 {
		logger.V(1).Info("All nodes are up to date")
	}
	return result, unreachable, nil
}

func (r *Reconciler) getSortedNodes(ctx context.Context, nodeSelector *metav1.LabelSelector) ([]corev1.Node, error) {
	nodes, err := nodeutil.ListSelectedNodes(ctx, r.Client, nodeSelector)
	if err != nil {
		return nil, err
	}

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

	targetVersion := nodeutil.TargetTalosVersion(node, crdTargetVersion)

	if currentVersion != targetVersion {
		logger.V(1).Info("Node version mismatch detected",
			"node", node.Name,
			"currentVersion", currentVersion,
			"targetVersion", targetVersion)
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
		log.FromContext(ctx).Error(err, "Failed to count nodes; assuming multi-node")
		return false
	}
	return count == 1
}

// tupprOwnsDrain reports whether tuppr should drain the node itself before the
// upgrade job (rather than delegating to Talos). True when the deprecated Drain
// spec is enabled, or when Policy.WaitForVolumeDetach is set on a multi-node
// cluster. WaitForVolumeDetach alone never drains a single node (pods have
// nowhere to go); the Drain spec does, and evicted pods stay Pending until the
// node is uncordoned after the upgrade.
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
			logger.Info("Volumes did not detach before timeout, proceeding with upgrade", "node", nodeName, "error", err.Error())
		}
	}

	return nil
}

// Matches the canonical generic installer and any mirror that keeps the "/siderolabs/installer" suffix.
func looksLikeGenericInstaller(repo string) bool {
	return repo == constants.GenericInstallerRepo || strings.HasSuffix(repo, "/siderolabs/installer")
}

func factoryInstallerRepo(platform string) (string, bool) {
	if platform == platforms.MetalPlatform().Name {
		return platform + "-installer", true
	}
	for _, supported := range platforms.CloudPlatforms() {
		if platform == supported.Name {
			// Equinix Metal's platform name is camel-cased, but its Factory
			// installer uses the generic repository.
			if platform == "equinixMetal" {
				return "installer", true
			}
			return platform + "-installer", true
		}
	}
	return "", false
}

// factoryImage builds the Image Factory installer ref for the node's platform.
// ghcr.io/siderolabs/installer stopped publishing at Talos 1.14, so a node still
// on it can only move forward through the Factory.
func (r *Reconciler) factoryImage(ctx context.Context, nodeIP, schematic, version string) (string, error) {
	platform, err := r.TalosClient.GetNodePlatform(ctx, nodeIP)
	if err != nil {
		return "", fmt.Errorf("failed to read platform: %w", err)
	}
	installerRepo, ok := factoryInstallerRepo(platform)
	if !ok {
		return "", fmt.Errorf("platform %q has no Image Factory installer", platform)
	}
	return fmt.Sprintf("factory.talos.dev/%s/%s:%s", installerRepo, schematic, version), nil
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

	targetVersion := nodeutil.TargetTalosVersion(node, talosUpgrade.Spec.Talos.Version)

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
		if repo == constants.GenericInstallerRepo {
			if targetImage, err := r.factoryImage(ctx, nodeIP, ext.Schematic, targetVersion); err == nil {
				logger.V(1).Info("Built target image from runtime metadata", "node", nodeName, "targetImage", targetImage, "schematic", ext.Schematic)
				return targetImage, nil
			}
		}
		// Talos 1.14+ fills an unset install image with the empty schematic,
		// which carries no extensions and so says nothing about the node.
		if base, ok := strings.CutSuffix(repo, "/"+constants.DefaultSchematic); ok {
			targetImage := fmt.Sprintf("%s/%s:%s", base, ext.Schematic, targetVersion)
			logger.V(1).Info("Built target image from runtime metadata", "node", nodeName, "targetImage", targetImage, "schematic", ext.Schematic)
			return targetImage, nil
		}
		return "", fmt.Errorf(
			"node %s: install image %q does not embed the runtime schematic %s; reinstalling would wipe extensions. Fix .machine.install.image to a factory image, or set annotation %s",
			nodeName, currentImage, ext.Schematic, constants.FactoryURLAnnotation)
	}
	if ext.Schematic == "" && len(ext.Extensions) > 0 && looksLikeGenericInstaller(repo) {
		return "", fmt.Errorf(
			"node %s: install image %q has no schematic but the node has extensions=%v; reinstalling would wipe them. Set annotation %s with %s to upgrade to a factory image",
			nodeName, currentImage, ext.Extensions, constants.FactoryURLAnnotation, constants.SchematicAnnotation)
	}
	if repo == constants.GenericInstallerRepo && parseTalosctlVersion(targetVersion).AtLeast(1, 14) {
		targetImage, err := r.factoryImage(ctx, nodeIP, constants.DefaultSchematic, targetVersion)
		if err != nil {
			return "", fmt.Errorf("node %s: cannot move the generic installer to the Image Factory: %w", nodeName, err)
		}
		logger.V(1).Info("Built target image from the default schematic", "node", nodeName, "targetImage", targetImage)
		return targetImage, nil
	}

	targetImage := fmt.Sprintf("%s:%s", repo, targetVersion)
	logger.V(1).Info("Built target image", "node", nodeName, "targetImage", targetImage, "targetVersion", targetVersion)
	return targetImage, nil
}

type nodeVersionMismatchError struct {
	nodeName       string
	currentVersion string
	targetVersion  string
}

func (e *nodeVersionMismatchError) Error() string {
	return fmt.Sprintf("node %s version mismatch: current=%s, target=%s", e.nodeName, e.currentVersion, e.targetVersion)
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

	targetVersion := nodeutil.TargetTalosVersion(node, talosUpgrade.Spec.Talos.Version)

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
		return false, &nodeVersionMismatchError{
			nodeName:       nodeName,
			currentVersion: currentVersion,
			targetVersion:  targetVersion,
		}
	}

	logger.V(1).Info("Node upgrade verification successful",
		"node", nodeName,
		"currentVersion", currentVersion)
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
