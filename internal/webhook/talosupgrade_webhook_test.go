package webhook

import (
	"context"
	"strings"
	"testing"
	"time"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTalosUpgrade(name string, opts ...func(*tupprv1alpha1.TalosUpgrade)) *tupprv1alpha1.TalosUpgrade {
	tu := &tupprv1alpha1.TalosUpgrade{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: tupprv1alpha1.TalosUpgradeSpec{
			Talos: tupprv1alpha1.TalosSpec{
				Version: "v1.11.0",
			},
		},
	}
	for _, opt := range opts {
		opt(tu)
	}
	return tu
}

func withTalosVersion(v string) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.Talos.Version = v
	}
}

func withTalosPhase(phase string) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Status.Phase = phase
	}
}

func withTalosHealthChecks(checks ...tupprv1alpha1.HealthCheckSpec) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.HealthChecks = checks
	}
}

func withTalosTalosctlImage(repo, tag string) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.Talosctl.Image.Repository = repo
		tu.Spec.Talosctl.Image.Tag = tag
	}
}

func withTalosPullPolicy(p corev1.PullPolicy) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.Talosctl.Image.PullPolicy = p
	}
}

func withRebootMode(mode string) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.Policy.RebootMode = mode
	}
}

func withPlacement(p string) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.Policy.Placement = p
	}
}

func withForce(f bool) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.Policy.Force = f
	}
}

func withDebug(d bool) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.Policy.Debug = d
	}
}

func newTalosValidator(objects ...runtime.Object) *TalosUpgradeValidator {
	scheme := runtime.NewScheme()
	_ = tupprv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objects...).
		Build()

	return &TalosUpgradeValidator{
		Client:            c,
		TalosConfigSecret: "talosconfig",
	}
}

func talosConfigSecretWithKey(ns string, data []byte) *corev1.Secret { //nolint:unparam
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "talosconfig",
			Namespace: ns,
		},
		Data: map[string][]byte{
			"config": data,
		},
	}
}

func TestTalosUpgrade_ValidateCreate_ValidResource(t *testing.T) {
	v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
	tu := newTalosUpgrade("test-upgrade")

	_, err := v.ValidateCreate(context.Background(), tu)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestTalosUpgrade_ValidateCreate_MissingSecret(t *testing.T) {
	v := newTalosValidator()
	tu := newTalosUpgrade("test-upgrade")

	_, err := v.ValidateCreate(context.Background(), tu)
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to contain %q, got: %v", "not found", err)
	}
}

func TestTalosUpgrade_ValidateCreate_SecretMissingKey(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "talosconfig", Namespace: "default"},
		Data:       map[string][]byte{"bad-key": validTalosConfig()},
	}
	v := newTalosValidator(secret)
	tu := newTalosUpgrade("test-upgrade")

	_, err := v.ValidateCreate(context.Background(), tu)
	if err == nil {
		t.Fatal("expected error for missing config key")
	}
	if !strings.Contains(err.Error(), "missing required key") {
		t.Errorf("expected error to contain %q, got: %v", "missing required key", err)
	}
}

func TestTalosUpgrade_ValidateCreate_EmptySecretData(t *testing.T) {
	v := newTalosValidator(talosConfigSecretWithKey("default", []byte{}))
	tu := newTalosUpgrade("test-upgrade")

	_, err := v.ValidateCreate(context.Background(), tu)
	if err == nil {
		t.Fatal("expected error for empty secret data")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected error to contain %q, got: %v", "empty", err)
	}
}

func TestTalosUpgrade_ValidateCreate_InvalidTalosConfig(t *testing.T) {
	v := newTalosValidator(talosConfigSecretWithKey("default", []byte("{not yaml")))
	tu := newTalosUpgrade("test-upgrade")

	_, err := v.ValidateCreate(context.Background(), tu)
	if err == nil {
		t.Fatal("expected error for invalid talosconfig")
	}
	if !strings.Contains(err.Error(), "cannot be parsed") {
		t.Errorf("expected error to contain %q, got: %v", "cannot be parsed", err)
	}
}

