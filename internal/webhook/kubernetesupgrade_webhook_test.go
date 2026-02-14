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

func newKubernetesUpgrade(name string, opts ...func(*tupprv1alpha1.KubernetesUpgrade)) *tupprv1alpha1.KubernetesUpgrade {
	ku := &tupprv1alpha1.KubernetesUpgrade{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: tupprv1alpha1.KubernetesUpgradeSpec{
			Kubernetes: tupprv1alpha1.KubernetesSpec{
				Version: "v1.34.0",
			},
		},
	}
	for _, opt := range opts {
		opt(ku)
	}
	return ku
}

func withKubernetesVersion(v string) func(*tupprv1alpha1.KubernetesUpgrade) {
	return func(ku *tupprv1alpha1.KubernetesUpgrade) {
		ku.Spec.Kubernetes.Version = v
	}
}

func withK8sPhase(phase string) func(*tupprv1alpha1.KubernetesUpgrade) {
	return func(ku *tupprv1alpha1.KubernetesUpgrade) {
		ku.Status.Phase = phase
	}
}

func withK8sHealthChecks(checks ...tupprv1alpha1.HealthCheckSpec) func(*tupprv1alpha1.KubernetesUpgrade) {
	return func(ku *tupprv1alpha1.KubernetesUpgrade) {
		ku.Spec.HealthChecks = checks
	}
}

func withK8sTalosctlImage(repo, tag string) func(*tupprv1alpha1.KubernetesUpgrade) {
	return func(ku *tupprv1alpha1.KubernetesUpgrade) {
		ku.Spec.Talosctl.Image.Repository = repo
		ku.Spec.Talosctl.Image.Tag = tag
	}
}

func withK8sPullPolicy(p corev1.PullPolicy) func(*tupprv1alpha1.KubernetesUpgrade) {
	return func(ku *tupprv1alpha1.KubernetesUpgrade) {
		ku.Spec.Talosctl.Image.PullPolicy = p
	}
}

func newK8sValidator(objects ...runtime.Object) *KubernetesUpgradeValidator {
	scheme := runtime.NewScheme()
	_ = tupprv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objects...).
		Build()

	return &KubernetesUpgradeValidator{
		Client:            c,
		TalosConfigSecret: "talosconfig",
		Namespace:         "default",
	}
}

func talosConfigSecret(ns string, data []byte) *corev1.Secret { //nolint:unparam
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "talosconfig",
			Namespace: ns,
		},
		Data: map[string][]byte{
			constants.TalosSecretKey: data,
		},
	}
}

func TestKubernetesUpgrade_ValidateCreate_ValidResource(t *testing.T) {
	v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
	ku := newKubernetesUpgrade("test-upgrade")

	warnings, err := v.ValidateCreate(context.Background(), ku)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "No talosctl version specified") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about missing talosctl version")
	}
}

func TestKubernetesUpgrade_ValidateCreate_MissingSecret(t *testing.T) {
	v := newK8sValidator()
	ku := newKubernetesUpgrade("test-upgrade")

	_, err := v.ValidateCreate(context.Background(), ku)
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to contain %q, got: %v", "not found", err)
	}
}

func TestKubernetesUpgrade_ValidateCreate_SecretMissingKey(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "talosconfig", Namespace: "default"},
		Data:       map[string][]byte{"wrong-key": validTalosConfig()},
	}
	v := newK8sValidator(secret)
	ku := newKubernetesUpgrade("test-upgrade")

	_, err := v.ValidateCreate(context.Background(), ku)
	if err == nil {
		t.Fatal("expected error for missing secret key")
	}
	if !strings.Contains(err.Error(), "missing required key") {
		t.Errorf("expected error to contain %q, got: %v", "missing required key", err)
	}
}

func TestKubernetesUpgrade_ValidateCreate_EmptySecretData(t *testing.T) {
	v := newK8sValidator(talosConfigSecret("default", []byte{}))
	ku := newKubernetesUpgrade("test-upgrade")

	_, err := v.ValidateCreate(context.Background(), ku)
	if err == nil {
		t.Fatal("expected error for empty secret data")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected error to contain %q, got: %v", "empty", err)
	}
}

func TestKubernetesUpgrade_ValidateCreate_InvalidTalosConfig(t *testing.T) {
	v := newK8sValidator(talosConfigSecret("default", []byte("not: valid: talosconfig: {{")))
	ku := newKubernetesUpgrade("test-upgrade")

	_, err := v.ValidateCreate(context.Background(), ku)
	if err == nil {
		t.Fatal("expected error for unparseable talosconfig")
	}
	if !strings.Contains(err.Error(), "cannot be parsed") {
		t.Errorf("expected error to contain %q, got: %v", "cannot be parsed", err)
	}
}

func TestKubernetesUpgrade_ValidateCreate_NoContextsInConfig(t *testing.T) {
	noContexts := []byte(`context: ""
contexts: {}
`)
	v := newK8sValidator(talosConfigSecret("default", noContexts))
	ku := newKubernetesUpgrade("test-upgrade")

	_, err := v.ValidateCreate(context.Background(), ku)
	if err == nil {
		t.Fatal("expected error for talosconfig with no contexts")
	}
	if !strings.Contains(err.Error(), "no contexts defined") {
		t.Errorf("expected error to contain %q, got: %v", "no contexts defined", err)
	}
}

