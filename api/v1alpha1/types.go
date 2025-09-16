package v1alpha1

// ImageSpec defines container image configuration
type ImageSpec struct {
	// Repository is the container image repository
	// +kubebuilder:validation:Required
	Repository string `json:"repository"`

	// Tag is the container image tag
	// +kubebuilder:validation:Required
	Tag string `json:"tag"`
}

// TalosctlSpec defines the talosctl image configuration
type TalosctlSpec struct {
	// Image specifies the talosctl container image
	// +kubebuilder:default={"repository":"ghcr.io/siderolabs/talosctl","tag":"latest"}
	Image ImageSpec `json:"image"`
}
