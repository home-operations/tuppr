package kubernetesupgrade

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/controller/upgradeaudit"
)

// CompletedAt != nil guard on terminal branch is idempotency against a stale
// client cache serving a copy where Phase rolled back but CompletedAt is still
// set. Stale caches rolling back both fields can still produce a rare duplicate
// entry; HistoryMaxEntries bounds the blast radius.
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

		// A run that never went active (StartedAt unset) did no work, e.g. a
		// spec re-apply with the cluster already at target. Recording it would
		// fabricate a zero-duration entry for an upgrade that never happened.
		if status.StartedAt == nil {
			return
		}
		entry := tupprv1alpha1.UpgradeHistoryEntry{
			FromVersion: status.CurrentVersion,
			ToVersion:   status.TargetVersion,
			StartedAt:   *status.StartedAt,
			CompletedAt: now,
			Phase:       nextPhase,
			Retries:     status.Retries,
		}
		if nextPhase == tupprv1alpha1.JobPhaseFailed {
			entry.LastError = upgradeaudit.FirstNonEmpty(status.LastError, message)
		}
		updates["history"] = upgradeaudit.PrependHistory(status.History, entry, upgradeaudit.HistoryMaxEntries)
	}
}

// Mirror updates onto in-memory status so re-entry guards and metrics in the
// same reconcile see the just-patched state.
func syncLocalAuditFields(status *tupprv1alpha1.KubernetesUpgradeStatus, updates map[string]any) {
	upgradeaudit.SyncTimingFields(updates, &status.StartedAt, &status.CompletedAt)
	if v, ok := updates["history"]; ok {
		if h, isHistory := v.([]tupprv1alpha1.UpgradeHistoryEntry); isHistory {
			status.History = h
		}
	}
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
