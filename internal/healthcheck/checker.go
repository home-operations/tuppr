package healthcheck

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/metrics"
)

// CheckTimeout bounds the API reads of one check, so a slow apiserver or a
// large cluster-wide List can neither hold the reconcile worker nor use up the
// time of the checks after it.
const CheckTimeout = 10 * time.Second

// Re-evaluation intervals while checks keep failing: RetryInterval at first,
// growing to MaxRetryInterval. Each TalosUpgrade pass also queries every
// pending node and the installer registry, so a run parked for hours on a
// failing check shouldn't pay that every 30 seconds.
const (
	RetryInterval    = 30 * time.Second
	MaxRetryInterval = 5 * time.Minute
)

// RetryAfter returns how long to wait before re-evaluating checks that have
// kept failing for failingFor: a quarter of that, within RetryInterval and
// MaxRetryInterval.
func RetryAfter(failingFor time.Duration) time.Duration {
	return min(max(failingFor/4, RetryInterval), MaxRetryInterval)
}

const celStatusKey = "status"

// MetricsRecorder defines the interface for metrics the health checker needs
type MetricsRecorder interface {
	RecordHealthCheckFailure(upgradeType, upgradeName string, checkIndex int)
}

type Checker struct {
	client.Client
	MetricsReporter MetricsRecorder
}

func NewChecker(c client.Client, mr MetricsRecorder) *Checker {
	return &Checker{
		Client:          c,
		MetricsReporter: mr,
	}
}

// CheckHealth evaluates every check once against the current cluster state
// and returns an error naming each check that failed or could not be read.
// It never waits for checks to pass: the caller requeues while they fail.
func (hc *Checker) CheckHealth(ctx context.Context, healthChecks []tupprv1alpha1.HealthCheckSpec) error {
	logger := log.FromContext(ctx)

	if len(healthChecks) == 0 {
		return nil
	}

	if err := hc.validateHealthChecks(healthChecks); err != nil {
		return fmt.Errorf("health check validation failed: %w", err)
	}

	programs := make([]cel.Program, len(healthChecks))
	for i, check := range healthChecks {
		program, err := compileExpression(check.Expr)
		if err != nil {
			return fmt.Errorf("failed to compile CEL expression for check %d: %w", i, err)
		}
		programs[i] = program
	}

	logger.V(1).Info("Performing health checks", "count", len(healthChecks))

	upgradeType, _ := ctx.Value(metrics.ContextKeyUpgradeType).(string)
	upgradeName, _ := ctx.Value(metrics.ContextKeyUpgradeName).(string)

	var checkErrors []error
	for i, check := range healthChecks {
		passed, err := hc.evaluateCheck(ctx, check, programs[i])
		if passed {
			continue
		}
		if err != nil {
			logger.V(1).Info("Health check evaluation failed",
				"index", i,
				"description", check.Description,
				"apiVersion", check.APIVersion,
				"kind", check.Kind,
				"error", err)
			checkErrors = append(checkErrors, fmt.Errorf("check %d (%s) evaluation error: %w", i, describeCheck(check), err))
		} else {
			logger.V(1).Info("Health check not satisfied",
				"index", i,
				"description", check.Description,
				"apiVersion", check.APIVersion,
				"kind", check.Kind,
			)
			checkErrors = append(checkErrors, fmt.Errorf("check %d (%s) not satisfied", i, describeCheck(check)))
		}
		if upgradeType != "" && upgradeName != "" && hc.MetricsReporter != nil {
			hc.MetricsReporter.RecordHealthCheckFailure(upgradeType, upgradeName, i)
		}
	}

	if len(checkErrors) > 0 {
		return fmt.Errorf("health checks failed: %w", errors.Join(checkErrors...))
	}
	logger.V(1).Info("All health checks passed")
	return nil
}

func (hc *Checker) evaluateCheck(ctx context.Context, check tupprv1alpha1.HealthCheckSpec, program cel.Program) (bool, error) {
	checkCtx, cancel := context.WithTimeout(ctx, CheckTimeout)
	defer cancel()
	return hc.evaluateExpression(checkCtx, check, program)
}

func compileExpression(expr string) (cel.Program, error) {
	env, err := cel.NewEnv(
		cel.Variable("object", cel.DynType),
		cel.Variable(celStatusKey, cel.DynType),
	)
	if err != nil {
		return nil, fmt.Errorf("create CEL environment: %w", err)
	}
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	return env.Program(ast)
}

