package talosupgrade

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/jobs"
)

const (
	defaultPreBatchRetryTimeout  = 30 * time.Minute
	defaultPreBatchRetryInterval = 30 * time.Second

	// preBatchHookValidity is how long succeeded pre-batch hooks still clear
	// their batch to start. The batch normally starts on the next reconcile;
	// a run held up for longer re-runs the hooks against the current state.
	preBatchHookValidity = 5 * time.Minute
)

// preBatchStage is where a batch stands with its pre-batch hooks.
type preBatchStage int

const (
	// preBatchNone: the run has no pre-batch hooks.
	preBatchNone preBatchStage = iota
	// preBatchStart: no hook has run for this batch yet, or an earlier
	// result no longer covers it.
	preBatchStart
	// preBatchReady: the hook at Index is due to start, or to start again
	// under its recorded Job name because that Job is missing.
	preBatchReady
	// preBatchWaiting: the hook at Index failed and waits for RetryAt.
	preBatchWaiting
	// preBatchExpired: the hook at Index has kept failing past the retry
	// timeout; the run fails without another attempt.
	preBatchExpired
	// preBatchCleared: every hook succeeded recently, or the batch has
	// already started.
	preBatchCleared
)

// skipsHealthChecks reports whether the health checks are left out this
// pass. They gate the start of every hook attempt, not a retry still waiting
// or a run about to fail. Once the hooks have succeeded they aren't run again
// before the batch starts: a hook may change what they check for its batch
// (e.g. moving a database primary away). A running hook Job never gets here:
// observeRunningPreBatchHook handles it before anything else.
func (s preBatchStage) skipsHealthChecks() bool {
	return s == preBatchWaiting || s == preBatchExpired || s == preBatchCleared
}

func hasPreBatchHooks(tu *tupprv1alpha1.TalosUpgrade) bool {
	return tu.Spec.Hooks != nil && len(tu.Spec.Hooks.PreBatch) > 0
}

func preBatchRetry(tu *tupprv1alpha1.TalosUpgrade) (timeout, interval time.Duration) {
	timeout, interval = defaultPreBatchRetryTimeout, defaultPreBatchRetryInterval
	if tu.Spec.Hooks == nil || tu.Spec.Hooks.PreBatchRetry == nil {
		return timeout, interval
	}
	retry := tu.Spec.Hooks.PreBatchRetry
	if retry.Timeout != nil {
		timeout = retry.Timeout.Duration
	}
	if retry.Interval != nil {
		interval = retry.Interval.Duration
	}
	return timeout, interval
}

// preBatchStageFor reports the pre-batch stage of the batch made of nodes,
// and when it is preBatchStart over an earlier result, why (for the log).
// touched reports whether any of the recorded batch's nodes has been cordoned
// or given an upgrade Job; it is only called for a batch recorded as started.
func preBatchStageFor(tu *tupprv1alpha1.TalosUpgrade, nodes []string, now time.Time, touched func() bool) (preBatchStage, string) {
	if !hasPreBatchHooks(tu) {
		return preBatchNone, ""
	}
	state := tu.Status.PreBatchHooks
	fresh := state != nil && state.CompletedAt != nil && now.Sub(state.CompletedAt.Time) < preBatchHookValidity
	timeout, _ := preBatchRetry(tu)
	switch {
	case state == nil:
		return preBatchStart, ""
	// BatchStarted is written before the batch's drain or upgrade Jobs, so it
	// may cover a start that never happened (say, every Job create failed).
	// The nodes left over from a batch that did start are resumed; a batch
	// nothing happened to is cleared only by a fresh result.
	case state.BatchStarted && isSubset(nodes, state.Nodes):
		if fresh || touched() {
			return preBatchCleared, ""
		}
		return preBatchStart, "batch never started and its hook result went stale"
	case !slices.Equal(state.Nodes, nodes):
		return preBatchStart, "batch changed from " + strings.Join(state.Nodes, ", ")
	case state.Index >= len(tu.Spec.Hooks.PreBatch):
		if fresh {
			return preBatchCleared, ""
		}
		return preBatchStart, "hook result went stale"
	case state.FailingSince != nil && now.Sub(state.FailingSince.Time) >= timeout:
		return preBatchExpired, ""
	case state.Job != "":
		// A running Job is observed before processNextBatch, so this one is
		// missing: it never got created, or was deleted before its outcome
		// was recorded. It runs again under its name, after fresh checks.
		return preBatchReady, ""
	case state.RetryAt != nil && state.RetryAt.After(now):
		return preBatchWaiting, ""
	}
	return preBatchReady, ""
}

