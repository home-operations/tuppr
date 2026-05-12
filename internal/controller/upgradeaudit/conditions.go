package upgradeaudit

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

// ConditionsEqual compares by Type, ignoring LastTransitionTime.
func ConditionsEqual(a, b []metav1.Condition) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range b {
		ac := meta.FindStatusCondition(a, b[i].Type)
		if ac == nil ||
			ac.Status != b[i].Status ||
			ac.Reason != b[i].Reason ||
			ac.Message != b[i].Message ||
			ac.ObservedGeneration != b[i].ObservedGeneration {
			return false
		}
	}
	return true
}

// Reasons for Progressing=False when an upgrade is parked on external state.
const (
	ReasonWaitingForImage        = "WaitingForImage"
	ReasonWaitingForOtherUpgrade = "WaitingForOtherUpgrade"
	ReasonSuspended              = "Suspended"
)

// ApplyConditions sets Progressing and Ready for the given phase. Empty reason
// defaults to the phase name.
func ApplyConditions(existing []metav1.Condition, phase tupprv1alpha1.JobPhase, reason, message string, observedGeneration int64) []metav1.Condition {
	if reason == "" {
		reason = string(phase)
	}
	if reason == "" {
		reason = "Initializing"
	}

	progressing := metav1.Condition{
		Type:               tupprv1alpha1.ConditionTypeProgressing,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: observedGeneration,
	}
	if phase.IsInFlight() {
		progressing.Status = metav1.ConditionTrue
	}

	ready := metav1.Condition{
		Type:               tupprv1alpha1.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: observedGeneration,
	}
	if phase == tupprv1alpha1.JobPhaseCompleted {
		ready.Status = metav1.ConditionTrue
	}

	out := append([]metav1.Condition(nil), existing...)
	meta.SetStatusCondition(&out, progressing)
	meta.SetStatusCondition(&out, ready)
	return out
}
