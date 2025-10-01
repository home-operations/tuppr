package controller

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
)

const (
	// DefaultHealthCheckTimeout is the default timeout for health checks if not specified
	DefaultHealthCheckTimeout = 10 * time.Minute
)

// HealthChecker evaluates CEL-based health checks
type HealthChecker struct {
	client.Client
}

// CheckHealth performs the health checks defined in the TalosUpgrade resource
func (hc *HealthChecker) CheckHealth(ctx context.Context, healthChecks []tupprv1alpha1.HealthCheckSpec) error {
	logger := log.FromContext(ctx)

	if len(healthChecks) == 0 {
		return nil
	}

	// Get upgrade info from context for metrics
	upgradeType := ctx.Value(ContextKeyUpgradeType)
	upgradeName := ctx.Value(ContextKeyUpgradeName)

	startTime := time.Now()
	defer func() {
		if upgradeType != nil && upgradeName != nil {
			duration := time.Since(startTime).Seconds()
			if metricsReporter != nil {
				metricsReporter.RecordHealthCheckDuration(
					upgradeType.(string),
					upgradeName.(string),
					duration,
				)
			}
		}
	}()

	// Validate health checks first
	if err := hc.validateHealthChecks(healthChecks); err != nil {
		return fmt.Errorf("health check validation failed: %w", err)
	}

	logger.Info("Performing health checks", "count", len(healthChecks))

	// Find the maximum timeout across all checks
	maxTimeout := DefaultHealthCheckTimeout
	for _, check := range healthChecks {
		if check.Timeout != nil && check.Timeout.Duration > maxTimeout {
			maxTimeout = check.Timeout.Duration
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, maxTimeout)
	defer cancel()

	// Compile all CEL programs upfront
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

	// Poll until ALL checks pass simultaneously
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("health checks failed: exceeded maximum timeout of %v", maxTimeout)

		case <-ticker.C:
			allPassed := true
			var checkErrors []error

			// Evaluate ALL checks in this iteration
			for i, check := range healthChecks {
				passed, err := hc.evaluateExpression(timeoutCtx, check, programs[i])
				if err != nil {
					checkErrors = append(checkErrors, fmt.Errorf("check %d evaluation error: %w", i, err))
					allPassed = false

					// Record health check failure metric
					if upgradeType != nil && upgradeName != nil && metricsReporter != nil {
						metricsReporter.RecordHealthCheckFailure(
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

					// Record health check failure metric
					if upgradeType != nil && upgradeName != nil && metricsReporter != nil {
						metricsReporter.RecordHealthCheckFailure(
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
				logger.Info("All health checks passed simultaneously")
				return nil
			}

			logger.V(1).Info("Not all health checks passed, will retry")
		}
	}
}

// evaluateExpression evaluates the CEL expression against the current resource state
func (hc *HealthChecker) evaluateExpression(ctx context.Context, check tupprv1alpha1.HealthCheckSpec, program cel.Program) (bool, error) {
	// Get the resource(s)
	gvk := schema.FromAPIVersionAndKind(check.APIVersion, check.Kind)

	if check.Name != "" {
		// Check specific resource
		return hc.evaluateSpecificResource(ctx, check, program, gvk)
	} else {
		// Check all resources of this kind
		return hc.evaluateAllResources(ctx, check, program, gvk)
	}
}

// evaluateSpecificResource evaluates the expression against a specific resource
func (hc *HealthChecker) evaluateSpecificResource(ctx context.Context, check tupprv1alpha1.HealthCheckSpec, program cel.Program, gvk schema.GroupVersionKind) (bool, error) {
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

// evaluateAllResources evaluates the expression against all resources of the kind
func (hc *HealthChecker) evaluateAllResources(ctx context.Context, check tupprv1alpha1.HealthCheckSpec, program cel.Program, gvk schema.GroupVersionKind) (bool, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)

	listOpts := []client.ListOption{}
	if check.Namespace != "" {
		listOpts = append(listOpts, client.InNamespace(check.Namespace))
	}

	if err := hc.List(ctx, list, listOpts...); err != nil {
		return false, fmt.Errorf("failed to list resources: %w", err)
	}

	// Check if all resources pass the health check
	for _, item := range list.Items {
		passed, err := hc.runCELExpression(program, item.Object)
		if err != nil {
			return false, fmt.Errorf("failed to evaluate CEL expression: %w", err)
		}
		if !passed {
			return false, nil // At least one resource didn't pass
		}
	}

	return true, nil
}

// runCELExpression executes the CEL program against the resource data
func (hc *HealthChecker) runCELExpression(program cel.Program, resourceData map[string]any) (bool, error) {
	// Clone the resource data to avoid mutations
	safeData := maps.Clone(resourceData)

	// Extract status field for convenience, default to empty map if missing
	statusData := make(map[string]any)
	if status, exists := safeData["status"]; exists {
		if statusMap, ok := status.(map[string]any); ok {
			statusData = statusMap
		}
	}

	out, _, err := program.Eval(map[string]any{
		"object": safeData,   // Full Kubernetes resource object
		"status": statusData, // Just the status field for convenience
	})

	if err != nil {
		return false, fmt.Errorf("CEL evaluation error: %w", err)
	}

	if out.Type() != types.BoolType {
		return false, fmt.Errorf("CEL expression must return a boolean, got %s", out.Type())
	}

	return out.Value().(bool), nil
}

// validateHealthChecks validates health check expressions before execution
func (hc *HealthChecker) validateHealthChecks(healthChecks []tupprv1alpha1.HealthCheckSpec) error {
	var validationErrors []error

	for i, check := range healthChecks {
		// Check for required fields
		if check.APIVersion == "" {
			validationErrors = append(validationErrors, fmt.Errorf("health check %d: apiVersion is required", i))
		}
		if check.Kind == "" {
			validationErrors = append(validationErrors, fmt.Errorf("health check %d: kind is required", i))
		}
		if check.Expr == "" {
			validationErrors = append(validationErrors, fmt.Errorf("health check %d: expr expression is required", i))
		}

		// Validate CEL expression syntax early - provide both variables
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

var metricsReporter *MetricsReporter

func init() {
	metricsReporter = NewMetricsReporter()
}