func isSubset(sub, of []string) bool {
	for _, n := range sub {
		if !slices.Contains(of, n) {
			return false
		}
	}
	return len(sub) > 0
}

// resumeStartedBatch keeps a started batch together. While some nodes of a
// batch recorded as started are still pending, they are the next batch, so a
// half-started batch (one of its upgrade Jobs failed to be created, say) isn't
// regrouped with nodes it never included and gated again as a new batch.
func resumeStartedBatch(tu *tupprv1alpha1.TalosUpgrade, pending, next []string) []string {
	state := tu.Status.PreBatchHooks
	if !hasPreBatchHooks(tu) || state == nil || !state.BatchStarted {
		return next
	}
	var left []string
	for _, n := range pending {
		if slices.Contains(state.Nodes, n) {
			left = append(left, n)
		}
	}
	if len(left) == 0 {
		return next
	}
	return left
}

// preBatchNodesTouched reports whether the recorded batch has disrupted any
// of its nodes: one has finished, failed or is rebooting, carries the
// upgrading label, or (when tuppr drains) has been cordoned. A cordon is
// ignored when tuppr doesn't drain, since it can't tell one from a user's.
// A node that can't be read counts as touched, which keeps the batch going
// rather than re-running its hooks.
func (r *Reconciler) preBatchNodesTouched(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade) bool {
	status := tu.Status
	for _, name := range status.PreBatchHooks.Nodes {
		if slices.Contains(status.CompletedNodes, name) ||
			slices.ContainsFunc(status.FailedNodes, func(n tupprv1alpha1.NodeUpgradeStatus) bool { return n.NodeName == name }) ||
			slices.ContainsFunc(status.RebootingNodes, func(n tupprv1alpha1.NodeRebootStatus) bool { return n.NodeName == name }) {
			return true
		}
	}
	ownsDrain := r.tupprOwnsDrain(ctx, tu)
	for _, name := range status.PreBatchHooks.Nodes {
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: name}, node); err != nil {
			if !apierrors.IsNotFound(err) {
				log.FromContext(ctx).Error(err, "Failed to read batch node; treating the batch as started", "node", name)
				return true
			}
			continue
		}
		if node.Labels[constants.NodeUpgradingLabel] != "" || (ownsDrain && node.Spec.Unschedulable) {
			return true
		}
	}
	return false
}

// gatePreBatch returns proceed=true when the batch may start: the run has
// no pre-batch hooks, or they have cleared this batch, which is then recorded
// as started. Otherwise it advances the hooks.
func (r *Reconciler) gatePreBatch(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade, nodes []string, stage preBatchStage, rerunReason string, now time.Time) (ctrl.Result, bool, error) {
	switch stage {
	case preBatchNone:
		return ctrl.Result{}, true, nil
	case preBatchCleared:
		err := r.markPreBatchStarted(ctx, tu)
		return ctrl.Result{}, err == nil, err
	}
	if rerunReason != "" {
		log.FromContext(ctx).Info("Running pre-batch hooks again", "nodes", nodes, "reason", rerunReason)
	}
	result, err := r.processPreBatchHooks(ctx, tu, nodes, stage, now)
	return result, false, err
}

