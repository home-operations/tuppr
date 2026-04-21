package kubernetesupgrade

import (
	"context"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/metrics"
)

const (
	fromVer = "v1.32.0"
	toVer   = "v1.33.0"
)

func TestApplyPhaseAuditFields_ActiveTransition(t *testing.T) {
	now := metav1.NewTime(time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC))
	earlier := metav1.NewTime(now.Add(-2 * time.Hour))

	tests := []struct {
		name           string
		status         tupprv1alpha1.KubernetesUpgradeStatus
		nextPhase      tupprv1alpha1.JobPhase
		wantStartedSet bool
	}{
		{
			name:           "first transition into active sets startedAt",
			status:         tupprv1alpha1.KubernetesUpgradeStatus{Phase: tupprv1alpha1.JobPhasePending},
			nextPhase:      tupprv1alpha1.JobPhaseHealthChecking,
			wantStartedSet: true,
		},
		{
			name: "active phase with startedAt already set is a no-op",
			status: tupprv1alpha1.KubernetesUpgradeStatus{
				Phase:     tupprv1alpha1.JobPhaseHealthChecking,
				StartedAt: &earlier,
			},
			nextPhase: tupprv1alpha1.JobPhaseUpgrading,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updates := map[string]any{}
			applyPhaseAuditFields(&tc.status, updates, tc.nextPhase, now, "")
			got, present := updates["startedAt"]
			if tc.wantStartedSet {
				if !present {
					t.Fatal("expected startedAt to be set")
				}
				gotT := got.(metav1.Time)
				if !gotT.Equal(&now) {
					t.Fatalf("want startedAt %v, got %v", now, got)
				}
				return
			}
			if present {
				t.Fatalf("did not expect startedAt update, got %v", got)
			}
		})
	}
}

func TestApplyPhaseAuditFields_TerminalTransition(t *testing.T) {
	now := metav1.NewTime(time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC))
	earlier := metav1.NewTime(now.Add(-2 * time.Hour))

	baseStatus := tupprv1alpha1.KubernetesUpgradeStatus{
		Phase:          tupprv1alpha1.JobPhaseUpgrading,
		StartedAt:      &earlier,
		CurrentVersion: fromVer,
		TargetVersion:  toVer,
		Retries:        1,
	}

	t.Run("Completed records full entry", func(t *testing.T) {
		updates := map[string]any{}
		applyPhaseAuditFields(&baseStatus, updates, tupprv1alpha1.JobPhaseCompleted, now, "done")

		if got := updates["completedAt"].(metav1.Time); !got.Equal(&now) {
			t.Fatalf("want completedAt %v, got %v", now, got)
		}
		h := updates["history"].([]tupprv1alpha1.UpgradeHistoryEntry)
		if len(h) != 1 {
			t.Fatalf("want 1 history entry, got %d", len(h))
		}
		entry := h[0]
		if entry.FromVersion != fromVer || entry.ToVersion != toVer {
			t.Fatalf("unexpected versions in entry: %+v", entry)
		}
		if entry.Phase != tupprv1alpha1.JobPhaseCompleted {
			t.Fatalf("want phase Completed, got %s", entry.Phase)
		}
		if !entry.StartedAt.Equal(&earlier) {
			t.Fatalf("want startedAt %v, got %v", earlier, entry.StartedAt)
		}
		if entry.Retries != 1 {
			t.Fatalf("want retries 1, got %d", entry.Retries)
		}
		if entry.LastError != "" {
			t.Fatalf("Completed entry should not carry LastError, got %q", entry.LastError)
		}
	})

	t.Run("Failed captures LastError from status", func(t *testing.T) {
		status := baseStatus
		status.LastError = "boom"
		updates := map[string]any{}
		applyPhaseAuditFields(&status, updates, tupprv1alpha1.JobPhaseFailed, now, "msg")
		h := updates["history"].([]tupprv1alpha1.UpgradeHistoryEntry)
		if h[0].LastError != "boom" {
			t.Fatalf("want LastError=boom, got %q", h[0].LastError)
		}
	})

	t.Run("Failed falls back to message", func(t *testing.T) {
		status := baseStatus
		status.LastError = ""
		updates := map[string]any{}
		applyPhaseAuditFields(&status, updates, tupprv1alpha1.JobPhaseFailed, now, "jobs crashed")
		h := updates["history"].([]tupprv1alpha1.UpgradeHistoryEntry)
		if h[0].LastError != "jobs crashed" {
			t.Fatalf("want message fallback, got %q", h[0].LastError)
		}
	})

	t.Run("already-terminal does not append duplicate", func(t *testing.T) {
		status := tupprv1alpha1.KubernetesUpgradeStatus{
			Phase:     tupprv1alpha1.JobPhaseCompleted,
			StartedAt: &earlier,
		}
		updates := map[string]any{}
		applyPhaseAuditFields(&status, updates, tupprv1alpha1.JobPhaseCompleted, now, "")
		if _, ok := updates["history"]; ok {
			t.Fatal("did not expect history update on terminal→terminal")
		}
		if _, ok := updates["completedAt"]; ok {
			t.Fatal("did not expect completedAt update on terminal→terminal")
		}
	})
}