// --- Version validation ---

func TestKubernetesUpgrade_ValidateCreate_InvalidVersionFormats(t *testing.T) {
	cases := []struct {
		name    string
		version string
	}{
		{"empty version", ""},
		{"missing v prefix", "1.34.0"},
		{"only major.minor", "v1.34"},
		{"garbage", "latest"},
		{"spaces", "v1.34.0 "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
			ku := newKubernetesUpgrade("test", withKubernetesVersion(tc.version))

			_, err := v.ValidateCreate(context.Background(), ku)
			if err == nil {
				t.Errorf("expected validation error for version %q", tc.version)
			}
		})
	}
}

func TestKubernetesUpgrade_ValidateCreate_ValidVersionFormats(t *testing.T) {
	cases := []string{
		"v1.34.0",
		"v1.34.0-rc.1",
		"v1.34.0-alpha.0",
		"v2.0.0",
		"v0.1.0",
	}

	for _, version := range cases {
		t.Run(version, func(t *testing.T) {
			v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
			ku := newKubernetesUpgrade("test", withKubernetesVersion(version))

			_, err := v.ValidateCreate(context.Background(), ku)
			if err != nil {
				t.Errorf("expected no error for version %q, got: %v", version, err)
			}
		})
	}
}

func TestKubernetesUpgrade_ValidateCreate_SingletonRejectsSecondResource(t *testing.T) {
	existing := newKubernetesUpgrade("existing-upgrade")
	v := newK8sValidator(existing, talosConfigSecret("default", validTalosConfig()))

	ku := newKubernetesUpgrade("second-upgrade")

	_, err := v.ValidateCreate(context.Background(), ku)
	if err == nil {
		t.Fatal("expected singleton violation error")
	}
	if !strings.Contains(err.Error(), "only one KubernetesUpgrade") {
		t.Errorf("expected singleton error message, got: %v", err)
	}
}

func TestKubernetesUpgrade_ValidateUpdate_SingletonAllowsSameResource(t *testing.T) {
	existing := newKubernetesUpgrade("my-upgrade")
	v := newK8sValidator(existing, talosConfigSecret("default", validTalosConfig()))

	old := newKubernetesUpgrade("my-upgrade")
	updated := newKubernetesUpgrade("my-upgrade", withKubernetesVersion("v1.35.0"))

	_, err := v.ValidateUpdate(context.Background(), old, updated)
	if err != nil {
		t.Fatalf("expected update of same resource to succeed, got: %v", err)
	}
}