// processPreBatchHooks advances the pre-batch hooks of the batch made of
// nodes by one step. The caller has run the health checks unless a hook Job
// is already running. A failed hook is re-run after the retry interval until
// it has kept failing for the retry timeout; then the run fails like a failed
// pre-hook, with no further batch started.
func (r *Reconciler) processPreBatchHooks(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade, nodes []string, stage preBatchStage, now time.Time) (ctrl.Result, error) {
	switch stage {
	case preBatchWaiting:
		// Wake at the deadline too, so a retry interval longer than the time
		// left still fails the run on time.
		state := tu.Status.PreBatchHooks
		wait := state.RetryAt.Sub(now)
		if state.FailingSince != nil {
			timeout, _ := preBatchRetry(tu)
			wait = min(wait, state.FailingSince.Add(timeout).Sub(now))
		}
		return ctrl.Result{RequeueAfter: wait}, nil
	case preBatchExpired:
		return r.failPreBatchHooks(ctx, tu, tu.Status.PreBatchHooks.DeepCopy())
	case preBatchStart:
		r.deletePreBatchHookJobs(ctx, tu)
		tu.Status.PreBatchHooks = &tupprv1alpha1.PreBatchHookStatus{Nodes: slices.Clone(nodes)}
	}
	return r.startPreBatchHookJob(ctx, tu)
}

// startPreBatchHookJob records the Job name before creating the Job, so an
// outcome is only ever read from the Job started for this attempt. A Job
// already named in status (one that went missing) keeps its name, so a Job
// that exists but isn't in the cache yet isn't duplicated.
func (r *Reconciler) startPreBatchHookJob(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	state := tu.Status.PreBatchHooks.DeepCopy()
	hook := tu.Spec.Hooks.PreBatch[state.Index]

	// The health checks before this attempt may have taken a while.
	if timeout, _ := preBatchRetry(tu); state.FailingSince != nil && r.Now.Now().Sub(state.FailingSince.Time) >= timeout {
		return r.failPreBatchHooks(ctx, tu, state)
	}

	job := r.buildHookJob(tu, hook, hookPhasePreBatch, state.Index)
	if state.Job != "" {
		job.Name = state.Job
	}
	state.Job = job.Name
	if err := r.writePreBatchHooks(ctx, tu, state, runningPreBatchMessage(tu, state)); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.createPreBatchHookJob(ctx, tu, job); err != nil {
		return ctrl.Result{}, err
	}
	log.FromContext(ctx).Info("Started hook job", "hookPhase", hookPhasePreBatch, "hook", hook.Name, "index", state.Index, "nodes", state.Nodes, "job", job.Name)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *Reconciler) createPreBatchHookJob(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade, job *batchv1.Job) error {
	if err := controllerutil.SetControllerReference(tu, job, r.Scheme); err != nil {
		return fmt.Errorf("set controller ref on hook job: %w", err)
	}
	if err := r.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create hook job %s: %w", job.Name, err)
	}
	return nil
}

// observeRunningPreBatchHook handles the pre-batch hook Job named in status
// before any other gate of the reconcile. A running hook keeps the run in
// PreHook (an active phase, which holds other plans back) until it finishes,
// even if the maintenance window closes or a node goes unreachable meanwhile.
// observed=false means there is no such Job, and the caller carries on. Not
// run during PostHook: by then the run is finishing.
func (r *Reconciler) observeRunningPreBatchHook(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade) (ctrl.Result, bool, error) {
	state := tu.Status.PreBatchHooks
	if state == nil || state.Job == "" || state.Failed || !hasPreBatchHooks(tu) ||
		state.Index >= len(tu.Spec.Hooks.PreBatch) || tu.Status.Phase == tupprv1alpha1.JobPhasePostHook {
		return ctrl.Result{}, false, nil
	}
	job, err := r.getPreBatchHookJob(ctx, state.Job)
	if apierrors.IsNotFound(err) {
		return ctrl.Result{}, false, nil
	}
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to get hook job", "hookPhase", hookPhasePreBatch, "job", state.Job)
		return ctrl.Result{RequeueAfter: time.Minute}, true, nil
	}
	result, err := r.observePreBatchHookJob(ctx, tu, job, r.Now.Now())
	return result, true, err
}

// getPreBatchHookJob reads the named hook Job. The cache can miss a Job
// created moments ago (the status write that names it wakes the next
// reconcile at once), so a NotFound is confirmed against the API server before
// the Job is treated as missing.
func (r *Reconciler) getPreBatchHookJob(ctx context.Context, name string) (*batchv1.Job, error) {
	key := types.NamespacedName{Namespace: r.ControllerNamespace, Name: name}
	job := &batchv1.Job{}
	err := r.Get(ctx, key, job)
	if apierrors.IsNotFound(err) && r.APIReader != nil {
		err = r.APIReader.Get(ctx, key, job)
	}
	return job, err
}

