package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordUpgradePhase(t *testing.T) {
	mr := NewReporter()

	for _, phase := range talosPhases {
		mr.RecordTalosUpgradePhase("talos-test", phase, "worker-01")
		for _, p := range talosPhases {
			var got float64
			if p == phase {
				got = testutil.ToFloat64(talosUpgradePhaseGauge.WithLabelValues("talos-test", p, "worker-01"))
			} else {
				got = testutil.ToFloat64(talosUpgradePhaseGauge.WithLabelValues("talos-test", p, ""))
			}
			want := 0.0
			if p == phase {
				want = 1.0
			}
			if got != want {
				t.Errorf("RecordTalosUpgradePhase(%q): phase label %q = %v, want %v", phase, p, got, want)
			}
		}
	}

	for _, phase := range kubernetesPhases {
		mr.RecordKubernetesUpgradePhase("k8s-test", phase)
		for _, p := range kubernetesPhases {
			got := testutil.ToFloat64(kubernetesUpgradePhaseGauge.WithLabelValues("k8s-test", p))
			want := 0.0
			if p == phase {
				want = 1.0
			}
			if got != want {
				t.Errorf("RecordKubernetesUpgradePhase(%q): phase label %q = %v, want %v", phase, p, got, want)
			}
		}
	}

	// Unknown phase: all labels must be 0 (no active phase)
	mr.RecordTalosUpgradePhase("talos-test", "Unknown", "")
	for _, p := range talosPhases {
		got := testutil.ToFloat64(talosUpgradePhaseGauge.WithLabelValues("talos-test", p, ""))
		if got != 0.0 {
			t.Errorf("RecordTalosUpgradePhase(\"Unknown\"): phase label %q = %v, want 0", p, got)
		}
	}
}

func TestMetricsReporter_CleanupRemovesTimers(t *testing.T) {
	mr := NewReporter()

	mr.StartPhaseTiming(UpgradeTypeTalos, "test-upgrade", "Upgrading")
	mr.StartPhaseTiming(UpgradeTypeTalos, "test-upgrade", "Completed")
	mr.StartPhaseTiming(UpgradeTypeTalos, "other-upgrade", "Upgrading")

	mr.CleanupUpgradeMetrics(UpgradeTypeTalos, "test-upgrade")

	mr.mu.RLock()
	defer mr.mu.RUnlock()

	for key := range mr.startTimes {
		if key == "talos:test-upgrade:Upgrading" || key == "talos:test-upgrade:Completed" {
			t.Fatalf("expected timer %q to be cleaned up", key)
		}
	}
	if _, exists := mr.startTimes["talos:other-upgrade:Upgrading"]; !exists {
		t.Fatal("expected other-upgrade timer to be preserved")
	}
}

func TestMetricsReporter_EndPhaseTimingCleansUp(t *testing.T) {
	mr := NewReporter()

	mr.StartPhaseTiming(UpgradeTypeKubernetes, "k8s-upgrade", "Upgrading")

	mr.mu.RLock()
	if _, exists := mr.startTimes["kubernetes:k8s-upgrade:Upgrading"]; !exists {
		mr.mu.RUnlock()
		t.Fatal("expected timer to exist after StartPhaseTiming")
	}
	mr.mu.RUnlock()

	mr.EndPhaseTiming(UpgradeTypeKubernetes, "k8s-upgrade", "Upgrading")

	mr.mu.RLock()
	defer mr.mu.RUnlock()
	if _, exists := mr.startTimes["kubernetes:k8s-upgrade:Upgrading"]; exists {
		t.Fatal("expected timer to be removed after EndPhaseTiming")
	}
}

func TestMetricsReporter_EndPhaseTimingNoopForMissingTimer(t *testing.T) {
	mr := NewReporter()
	mr.EndPhaseTiming(UpgradeTypeTalos, "nonexistent", "Pending")
}

func TestMetricsReporter_RecordMaintenanceWindow(t *testing.T) {
	mr := NewReporter()

	// Test active window
	mr.RecordMaintenanceWindow(UpgradeTypeTalos, "test", true, nil)

	// Test blocked window
	nextTimestamp := int64(1234567890)
	mr.RecordMaintenanceWindow(UpgradeTypeKubernetes, "k8s-test", false, &nextTimestamp)

	// Test nil nextTimestamp when blocked
	mr.RecordMaintenanceWindow(UpgradeTypeTalos, "test2", false, nil)
}

func TestMetricsReporter_CleanupMaintenanceWindowMetrics(t *testing.T) {
	mr := NewReporter()

	nextTimestamp := int64(1234567890)
	mr.RecordMaintenanceWindow(UpgradeTypeTalos, "test", false, &nextTimestamp)

	mr.CleanupUpgradeMetrics(UpgradeTypeTalos, "test")
}