func TestApplyPhaseAuditFields_PendingResetsTimestamps(t *testing.T) {
	now := metav1.NewTime(time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC))
	earlier := metav1.NewTime(now.Add(-2 * time.Hour))
	status := tupprv1alpha1.KubernetesUpgradeStatus{
		Phase:       tupprv1alpha1.JobPhaseCompleted,
		StartedAt:   &earlier,
		CompletedAt: &earlier,
	}
	updates := map[string]any{}
	applyPhaseAuditFields(&status, updates, tupprv1alpha1.JobPhasePending, now, "")

	if v, ok := updates["startedAt"]; !ok || v != nil {
		t.Fatalf("want startedAt cleared to nil, got present=%v value=%v", ok, v)
	}
	if v, ok := updates["completedAt"]; !ok || v != nil {
		t.Fatalf("want completedAt cleared to nil, got present=%v value=%v", ok, v)
	}
}

func TestPrependHistoryCapsAtMax(t *testing.T) {
	base := tupprv1alpha1.UpgradeHistoryEntry{ToVersion: "v1.0.0", Phase: tupprv1alpha1.JobPhaseCompleted}
	history := make([]tupprv1alpha1.UpgradeHistoryEntry, 10)
	for i := range history {
		history[i] = base
	}

	fresh := tupprv1alpha1.UpgradeHistoryEntry{ToVersion: "v2.0.0", Phase: tupprv1alpha1.JobPhaseCompleted}
	result := prependHistory(history, fresh, historyMaxEntries)

	if len(result) != historyMaxEntries {
		t.Fatalf("want history cap %d, got %d", historyMaxEntries, len(result))
	}
	if result[0].ToVersion != "v2.0.0" {
		t.Fatalf("newest entry should be first, got %q", result[0].ToVersion)
	}
}

func TestSetPhase_WritesTimestampsAndEmitsEvent(t *testing.T) {
	scheme := newTestScheme()
	now := time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC)

	ku := newKubernetesUpgrade("test", func(ku *tupprv1alpha1.KubernetesUpgrade) {
		ku.Status.Phase = tupprv1alpha1.JobPhaseUpgrading
		started := metav1.NewTime(now.Add(-30 * time.Minute))
		ku.Status.StartedAt = &started
		ku.Status.CurrentVersion = fromVer
		ku.Status.TargetVersion = toVer
	})
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ku).WithStatusSubresource(ku).Build()
	recorder := record.NewFakeRecorder(4)
	r := &Reconciler{
		Client:          cl,
		Scheme:          scheme,
		MetricsReporter: metrics.NewReporter(),
		Now:             &fixedClock{t: now},
		Recorder:        recorder,
	}

	if err := r.setPhase(context.Background(), ku, tupprv1alpha1.JobPhaseCompleted, "", "done"); err != nil {
		t.Fatalf("setPhase: %v", err)
	}

	var got tupprv1alpha1.KubernetesUpgrade
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "test"}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.Phase != tupprv1alpha1.JobPhaseCompleted {
		t.Fatalf("want phase Completed, got %s", got.Status.Phase)
	}
	if got.Status.CompletedAt == nil {
		t.Fatal("want CompletedAt set")
	}
	if len(got.Status.History) != 1 {
		t.Fatalf("want 1 history entry, got %d", len(got.Status.History))
	}
	if got.Status.History[0].ToVersion != toVer {
		t.Fatalf("want ToVersion=%s, got %q", toVer, got.Status.History[0].ToVersion)
	}

	select {
	case ev := <-recorder.Events:
		if !strings.Contains(ev, "UpgradeCompleted") {
			t.Fatalf("want UpgradeCompleted event, got %q", ev)
		}
	default:
		t.Fatal("expected an event to be recorded")
	}
}

func TestSetPhase_NoRecorderIsSafe(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ku).WithStatusSubresource(ku).Build()
	r := &Reconciler{
		Client:          cl,
		Scheme:          scheme,
		MetricsReporter: metrics.NewReporter(),
		Now:             &fixedClock{t: time.Now()},
	}
	if err := r.setPhase(context.Background(), ku, tupprv1alpha1.JobPhaseHealthChecking, "", "ok"); err != nil {
		t.Fatalf("setPhase without recorder must not fail: %v", err)
	}
}