func (r *Reconciler) observePreBatchHookJob(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade, job *batchv1.Job, now time.Time) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	state := tu.Status.PreBatchHooks
	hook := tu.Spec.Hooks.PreBatch[state.Index]

	if !jobs.IsTerminal(job) {
		// Back from a suspend or a wait elsewhere while the Job kept running.
		if tu.Status.Phase != tupprv1alpha1.JobPhasePreHook {
			if err := r.writePreBatchHooks(ctx, tu, state, runningPreBatchMessage(tu, state)); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Delete before recording the outcome: if the status write then fails,
	// the hook is re-run rather than its result being counted twice.
	if err := jobs.DeleteJob(ctx, r.Client, job); err != nil {
		return ctrl.Result{}, fmt.Errorf("delete pre-batch hook job %s: %w", job.Name, err)
	}

	next := state.DeepCopy()
	next.Job = ""
	nodes := strings.Join(state.Nodes, ", ")

	if jobs.IsSucceeded(job) {
		logger.Info("Hook job succeeded", "hookPhase", hookPhasePreBatch, "hook", hook.Name, "index", state.Index, "nodes", state.Nodes, "job", job.Name)
		r.MetricsReporter.RecordHookExecution(tu.Name, hookPhasePreBatch, hook.Name, "success")
		next.Index++
		next.FailingSince, next.RetryAt = nil, nil
		message := fmt.Sprintf("Pre-batch hook %q succeeded for node(s) %s", hook.Name, nodes)
		if next.Index >= len(tu.Spec.Hooks.PreBatch) {
			next.CompletedAt = &metav1.Time{Time: now}
			message = "Pre-batch hooks complete for node(s) " + nodes
		}
		return ctrl.Result{RequeueAfter: time.Second}, r.writePreBatchHooks(ctx, tu, next, message)
	}

	r.MetricsReporter.RecordHookExecution(tu.Name, hookPhasePreBatch, hook.Name, "failure")
	failingSince := now
	if state.FailingSince != nil {
		failingSince = state.FailingSince.Time
	}
	next.FailingSince = &metav1.Time{Time: failingSince}
	timeout, interval := preBatchRetry(tu)

	if now.Sub(failingSince) >= timeout {
		return r.failPreBatchHooks(ctx, tu, next)
	}

	logger.Info("Pre-batch hook failed, retrying", "hook", hook.Name, "nodes", state.Nodes, "retryIn", interval, "job", job.Name)
	next.RetryAt = &metav1.Time{Time: now.Add(interval)}
	message := fmt.Sprintf("Pre-batch hook %q failed for node(s) %s; retrying in %s (failing for %s, gives up after %s)",
		hook.Name, nodes, interval, now.Sub(failingSince).Round(time.Second), timeout)
	if err := r.writePreBatchHooks(ctx, tu, next, message); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: interval}, nil
}

// failPreBatchHooks records that the hook at state.Index kept failing past
// its retry timeout and stops the run through the post-hooks.
func (r *Reconciler) failPreBatchHooks(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade, state *tupprv1alpha1.PreBatchHookStatus) (ctrl.Result, error) {
	hook := tu.Spec.Hooks.PreBatch[state.Index]
	timeout, _ := preBatchRetry(tu)
	log.FromContext(ctx).Info("Pre-batch hook kept failing past its retry timeout, stopping the run",
		"hook", hook.Name, "nodes", state.Nodes, "timeout", timeout)
	if state.Job != "" {
		// An attempt the cache hadn't seen yet may still be running.
		job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: state.Job, Namespace: r.ControllerNamespace}}
		if err := jobs.DeleteJob(ctx, r.Client, job); err != nil {
			log.FromContext(ctx).Error(err, "Failed to delete pre-batch hook job (continuing)", "job", state.Job)
		}
	}
	state.Failed = true
	state.Job, state.RetryAt = "", nil
	message := fmt.Sprintf("Pre-batch hook %q kept failing for node(s) %s", hook.Name, strings.Join(state.Nodes, ", "))
	if err := r.writePreBatchHooks(ctx, tu, state, message); err != nil {
		return ctrl.Result{}, err
	}
	return r.transitionToFinalize(ctx, tu)
}