func TestKubernetesUpgrade_ValidateUpdate_RejectsSpecChangeWhileInProgress(t *testing.T) {
	old := newKubernetesUpgrade("test", withK8sPhase(constants.PhaseInProgress))
	updated := newKubernetesUpgrade("test",
		withK8sPhase(constants.PhaseInProgress),
		withKubernetesVersion("v1.35.0"),
	)

	v := newK8sValidator(old, talosConfigSecret("default", validTalosConfig()))

	_, err := v.ValidateUpdate(context.Background(), old, updated)
	if err == nil {
		t.Fatal("expected error when updating spec during in-progress upgrade")
	}
	if !strings.Contains(err.Error(), "cannot update spec while upgrade is in progress") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestKubernetesUpgrade_ValidateUpdate_AllowsNoSpecChangeWhileInProgress(t *testing.T) {
	old := newKubernetesUpgrade("test", withK8sPhase(constants.PhaseInProgress))
	updated := newKubernetesUpgrade("test", withK8sPhase(constants.PhaseInProgress))

	v := newK8sValidator(old, talosConfigSecret("default", validTalosConfig()))

	_, err := v.ValidateUpdate(context.Background(), old, updated)
	if err != nil {
		t.Fatalf("expected no error when spec unchanged during in-progress, got: %v", err)
	}
}

func TestKubernetesUpgrade_ValidateUpdate_AllowsSpecChangeWhenNotInProgress(t *testing.T) {
	for _, phase := range []string{constants.PhasePending, constants.PhaseCompleted, constants.PhaseFailed, ""} {
		t.Run("phase_"+phase, func(t *testing.T) {
			old := newKubernetesUpgrade("test", withK8sPhase(phase))
			updated := newKubernetesUpgrade("test", withK8sPhase(phase), withKubernetesVersion("v1.35.0"))

			v := newK8sValidator(old, talosConfigSecret("default", validTalosConfig()))

			_, err := v.ValidateUpdate(context.Background(), old, updated)
			if err != nil {
				t.Fatalf("expected spec change to be allowed in phase %q, got: %v", phase, err)
			}
		})
	}
}

func TestKubernetesUpgrade_ValidateDelete_WarnsWhenInProgress(t *testing.T) {
	ku := newKubernetesUpgrade("test", withK8sPhase(constants.PhaseInProgress))
	v := newK8sValidator()

	warnings, err := v.ValidateDelete(context.Background(), ku)
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

func TestKubernetesUpgrade_ValidateDelete_NoWarningWhenIdle(t *testing.T) {
	for _, phase := range []string{constants.PhasePending, constants.PhaseCompleted, constants.PhaseFailed, ""} {
		t.Run("phase_"+phase, func(t *testing.T) {
			ku := newKubernetesUpgrade("test", withK8sPhase(phase))
			v := newK8sValidator()

			warnings, err := v.ValidateDelete(context.Background(), ku)
			if err != nil {
				t.Fatalf("delete should not error, got: %v", err)
			}
			if len(warnings) != 0 {
				t.Fatalf("expected no warnings for phase %q, got: %v", phase, warnings)
			}
		})
	}
}

func TestKubernetesUpgrade_ValidateCreate_HealthCheckValidation(t *testing.T) {
	validCheck := tupprv1alpha1.HealthCheckSpec{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Expr:       "object.status.readyReplicas == object.status.replicas",
	}

	t.Run("valid health check", func(t *testing.T) {
		v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
		ku := newKubernetesUpgrade("test", withK8sHealthChecks(validCheck))

		_, err := v.ValidateCreate(context.Background(), ku)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("empty apiVersion", func(t *testing.T) {
		check := validCheck
		check.APIVersion = ""
		v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
		ku := newKubernetesUpgrade("test", withK8sHealthChecks(check))

		_, err := v.ValidateCreate(context.Background(), ku)
		if err == nil {
			t.Fatal("expected error for empty apiVersion")
		}
	})

	t.Run("empty kind", func(t *testing.T) {
		check := validCheck
		check.Kind = ""
		v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
		ku := newKubernetesUpgrade("test", withK8sHealthChecks(check))

		_, err := v.ValidateCreate(context.Background(), ku)
		if err == nil {
			t.Fatal("expected error for empty kind")
		}
	})

	t.Run("empty expr", func(t *testing.T) {
		check := validCheck
		check.Expr = ""
		v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
		ku := newKubernetesUpgrade("test", withK8sHealthChecks(check))

		_, err := v.ValidateCreate(context.Background(), ku)
		if err == nil {
			t.Fatal("expected error for empty expr")
		}
	})

	t.Run("negative timeout", func(t *testing.T) {
		check := validCheck
		d := metav1.Duration{Duration: -1 * time.Second}
		check.Timeout = &d
		v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
		ku := newKubernetesUpgrade("test", withK8sHealthChecks(check))

		_, err := v.ValidateCreate(context.Background(), ku)
		if err == nil {
			t.Fatal("expected error for negative timeout")
		}
	})
}

func TestKubernetesUpgrade_ValidateCreate_TalosctlImagePartialSpec(t *testing.T) {
	t.Run("repo without tag", func(t *testing.T) {
		v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
		ku := newKubernetesUpgrade("test", withK8sTalosctlImage("ghcr.io/custom/talosctl", ""))

		_, err := v.ValidateCreate(context.Background(), ku)
		if err == nil {
			t.Fatal("expected error when repo set without tag")
		}
		if !strings.Contains(err.Error(), "must be specified together") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("tag without repo", func(t *testing.T) {
		v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
		ku := newKubernetesUpgrade("test", withK8sTalosctlImage("", "v1.11.0"))

		_, err := v.ValidateCreate(context.Background(), ku)
		if err == nil {
			t.Fatal("expected error when tag set without repo")
		}
	})

	t.Run("both specified", func(t *testing.T) {
		v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
		ku := newKubernetesUpgrade("test", withK8sTalosctlImage("ghcr.io/custom/talosctl", "v1.11.0"))

		_, err := v.ValidateCreate(context.Background(), ku)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})
}

func TestKubernetesUpgrade_ValidateCreate_InvalidPullPolicy(t *testing.T) {
	v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
	ku := newKubernetesUpgrade("test", withK8sPullPolicy(corev1.PullPolicy("BadPolicy")))

	_, err := v.ValidateCreate(context.Background(), ku)
	if err == nil {
		t.Fatal("expected error for invalid pull policy")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected error to mention 'invalid', got: %v", err)
	}
}

func TestKubernetesUpgrade_Warnings_PreReleaseVersion(t *testing.T) {
	v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
	ku := newKubernetesUpgrade("test", withKubernetesVersion("v1.34.0-rc.1"))

	warnings, err := v.ValidateCreate(context.Background(), ku)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "pre-release") {
		t.Errorf("expected pre-release warning, got: %v", warnings)
	}
}

func TestKubernetesUpgrade_Warnings_HealthCheckNoTimeout(t *testing.T) {
	check := tupprv1alpha1.HealthCheckSpec{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Expr:       "true",
	}
	v := newK8sValidator(talosConfigSecret("default", validTalosConfig()))
	ku := newKubernetesUpgrade("test", withK8sHealthChecks(check))

	warnings, err := v.ValidateCreate(context.Background(), ku)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "no timeout") {
		t.Errorf("expected timeout warning, got: %v", warnings)
	}
}

func containsWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}
