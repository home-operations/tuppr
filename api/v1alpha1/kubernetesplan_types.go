package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubernetesPlanSpec defines the desired state of KubernetesPlan
//
// Prerequisites:
// - A Secret named "talup" must exist in the same namespace containing the talosconfig
// - The secret must have a key "talosconfig" with valid Talos configuration data
// - At least one control plane node must be accessible via the provided talosconfig
type KubernetesPlanSpec struct {
	// Version specifies the target Kubernetes version to upgrade to
	// Example: "1.31.0"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[0-9]+\.[0-9]+\.[0-9]+$`
	Version string `json:"version"`

	// Talosctl specifies the talosctl image to use for upgrade operations
	// +optional
	Talosctl *ImageSpec `json:"talosctl,omitempty"`

	// PrePullImages determines whether to pre-pull images before upgrade
	// +kubebuilder:default=true
	// +optional
	PrePullImages *bool `json:"prePullImages,omitempty"`

	// NodeSelector specifies which nodes to target for the upgrade
	// If not specified, all control plane nodes will be targeted
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// KubernetesPlanStatus defines the observed state of KubernetesPlan
type KubernetesPlanStatus struct {
	// Phase represents the current phase of the Kubernetes upgrade
	// +optional
	Phase string `json:"phase,omitempty"`

	// Message provides additional information about the current status
	// +optional
	Message string `json:"message,omitempty"`

	// CurrentVersion represents the current Kubernetes version in the cluster
	// +optional
	CurrentVersion string `json:"currentVersion,omitempty"`

	// TargetVersion represents the target Kubernetes version for upgrade
	// +optional
	TargetVersion string `json:"targetVersion,omitempty"`

	// UpgradedNodes contains the list of control plane nodes that have been successfully upgraded
	// +optional
	UpgradedNodes []string `json:"upgradedNodes,omitempty"`

	// FailedNodes contains information about nodes that failed to upgrade
	// +optional
	FailedNodes []NodeUpgradeStatus `json:"failedNodes,omitempty"`

	// CurrentNode represents the node currently being upgraded
	// +optional
	CurrentNode string `json:"currentNode,omitempty"`

	// LastUpdated represents the last time the status was updated
	// +optional
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// ObservedGeneration represents the generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the resource's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// KubernetesPlan is the Schema for the kubernetesplans API
//
// KubernetesPlan manages Kubernetes control plane upgrades in a Talos cluster.
// It performs upgrades on control plane nodes sequentially to ensure cluster stability.
//
// Prerequisites:
// - A Secret named "talup" must exist in the same namespace containing the talosconfig
// - The secret must have a key "talosconfig" with valid Talos configuration data
// - Target control plane nodes must be accessible via the provided talosconfig
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Current Version",type=string,JSONPath=`.status.currentVersion`
// +kubebuilder:printcolumn:name="Target Version",type=string,JSONPath=`.status.targetVersion`
// +kubebuilder:printcolumn:name="Current Node",type=string,JSONPath=`.status.currentNode`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type KubernetesPlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubernetesPlanSpec   `json:"spec,omitempty"`
	Status KubernetesPlanStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KubernetesPlanList contains a list of KubernetesPlan
type KubernetesPlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubernetesPlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubernetesPlan{}, &KubernetesPlanList{})
}