func TestTalosUpgrade_ValidateCreate_NoContextsInConfig(t *testing.T) {
	noCtx := []byte("context: \"\"\ncontexts: {}\n")
	v := newTalosValidator(talosConfigSecretWithKey("default", noCtx))
	tu := newTalosUpgrade("test-upgrade")

	_, err := v.ValidateCreate(context.Background(), tu)
	if err == nil {
		t.Fatal("expected error for empty contexts")
	}
	if !strings.Contains(err.Error(), "no contexts defined") {
		t.Errorf("expected error to contain %q, got: %v", "no contexts defined", err)
	}
}

func TestTalosUpgrade_ValidateCreate_InvalidVersionFormats(t *testing.T) {
	cases := []struct {
		name    string
		version string
	}{
		{"empty", ""},
		{"no v prefix", "1.11.0"},
		{"major.minor only", "v1.11"},
		{"garbage", "latest"},
		{"trailing space", "v1.11.0 "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
			tu := newTalosUpgrade("test", withTalosVersion(tc.version))

			_, err := v.ValidateCreate(context.Background(), tu)
			if err == nil {
				t.Errorf("expected error for version %q", tc.version)
			}
		})
	}
}

func TestTalosUpgrade_ValidateCreate_ValidVersionFormats(t *testing.T) {
	versions := []string{"v1.11.0", "v1.11.0-alpha.1", "v2.0.0", "v0.1.0"}

	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
			tu := newTalosUpgrade("test", withTalosVersion(version))

			_, err := v.ValidateCreate(context.Background(), tu)
			if err != nil {
				t.Errorf("expected no error for version %q, got: %v", version, err)
			}
		})
	}
}

func TestTalosUpgrade_ValidateCreate_SingletonRejectsSecondResource(t *testing.T) {
	existing := newTalosUpgrade("existing")
	v := newTalosValidator(existing, talosConfigSecretWithKey("default", validTalosConfig()))

	tu := newTalosUpgrade("second")

	_, err := v.ValidateCreate(context.Background(), tu)
	if err == nil {
		t.Fatal("expected singleton violation")
	}
	if !strings.Contains(err.Error(), "only one TalosUpgrade") {
		t.Errorf("expected singleton error, got: %v", err)
	}
}

func TestTalosUpgrade_ValidateUpdate_SingletonAllowsSameResource(t *testing.T) {
	existing := newTalosUpgrade("my-upgrade")
	v := newTalosValidator(existing, talosConfigSecretWithKey("default", validTalosConfig()))

	old := newTalosUpgrade("my-upgrade")
	updated := newTalosUpgrade("my-upgrade", withTalosVersion("v1.12.0"))

	_, err := v.ValidateUpdate(context.Background(), old, updated)
	if err != nil {
		t.Fatalf("update of same resource should succeed, got: %v", err)
	}
}

// --- ValidateUpdate: in-progress protection ---

