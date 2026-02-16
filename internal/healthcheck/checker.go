package healthcheck

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/metrics"
)

const DefaultHealthCheckTimeout = 10 * time.Minute

// MetricsRecorder defines the interface for metrics the health checker needs
type MetricsRecorder interface {
	RecordHealthCheckDuration(upgradeType, upgradeName string, duration float64)
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

func (hc *Checker) CheckHealth(ctx context.Context, healthChecks []tupprv1alpha1.HealthCheckSpec) error {
	logger := log.FromContext(ctx)

	if len(healthChecks) == 0 {
		return nil
	}

	upgradeType := ctx.Value(metrics.ContextKeyUpgradeType)
	upgradeName := ctx.Value(metrics.ContextKeyUpgradeName)

	startTime := time.Now()
	defer func() {
		if upgradeType != nil && upgradeName != nil {
			duration := time.Since(startTime).Seconds()
			if hc.MetricsReporter != nil {
				hc.MetricsReporter.RecordHealthCheckDuration(
					upgradeType.(string),
					upgradeName.(string),
					duration,
				)
			}
		}
	}()

	if err := hc.validateHealthChecks(healthChecks); err != nil {
		return fmt.Errorf("health check validation failed: %w", err)
	}

	logger.V(1).Info("Performing health checks", "count", len(healthChecks))

	maxTimeout := DefaultHealthCheckTimeout
	for _, check := range healthChecks {
		if check.Timeout != nil && check.Timeout.Duration > maxTimeout {
			maxTimeout = check.Timeout.Duration
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, maxTimeout)
	defer cancel()

	programs := make([]cel.Program, len(healthChecks))
	for i, check := range healthChecks {
		env, err := cel.NewEnv(
			cel.Variable("object", cel.DynType),
			cel.Variable("status", cel.DynType),
		)
		if err != nil {
			return fmt.Errorf("failed to create CEL environment for check %d: %w", i, err)
		}

		ast, issues := env.Compile(check.Expr)
		if issues != nil && issues.Err() != nil {
			return fmt.Errorf("failed to compile CEL expression for check %d: %w", i, issues.Err())
		}

		program, err := env.Program(ast)
		if err != nil {
			return fmt.Errorf("failed to create CEL program for check %d: %w", i, err)
		}
		programs[i] = program
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("health checks failed: exceeded maximum timeout of %v", maxTimeout)

		case <-ticker.C:
			allPassed := true
			var checkErrors []error

			for i, check := range healthChecks {
				passed, err := hc.evaluateExpression(timeoutCtx, check, programs[i])
				if err != nil {
					checkErrors = append(checkErrors, fmt.Errorf("check %d evaluation error: %w", i, err))
					allPassed = false

					if upgradeType != nil && upgradeName != nil && hc.MetricsReporter != nil {
						hc.MetricsReporter.RecordHealthCheckFailure(
							upgradeType.(string),
							upgradeName.(string),
							i,
						)
					}
					continue
				}

				if !passed {
					logger.V(1).Info("Health check not satisfied",
						"index", i,
						"description", check.Description,
						"apiVersion", check.APIVersion,
						"kind", check.Kind,
					)
					allPassed = false

					if upgradeType != nil && upgradeName != nil && hc.MetricsReporter != nil {
						hc.MetricsReporter.RecordHealthCheckFailure(
							upgradeType.(string),
							upgradeName.(string),
							i,
						)
					}
				}
			}

			if len(checkErrors) > 0 {
				logger.V(1).Info("Some health checks had evaluation errors (will retry)", "errors", len(checkErrors))
				continue
			}

			if allPassed {
				logger.V(1).Info("All health checks passed")
				return nil
			}

			logger.V(1).Info("Not all health checks passed, will retry")
		}
	}
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

	if err := hc.List(ctx, list, listOpts...); err != nil {
		return false, fmt.Errorf("failed to list resources: %w", err)
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
	if status, exists := safeData["status"]; exists {
		if statusMap, ok := status.(map[string]any); ok {
			statusData = statusMap
		}
	}

	out, _, err := program.Eval(map[string]any{
		"object": safeData,
		"status": statusData,
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

		env, err := cel.NewEnv(
			cel.Variable("object", cel.DynType),
			cel.Variable("status", cel.DynType),
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