func describeCheck(check tupprv1alpha1.HealthCheckSpec) string {
	if check.Description != "" {
		return check.Description
	}
	target := check.Kind
	if check.Name != "" {
		target += " " + check.Name
	}
	if check.Namespace != "" {
		target = check.Namespace + "/" + target
	}
	if check.LabelSelector != nil {
		target += " " + metav1.FormatLabelSelector(check.LabelSelector)
	}
	return target
}

func (hc *Checker) evaluateExpression(ctx context.Context, check tupprv1alpha1.HealthCheckSpec, program cel.Program) (bool, error) {
	gvk := schema.FromAPIVersionAndKind(check.APIVersion, check.Kind)

	if check.Name != "" {
		return hc.evaluateSpecificResource(ctx, check, program, gvk)
	}
	return hc.evaluateAllResources(ctx, check, program, gvk)
}

func (hc *Checker) evaluateSpecificResource(ctx context.Context, check tupprv1alpha1.HealthCheckSpec, program cel.Program, gvk schema.GroupVersionKind) (bool, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	key := client.ObjectKey{Name: check.Name}
	if check.Namespace != "" {
		key.Namespace = check.Namespace
	}

	if err := hc.Get(ctx, key, obj); err != nil {
		return false, fmt.Errorf("failed to get resource: %w", err)
	}

	return hc.runCELExpression(program, obj.Object)
}

func (hc *Checker) evaluateAllResources(ctx context.Context, check tupprv1alpha1.HealthCheckSpec, program cel.Program, gvk schema.GroupVersionKind) (bool, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)

	listOpts := []client.ListOption{}
	if check.Namespace != "" {
		listOpts = append(listOpts, client.InNamespace(check.Namespace))
	}
	if check.LabelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(check.LabelSelector)
		if err != nil {
			return false, fmt.Errorf("invalid label selector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	if err := hc.List(ctx, list, listOpts...); err != nil {
		return false, fmt.Errorf("failed to list resources: %w", err)
	}

	if check.LabelSelector != nil && len(list.Items) == 0 {
		return false, nil
	}

	for _, item := range list.Items {
		passed, err := hc.runCELExpression(program, item.Object)
		if err != nil {
			return false, fmt.Errorf("failed to evaluate CEL expression: %w", err)
		}
		if !passed {
			return false, nil
		}
	}

	return true, nil
}

func (hc *Checker) runCELExpression(program cel.Program, resourceData map[string]any) (bool, error) {
	safeData := maps.Clone(resourceData)

	statusData := make(map[string]any)
	if status, exists := safeData[celStatusKey]; exists {
		if statusMap, ok := status.(map[string]any); ok {
			statusData = statusMap
		}
	}

	out, _, err := program.Eval(map[string]any{
		"object":     safeData,
		celStatusKey: statusData,
	})
	if err != nil {
		return false, fmt.Errorf("CEL evaluation error: %w", err)
	}

	if out.Type() != types.BoolType {
		return false, fmt.Errorf("CEL expression must return a boolean, got %s", out.Type())
	}

	return out.Value().(bool), nil
}

func (hc *Checker) validateHealthChecks(healthChecks []tupprv1alpha1.HealthCheckSpec) error {
	var validationErrors []error

	for i, check := range healthChecks {
		if check.APIVersion == "" {
			validationErrors = append(validationErrors, fmt.Errorf("health check %d: apiVersion is required", i))
		}
		if check.Kind == "" {
			validationErrors = append(validationErrors, fmt.Errorf("health check %d: kind is required", i))
		}
		if check.Expr == "" {
			validationErrors = append(validationErrors, fmt.Errorf("health check %d: expr expression is required", i))
		}
		if check.Name != "" && check.LabelSelector != nil {
			validationErrors = append(validationErrors, fmt.Errorf("health check %d: name and labelSelector are mutually exclusive", i))
		}
		if check.LabelSelector != nil {
			if _, err := metav1.LabelSelectorAsSelector(check.LabelSelector); err != nil {
				validationErrors = append(validationErrors, fmt.Errorf("health check %d: invalid labelSelector: %w", i, err))
			}
		}

		env, err := cel.NewEnv(
			cel.Variable("object", cel.DynType),
			cel.Variable(celStatusKey, cel.DynType),
		)
		if err == nil {
			if _, issues := env.Compile(check.Expr); issues != nil && issues.Err() != nil {
				validationErrors = append(validationErrors, fmt.Errorf("health check %d: invalid CEL expression '%s': %w", i, check.Expr, issues.Err()))
			}
		}
	}

	if len(validationErrors) > 0 {
		return errors.Join(validationErrors...)
	}

	return nil
}
