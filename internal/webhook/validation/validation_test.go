package validation

import (
	"context"
	"testing"
	"time"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Helper to create a fake client with objects
func newFakeClient(objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = tupprv1alpha1.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

// Helper to generate valid Talos config for tests
func validTalosConfig() []byte {
	return []byte(`context: default
contexts:
  default:
    endpoints:
      - https://10.0.0.1:50000
    ca: ""
    crt: ""
    key: ""
`)
}

func TestValidateTalosConfigSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	ns := "default"
	name := "talos-secret"

	tests := []struct {
		name        string
		secretName  string
		objs        []client.Object
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid secret with primary key",
			secretName: name,
			objs: []client.Object{&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Data: map[string][]byte{
					constants.TalosSecretKey: validTalosConfig(),
				},
			}},
			wantErr: false,
		},
		{
			name:       "valid secret with fallback key 'config'",
			secretName: name,
			objs: []client.Object{&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Data: map[string][]byte{
					"config": validTalosConfig(),
				},
			}},
			wantErr: false,
		},
		{
			name:        "empty secret name",
			secretName:  "",
			wantErr:     true,
			errContains: "talos config secret name is empty",
		},
		{
			name:        "secret not found",
			secretName:  "missing",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:       "missing required keys",
			secretName: name,
			objs: []client.Object{&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Data: map[string][]byte{
					"other": []byte("data"),
				},
			}},
			wantErr:     true,
			errContains: "missing required key",
		},
		{
			name:       "empty secret data (no keys)",
			secretName: name,
			objs: []client.Object{&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Data:       map[string][]byte{}, // completely empty data map
			}},
			wantErr:     true,
			errContains: "missing required key", // Logic hits key check first for empty map
		},
		{
			name:       "empty key content",
			secretName: name,
			objs: []client.Object{&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Data: map[string][]byte{
					constants.TalosSecretKey: {},
				},
			}},
			wantErr: true,
			// [FIXED] The code returns "data is empty" when the key exists but has 0 length.
			errContains: "data is empty",
		},
		{
			name:       "invalid yaml",
			secretName: name,
			objs: []client.Object{&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Data: map[string][]byte{
					constants.TalosSecretKey: []byte(":::invalid:::yaml"),
				},
			}},
			wantErr:     true,
			errContains: "cannot be parsed",
		},
		{
			name:       "no contexts",
			secretName: name,
			objs: []client.Object{&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Data: map[string][]byte{
					constants.TalosSecretKey: []byte("contexts: {}"),
				},
			}},
			wantErr:     true,
			errContains: "has no contexts defined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newFakeClient(tt.objs...)
			_, err := ValidateTalosConfigSecret(ctx, c, tt.secretName, ns)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.errContains))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestValidateHealthChecks(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name        string
		checks      []tupprv1alpha1.HealthCheckSpec
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid check",
			wantErr: false,
			checks: []tupprv1alpha1.HealthCheckSpec{{
				APIVersion: "v1", Kind: "Node", Expr: "true",
			}},
		},
		{
			name:        "missing apiVersion",
			wantErr:     true,
			errContains: "apiVersion cannot be empty",
			checks: []tupprv1alpha1.HealthCheckSpec{{
				Kind: "Node", Expr: "true",
			}},
		},
		{
			name:        "missing kind",
			wantErr:     true,
			errContains: "kind cannot be empty",
			checks: []tupprv1alpha1.HealthCheckSpec{{
				APIVersion: "v1", Expr: "true",
			}},
		},
		{
			name:        "missing expr",
			wantErr:     true,
			errContains: "expr cannot be empty",
			checks: []tupprv1alpha1.HealthCheckSpec{{
				APIVersion: "v1", Kind: "Node",
			}},
		},
		{
			name:        "negative timeout",
			wantErr:     true,
			errContains: "timeout must be positive",
			checks: []tupprv1alpha1.HealthCheckSpec{{
				APIVersion: "v1", Kind: "Node", Expr: "true",
				Timeout: &metav1.Duration{Duration: -1 * time.Second},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHealthChecks(tt.checks)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.errContains))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestValidateTalosctlSpec(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name        string
		spec        tupprv1alpha1.TalosctlSpec
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid empty",
			spec:    tupprv1alpha1.TalosctlSpec{},
			wantErr: false,
		},
		{
			name: "valid repo and tag",
			spec: tupprv1alpha1.TalosctlSpec{
				Image: tupprv1alpha1.TalosctlImageSpec{
					Repository: "ghcr.io/siderolabs/talosctl",
					Tag:        "v1.5.0",
				},
			},
			wantErr: false,
		},
		{
			name: "valid repo only (default tag allowed by util, warning generated elsewhere)",
			spec: tupprv1alpha1.TalosctlSpec{
				Image: tupprv1alpha1.TalosctlImageSpec{
					Repository: "ghcr.io/siderolabs/talosctl",
				},
			},
			wantErr: false,
		},
		{
			name: "tag without repo",
			spec: tupprv1alpha1.TalosctlSpec{
				Image: tupprv1alpha1.TalosctlImageSpec{
					Tag: "v1.5.0",
				},
			},
			wantErr:     true,
			errContains: "cannot be set without a repository",
		},
		{
			name: "invalid pull policy",
			spec: tupprv1alpha1.TalosctlSpec{
				Image: tupprv1alpha1.TalosctlImageSpec{
					PullPolicy: "InvalidPolicy",
				},
			},
			wantErr:     true,
			errContains: "invalid pullPolicy",
		},
		{
			name: "valid pull policy",
			spec: tupprv1alpha1.TalosctlSpec{
				Image: tupprv1alpha1.TalosctlImageSpec{
					PullPolicy: corev1.PullAlways,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTalosctlSpec(tt.spec)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.errContains))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestValidateVersionFormat(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"valid standard", "v1.30.0", false},
		{"valid rc", "v1.30.0-rc.1", false},
		{"valid alpha", "v1.30.0-alpha.0", false},
		{"empty", "", true},
		{"no v prefix", "1.30.0", true},
		{"short", "v1.30", true},
		{"garbage", "latest", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersionFormat(tt.version)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestValidateSingleton(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	existing := &tupprv1alpha1.TalosUpgrade{
		ObjectMeta: metav1.ObjectMeta{Name: "existing"},
	}
	c := newFakeClient(existing)

	err := ValidateSingleton(ctx, c, "TalosUpgrade", "existing", &tupprv1alpha1.TalosUpgradeList{})
	g.Expect(err).NotTo(HaveOccurred())

	err = ValidateSingleton(ctx, c, "TalosUpgrade", "new-one", &tupprv1alpha1.TalosUpgradeList{})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("only one TalosUpgrade resource is allowed"))
}

func TestValidateUpdateInProgress(t *testing.T) {
	g := NewWithT(t)

	specA := tupprv1alpha1.TalosSpec{Version: "v1.0.0"}
	specB := tupprv1alpha1.TalosSpec{Version: "v1.1.0"}

	// Case 1: Not in progress -> Change allowed
	err := ValidateUpdateInProgress(tupprv1alpha1.JobPhasePending, specA, specB)
	g.Expect(err).NotTo(HaveOccurred())

	// Case 2: In progress -> Same spec -> Allowed
	err = ValidateUpdateInProgress(tupprv1alpha1.JobPhaseUpgrading, specA, specA)
	g.Expect(err).NotTo(HaveOccurred())

	// Case 3: In progress -> Different spec -> Denied
	err = ValidateUpdateInProgress(tupprv1alpha1.JobPhaseUpgrading, specA, specB)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("cannot update spec while upgrade is in progress"))
}

func TestGenerateCommonWarnings(t *testing.T) {
	g := NewWithT(t)

	// Case 1: Clean
	warnings := GenerateCommonWarnings("v1.0.0", nil, "v1.0.0")
	g.Expect(warnings).To(BeEmpty())

	// Case 2: Pre-release
	warnings = GenerateCommonWarnings("v1.0.0-rc.1", nil, "v1.0.0")
	g.Expect(warnings).To(ContainElement(ContainSubstring("pre-release")))

	// Case 3: Missing Talosctl Tag (Auto-detect)
	warnings = GenerateCommonWarnings("v1.0.0", nil, "")
	g.Expect(warnings).To(ContainElement(ContainSubstring("No talosctl version specified")))

	// Case 4: No timeout
	check := tupprv1alpha1.HealthCheckSpec{Expr: "true"} // timeout is nil
	warnings = GenerateCommonWarnings("v1.0.0", []tupprv1alpha1.HealthCheckSpec{check}, "v1.0.0")
	g.Expect(warnings).To(ContainElement(ContainSubstring("no timeout specified")))
}

func mwSpec(windows ...tupprv1alpha1.WindowSpec) *tupprv1alpha1.MaintenanceSpec {
	return &tupprv1alpha1.MaintenanceSpec{Windows: windows}
}

func mwWindow(start string, duration time.Duration, tz string) tupprv1alpha1.WindowSpec {
	return tupprv1alpha1.WindowSpec{
		Start:    start,
		Duration: metav1.Duration{Duration: duration},
		Timezone: tz,
	}
}

func TestValidateMaintenanceWindows_NilAndEmpty(t *testing.T) {
	for _, s := range []*tupprv1alpha1.MaintenanceSpec{nil, {}} {
		warnings, err := ValidateMaintenanceWindows(s)
		if err != nil || len(warnings) > 0 {
			t.Fatalf("expected no error/warnings for nil or empty spec, got err=%v warnings=%v", err, warnings)
		}
	}
}

func TestValidateMaintenanceWindows_Valid(t *testing.T) {
	warnings, err := ValidateMaintenanceWindows(mwSpec(mwWindow("0 2 * * 0", 4*time.Hour, "UTC")))
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
		spec *tupprv1alpha1.MaintenanceSpec
	}{
		{"invalid cron", mwSpec(mwWindow("not valid", 4*time.Hour, "UTC"))},
		{"invalid timezone", mwSpec(mwWindow("0 2 * * *", 4*time.Hour, "Not/Real"))},
		{"zero duration", mwSpec(mwWindow("0 2 * * *", 0, "UTC"))},
		{"negative duration", mwSpec(mwWindow("0 2 * * *", -time.Hour, "UTC"))},
		{"exceeds 7 days", mwSpec(mwWindow("0 2 * * *", 169*time.Hour, "UTC"))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ValidateMaintenanceWindows(tc.spec); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestValidateMaintenanceWindows_DurationBoundaries(t *testing.T) {
	// Exactly 7 days — should pass
	if _, err := ValidateMaintenanceWindows(mwSpec(mwWindow("0 2 * * *", 168*time.Hour, "UTC"))); err != nil {
		t.Fatalf("expected 168h to be accepted, got: %v", err)
	}

	// Under 1 hour — should warn
	warnings, err := ValidateMaintenanceWindows(mwSpec(mwWindow("0 2 * * *", 30*time.Minute, "UTC")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for short duration, got %d", len(warnings))
	}
}
