package kubernetesupgrade

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
func applyPhaseAuditFields(status *tupprv1alpha1.KubernetesUpgradeStatus, updates map[string]any, nextPhase tupprv1alpha1.JobPhase, now metav1.Time, message string) {
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
		entry := tupprv1alpha1.UpgradeHistoryEntry{
			FromVersion: status.CurrentVersion,
			ToVersion:   status.TargetVersion,
			StartedAt:   startedAt,
			CompletedAt: now,
			Phase:       nextPhase,
			Retries:     status.Retries,
		}
		if nextPhase == tupprv1alpha1.JobPhaseFailed {
			entry.LastError = firstNonEmpty(status.LastError, message)
		}
		updates["history"] = prependHistory(status.History, entry, historyMaxEntries)
	}
}

// syncLocalAuditFields applies updates to the in-memory status so re-entry
// guards and metrics in the same reconcile see the just-patched state.
func syncLocalAuditFields(status *tupprv1alpha1.KubernetesUpgradeStatus, updates map[string]any) {
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
		if h, isHistory := v.([]tupprv1alpha1.UpgradeHistoryEntry); isHistory {
			status.History = h
		}
	}
}

func prependHistory(history []tupprv1alpha1.UpgradeHistoryEntry, entry tupprv1alpha1.UpgradeHistoryEntry, max int) []tupprv1alpha1.UpgradeHistoryEntry {
	next := make([]tupprv1alpha1.UpgradeHistoryEntry, 0, min(len(history)+1, max))
	next = append(next, entry)
	for i := 0; i < len(history) && len(next) < max; i++ {
		next = append(next, history[i])
	}
	return next
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (r *Reconciler) emitPhaseEvent(ku *tupprv1alpha1.KubernetesUpgrade, prev, next tupprv1alpha1.JobPhase, message string) {
	if r.Recorder == nil || prev == next {
		return
	}

	switch {
	case next.IsActive() && !prev.IsActive():
		r.Recorder.Eventf(ku, corev1.EventTypeNormal, "UpgradeStarted",
			"Kubernetes upgrade to %s started", ku.Spec.Kubernetes.Version)
	case next == tupprv1alpha1.JobPhaseCompleted:
		r.Recorder.Eventf(ku, corev1.EventTypeNormal, "UpgradeCompleted",
			"Kubernetes upgraded to %s", ku.Spec.Kubernetes.Version)
	case next == tupprv1alpha1.JobPhaseFailed:
		r.Recorder.Eventf(ku, corev1.EventTypeWarning, "UpgradeFailed",
			"Kubernetes upgrade to %s failed: %s", ku.Spec.Kubernetes.Version, message)
	}
}
