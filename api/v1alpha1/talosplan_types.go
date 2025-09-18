package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TalosImageSpec defines container image details
type TalosImageSpec struct {
	// Repository is the container image repository
	// +kubebuilder:validation:Required
	Repository string `json:"repository"`

	// Tag is the container image tag
	// +kubebuilder:validation:Required
	Tag string `json:"tag"`
}

// TalosPlanSpec defines the desired state of TalosPlan
type TalosPlanSpec struct {
	// Image is the Talos installer image to upgrade to
	// +kubebuilder:validation:Required
	Image TalosImageSpec `json:"image"`

	// Talosctl specifies the talosctl image to use for upgrade operations
	// +optional
	Talosctl *ImageSpec `json:"talosctl,omitempty"`

	// Force the upgrade (skip checks on etcd health and members)
	// +kubebuilder:default=false
	// +optional
	Force bool `json:"force,omitempty"`

	// RebootMode select the reboot mode during upgrade
	// +kubebuilder:validation:Enum=default;powercycle
	// +kubebuilder:default="default"
	// +optional
	RebootMode string `json:"rebootMode,omitempty"`

	// NodeSelector specifies which nodes to target for the upgrade
	// If not specified, all nodes will be targeted
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// TalosPlanStatus defines the observed state of TalosPlan
type TalosPlanStatus struct {
	// Phase represents the current phase of the upgrade
	// +kubebuilder:validation:Enum=Pending;InProgress;Completed;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// CurrentNode is the node currently being upgraded
	// +optional
	CurrentNode string `json:"currentNode,omitempty"`

	// CompletedNodes are nodes that have been successfully upgraded
	// +optional
	CompletedNodes []string `json:"completedNodes,omitempty"`

	// FailedNodes are nodes that failed to upgrade
	// +optional
	FailedNodes []NodeUpgradeStatus `json:"failedNodes,omitempty"`

	// LastUpdated timestamp of last status update
	// +optional
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// Message provides details about the current state
	// +optional
	Message string `json:"message,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// NodeUpgradeStatus tracks the upgrade status of individual nodes
type NodeUpgradeStatus struct {
	// NodeName is the name of the node
	// +kubebuilder:validation:Required
	NodeName string `json:"nodeName"`

	// Retries is the number of times upgrade was attempted
	// +kubebuilder:validation:Minimum=0
	// +optional
	Retries int `json:"retries"`

	// LastError contains the last error message
	// +optional
	LastError string `json:"lastError,omitempty"`

	// JobName is the name of the job handling this node's upgrade
	// +optional
	JobName string `json:"jobName,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Current Node",type="string",JSONPath=".status.currentNode"
//+kubebuilder:printcolumn:name="Completed",type="integer",JSONPath=".status.completedNodes",priority=1
//+kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=".status.failedNodes",priority=1
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// TalosPlan is the Schema for the talosupgrades API
type TalosPlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TalosPlanSpec   `json:"spec,omitempty"`
	Status TalosPlanStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TalosPlanList contains a list of TalosPlan
type TalosPlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TalosPlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TalosPlan{}, &TalosPlanList{})
}
