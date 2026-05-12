package upgradeaudit

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

func TestApplyConditions_ProgressingTrueOnInFlight(t *testing.T) {
	for _, phase := range []tupprv1alpha1.JobPhase{
		tupprv1alpha1.JobPhasePreHook,
		tupprv1alpha1.JobPhaseDraining,
		tupprv1alpha1.JobPhaseUpgrading,
		tupprv1alpha1.JobPhaseRebooting,
		tupprv1alpha1.JobPhasePostHook,
	} {
		t.Run(string(phase), func(t *testing.T) {
			conds := ApplyConditions(nil, phase, "", "msg", 1)
			p := findCond(conds, tupprv1alpha1.ConditionTypeProgressing)
			if p == nil || p.Status != metav1.ConditionTrue {
				t.Fatalf("want Progressing=True, got %+v", p)
			}
			if p.Reason != string(phase) {
				t.Fatalf("want Reason=%s, got %s", phase, p.Reason)
			}
		})
	}
}

func TestApplyConditions_ProgressingFalseOffInFlight(t *testing.T) {
	for _, phase := range []tupprv1alpha1.JobPhase{
		tupprv1alpha1.JobPhasePending,
		tupprv1alpha1.JobPhaseHealthChecking,
		tupprv1alpha1.JobPhaseMaintenanceWindow,
		tupprv1alpha1.JobPhaseCompleted,
		tupprv1alpha1.JobPhaseFailed,
	} {
		t.Run(string(phase), func(t *testing.T) {
			conds := ApplyConditions(nil, phase, "", "msg", 1)
			p := findCond(conds, tupprv1alpha1.ConditionTypeProgressing)
			if p == nil || p.Status != metav1.ConditionFalse {
				t.Fatalf("want Progressing=False, got %+v", p)
			}
		})
	}
}

func TestApplyConditions_ReadyOnlyOnCompleted(t *testing.T) {
	completed := ApplyConditions(nil, tupprv1alpha1.JobPhaseCompleted, "", "done", 1)
	if r := findCond(completed, tupprv1alpha1.ConditionTypeReady); r == nil || r.Status != metav1.ConditionTrue {
		t.Fatalf("want Ready=True on Completed, got %+v", r)
	}

	for _, phase := range []tupprv1alpha1.JobPhase{
		tupprv1alpha1.JobPhasePending,
		tupprv1alpha1.JobPhaseUpgrading,
		tupprv1alpha1.JobPhaseFailed,
	} {
		t.Run(string(phase), func(t *testing.T) {
			conds := ApplyConditions(nil, phase, "", "msg", 1)
			r := findCond(conds, tupprv1alpha1.ConditionTypeReady)
			if r == nil || r.Status != metav1.ConditionFalse {
				t.Fatalf("want Ready=False, got %+v", r)
			}
		})
	}
}

func TestApplyConditions_ReasonOverride(t *testing.T) {
	conds := ApplyConditions(nil, tupprv1alpha1.JobPhasePending, ReasonWaitingForImage, "msg", 1)
	p := findCond(conds, tupprv1alpha1.ConditionTypeProgressing)
	if p == nil || p.Reason != ReasonWaitingForImage {
		t.Fatalf("want Reason=%s, got %+v", ReasonWaitingForImage, p)
	}
}

func TestApplyConditions_EmptyReasonAndPhaseFallsBackToInitializing(t *testing.T) {
	conds := ApplyConditions(nil, tupprv1alpha1.JobPhase(""), "", "", 0)
	p := findCond(conds, tupprv1alpha1.ConditionTypeProgressing)
	if p == nil || p.Reason != "Initializing" {
		t.Fatalf("want Reason=Initializing for empty phase, got %+v", p)
	}
}

func TestApplyConditions_PreservesLastTransitionTimeOnNoOp(t *testing.T) {
	first := ApplyConditions(nil, tupprv1alpha1.JobPhaseUpgrading, "", "msg", 1)
	original := findCond(first, tupprv1alpha1.ConditionTypeProgressing).LastTransitionTime

	time.Sleep(10 * time.Millisecond)
	second := ApplyConditions(first, tupprv1alpha1.JobPhaseUpgrading, "", "msg", 1)
	got := findCond(second, tupprv1alpha1.ConditionTypeProgressing).LastTransitionTime

	if !got.Equal(&original) {
		t.Fatalf("LastTransitionTime changed across no-op: was %v, now %v", original, got)
	}
}

func TestApplyConditions_AdvancesLastTransitionTimeOnStatusFlip(t *testing.T) {
	first := ApplyConditions(nil, tupprv1alpha1.JobPhasePending, "", "msg", 1)
	original := findCond(first, tupprv1alpha1.ConditionTypeProgressing).LastTransitionTime

	time.Sleep(10 * time.Millisecond)
	second := ApplyConditions(first, tupprv1alpha1.JobPhaseUpgrading, "", "msg", 1)
	got := findCond(second, tupprv1alpha1.ConditionTypeProgressing).LastTransitionTime

	if got.Equal(&original) {
		t.Fatalf("LastTransitionTime should advance when status flips False→True, stayed %v", got)
	}
}

func TestConditionsEqual(t *testing.T) {
	base := ApplyConditions(nil, tupprv1alpha1.JobPhaseUpgrading, "", "msg", 1)

	t.Run("same input", func(t *testing.T) {
		other := ApplyConditions(nil, tupprv1alpha1.JobPhaseUpgrading, "", "msg", 1)
		if !ConditionsEqual(base, other) {
			t.Fatal("expected equal")
		}
	})

	t.Run("different reason", func(t *testing.T) {
		other := ApplyConditions(nil, tupprv1alpha1.JobPhaseUpgrading, "X", "msg", 1)
		if ConditionsEqual(base, other) {
			t.Fatal("expected not equal when reason differs")
		}
	})

	t.Run("different message", func(t *testing.T) {
		other := ApplyConditions(nil, tupprv1alpha1.JobPhaseUpgrading, "", "other", 1)
		if ConditionsEqual(base, other) {
			t.Fatal("expected not equal when message differs")
		}
	})

	t.Run("different generation", func(t *testing.T) {
		other := ApplyConditions(nil, tupprv1alpha1.JobPhaseUpgrading, "", "msg", 2)
		if ConditionsEqual(base, other) {
			t.Fatal("expected not equal when observedGeneration differs")
		}
	})

	t.Run("ignores LastTransitionTime", func(t *testing.T) {
		other := append([]metav1.Condition(nil), base...)
		other[0].LastTransitionTime = metav1.NewTime(other[0].LastTransitionTime.Add(time.Hour))
		if !ConditionsEqual(base, other) {
			t.Fatal("expected equal when only LastTransitionTime differs")
		}
	})

	t.Run("order-independent", func(t *testing.T) {
		reversed := []metav1.Condition{base[1], base[0]}
		if !ConditionsEqual(base, reversed) {
			t.Fatal("expected equal regardless of slice order")
		}
	})

	t.Run("missing type", func(t *testing.T) {
		other := append([]metav1.Condition(nil), base...)
		other[0].Type = "Unknown"
		if ConditionsEqual(base, other) {
			t.Fatal("expected not equal when a type is replaced")
		}
	})
}

func findCond(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}