func TestApplyPhaseAuditFields_EmptyPhaseEdge(t *testing.T) {
	now := metav1.NewTime(time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC))

	t.Run("empty to active sets startedAt", func(t *testing.T) {
		status := tupprv1alpha1.KubernetesUpgradeStatus{}
		updates := map[string]any{}
		applyPhaseAuditFields(&status, updates, tupprv1alpha1.JobPhaseHealthChecking, now, "")
		if _, ok := updates["startedAt"]; !ok {
			t.Fatal("want startedAt set for fresh CR going active")
		}
	})

	t.Run("empty to pending clears timestamps", func(t *testing.T) {
		status := tupprv1alpha1.KubernetesUpgradeStatus{}
		updates := map[string]any{}
		applyPhaseAuditFields(&status, updates, tupprv1alpha1.JobPhasePending, now, "")
		if v, present := updates["startedAt"]; !present || v != nil {
			t.Fatalf("want startedAt cleared to nil, got present=%v value=%v", present, v)
		}
	})
}

func TestSetPhase_IdempotentOnRepeatedTerminalCalls(t *testing.T) {
	scheme := newTestScheme()
	now := time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC)

	ku := newKubernetesUpgrade("test", func(ku *tupprv1alpha1.KubernetesUpgrade) {
		ku.Status.Phase = tupprv1alpha1.JobPhaseUpgrading
		started := metav1.NewTime(now.Add(-30 * time.Minute))
		ku.Status.StartedAt = &started
		ku.Status.TargetVersion = toVer
	})
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ku).WithStatusSubresource(ku).Build()
	r := &Reconciler{
		Client:          cl,
		Scheme:          scheme,
		MetricsReporter: metrics.NewReporter(),
		Now:             &fixedClock{t: now},
	}

	if err := r.setPhase(context.Background(), ku, tupprv1alpha1.JobPhaseCompleted, "", "done"); err != nil {
		t.Fatalf("first setPhase: %v", err)
	}
	if err := r.setPhase(context.Background(), ku, tupprv1alpha1.JobPhaseCompleted, "", "done again"); err != nil {
		t.Fatalf("second setPhase: %v", err)
	}

	var got tupprv1alpha1.KubernetesUpgrade
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "test"}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Status.History) != 1 {
		t.Fatalf("want exactly 1 history entry after repeat terminal setPhase, got %d", len(got.Status.History))
	}
}

func TestSyncLocalAuditFields(t *testing.T) {
	earlier := metav1.NewTime(time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC))
	now := metav1.NewTime(time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC))

	t.Run("propagates metav1.Time values", func(t *testing.T) {
		status := tupprv1alpha1.KubernetesUpgradeStatus{}
		updates := map[string]any{
			"startedAt":   earlier,
			"completedAt": now,
			"history":     []tupprv1alpha1.UpgradeHistoryEntry{{ToVersion: toVer}},
		}
		syncLocalAuditFields(&status, updates)
		if status.StartedAt == nil || !status.StartedAt.Equal(&earlier) {
			t.Fatalf("want StartedAt=%v, got %v", earlier, status.StartedAt)
		}
		if status.CompletedAt == nil || !status.CompletedAt.Equal(&now) {
			t.Fatalf("want CompletedAt=%v, got %v", now, status.CompletedAt)
		}
		if len(status.History) != 1 {
			t.Fatalf("want 1 history entry, got %d", len(status.History))
		}
	})

	t.Run("nil in updates clears pointer fields", func(t *testing.T) {
		status := tupprv1alpha1.KubernetesUpgradeStatus{
			StartedAt:   &earlier,
			CompletedAt: &earlier,
		}
		syncLocalAuditFields(&status, map[string]any{"startedAt": nil, "completedAt": nil})
		if status.StartedAt != nil {
			t.Fatal("want StartedAt cleared")
		}
		if status.CompletedAt != nil {
			t.Fatal("want CompletedAt cleared")
		}
	})
}

func TestEmitPhaseEvent_SkipsIdenticalPhase(t *testing.T) {
	recorder := record.NewFakeRecorder(2)
	r := &Reconciler{Recorder: recorder}
	ku := newKubernetesUpgrade("test")
	r.emitPhaseEvent(ku, tupprv1alpha1.JobPhaseUpgrading, tupprv1alpha1.JobPhaseUpgrading, "")
	select {
	case ev := <-recorder.Events:
		t.Fatalf("did not expect an event, got %q", ev)
	default:
	}
}
