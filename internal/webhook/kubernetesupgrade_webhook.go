package webhook

import (
	"context"
	"fmt"
	"regexp"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	talosclientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
)

// log is for logging in this package.
var kuberneteslog = logf.Log.WithName("kubernetes-resource")

// KubernetesUpgradeValidator validates KubernetesUpgrade resources
type KubernetesUpgradeValidator struct {
	Client            client.Client
	TalosConfigSecret string
}

// +kubebuilder:webhook:path=/validate-tuppr-home-operations-com-v1alpha1-kubernetesupgrade,mutating=false,failurePolicy=fail,sideEffects=None,groups=tuppr.home-operations.com,resources=kubernetesupgrades,verbs=create;update,versions=v1alpha1,name=vkubernetesupgrade.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &KubernetesUpgradeValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *KubernetesUpgradeValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	kubernetes := obj.(*tupprv1alpha1.KubernetesUpgrade)
	kuberneteslog.Info("validate create", "name", kubernetes.Name, "version", kubernetes.Spec.Kubernetes.Version, "talosConfigSecret", v.TalosConfigSecret)

	return v.validateKubernetes(ctx, kubernetes)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *KubernetesUpgradeValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	kubernetes := newObj.(*tupprv1alpha1.KubernetesUpgrade)
	oldKubernetes := oldObj.(*tupprv1alpha1.KubernetesUpgrade)
	kuberneteslog.Info("validate update", "name", kubernetes.Name, "version", kubernetes.Spec.Kubernetes.Version, "talosConfigSecret", v.TalosConfigSecret)

	// Prevent updates to certain fields if upgrade is in progress
	if oldKubernetes.Status.Phase == constants.PhaseInProgress {
		if kubernetes.Spec.Kubernetes.Version != oldKubernetes.Spec.Kubernetes.Version {
			return nil, fmt.Errorf("cannot update spec.kubernetes.version while upgrade is in progress (current phase: %s)", oldKubernetes.Status.Phase)
		}
	}

	return v.validateKubernetes(ctx, kubernetes)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (v *KubernetesUpgradeValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	kubernetes := obj.(*tupprv1alpha1.KubernetesUpgrade)

	// Warn about deleting an in-progress upgrade
	if kubernetes.Status.Phase == constants.PhaseInProgress {
		return []string{
			fmt.Sprintf("Deleting KubernetesUpgrade '%s' while upgrade is in progress. This may leave the cluster in an inconsistent state.", kubernetes.Name),
		}, nil
	}

	return nil, nil
}

func (v *KubernetesUpgradeValidator) validateKubernetes(ctx context.Context, kubernetes *tupprv1alpha1.KubernetesUpgrade) (admission.Warnings, error) {
	var warnings admission.Warnings

	kuberneteslog.Info("validating kubernetes upgrade",
		"name", kubernetes.Name,
		"namespace", kubernetes.Namespace,
		"version", kubernetes.Spec.Kubernetes.Version,
		"secretName", v.TalosConfigSecret)

	// Validate that the Talos config secret exists
	secret := &corev1.Secret{}
	err := v.Client.Get(ctx, types.NamespacedName{
		Name:      v.TalosConfigSecret,
		Namespace: kubernetes.Namespace,
	}, secret)

	if err != nil {
		return warnings, fmt.Errorf("talosconfig secret '%s' not found in namespace '%s'. Please create this secret with your Talos configuration before creating KubernetesUpgrade resources: %w",
			v.TalosConfigSecret, kubernetes.Namespace, err)
	}

	// Validate that the secret has the required key (use consistent key name)
	configData, exists := secret.Data[constants.TalosSecretKey]
	if !exists {
		return warnings, fmt.Errorf("talosconfig secret '%s' missing required key '%s'. The secret must contain your talosconfig data under this key",
			v.TalosConfigSecret, constants.TalosSecretKey)
	}

	// Add warning if secret data is empty
	if len(configData) == 0 {
		return warnings, fmt.Errorf("talosconfig secret data is empty. Please ensure the secret contains valid Talos configuration data")
	}

	// Validate that the talosconfig can be parsed
	config, err := talosclientconfig.FromBytes(configData)
	if err != nil {
		return warnings, fmt.Errorf("talosconfig in secret '%s' is invalid and cannot be parsed: %w. Please ensure the secret contains valid Talos configuration data",
			v.TalosConfigSecret, err)
	}

	// Validate that talosconfig has required endpoints
	if len(config.Contexts) == 0 {
		return warnings, fmt.Errorf("talosconfig has no contexts defined")
	}

	// Validate spec fields
	if err := v.validateKubernetesSpec(kubernetes); err != nil {
		return warnings, fmt.Errorf("spec validation failed: %w", err)
	}

	// Add warnings for risky configurations
	warnings = append(warnings, v.generateKubernetesWarnings(kubernetes)...)

	kuberneteslog.Info("kubernetes upgrade validation successful", "name", kubernetes.Name, "version", kubernetes.Spec.Kubernetes.Version)
	return warnings, nil
}

