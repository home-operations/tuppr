package validation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/netresearch/go-cron"
	talosclientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
)

// ValidateTalosConfigSecret checks if the secret exists and contains valid Talos configuration
func ValidateTalosConfigSecret(ctx context.Context, c client.Client, name, namespace string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("talos config secret name is empty")
	}

	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: name, Namespace: namespace}

	if err := c.Get(ctx, key, secret); err != nil {
		return "", fmt.Errorf("talosconfig secret '%s' not found in namespace '%s': %w", name, namespace, err)
	}

	// Helper to check keys
	checkKey := func(k string) ([]byte, bool) {
		data, ok := secret.Data[k]
		return data, ok
	}

	var configData []byte
	var ok bool

	if configData, ok = checkKey(constants.TalosSecretKey); !ok {
		if configData, ok = checkKey("config"); !ok {
			return "", fmt.Errorf("talosconfig secret '%s' missing required key '%s'", name, constants.TalosSecretKey)
		}
	}

	if len(configData) == 0 {
		return "", fmt.Errorf("talosconfig secret data is empty")
	}

	config, err := talosclientconfig.FromBytes(configData)
	if err != nil {
		return "", fmt.Errorf("talosconfig in secret '%s' cannot be parsed: %w", name, err)
	}

	if len(config.Contexts) == 0 {
		return "", fmt.Errorf("talosconfig in secret '%s' has no contexts defined", name)
	}

	return "", nil
}

// ValidateHealthChecks validates a list of health check specs
func ValidateHealthChecks(checks []tupprv1alpha1.HealthCheckSpec) error {
	for i, check := range checks {
		if check.APIVersion == "" {
			return fmt.Errorf("healthChecks[%d]: apiVersion cannot be empty", i)
		}
		if check.Kind == "" {
			return fmt.Errorf("healthChecks[%d]: kind cannot be empty", i)
		}
		if check.Expr == "" {
			return fmt.Errorf("healthChecks[%d]: expr cannot be empty", i)
		}
		if check.Timeout != nil && check.Timeout.Duration <= 0 {
			return fmt.Errorf("healthChecks[%d]: timeout must be positive", i)
		}
	}
	return nil
}

// ValidateTalosctlSpec validates the image and pull policy
func ValidateTalosctlSpec(spec tupprv1alpha1.TalosctlSpec) error {
	repo := spec.Image.Repository
	tag := spec.Image.Tag

	// Strict check matches existing tests
	if repo == "" && tag != "" {
		return fmt.Errorf("spec.talosctl.image.tag cannot be set without a repository")
	}

	if spec.Image.PullPolicy != "" {
		validPolicies := []corev1.PullPolicy{corev1.PullAlways, corev1.PullNever, corev1.PullIfNotPresent}
		if !slices.Contains(validPolicies, spec.Image.PullPolicy) {
			return fmt.Errorf("invalid pullPolicy '%s'. Valid values: %v", spec.Image.PullPolicy, validPolicies)
		}
	}
	return nil
}

// ValidateVersionFormat checks if the version string matches the standard pattern
func ValidateVersionFormat(version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}
	pattern := `^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9\-\.]+)?$`
	matched, err := regexp.MatchString(pattern, version)
	if err != nil {
		return fmt.Errorf("regex error: %w", err)
	}
	if !matched {
		return fmt.Errorf("version '%s' invalid. Must be 'vX.Y.Z' or 'vX.Y.Z-suffix'", version)
	}
	return nil
}

// ValidateSingleton ensures only one instance of the specific list type exists
func ValidateSingleton(ctx context.Context, c client.Client, kindName, currentName string, list client.ObjectList) error {
	if err := c.List(ctx, list); err != nil {
		return fmt.Errorf("failed to list resources: %w", err)
	}

	items := reflect.ValueOf(list).Elem().FieldByName("Items")
	if items.IsValid() {
		for i := 0; i < items.Len(); i++ {
			item := items.Index(i)
			name := item.FieldByName("Name").String()

			if name != currentName {
				return fmt.Errorf("only one %s resource is allowed per cluster. Found existing: '%s'", kindName, name)
			}
		}
	}
	return nil
}

// ValidateUpdateInProgress checks if specs are changing while an upgrade is running
func ValidateUpdateInProgress(oldPhase tupprv1alpha1.JobPhase, oldSpec, newSpec interface{}) error {
	if oldPhase.IsActive() {
		if !reflect.DeepEqual(oldSpec, newSpec) {
			return fmt.Errorf("cannot update spec while upgrade is in progress (phase: %s)", oldPhase)
		}
	}
	return nil
}

// GenerateCommonWarnings checks for PreReleases, Timeouts, and defaults
func GenerateCommonWarnings(version string, checks []tupprv1alpha1.HealthCheckSpec, talosctlTag string) admission.Warnings {
	var warnings admission.Warnings

	// Warn about health checks without timeouts
	for i, check := range checks {
		if check.Timeout == nil {
			warnings = append(warnings, fmt.Sprintf("Health check %d has no timeout specified", i))
		}
	}

	// Warn about pre-release versions
	if matched, _ := regexp.MatchString(`-[a-zA-Z]`, version); matched {
		warnings = append(warnings, "Target version appears to be a pre-release.")
	}

	// Warn if talosctl version is not specified
	if talosctlTag == "" {
		warnings = append(warnings, "No talosctl version specified, will auto-detect.")
	}

	return warnings
}

// ValidateMaintenanceWindows validates all maintenance windows
func ValidateMaintenanceWindows(spec *tupprv1alpha1.MaintenanceSpec) (admission.Warnings, error) {
	if spec == nil || len(spec.Windows) == 0 {
		return nil, nil
	}
	var warnings admission.Warnings
	for _, window := range spec.Windows {
		warn, err := ValidateMaintenanceWindow(&window)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}
	return warnings, nil
}

// ValidateMaintenanceWindow validates a single maintenance window
func ValidateMaintenanceWindow(window *tupprv1alpha1.WindowSpec) (admission.Warnings, error) {
	var warnings admission.Warnings

	_, err := time.LoadLocation(window.Timezone)
	if err != nil {
		return nil, err
	}
	specParser := cron.MustNewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

	_, err = specParser.Parse(window.Start)
	if err != nil {
		return nil, err
	}
	if window.Duration.Duration <= 0 {
		return nil, errors.New("duration must be positive")
	}
	if window.Duration.Duration > 168*time.Hour {
		return nil, errors.New("duration must not exceed 7 days (168h)")
	}
	if window.Duration.Duration < time.Hour {
		warnings = append(warnings, "maintenance window duration < 1h: may not be enough time to complete upgrades")
	}
	return warnings, nil
}
