package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

const (
	testWindowStartA = "0 2 * * *"
	testWindowStartB = "0 14 * * 0"
	testDuration4h   = "4h0m0s"
	fleet            = "fleet"
	cluster          = "cluster"
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

func TestRecordUpgradeCompleted(t *testing.T) {
	mr := NewReporter()

	before := time.Now().Unix()
	mr.RecordUpgradeCompleted(UpgradeTypeTalos, cluster, ResultSuccess)
	mr.RecordUpgradeCompleted(UpgradeTypeTalos, cluster, ResultSuccess)
	mr.RecordUpgradeCompleted(UpgradeTypeKubernetes, cluster, ResultFailure)
	after := time.Now().Unix()

	if got := testutil.ToFloat64(upgradesCompletedTotal.WithLabelValues(UpgradeTypeTalos, ResultSuccess)); got != 2 {
		t.Errorf("upgrades_completed_total{talos,success} = %v, want 2", got)
	}
	if got := testutil.ToFloat64(upgradesCompletedTotal.WithLabelValues(UpgradeTypeKubernetes, ResultFailure)); got != 1 {
		t.Errorf("upgrades_completed_total{kubernetes,failure} = %v, want 1", got)
	}

	ts := testutil.ToFloat64(upgradeLastCompletionTimestamp.WithLabelValues(UpgradeTypeTalos, cluster, ResultSuccess))
	if int64(ts) < before || int64(ts) > after {
		t.Errorf("last_completion_timestamp = %v, want in [%d, %d]", ts, before, after)
	}

	mr.CleanupUpgradeMetrics(UpgradeTypeTalos, cluster)
	if got := testutil.ToFloat64(upgradeLastCompletionTimestamp.WithLabelValues(UpgradeTypeTalos, cluster, ResultSuccess)); got != 0 {
		t.Errorf("last_completion_timestamp after cleanup = %v, want 0", got)
	}
}

func TestRecordNodeTargets(t *testing.T) {
	mr := NewReporter()

	mr.RecordNodeTargets([]NodeTargetSnapshot{
		{Node: "n1", Role: NodeRoleControlPlane, Kind: UpgradeTypeTalos, Version: "v1.11.0", UpgradeName: fleet},
		{Node: "n2", Role: NodeRoleWorker, Kind: UpgradeTypeTalos, Version: "v1.11.0", UpgradeName: fleet},
		{Node: "n1", Role: NodeRoleControlPlane, Kind: UpgradeTypeKubernetes, Version: "v1.34.0", UpgradeName: cluster},
	})

	if got := testutil.CollectAndCount(nodeTargetVersion); got != 3 {
		t.Errorf("nodeTargetVersion series after first record = %d, want 3", got)
	}
	if got := testutil.ToFloat64(nodeTargetVersion.WithLabelValues("n1", NodeRoleControlPlane, UpgradeTypeTalos, "v1.11.0", fleet)); got != 1 {
		t.Errorf("nodeTargetVersion{n1,talos,fleet} = %v, want 1", got)
	}

	mr.RecordNodeTargets([]NodeTargetSnapshot{
		{Node: "n1", Role: NodeRoleControlPlane, Kind: UpgradeTypeTalos, Version: "v1.12.0", UpgradeName: fleet},
	})
	if got := testutil.CollectAndCount(nodeTargetVersion); got != 1 {
		t.Errorf("nodeTargetVersion series after replacement = %d, want 1", got)
	}
	if got := testutil.ToFloat64(nodeTargetVersion.WithLabelValues("n1", NodeRoleControlPlane, UpgradeTypeTalos, "v1.12.0", fleet)); got != 1 {
		t.Errorf("nodeTargetVersion{n1,talos,v1.12.0} = %v, want 1", got)
	}

	mr.RecordNodeTargets(nil)
	if got := testutil.CollectAndCount(nodeTargetVersion); got != 0 {
		t.Errorf("nodeTargetVersion series after empty record = %d, want 0", got)
	}
}

func TestRecordMaintenanceWindows(t *testing.T) {
	mr := NewReporter()

	mr.RecordMaintenanceWindows([]MaintenanceWindowSnapshot{
		{UpgradeType: UpgradeTypeTalos, Name: fleet, Index: "0", Start: testWindowStartA, Duration: testDuration4h, Timezone: defaultTimezone},
		{UpgradeType: UpgradeTypeTalos, Name: fleet, Index: "1", Start: testWindowStartB, Duration: "2h0m0s", Timezone: "Europe/Paris"},
		{UpgradeType: UpgradeTypeKubernetes, Name: cluster, Index: "0", Start: "0 3 * * *", Duration: "1h0m0s", Timezone: defaultTimezone},
	})

	if got := testutil.CollectAndCount(maintenanceWindowInfo); got != 3 {
		t.Errorf("maintenanceWindowInfo series after first record = %d, want 3", got)
	}
	if got := testutil.ToFloat64(maintenanceWindowInfo.WithLabelValues(UpgradeTypeTalos, fleet, "0", testWindowStartA, testDuration4h, defaultTimezone)); got != 1 {
		t.Errorf("maintenanceWindowInfo{talos,fleet,0} = %v, want 1", got)
	}

	mr.RecordMaintenanceWindows([]MaintenanceWindowSnapshot{
		{UpgradeType: UpgradeTypeTalos, Name: fleet, Index: "0", Start: "30 1 * * *", Duration: testDuration4h, Timezone: defaultTimezone},
	})
	if got := testutil.CollectAndCount(maintenanceWindowInfo); got != 1 {
		t.Errorf("maintenanceWindowInfo series after replacement = %d, want 1", got)
	}
	if got := testutil.ToFloat64(maintenanceWindowInfo.WithLabelValues(UpgradeTypeTalos, fleet, "0", "30 1 * * *", testDuration4h, defaultTimezone)); got != 1 {
		t.Errorf("maintenanceWindowInfo after start change = %v, want 1", got)
	}

	mr.RecordMaintenanceWindows(nil)
	if got := testutil.CollectAndCount(maintenanceWindowInfo); got != 0 {
		t.Errorf("maintenanceWindowInfo series after empty record = %d, want 0", got)
	}

	mr.RecordMaintenanceWindows([]MaintenanceWindowSnapshot{
		{UpgradeType: UpgradeTypeTalos, Name: fleet, Index: "0", Start: testWindowStartA, Duration: testDuration4h, Timezone: defaultTimezone},
		{UpgradeType: UpgradeTypeTalos, Name: fleet, Index: "1", Start: testWindowStartB, Duration: "2h0m0s", Timezone: defaultTimezone},
		{UpgradeType: UpgradeTypeKubernetes, Name: cluster, Index: "0", Start: "0 3 * * *", Duration: "1h0m0s", Timezone: defaultTimezone},
	})
	mr.CleanupUpgradeMetrics(UpgradeTypeTalos, fleet)
	if got := testutil.CollectAndCount(maintenanceWindowInfo); got != 1 {
		t.Errorf("maintenanceWindowInfo series after cleanup of fleet = %d, want 1", got)
	}
	if got := testutil.ToFloat64(maintenanceWindowInfo.WithLabelValues(UpgradeTypeKubernetes, cluster, "0", "0 3 * * *", "1h0m0s", defaultTimezone)); got != 1 {
		t.Errorf("maintenanceWindowInfo{kubernetes,cluster} = %v, want 1 (sibling CR's series should survive cleanup)", got)
	}
	mr.RecordMaintenanceWindows(nil)
}

func TestRecordProgressing(t *testing.T) {
	mr := NewReporter()

	mr.RecordProgressing(UpgradeTypeTalos, "talos", "Upgrading", true)
	if got := testutil.ToFloat64(upgradeProgressing.WithLabelValues(UpgradeTypeTalos, "talos", "Upgrading")); got != 1 {
		t.Errorf("progressing{Upgrading} = %v, want 1", got)
	}

	mr.RecordProgressing(UpgradeTypeTalos, "talos", "WaitingForImage", false)
	if got := testutil.ToFloat64(upgradeProgressing.WithLabelValues(UpgradeTypeTalos, "talos", "WaitingForImage")); got != 0 {
		t.Errorf("progressing{WaitingForImage} = %v, want 0", got)
	}
	if got := testutil.CollectAndCount(upgradeProgressing); got != 1 {
		t.Errorf("expected 1 series after reason change, got %d", got)
	}

	mr.CleanupUpgradeMetrics(UpgradeTypeTalos, "talos")
	if got := testutil.CollectAndCount(upgradeProgressing); got != 0 {
		t.Errorf("expected 0 series after cleanup, got %d", got)
	}
}

func TestTerminalResult(t *testing.T) {
	cases := []struct {
		phase tupprv1alpha1.JobPhase
		want  string
	}{
		{tupprv1alpha1.JobPhaseCompleted, ResultSuccess},
		{tupprv1alpha1.JobPhaseFailed, ResultFailure},
		{tupprv1alpha1.JobPhasePending, ResultFailure},
	}
	for _, c := range cases {
		if got := TerminalResult(c.phase); got != c.want {
			t.Errorf("TerminalResult(%q) = %q, want %q", c.phase, got, c.want)
		}
	}
}