func (v *KubernetesUpgradeValidator) validateKubernetesSpec(kubernetes *tupprv1alpha1.KubernetesUpgrade) error {
	// Validate version is not empty and follows semantic versioning pattern
	if kubernetes.Spec.Kubernetes.Version == "" {
		return fmt.Errorf("spec.kubernetes.version cannot be empty")
	}

	// Validate version format (should match the kubebuilder validation pattern)
	versionPattern := `^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9\-\.]+)?$`
	matched, err := regexp.MatchString(versionPattern, kubernetes.Spec.Kubernetes.Version)
	if err != nil {
		return fmt.Errorf("error validating version pattern: %w", err)
	}
	if !matched {
		return fmt.Errorf("spec.kubernetes.version '%s' does not match required pattern. Must be in format 'vX.Y.Z' or 'vX.Y.Z-suffix' (e.g., 'v1.34.0', 'v1.34.0-rc.1')", kubernetes.Spec.Kubernetes.Version)
	}

	// Validate health checks
	for i, check := range kubernetes.Spec.HealthChecks {
		if err := v.validateHealthCheck(check); err != nil {
			return fmt.Errorf("spec.healthChecks[%d] validation failed: %w", i, err)
		}
	}

	// Validate talosctl image if specified
	talosctlRepoEmpty := kubernetes.Spec.Talosctl.Image.Repository == ""
	talosctlTagEmpty := kubernetes.Spec.Talosctl.Image.Tag == ""

	if talosctlRepoEmpty != talosctlTagEmpty {
		return fmt.Errorf("both spec.talosctl.image.repository and spec.talosctl.image.tag must be specified together, or both omitted for defaults")
	}

	// Validate pull policy if specified
	if kubernetes.Spec.Talosctl.Image.PullPolicy != "" {
		validPolicies := []corev1.PullPolicy{corev1.PullAlways, corev1.PullNever, corev1.PullIfNotPresent}
		if !slices.Contains(validPolicies, kubernetes.Spec.Talosctl.Image.PullPolicy) {
			return fmt.Errorf("spec.talosctl.image.pullPolicy '%s' is invalid. Valid values are: %v", kubernetes.Spec.Talosctl.Image.PullPolicy, validPolicies)
		}
	}

	return nil
}

func (v *KubernetesUpgradeValidator) validateHealthCheck(check tupprv1alpha1.HealthCheckSpec) error {
	if check.APIVersion == "" {
		return fmt.Errorf("apiVersion cannot be empty")
	}
	if check.Kind == "" {
		return fmt.Errorf("kind cannot be empty")
	}
	if check.Expr == "" {
		return fmt.Errorf("expr cannot be empty")
	}

	// Validate timeout if specified
	if check.Timeout != nil && check.Timeout.Duration <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	return nil
}

func (v *KubernetesUpgradeValidator) generateKubernetesWarnings(kubernetes *tupprv1alpha1.KubernetesUpgrade) []string {
	var warnings []string

	// Warn about health checks without timeouts
	for i, check := range kubernetes.Spec.HealthChecks {
		if check.Timeout == nil {
			warnings = append(warnings, fmt.Sprintf("Health check %d has no timeout specified, will use default 10 minutes", i))
		}
	}

	// Warn about pre-release versions
	if matched, _ := regexp.MatchString(`-[a-zA-Z]`, kubernetes.Spec.Kubernetes.Version); matched {
		warnings = append(warnings, fmt.Sprintf("Target version '%s' appears to be a pre-release. Ensure this version is stable and tested in your environment.", kubernetes.Spec.Kubernetes.Version))
	}

	// Warn if talosctl version is not specified (will auto-detect)
	if kubernetes.Spec.Talosctl.Image.Tag == "" {
		warnings = append(warnings, "No talosctl version specified, will auto-detect from cluster. Ensure talosctl version compatibility with your target Kubernetes version.")
	}

	return warnings
}

func (v *KubernetesUpgradeValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&tupprv1alpha1.KubernetesUpgrade{}).
		WithValidator(v).
		Complete()
}
