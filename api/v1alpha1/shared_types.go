package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HealthCheck defines a CEL-based health check
type HealthCheckSpec struct {
	// APIVersion of the resource to check
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// Kind of the resource to check
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Name of the specific resource (optional, if empty checks all resources of this kind)
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace of the resource (optional, for namespaced resources)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// CEL expression that must evaluate to true for the check to pass
	// The resource object is available as 'object' and status as 'status'
	// +kubebuilder:validation:Required
	Expr string `json:"expr"`

	// Timeout for this health check
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern=`^([0-9]+[smh])+$`
	// +kubebuilder:validation:MinLength=2
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Description of what this check validates (for status/logging)
	// +optional
	Description string `json:"description,omitempty"`
}

// TalosctlImage defines talosctl container image details
type TalosctlImageSpec struct {
	// Repository is the talosctl container image repository
	// +kubebuilder:default="ghcr.io/siderolabs/talosctl"
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag is the talosctl container image tag
	// If not specified, defaults to the target version
	// +optional
	Tag string `json:"tag,omitempty"`

	// PullPolicy describes a policy for if/when to pull a container image
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	// +kubebuilder:default="IfNotPresent"
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

// Talosctl defines the talosctl configuration
type TalosctlSpec struct {
	// Image specifies the talosctl container image
	// +optional
	Image TalosctlImageSpec `json:"image,omitempty"`
}