func TestTalosUpgrade_ValidateUpdate_RejectsSpecChangeWhileInProgress(t *testing.T) {
	old := newTalosUpgrade("test", withTalosPhase(constants.PhaseInProgress))
	updated := newTalosUpgrade("test",
		withTalosPhase(constants.PhaseInProgress),
		withTalosVersion("v1.12.0"),
	)

	v := newTalosValidator(old, talosConfigSecretWithKey("default", validTalosConfig()))

	_, err := v.ValidateUpdate(context.Background(), old, updated)
	if err == nil {
		t.Fatal("expected error when updating spec during in-progress upgrade")
	}
	if !strings.Contains(err.Error(), "cannot update spec while upgrade is in progress") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestTalosUpgrade_ValidateUpdate_AllowsNoSpecChangeWhileInProgress(t *testing.T) {
	old := newTalosUpgrade("test", withTalosPhase(constants.PhaseInProgress))
	updated := newTalosUpgrade("test", withTalosPhase(constants.PhaseInProgress))

	v := newTalosValidator(old, talosConfigSecretWithKey("default", validTalosConfig()))

	_, err := v.ValidateUpdate(context.Background(), old, updated)
	if err != nil {
		t.Fatalf("expected no error when spec unchanged, got: %v", err)
	}
}

func TestTalosUpgrade_ValidateUpdate_AllowsSpecChangeWhenNotInProgress(t *testing.T) {
	for _, phase := range []string{constants.PhasePending, constants.PhaseCompleted, constants.PhaseFailed, ""} {
		t.Run("phase_"+phase, func(t *testing.T) {
			old := newTalosUpgrade("test", withTalosPhase(phase))
			updated := newTalosUpgrade("test", withTalosPhase(phase), withTalosVersion("v1.12.0"))

			v := newTalosValidator(old, talosConfigSecretWithKey("default", validTalosConfig()))

			_, err := v.ValidateUpdate(context.Background(), old, updated)
			if err != nil {
				t.Fatalf("spec change should be allowed in phase %q, got: %v", phase, err)
			}
		})
	}
}

func TestTalosUpgrade_ValidateDelete_WarnsWhenInProgress(t *testing.T) {
	tu := newTalosUpgrade("test", withTalosPhase(constants.PhaseInProgress))
	v := newTalosValidator()

	warnings, err := v.ValidateDelete(context.Background(), tu)
	if err != nil {
		t.Fatalf("delete should not error, got: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning when deleting in-progress upgrade")
	}
	if !strings.Contains(warnings[0], "inconsistent state") {
		t.Errorf("expected warning about inconsistent state, got: %q", warnings[0])
	}
}

func TestTalosUpgrade_ValidateDelete_NoWarningWhenIdle(t *testing.T) {
	for _, phase := range []string{constants.PhasePending, constants.PhaseCompleted, constants.PhaseFailed, ""} {
		t.Run("phase_"+phase, func(t *testing.T) {
			tu := newTalosUpgrade("test", withTalosPhase(phase))
			v := newTalosValidator()

			warnings, err := v.ValidateDelete(context.Background(), tu)
			if err != nil {
				t.Fatalf("delete should not error, got: %v", err)
			}
			if len(warnings) != 0 {
				t.Fatalf("expected no warnings for phase %q, got: %v", phase, warnings)
			}
		})
	}
}

func TestTalosUpgrade_ValidateCreate_HealthCheckValidation(t *testing.T) {
	validCheck := tupprv1alpha1.HealthCheckSpec{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Expr:       "object.status.readyReplicas == object.status.replicas",
	}

	t.Run("valid", func(t *testing.T) {
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withTalosHealthChecks(validCheck))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("empty apiVersion", func(t *testing.T) {
		check := validCheck
		check.APIVersion = ""
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withTalosHealthChecks(check))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty kind", func(t *testing.T) {
		check := validCheck
		check.Kind = ""
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withTalosHealthChecks(check))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty expr", func(t *testing.T) {
		check := validCheck
		check.Expr = ""
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withTalosHealthChecks(check))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("negative timeout", func(t *testing.T) {
		check := validCheck
		d := metav1.Duration{Duration: -1 * time.Second}
		check.Timeout = &d
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withTalosHealthChecks(check))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err == nil {
			t.Fatal("expected error for negative timeout")
		}
	})

	t.Run("multiple checks with second invalid", func(t *testing.T) {
		bad := tupprv1alpha1.HealthCheckSpec{APIVersion: "", Kind: "Node", Expr: "true"}
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withTalosHealthChecks(validCheck, bad))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err == nil {
			t.Fatal("expected error for invalid second check")
		}
		if !strings.Contains(err.Error(), "healthChecks[1]") {
			t.Errorf("expected error to reference healthChecks[1], got: %v", err)
		}
	})
}

// --- Talosctl image validation ---

