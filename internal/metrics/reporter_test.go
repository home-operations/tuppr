package metrics

import (
	"testing"
)

func TestPhaseToFloat64(t *testing.T) {
	tests := []struct {
		phase string
		want  float64
	}{
		{"Pending", 0},
		{"HealthChecking", 1},
		{"Draining", 2},
		{"Upgrading", 3},
		{"Rebooting", 4},
		{"Completed", 5},
		{"Failed", 6},
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
	// Metric should be set to 0, next timestamp should be set

	// Test nil nextTimestamp when blocked
	mr.RecordMaintenanceWindow(UpgradeTypeTalos, "test2", false, nil)
}

func TestMetricsReporter_CleanupMaintenanceWindowMetrics(t *testing.T) {
	mr := NewReporter()

	nextTimestamp := int64(1234567890)
	mr.RecordMaintenanceWindow(UpgradeTypeTalos, "test", false, &nextTimestamp)

	mr.CleanupUpgradeMetrics(UpgradeTypeTalos, "test")
}
