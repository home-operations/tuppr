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

	upgradev1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

// HealthChecker evaluates CEL-based health checks
type HealthChecker struct {
	client.Client
}

// CheckHealth performs the health checks defined in the TalosUpgrade resource
func (hc *HealthChecker) CheckHealth(ctx context.Context, healthChecks []upgradev1alpha1.HealthCheckExpr) error {
	logger := log.FromContext(ctx)

	if len(healthChecks) == 0 {
		return nil // No health checks to perform
	}

	// Validate health checks first
	if err := hc.validateHealthChecks(healthChecks); err != nil {
		return fmt.Errorf("health check validation failed: %w", err)
	}

	logger.Info("Performing health checks", "count", len(healthChecks))

	var healthErrors []error
	for i := range len(healthChecks) {
		if err := hc.evaluateHealthCheck(ctx, healthChecks[i], i); err != nil {
			healthErrors = append(healthErrors, fmt.Errorf("health check %d failed: %w", i, err))
		}
	}

	if len(healthErrors) > 0 {
		return errors.Join(healthErrors...)
	}

	logger.Info("All health checks passed")
	return nil
}

// evaluateHealthCheck evaluates a single health check with timeout and retries
func (hc *HealthChecker) evaluateHealthCheck(ctx context.Context, check upgradev1alpha1.HealthCheckExpr, index int) error {
	logger := log.FromContext(ctx).WithValues(
		"checkIndex", index,
		"apiVersion", check.APIVersion,
		"kind", check.Kind,
		"name", check.Name,
		"namespace", check.Namespace,
	)

	// Set default timeout
	timeout := 5 * time.Minute
	if check.Timeout != nil {
		timeout = check.Timeout.Duration
	}

	description := check.Description
	if description == "" {
		description = fmt.Sprintf("%s/%s health check", check.APIVersion, check.Kind)
	}

	logger.Info("Starting health check", "description", description, "timeout", timeout)

	// Create timeout context with cause
	timeoutCause := fmt.Errorf("health check '%s' exceeded timeout of %v", description, timeout)
	timeoutCtx, cancel := context.WithTimeoutCause(ctx, timeout, timeoutCause)
	defer cancel()

	// Compile CEL expression - provide both 'object' for full resource and 'status' for convenience
	env, err := cel.NewEnv(
		cel.Variable("object", cel.DynType), // Full Kubernetes resource object
		cel.Variable("status", cel.DynType), // Just the status field for convenience
	)
	if err != nil {
		return fmt.Errorf("failed to create CEL environment: %w", err)
	}

	ast, issues := env.Compile(check.Expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("failed to compile CEL expression '%s': %w", check.Expr, issues.Err())
	}

	program, err := env.Program(ast)
	if err != nil {
		return fmt.Errorf("failed to create CEL program: %w", err)
	}

	// Poll until expression evaluates to true or timeout
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("health check failed: %w", context.Cause(timeoutCtx))

		case <-ticker.C:
			passed, err := hc.evaluateExpression(timeoutCtx, check, program)
			if err != nil {
				logger.V(1).Info("Health check evaluation error (will retry)", "error", err.Error())
				continue
			}

			if passed {
				logger.Info("Health check passed", "description", description)
				return nil
			}

			logger.V(1).Info("Health check not yet satisfied (will retry)", "description", description)
		}
	}
}

// evaluateExpression evaluates the CEL expression against the current resource state
func (hc *HealthChecker) evaluateExpression(ctx context.Context, check upgradev1alpha1.HealthCheckExpr, program cel.Program) (bool, error) {
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
func (hc *HealthChecker) evaluateSpecificResource(ctx context.Context, check upgradev1alpha1.HealthCheckExpr, program cel.Program, gvk schema.GroupVersionKind) (bool, error) {
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
func (hc *HealthChecker) evaluateAllResources(ctx context.Context, check upgradev1alpha1.HealthCheckExpr, program cel.Program, gvk schema.GroupVersionKind) (bool, error) {
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
func (hc *HealthChecker) validateHealthChecks(healthChecks []upgradev1alpha1.HealthCheckExpr) error {
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