func TestTalosUpgrade_ValidateCreate_TalosctlImagePartialSpec(t *testing.T) {
	t.Run("repo without tag", func(t *testing.T) {
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withTalosTalosctlImage("ghcr.io/custom/talosctl", ""))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err == nil {
			t.Fatal("expected error when repo set without tag")
		}
		if !strings.Contains(err.Error(), "must be specified together") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("tag without repo", func(t *testing.T) {
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withTalosTalosctlImage("", "v1.11.0"))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err == nil {
			t.Fatal("expected error when tag set without repo")
		}
	})

	t.Run("both specified", func(t *testing.T) {
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withTalosTalosctlImage("ghcr.io/custom/talosctl", "v1.11.0"))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})
}

func TestTalosUpgrade_ValidateCreate_InvalidPullPolicy(t *testing.T) {
	v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
	tu := newTalosUpgrade("test", withTalosPullPolicy(corev1.PullPolicy("BadPolicy")))

	_, err := v.ValidateCreate(context.Background(), tu)
	if err == nil {
		t.Fatal("expected error for invalid pull policy")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected error to mention 'invalid', got: %v", err)
	}
}

func TestTalosUpgrade_ValidateCreate_RebootMode(t *testing.T) {
	t.Run("valid default", func(t *testing.T) {
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withRebootMode("default"))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("valid powercycle", func(t *testing.T) {
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withRebootMode("powercycle"))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("invalid mode", func(t *testing.T) {
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withRebootMode("hard-reset"))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err == nil {
			t.Fatal("expected error for invalid reboot mode")
		}
		if !strings.Contains(err.Error(), "rebootMode") {
			t.Errorf("expected error to mention rebootMode, got: %v", err)
		}
	})
}

func TestTalosUpgrade_ValidateCreate_Placement(t *testing.T) {
	t.Run("valid hard", func(t *testing.T) {
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withPlacement("hard"))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("valid soft", func(t *testing.T) {
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withPlacement("soft"))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("invalid placement", func(t *testing.T) {
		v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
		tu := newTalosUpgrade("test", withPlacement("medium"))

		_, err := v.ValidateCreate(context.Background(), tu)
		if err == nil {
			t.Fatal("expected error for invalid placement")
		}
		if !strings.Contains(err.Error(), "placement") {
			t.Errorf("expected error to mention placement, got: %v", err)
		}
	})
}

func TestTalosUpgrade_Warnings_ForceUpgrade(t *testing.T) {
	v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
	tu := newTalosUpgrade("test", withForce(true))

	warnings, err := v.ValidateCreate(context.Background(), tu)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "Force upgrade enabled") {
		t.Errorf("expected force warning, got: %v", warnings)
	}
}

func TestTalosUpgrade_Warnings_PowercycleReboot(t *testing.T) {
	v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
	tu := newTalosUpgrade("test", withRebootMode("powercycle"))

	warnings, err := v.ValidateCreate(context.Background(), tu)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "Powercycle reboot mode") {
		t.Errorf("expected powercycle warning, got: %v", warnings)
	}
}

func TestTalosUpgrade_Warnings_DebugMode(t *testing.T) {
	v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
	tu := newTalosUpgrade("test", withDebug(true))

	warnings, err := v.ValidateCreate(context.Background(), tu)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "Debug mode enabled") {
		t.Errorf("expected debug warning, got: %v", warnings)
	}
}

func TestTalosUpgrade_Warnings_SoftPlacement(t *testing.T) {
	v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
	tu := newTalosUpgrade("test", withPlacement("soft"))

	warnings, err := v.ValidateCreate(context.Background(), tu)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "Soft placement") {
		t.Errorf("expected soft placement warning, got: %v", warnings)
	}
}

func TestTalosUpgrade_Warnings_PreReleaseVersion(t *testing.T) {
	v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
	tu := newTalosUpgrade("test", withTalosVersion("v1.11.0-alpha.1"))

	warnings, err := v.ValidateCreate(context.Background(), tu)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "pre-release") {
		t.Errorf("expected pre-release warning, got: %v", warnings)
	}
}

func TestTalosUpgrade_Warnings_NoTalosctlVersion(t *testing.T) {
	v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
	tu := newTalosUpgrade("test")

	warnings, err := v.ValidateCreate(context.Background(), tu)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "No talosctl version specified") {
		t.Errorf("expected talosctl version warning, got: %v", warnings)
	}
}

