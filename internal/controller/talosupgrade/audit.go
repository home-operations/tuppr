package talosupgrade

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

const historyMaxEntries = 10

// applyPhaseAuditFields adds startedAt/completedAt/history bookkeeping to
// updates based on the transition from status.Phase to nextPhase.
//
// The CompletedAt != nil check on the terminal branch is an idempotency guard
// against a stale client cache serving a copy where Phase has rolled back but
// CompletedAt is still set. Stale caches that roll back both fields can still
// produce a rare duplicate entry; historyMaxEntries bounds the blast radius.
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
		updates["history"] = prependHistory(status.History, entry, historyMaxEntries)
	}
}

// syncLocalAuditFields applies updates to the in-memory status so re-entry
// guards and metrics in the same reconcile see the just-patched state.
func syncLocalAuditFields(status *tupprv1alpha1.TalosUpgradeStatus, updates map[string]any) {
	if v, ok := updates["startedAt"]; ok {
		if t, isTime := v.(metav1.Time); isTime {
			status.StartedAt = &t
		} else {
			status.StartedAt = nil
		}
	}
	if v, ok := updates["completedAt"]; ok {
		if t, isTime := v.(metav1.Time); isTime {
			status.CompletedAt = &t
		} else {
			status.CompletedAt = nil
		}
	}
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
}

func prependHistory(history []tupprv1alpha1.TalosUpgradeHistoryEntry, entry tupprv1alpha1.TalosUpgradeHistoryEntry, max int) []tupprv1alpha1.TalosUpgradeHistoryEntry {
	next := make([]tupprv1alpha1.TalosUpgradeHistoryEntry, 0, min(len(history)+1, max))
	next = append(next, entry)
	for i := 0; i < len(history) && len(next) < max; i++ {
		next = append(next, history[i])
	}
	return next
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
