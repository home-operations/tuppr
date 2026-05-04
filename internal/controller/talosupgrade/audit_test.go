package talosupgrade

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

func TestApplyPhaseAuditFields_Talos(t *testing.T) {
	now := metav1.NewTime(time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC))
	earlier := metav1.NewTime(now.Add(-2 * time.Hour))

	tests := []struct {
		name          string
		status        tupprv1alpha1.TalosUpgradeStatus
		nextPhase     tupprv1alpha1.JobPhase
		targetVersion string
		wantStartedAt any
		wantCompleted any
		wantHistoryOp func(t *testing.T, updates map[string]any)
	}{
		{
			name:          "first transition into active sets startedAt",
			status:        tupprv1alpha1.TalosUpgradeStatus{Phase: tupprv1alpha1.JobPhasePending},
			nextPhase:     tupprv1alpha1.JobPhaseHealthChecking,
			targetVersion: fakeTalosVersion,
			wantStartedAt: now,
		},
		{
			name: "terminal Completed appends history with node snapshot",
			status: tupprv1alpha1.TalosUpgradeStatus{
				Phase:          tupprv1alpha1.JobPhaseUpgrading,
				StartedAt:      &earlier,
				CompletedNodes: []string{fakeNodeA, fakeNodeB},
			},
			nextPhase:     tupprv1alpha1.JobPhaseCompleted,
			targetVersion: fakeTalosVersion,
			wantCompleted: now,
			wantHistoryOp: func(t *testing.T, updates map[string]any) {
				t.Helper()
				h := updates["history"].([]tupprv1alpha1.TalosUpgradeHistoryEntry)
				if len(h) != 1 {
					t.Fatalf("want 1 history entry, got %d", len(h))
				}
				if h[0].ToVersion != fakeTalosVersion || h[0].Phase != tupprv1alpha1.JobPhaseCompleted {
					t.Fatalf("unexpected entry: %+v", h[0])
				}
				if len(h[0].CompletedNodes) != 2 || h[0].CompletedNodes[0] != fakeNodeA {
					t.Fatalf("want CompletedNodes snapshot, got %v", h[0].CompletedNodes)
				}
				if len(h[0].FailedNodes) != 0 {
					t.Fatalf("want empty FailedNodes, got %v", h[0].FailedNodes)
				}
				// Verify snapshot is a copy, not the live slice
				h[0].CompletedNodes[0] = "mutated"
			},
		},
		{
			name: "terminal Failed captures failed node names",
			status: tupprv1alpha1.TalosUpgradeStatus{
				Phase:     tupprv1alpha1.JobPhaseUpgrading,
				StartedAt: &earlier,
				FailedNodes: []tupprv1alpha1.NodeUpgradeStatus{
					{NodeName: "node-c", LastError: "timeout"},
				},
			},
			nextPhase:     tupprv1alpha1.JobPhaseFailed,
			targetVersion: fakeTalosVersion,
			wantCompleted: now,
			wantHistoryOp: func(t *testing.T, updates map[string]any) {
				t.Helper()
				h := updates["history"].([]tupprv1alpha1.TalosUpgradeHistoryEntry)
				if h[0].Phase != tupprv1alpha1.JobPhaseFailed {
					t.Fatalf("want Failed, got %s", h[0].Phase)
				}
				if len(h[0].FailedNodes) != 1 || h[0].FailedNodes[0] != "node-c" {
					t.Fatalf("want FailedNodes=[node-c], got %v", h[0].FailedNodes)
				}
			},
		},
		{
			name: "re-entry to Pending clears timestamps",
			status: tupprv1alpha1.TalosUpgradeStatus{
				Phase:       tupprv1alpha1.JobPhaseCompleted,
				StartedAt:   &earlier,
				CompletedAt: &earlier,
			},
			nextPhase:     tupprv1alpha1.JobPhasePending,
			targetVersion: testV121,
			wantStartedAt: nil,
			wantCompleted: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updates := map[string]any{}
			applyPhaseAuditFields(&tc.status, updates, tc.nextPhase, now, tc.targetVersion)

			if v, ok := tc.wantStartedAt.(metav1.Time); ok {
				got, present := updates["startedAt"]
				if !present {
					t.Fatalf("want startedAt set")
				}
				gotT := got.(metav1.Time)
				if !gotT.Equal(&v) {
					t.Fatalf("startedAt mismatch: %v", got)
				}
			}
			if tc.wantStartedAt == nil {
				if v, present := updates["startedAt"]; present && v != nil {
					t.Fatalf("expected startedAt nil/unset, got %v", v)
				}
			}

			if v, ok := tc.wantCompleted.(metav1.Time); ok {
				got, present := updates["completedAt"]
				if !present {
					t.Fatalf("want completedAt set")
				}
				gotT := got.(metav1.Time)
				if !gotT.Equal(&v) {
					t.Fatalf("completedAt mismatch: %v", got)
				}
			}
			if tc.wantCompleted == nil {
				if v, present := updates["completedAt"]; present && v != nil {
					t.Fatalf("expected completedAt nil/unset, got %v", v)
				}
			}

			if tc.wantHistoryOp != nil {
				tc.wantHistoryOp(t, updates)
			} else if _, present := updates["history"]; present {
				t.Fatalf("did not expect history update")
			}
		})
	}
}

