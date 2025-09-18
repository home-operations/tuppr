package v1alpha1

// ImageSpec defines container image details
type ImageSpec struct {
	// Repository is the container image repository
	// +kubebuilder:validation:Required
	Repository string `json:"repository"`

	// Tag is the container image tag
	// +kubebuilder:validation:Required
	Tag string `json:"tag"`

	// PullPolicy is the image pull policy
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	// +kubebuilder:default=IfNotPresent
	// +optional
	PullPolicy string `json:"pullPolicy,omitempty"`
}

// TalosctlSpec defines the talosctl configuration
type TalosctlSpec struct {
	// Image specifies the talosctl container image
	// +optional
	Image *ImageSpec `json:"image,omitempty"`
}