func runningPreBatchMessage(tu *tupprv1alpha1.TalosUpgrade, state *tupprv1alpha1.PreBatchHookStatus) string {
	hooks := tu.Spec.Hooks.PreBatch
	return fmt.Sprintf("Running pre-batch hook %q (%d/%d) for node(s) %s",
		hooks[state.Index].Name, state.Index+1, len(hooks), strings.Join(state.Nodes, ", "))
}

func (r *Reconciler) writePreBatchHooks(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade, state *tupprv1alpha1.PreBatchHookStatus, message string) error {
	if err := r.setPhaseWithUpdates(ctx, tu, tupprv1alpha1.JobPhasePreHook, "", state.Nodes, message, map[string]any{
		statusPreBatchHooks: preBatchHooksPatch(state),
	}); err != nil {
		return fmt.Errorf("update pre-batch hook status: %w", err)
	}
	tu.Status.PreBatchHooks = state
	return nil
}

// markPreBatchStarted records that the batch is about to be drained or have
// its upgrade Jobs created, before either happens.
func (r *Reconciler) markPreBatchStarted(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade) error {
	state := tu.Status.PreBatchHooks
	if state == nil || state.BatchStarted {
		return nil
	}
	next := state.DeepCopy()
	next.BatchStarted = true
	if err := r.updateStatus(ctx, tu, map[string]any{statusPreBatchHooks: preBatchHooksPatch(next)}); err != nil {
		return fmt.Errorf("record pre-batch hooks batch start: %w", err)
	}
	tu.Status.PreBatchHooks = next
	return nil
}

// resetPreBatchHooks drops the pre-batch state after a drain was rolled
// back: the nodes are schedulable again, so the batch has to pass its health
// checks and hooks again before the retry.
func (r *Reconciler) resetPreBatchHooks(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade) {
	if tu.Status.PreBatchHooks == nil {
		return
	}
	if err := r.updateStatus(ctx, tu, map[string]any{statusPreBatchHooks: nil}); err != nil {
		log.FromContext(ctx).Error(err, "Failed to clear pre-batch hook state after drain rollback")
		return
	}
	tu.Status.PreBatchHooks = nil
}

// preBatchHooksPatch renders state for a JSON merge patch. Status is written
// with merge patches, which merge nested objects, so every field is spelled
// out and a cleared one is sent as null instead of being left as it was.
func preBatchHooksPatch(state *tupprv1alpha1.PreBatchHookStatus) any {
	if state == nil {
		return nil
	}
	return map[string]any{
		"nodes":        state.Nodes,
		"index":        state.Index,
		"job":          state.Job,
		"failingSince": state.FailingSince,
		"retryAt":      state.RetryAt,
		"completedAt":  state.CompletedAt,
		"batchStarted": state.BatchStarted,
		"failed":       state.Failed,
	}
}

// deletePreBatchHookJobs removes hook Jobs left from earlier batches. Only
// the Job named in status counts, so a leftover can't be mistaken for a later
// batch's and this is best-effort cleanup.
func (r *Reconciler) deletePreBatchHookJobs(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade) {
	logger := log.FromContext(ctx)
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(r.ControllerNamespace),
		client.MatchingLabels{
			appLabelKey:         hookJobAppName,
			appInstanceLabelKey: tu.Name,
			hookPhaseLabel:      hookPhasePreBatch,
		},
	); err != nil {
		logger.Error(err, "Failed to list stale pre-batch hook jobs (continuing)")
		return
	}
	for i := range jobList.Items {
		job := &jobList.Items[i]
		if owner := metav1.GetControllerOf(job); owner == nil || owner.UID != tu.UID || job.DeletionTimestamp != nil {
			continue
		}
		if err := jobs.DeleteJob(ctx, r.Client, job); err != nil {
			logger.Error(err, "Failed to delete stale pre-batch hook job (continuing)", "job", job.Name)
		}
	}
}