func TestPrependHistoryCapsAtMax_Talos(t *testing.T) {
	base := tupprv1alpha1.TalosUpgradeHistoryEntry{ToVersion: "v1.0.0", Phase: tupprv1alpha1.JobPhaseCompleted}
	history := make([]tupprv1alpha1.TalosUpgradeHistoryEntry, 10)
	for i := range history {
		history[i] = base
	}
	fresh := tupprv1alpha1.TalosUpgradeHistoryEntry{ToVersion: "v2.0.0", Phase: tupprv1alpha1.JobPhaseCompleted}
	result := prependHistory(history, fresh, historyMaxEntries)

	if len(result) != historyMaxEntries {
		t.Fatalf("want history cap %d, got %d", historyMaxEntries, len(result))
	}
	if result[0].ToVersion != "v2.0.0" {
		t.Fatalf("newest entry should be first, got %q", result[0].ToVersion)
	}
}

func TestSetPhase_Talos_WritesTimestampsAndEmitsEvent(t *testing.T) {
	scheme := newTestScheme()
	now := time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC)
	tu := newTalosUpgrade("test", func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Status.Phase = tupprv1alpha1.JobPhaseUpgrading
		started := metav1.NewTime(now.Add(-30 * time.Minute))
		tu.Status.StartedAt = &started
		tu.Status.CompletedNodes = []string{fakeNodeA}
	})
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tu).WithStatusSubresource(tu).Build()
	recorder := record.NewFakeRecorder(4)
	r := &Reconciler{
		Client:          cl,
		Scheme:          scheme,
		MetricsReporter: metrics.NewReporter(),
		Now:             &fixedClock{t: now},
		Recorder:        recorder,
	}

	if err := r.setPhase(context.Background(), tu, tupprv1alpha1.JobPhaseCompleted, "", "done"); err != nil {
		t.Fatalf("setPhase: %v", err)
	}

	var got tupprv1alpha1.TalosUpgrade
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "test"}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.Phase != tupprv1alpha1.JobPhaseCompleted {
		t.Fatalf("want Completed, got %s", got.Status.Phase)
	}
	if got.Status.CompletedAt == nil {
		t.Fatal("want CompletedAt set")
	}
	if len(got.Status.History) != 1 {
		t.Fatalf("want 1 history entry, got %d", len(got.Status.History))
	}
	if got.Status.History[0].CompletedNodes[0] != fakeNodeA {
		t.Fatalf("want node-a in history, got %v", got.Status.History[0].CompletedNodes)
	}

	select {
	case ev := <-recorder.Events:
		if !strings.Contains(ev, "UpgradeCompleted") {
			t.Fatalf("want UpgradeCompleted event, got %q", ev)
		}
	default:
		t.Fatal("expected event to be recorded")
	}
}

func TestSetPhase_Talos_NoRecorderIsSafe(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade("test")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tu).WithStatusSubresource(tu).Build()
	r := &Reconciler{
		Client:          cl,
		Scheme:          scheme,
		MetricsReporter: metrics.NewReporter(),
		Now:             &fixedClock{t: time.Now()},
	}
	if err := r.setPhase(context.Background(), tu, tupprv1alpha1.JobPhaseHealthChecking, "", "ok"); err != nil {
		t.Fatalf("setPhase without recorder must not fail: %v", err)
	}
}

func TestApplyPhaseAuditFields_Talos_EmptyPhaseEdge(t *testing.T) {
	now := metav1.NewTime(time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC))

	t.Run("empty to active sets startedAt", func(t *testing.T) {
		status := tupprv1alpha1.TalosUpgradeStatus{}
		updates := map[string]any{}
		applyPhaseAuditFields(&status, updates, tupprv1alpha1.JobPhaseHealthChecking, now, fakeTalosVersion)
		if _, ok := updates["startedAt"]; !ok {
			t.Fatal("want startedAt set for fresh CR going active")
		}
	})
}

func TestSetPhase_Talos_IdempotentOnRepeatedTerminalCalls(t *testing.T) {
	scheme := newTestScheme()
	now := time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC)
	tu := newTalosUpgrade("test", func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Status.Phase = tupprv1alpha1.JobPhaseUpgrading
		started := metav1.NewTime(now.Add(-30 * time.Minute))
		tu.Status.StartedAt = &started
	})
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tu).WithStatusSubresource(tu).Build()
	r := &Reconciler{
		Client:          cl,
		Scheme:          scheme,
		MetricsReporter: metrics.NewReporter(),
		Now:             &fixedClock{t: now},
	}
	if err := r.setPhase(context.Background(), tu, tupprv1alpha1.JobPhaseCompleted, "", "done"); err != nil {
		t.Fatalf("first setPhase: %v", err)
	}
	if err := r.setPhase(context.Background(), tu, tupprv1alpha1.JobPhaseCompleted, "", "done again"); err != nil {
		t.Fatalf("second setPhase: %v", err)
	}
	var got tupprv1alpha1.TalosUpgrade
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "test"}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Status.History) != 1 {
		t.Fatalf("want exactly 1 history entry after repeat, got %d", len(got.Status.History))
	}
}

func TestSyncLocalAuditFields_Talos(t *testing.T) {
	earlier := metav1.NewTime(time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC))

	t.Run("nil in updates clears pointer fields", func(t *testing.T) {
		status := tupprv1alpha1.TalosUpgradeStatus{
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
