package controller

import (
	"testing"
	"time"

	"github.com/home-operations/tuppr/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func window(start string, duration time.Duration, tz string) v1alpha1.WindowSpec {
	return v1alpha1.WindowSpec{
		Start:    start,
		Duration: metav1.Duration{Duration: duration},
		Timezone: tz,
	}
}

func spec(windows ...v1alpha1.WindowSpec) *v1alpha1.MaintenanceWindowSpec {
	return &v1alpha1.MaintenanceWindowSpec{Windows: windows}
}

func TestCheckMaintenanceWindow_NilAndEmpty(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec *v1alpha1.MaintenanceWindowSpec
	}{
		{"nil spec", nil},
		{"empty windows", &v1alpha1.MaintenanceWindowSpec{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := CheckMaintenanceWindow(tc.spec, time.Now())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Allowed {
				t.Fatal("expected allowed")
			}
		})
	}
}

func TestCheckMaintenanceWindow_Boundaries(t *testing.T) {
	// Daily window 02:00-06:00 UTC
	s := spec(window("0 2 * * *", 4*time.Hour, "UTC"))

	tests := []struct {
		name    string
		now     time.Time
		allowed bool
	}{
		{"exactly at start", time.Date(2025, 6, 15, 2, 0, 0, 0, time.UTC), true},
		{"inside window", time.Date(2025, 6, 15, 3, 0, 0, 0, time.UTC), true},
		{"exactly at end (exclusive)", time.Date(2025, 6, 15, 6, 0, 0, 0, time.UTC), false},
		{"outside window", time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := CheckMaintenanceWindow(s, tc.now)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Allowed != tc.allowed {
				t.Fatalf("expected allowed=%v, got %v", tc.allowed, result.Allowed)
			}
		})
	}
}

func TestCheckMaintenanceWindow_ReportsWindowEnd(t *testing.T) {
	s := spec(window("0 2 * * *", 4*time.Hour, "UTC"))
	now := time.Date(2025, 6, 15, 3, 0, 0, 0, time.UTC)

	result, _ := CheckMaintenanceWindow(s, now)
	if result.CurrentWindowEnd == nil {
		t.Fatal("expected CurrentWindowEnd to be set")
	}
	expected := time.Date(2025, 6, 15, 6, 0, 0, 0, time.UTC)
	if !result.CurrentWindowEnd.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, *result.CurrentWindowEnd)
	}
}

func TestCheckMaintenanceWindow_ReportsNextWindowStart(t *testing.T) {
	s := spec(window("0 2 * * *", 4*time.Hour, "UTC"))
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	result, _ := CheckMaintenanceWindow(s, now)
	if result.NextWindowStart == nil {
		t.Fatal("expected NextWindowStart to be set")
	}
	expected := time.Date(2025, 6, 16, 2, 0, 0, 0, time.UTC)
	if !result.NextWindowStart.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, *result.NextWindowStart)
	}
}

func TestCheckMaintenanceWindow_MultipleWindowsUnion(t *testing.T) {
	s := spec(
		window("0 2 * * *", 4*time.Hour, "UTC"),
		window("0 20 * * *", 2*time.Hour, "UTC"),
	)

	// Inside second window only — union allows it
	now := time.Date(2025, 6, 15, 21, 0, 0, 0, time.UTC)
	result, err := CheckMaintenanceWindow(s, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed inside second window")
	}

	// Outside both — reports earliest next
	now = time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	result, _ = CheckMaintenanceWindow(s, now)
	if result.Allowed {
		t.Fatal("expected not allowed outside all windows")
	}
	expected := time.Date(2025, 6, 15, 20, 0, 0, 0, time.UTC)
	if !result.NextWindowStart.Equal(expected) {
		t.Fatalf("expected earliest next %v, got %v", expected, *result.NextWindowStart)
	}
}

func TestCheckMaintenanceWindow_Timezone(t *testing.T) {
	// 2:00 AM Paris time, 4h window
	s := spec(window("0 2 * * *", 4*time.Hour, "Europe/Paris"))

	// 2:30 AM Paris = 0:30 UTC (CEST +2) — inside
	now := time.Date(2025, 6, 15, 0, 30, 0, 0, time.UTC)
	result, err := CheckMaintenanceWindow(s, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed at 2:30 AM Paris")
	}

	// 7:00 AM Paris = 5:00 UTC — outside (window ends 6:00 AM Paris = 4:00 UTC)
	now = time.Date(2025, 6, 15, 5, 0, 0, 0, time.UTC)
	result, _ = CheckMaintenanceWindow(s, now)
	if result.Allowed {
		t.Fatal("expected not allowed at 7:00 AM Paris")
	}
}

func TestCheckMaintenanceWindow_SpansMidnight(t *testing.T) {
	// 22:00 UTC, 6h duration → ends 04:00 next day
	s := spec(window("0 22 * * *", 6*time.Hour, "UTC"))

	now := time.Date(2025, 6, 16, 1, 0, 0, 0, time.UTC)
	result, err := CheckMaintenanceWindow(s, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed inside window spanning midnight")
	}
}

func TestCheckMaintenanceWindow_InvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		spec *v1alpha1.MaintenanceWindowSpec
	}{
		{"invalid cron", spec(window("not a cron", 4*time.Hour, "UTC"))},
		{"invalid timezone", spec(window("0 2 * * *", 4*time.Hour, "Not/Real"))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CheckMaintenanceWindow(tc.spec, time.Now())
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
