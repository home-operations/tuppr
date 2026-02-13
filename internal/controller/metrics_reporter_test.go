package controller

import (
	"testing"
)

func TestPhaseToFloat64(t *testing.T) {
	tests := []struct {
		phase string
		want  float64
	}{
		{"Pending", 0},
		{"InProgress", 1},
		{"Completed", 2},
		{"Failed", 3},
		{"Unknown", -1},
		{"", -1},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			got := phaseToFloat64(tt.phase)
			if got != tt.want {
				t.Fatalf("phaseToFloat64(%q) = %v, want %v", tt.phase, got, tt.want)
			}
		})
	}
}

func TestMetricsReporter_CleanupRemovesTimers(t *testing.T) {
	mr := NewMetricsReporter()

	mr.StartPhaseTiming(UpgradeTypeTalos, "test-upgrade", "InProgress")
	mr.StartPhaseTiming(UpgradeTypeTalos, "test-upgrade", "Completed")
	mr.StartPhaseTiming(UpgradeTypeTalos, "other-upgrade", "InProgress")

	mr.CleanupUpgradeMetrics(UpgradeTypeTalos, "test-upgrade")

	mr.mu.RLock()
	defer mr.mu.RUnlock()

	for key := range mr.startTimes {
		if key == "talos:test-upgrade:InProgress" || key == "talos:test-upgrade:Completed" {
			t.Fatalf("expected timer %q to be cleaned up", key)
		}
	}
	if _, exists := mr.startTimes["talos:other-upgrade:InProgress"]; !exists {
		t.Fatal("expected other-upgrade timer to be preserved")
	}
}

func TestMetricsReporter_EndPhaseTimingCleansUp(t *testing.T) {
	mr := NewMetricsReporter()

	mr.StartPhaseTiming(UpgradeTypeKubernetes, "k8s-upgrade", "InProgress")

	mr.mu.RLock()
	if _, exists := mr.startTimes["kubernetes:k8s-upgrade:InProgress"]; !exists {
		mr.mu.RUnlock()
		t.Fatal("expected timer to exist after StartPhaseTiming")
	}
	mr.mu.RUnlock()

	mr.EndPhaseTiming(UpgradeTypeKubernetes, "k8s-upgrade", "InProgress")

	mr.mu.RLock()
	defer mr.mu.RUnlock()
	if _, exists := mr.startTimes["kubernetes:k8s-upgrade:InProgress"]; exists {
		t.Fatal("expected timer to be removed after EndPhaseTiming")
	}
}

func TestMetricsReporter_EndPhaseTimingNoopForMissingTimer(t *testing.T) {
	mr := NewMetricsReporter()
	// Should not panic when ending a timer that was never started
	mr.EndPhaseTiming(UpgradeTypeTalos, "nonexistent", "Pending")
}
