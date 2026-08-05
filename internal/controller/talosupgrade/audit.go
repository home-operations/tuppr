package talosupgrade

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/controller/upgradeaudit"
)

// CompletedAt != nil guard on terminal branch is idempotency against a stale
// client cache serving a copy where Phase rolled back but CompletedAt is still
// set. Stale caches rolling back both fields can still produce a rare duplicate
// entry; HistoryMaxEntries bounds the blast radius.
func applyPhaseAuditFields(status *tupprv1alpha1.TalosUpgradeStatus, updates map[string]any, nextPhase tupprv1alpha1.JobPhase, now metav1.Time, targetVersion string) {
	prev := status.Phase

	switch {
	case nextPhase == tupprv1alpha1.JobPhasePending && prev != tupprv1alpha1.JobPhasePending:
		updates["startedAt"] = nil
		updates["completedAt"] = nil

	case nextPhase.IsActive() && status.StartedAt == nil:
		updates["startedAt"] = now

	case nextPhase.IsTerminal() && !prev.IsTerminal():
		if status.CompletedAt != nil {
			return
		}
		updates["completedAt"] = now

		// A Completed run that never went active (StartedAt unset) did no
		// work, e.g. a spec re-apply with every node already at target;
		// recording it would fabricate a zero-duration entry claiming nodes
		// the run never touched. Failed runs are always recorded.
		if nextPhase == tupprv1alpha1.JobPhaseCompleted && status.StartedAt == nil {
			return
		}
		startedAt := now
		if status.StartedAt != nil {
			startedAt = *status.StartedAt
		}

		failedNames := make([]string, 0, len(status.FailedNodes))
		for _, n := range status.FailedNodes {
			failedNames = append(failedNames, n.NodeName)
		}
		completed := append([]string(nil), status.CompletedNodes...)

		entry := tupprv1alpha1.TalosUpgradeHistoryEntry{
			ToVersion:      targetVersion,
			StartedAt:      startedAt,
			CompletedAt:    now,
			Phase:          nextPhase,
			CompletedNodes: completed,
			FailedNodes:    failedNames,
		}
		updates["history"] = upgradeaudit.PrependHistory(status.History, entry, upgradeaudit.HistoryMaxEntries)
	}
}

// Mirror updates onto in-memory status so re-entry guards and metrics in the
// same reconcile see the just-patched state.
func syncLocalAuditFields(status *tupprv1alpha1.TalosUpgradeStatus, updates map[string]any) {
	upgradeaudit.SyncTimingFields(updates, &status.StartedAt, &status.CompletedAt)
	if v, ok := updates["history"]; ok {
		if h, isHistory := v.([]tupprv1alpha1.TalosUpgradeHistoryEntry); isHistory {
			status.History = h
		}
	}
	if v, ok := updates[statusCompletedNodes]; ok {
		if s, isSlice := v.([]string); isSlice {
			status.CompletedNodes = s
		}
	}
	if v, ok := updates[statusFailedNodes]; ok {
		if s, isSlice := v.([]tupprv1alpha1.NodeUpgradeStatus); isSlice {
			status.FailedNodes = s
		}
	}
	if v, ok := updates[statusRebootingNodes]; ok {
		if s, isSlice := v.([]tupprv1alpha1.NodeRebootStatus); isSlice {
			status.RebootingNodes = s
		}
	}
}

// runHealthChecks wraps CheckHealth with start/result Events. CheckHealth
// blocks polling until the checks pass or time out, and the HealthChecking
// phase is only written after it returns, so the started event is the only
// signal that a (possibly minutes-long) check attempt is in progress.
func (r *Reconciler) runHealthChecks(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade) error {
	emit := r.Recorder != nil && len(tu.Spec.HealthChecks) > 0
	if emit {
		r.Recorder.Eventf(tu, corev1.EventTypeNormal, "HealthChecksStarted",
			"Running %d health check(s)", len(tu.Spec.HealthChecks))
	}
	checkErr := r.HealthChecker.CheckHealth(ctx, tu.Spec.HealthChecks)
	if emit {
		if checkErr != nil {
			r.Recorder.Event(tu, corev1.EventTypeWarning, "HealthChecksFailed", checkErr.Error())
		} else {
			r.Recorder.Eventf(tu, corev1.EventTypeNormal, "HealthChecksPassed",
				"All %d health check(s) passed", len(tu.Spec.HealthChecks))
		}
	}
	return checkErr
}

func (r *Reconciler) emitPhaseEvent(tu *tupprv1alpha1.TalosUpgrade, prev, next tupprv1alpha1.JobPhase, message string) {
	if r.Recorder == nil || prev == next {
		return
	}

	switch {
	case next.IsActive() && !prev.IsActive():
		r.Recorder.Eventf(tu, corev1.EventTypeNormal, "UpgradeStarted",
			"Talos upgrade to %s started", tu.Spec.Talos.Version)
	case next == tupprv1alpha1.JobPhaseCompleted:
		r.Recorder.Eventf(tu, corev1.EventTypeNormal, "UpgradeCompleted",
			"Talos upgraded to %s on %d node(s)", tu.Spec.Talos.Version, len(tu.Status.CompletedNodes))
	case next == tupprv1alpha1.JobPhaseFailed:
		r.Recorder.Eventf(tu, corev1.EventTypeWarning, "UpgradeFailed",
			"Talos upgrade to %s failed: %s", tu.Spec.Talos.Version, message)
	}
}