func TestTalosUpgrade_Warnings_HealthCheckNoTimeout(t *testing.T) {
	check := tupprv1alpha1.HealthCheckSpec{
		APIVersion: "v1",
		Kind:       "Node",
		Expr:       "true",
	}
	v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
	tu := newTalosUpgrade("test", withTalosHealthChecks(check))

	warnings, err := v.ValidateCreate(context.Background(), tu)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "no timeout") {
		t.Errorf("expected timeout warning, got: %v", warnings)
	}
}

func TestTalosUpgrade_Warnings_NoWarningsForSafeDefaults(t *testing.T) {
	timeout := metav1.Duration{Duration: 5 * time.Minute}
	check := tupprv1alpha1.HealthCheckSpec{
		APIVersion: "v1",
		Kind:       "Node",
		Expr:       "true",
		Timeout:    &timeout,
	}
	v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
	tu := newTalosUpgrade("test",
		withTalosTalosctlImage("ghcr.io/siderolabs/talosctl", "v1.11.0"),
		withRebootMode("default"),
		withPlacement("hard"),
		withTalosHealthChecks(check),
	)

	warnings, err := v.ValidateCreate(context.Background(), tu)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for safe config, got: %v", warnings)
	}
}

func TestTalosUpgrade_ValidateCreate_MaintenanceWindowValid(t *testing.T) {
	v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
	tu := newTalosUpgrade("test", func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.MaintenanceWindow = &tupprv1alpha1.MaintenanceWindowSpec{
			Windows: []tupprv1alpha1.WindowSpec{
				{
					Start:    "0 2 * * 0",
					Duration: metav1.Duration{Duration: 4 * time.Hour},
					Timezone: "UTC",
				},
			},
		}
	})

	warnings, err := v.ValidateCreate(context.Background(), tu)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// Check that there are no maintenance window-specific warnings (other warnings like talosctl version are OK)
	for _, w := range warnings {
		if strings.Contains(strings.ToLower(w), "maintenance") || strings.Contains(strings.ToLower(w), "window") {
			t.Errorf("unexpected maintenance window warning: %s", w)
		}
	}
}

func TestTalosUpgrade_ValidateCreate_MaintenanceWindowInvalidCron(t *testing.T) {
	v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
	tu := newTalosUpgrade("test", func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.MaintenanceWindow = &tupprv1alpha1.MaintenanceWindowSpec{
			Windows: []tupprv1alpha1.WindowSpec{
				{
					Start:    "not a cron",
					Duration: metav1.Duration{Duration: 4 * time.Hour},
					Timezone: "UTC",
				},
			},
		}
	})

	_, err := v.ValidateCreate(context.Background(), tu)
	if err == nil {
		t.Fatal("expected error for invalid cron")
	}
	if !strings.Contains(err.Error(), "maintenanceWindow") {
		t.Errorf("expected error to mention maintenanceWindow, got: %v", err)
	}
}

func TestTalosUpgrade_ValidateCreate_MaintenanceWindowShortDuration(t *testing.T) {
	v := newTalosValidator(talosConfigSecretWithKey("default", validTalosConfig()))
	tu := newTalosUpgrade("test", func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.MaintenanceWindow = &tupprv1alpha1.MaintenanceWindowSpec{
			Windows: []tupprv1alpha1.WindowSpec{
				{
					Start:    "0 2 * * *",
					Duration: metav1.Duration{Duration: 30 * time.Minute},
					Timezone: "UTC",
				},
			},
		}
	})

	warnings, err := v.ValidateCreate(context.Background(), tu)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Count maintenance window warnings (not other warnings like talosctl version)
	maintenanceWarnings := 0
	for _, w := range warnings {
		if strings.Contains(strings.ToLower(w), "maintenance") || strings.Contains(strings.ToLower(w), "window") {
			maintenanceWarnings++
		}
	}
	if maintenanceWarnings != 1 {
		t.Fatalf("expected 1 maintenance window warning for short duration, got %d: %v", maintenanceWarnings, warnings)
	}
}
