package webhook

import (
	"testing"
	"time"

	"github.com/home-operations/tuppr/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func mwSpec(windows ...v1alpha1.WindowSpec) *v1alpha1.MaintenanceWindowSpec {
	return &v1alpha1.MaintenanceWindowSpec{Windows: windows}
}

func mwWindow(start string, duration time.Duration, tz string) v1alpha1.WindowSpec {
	return v1alpha1.WindowSpec{
		Start:    start,
		Duration: metav1.Duration{Duration: duration},
		Timezone: tz,
	}
}

func TestValidateMaintenanceWindows_NilAndEmpty(t *testing.T) {
	for _, s := range []*v1alpha1.MaintenanceWindowSpec{nil, {}} {
		warnings, err := validateMaintenanceWindows(s)
		if err != nil || len(warnings) > 0 {
			t.Fatalf("expected no error/warnings for nil or empty spec, got err=%v warnings=%v", err, warnings)
		}
	}
}

func TestValidateMaintenanceWindows_Valid(t *testing.T) {
	warnings, err := validateMaintenanceWindows(mwSpec(mwWindow("0 2 * * 0", 4*time.Hour, "UTC")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) > 0 {
		t.Fatalf("expected no warnings, got: %v", warnings)
	}
}

func TestValidateMaintenanceWindows_Rejections(t *testing.T) {
	tests := []struct {
		name string
		spec *v1alpha1.MaintenanceWindowSpec
	}{
		{"invalid cron", mwSpec(mwWindow("not valid", 4*time.Hour, "UTC"))},
		{"invalid timezone", mwSpec(mwWindow("0 2 * * *", 4*time.Hour, "Not/Real"))},
		{"zero duration", mwSpec(mwWindow("0 2 * * *", 0, "UTC"))},
		{"negative duration", mwSpec(mwWindow("0 2 * * *", -time.Hour, "UTC"))},
		{"exceeds 7 days", mwSpec(mwWindow("0 2 * * *", 169*time.Hour, "UTC"))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateMaintenanceWindows(tc.spec); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestValidateMaintenanceWindows_DurationBoundaries(t *testing.T) {
	// Exactly 7 days — should pass
	if _, err := validateMaintenanceWindows(mwSpec(mwWindow("0 2 * * *", 168*time.Hour, "UTC"))); err != nil {
		t.Fatalf("expected 168h to be accepted, got: %v", err)
	}

	// Under 1 hour — should warn
	warnings, err := validateMaintenanceWindows(mwSpec(mwWindow("0 2 * * *", 30*time.Minute, "UTC")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for short duration, got %d", len(warnings))
	}
}
