package healthcheck

import (
	"context"
	"testing"
	"time"

	"github.com/google/cel-go/cel"
	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/metrics"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = tupprv1alpha1.AddToScheme(scheme)
	return scheme
}

func TestHealthChecker_CheckHealth_NoChecks(t *testing.T) {
	scheme := newTestScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	hc := &Checker{Client: cl}

	err := hc.CheckHealth(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error for empty health checks, got: %v", err)
	}
}

func TestHealthChecker_ValidateHealthChecks(t *testing.T) {
	scheme := newTestScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	hc := &Checker{Client: cl}

	tests := []struct {
		name    string
		checks  []tupprv1alpha1.HealthCheckSpec
		wantErr bool
	}{
		{
			name:    "empty checks",
			checks:  nil,
			wantErr: false,
		},
		{
			name: "valid check",
			checks: []tupprv1alpha1.HealthCheckSpec{
				{
					APIVersion: "v1",
					Kind:       "Node",
					Expr:       "true",
				},
			},
			wantErr: false,
		},
		{
			name: "missing apiVersion",
			checks: []tupprv1alpha1.HealthCheckSpec{
				{
					Kind: "Node",
					Expr: "true",
				},
			},
			wantErr: true,
		},
		{
			name: "missing kind",
			checks: []tupprv1alpha1.HealthCheckSpec{
				{
					APIVersion: "v1",
					Expr:       "true",
				},
			},
			wantErr: true,
		},
		{
			name: "missing expression",
			checks: []tupprv1alpha1.HealthCheckSpec{
				{
					APIVersion: "v1",
					Kind:       "Node",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid CEL expression",
			checks: []tupprv1alpha1.HealthCheckSpec{
				{
					APIVersion: "v1",
					Kind:       "Node",
					Expr:       "this is not valid CEL !!!",
				},
			},
			wantErr: true,
		},
		{
			name: "multiple errors",
			checks: []tupprv1alpha1.HealthCheckSpec{
				{Expr: "true"},
				{APIVersion: "v1", Kind: "Node"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := hc.validateHealthChecks(tt.checks)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateHealthChecks() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHealthChecker_RunCELExpression(t *testing.T) {
	scheme := newTestScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	hc := &Checker{Client: cl}

	tests := []struct {
		name     string
		expr     string
		resource map[string]any
		want     bool
		wantErr  bool
	}{
		{
			name: "simple true expression",
			expr: "true",
			resource: map[string]any{
				"metadata": map[string]any{"name": "test"},
			},
			want: true,
		},
		{
			name: "simple false expression",
			expr: "false",
			resource: map[string]any{
				"metadata": map[string]any{"name": "test"},
			},
			want: false,
		},
		{
			name: "check status field",
			expr: `status.phase == "Running"`,
			resource: map[string]any{
				"status": map[string]any{"phase": "Running"},
			},
			want: true,
		},
		{
			name: "check status field mismatch",
			expr: `status.phase == "Running"`,
			resource: map[string]any{
				"status": map[string]any{"phase": "Pending"},
			},
			want: false,
		},
		{
			name: "check object metadata",
			expr: `object.metadata.name == "my-resource"`,
			resource: map[string]any{
				"metadata": map[string]any{"name": "my-resource"},
			},
			want: true,
		},
		{
			name: "status missing defaults to empty map",
			expr: `!has(status.phase)`,
			resource: map[string]any{
				"metadata": map[string]any{"name": "test"},
			},
			want: true,
		},
		{
			name:     "non-boolean return type",
			expr:     `"not a bool"`,
			resource: map[string]any{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := newCELEnv()
			if err != nil {
				t.Fatalf("failed to create CEL env: %v", err)
			}
			ast, issues := env.Compile(tt.expr)
			if issues != nil && issues.Err() != nil {
				t.Fatalf("failed to compile: %v", issues.Err())
			}
			program, err := env.Program(ast)
			if err != nil {
				t.Fatalf("failed to create program: %v", err)
			}

			got, err := hc.runCELExpression(program, tt.resource)
			if (err != nil) != tt.wantErr {
				t.Fatalf("runCELExpression() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("runCELExpression() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHealthChecker_EvaluateSpecificResource(t *testing.T) {
	scheme := newTestScheme()

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	obj.SetName("test-cm")
	obj.SetNamespace("default")
	obj.Object["data"] = map[string]any{"key": "value"}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
	hc := &Checker{Client: cl}

	check := tupprv1alpha1.HealthCheckSpec{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Name:       "test-cm",
		Namespace:  "default",
		Expr:       `has(object.data)`,
	}

	env, _ := newCELEnv()
	ast, _ := env.Compile(check.Expr)
	program, _ := env.Program(ast)

	gvk := corev1.SchemeGroupVersion.WithKind("ConfigMap")
	passed, err := hc.evaluateSpecificResource(context.Background(), check, program, gvk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passed {
		t.Fatal("expected health check to pass")
	}
}

func TestHealthChecker_EvaluateSpecificResource_NotFound(t *testing.T) {
	scheme := newTestScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	hc := &Checker{Client: cl}

	check := tupprv1alpha1.HealthCheckSpec{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Name:       "nonexistent",
		Namespace:  "default",
		Expr:       `true`,
	}

	env, _ := newCELEnv()
	ast, _ := env.Compile(check.Expr)
	program, _ := env.Program(ast)

	gvk := corev1.SchemeGroupVersion.WithKind("ConfigMap")
	_, err := hc.evaluateSpecificResource(context.Background(), check, program, gvk)
	if err == nil {
		t.Fatal("expected error for missing resource")
	}
}

func TestHealthChecker_EvaluateAllResources(t *testing.T) {
	scheme := newTestScheme()

	cm1 := &unstructured.Unstructured{}
	cm1.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm1.SetName("cm-1")
	cm1.SetNamespace("default")
	cm1.Object["data"] = map[string]any{"ready": "true"}

	cm2 := &unstructured.Unstructured{}
	cm2.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm2.SetName("cm-2")
	cm2.SetNamespace("default")
	cm2.Object["data"] = map[string]any{"ready": "true"}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm1, cm2).Build()
	hc := &Checker{Client: cl}

	check := tupprv1alpha1.HealthCheckSpec{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Namespace:  "default",
		Expr:       `has(object.data)`,
	}

	env, _ := newCELEnv()
	ast, _ := env.Compile(check.Expr)
	program, _ := env.Program(ast)

	gvk := corev1.SchemeGroupVersion.WithKind("ConfigMap")
	passed, err := hc.evaluateAllResources(context.Background(), check, program, gvk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passed {
		t.Fatal("expected all resources to pass")
	}
}

func TestHealthChecker_EvaluateSpecificResource_FalseExpression(t *testing.T) {
	scheme := newTestScheme()

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	obj.SetName("test-cm")
	obj.SetNamespace("default")

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
	hc := &Checker{Client: cl}

	check := tupprv1alpha1.HealthCheckSpec{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Name:       "test-cm",
		Namespace:  "default",
		Expr:       `has(object.nonexistent)`,
	}

	env, _ := newCELEnv()
	ast, _ := env.Compile(check.Expr)
	program, _ := env.Program(ast)

	gvk := corev1.SchemeGroupVersion.WithKind("ConfigMap")
	passed, err := hc.evaluateSpecificResource(context.Background(), check, program, gvk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if passed {
		t.Fatal("expected health check to fail for missing field")
	}
}

func TestHealthChecker_EvaluateAllResources_OneFails(t *testing.T) {
	scheme := newTestScheme()

	cm1 := &unstructured.Unstructured{}
	cm1.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm1.SetName("cm-healthy")
	cm1.SetNamespace("default")
	cm1.Object["data"] = map[string]any{"ready": "true"}

	cm2 := &unstructured.Unstructured{}
	cm2.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm2.SetName("cm-unhealthy")
	cm2.SetNamespace("default")
	// No data field - will fail has(object.data)

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm1, cm2).Build()
	hc := &Checker{Client: cl}

	check := tupprv1alpha1.HealthCheckSpec{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Namespace:  "default",
		Expr:       `has(object.data)`,
	}

	env, _ := newCELEnv()
	ast, _ := env.Compile(check.Expr)
	program, _ := env.Program(ast)

	gvk := corev1.SchemeGroupVersion.WithKind("ConfigMap")
	passed, err := hc.evaluateAllResources(context.Background(), check, program, gvk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if passed {
		t.Fatal("expected evaluateAllResources to return false when one resource fails")
	}
}

func TestHealthChecker_EvaluateAllResources_EmptyList(t *testing.T) {
	scheme := newTestScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	hc := &Checker{Client: cl}

	check := tupprv1alpha1.HealthCheckSpec{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Namespace:  "default",
		Expr:       `has(object.data)`,
	}

	env, _ := newCELEnv()
	ast, _ := env.Compile(check.Expr)
	program, _ := env.Program(ast)

	gvk := corev1.SchemeGroupVersion.WithKind("ConfigMap")
	passed, err := hc.evaluateAllResources(context.Background(), check, program, gvk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passed {
		t.Fatal("expected evaluateAllResources to return true for empty list (vacuous truth)")
	}
}

func TestHealthChecker_CheckHealth_WithMetrics(t *testing.T) {
	scheme := newTestScheme()

	cm := &unstructured.Unstructured{}
	cm.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm.SetName("test-cm")
	cm.SetNamespace("default")

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	reporter := metrics.NewReporter()
	hc := &Checker{Client: cl, MetricsReporter: reporter}

	checks := []tupprv1alpha1.HealthCheckSpec{
		{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Name:       "test-cm",
			Namespace:  "default",
			Expr:       `true`,
			Timeout:    &metav1.Duration{Duration: 30 * time.Second},
		},
	}

	ctx := context.WithValue(context.Background(), metrics.ContextKeyUpgradeType, metrics.UpgradeTypeTalos)
	ctx = context.WithValue(ctx, metrics.ContextKeyUpgradeName, "test-upgrade")

	err := hc.CheckHealth(ctx, checks)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestHealthChecker_CheckHealth_InvalidExpression(t *testing.T) {
	scheme := newTestScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	hc := &Checker{Client: cl}

	checks := []tupprv1alpha1.HealthCheckSpec{
		{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Expr:       "this is invalid !!!",
		},
	}

	err := hc.CheckHealth(context.Background(), checks)
	if err == nil {
		t.Fatal("expected error for invalid expression")
	}
}

func TestHealthChecker_CheckHealth_NilMetricsReporter(t *testing.T) {
	scheme := newTestScheme()

	cm := &unstructured.Unstructured{}
	cm.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm.SetName("test-cm")
	cm.SetNamespace("default")

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	hc := &Checker{Client: cl} // no MetricsReporter

	checks := []tupprv1alpha1.HealthCheckSpec{
		{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Name:       "test-cm",
			Namespace:  "default",
			Expr:       `true`,
			Timeout:    &metav1.Duration{Duration: 30 * time.Second},
		},
	}

	ctx := context.WithValue(context.Background(), metrics.ContextKeyUpgradeType, metrics.UpgradeTypeTalos)
	ctx = context.WithValue(ctx, metrics.ContextKeyUpgradeName, "test-upgrade")

	err := hc.CheckHealth(ctx, checks)
	if err != nil {
		t.Fatalf("expected no error with nil MetricsReporter, got: %v", err)
	}
}

// newCELEnv creates a CEL environment matching the one used by HealthChecker
func newCELEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("object", cel.DynType),
		cel.Variable("status", cel.DynType),
	)
}
