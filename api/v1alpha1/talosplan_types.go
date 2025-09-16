package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TalosPlanSpec defines the desired state of TalosPlan
type TalosPlanSpec struct {
	// Image is the Talos installer image to upgrade to
	// +kubebuilder:validation:Required
	Image ImageSpec `json:"image"`

	// Talosctl specifies the talosctl image configuration
	// +kubebuilder:default={"image":{"repository":"ghcr.io/siderolabs/talosctl","tag":"latest"}}
	Talosctl TalosctlSpec `json:"talosctl"`

	// Force the upgrade (skip checks on etcd health and members)
	// +kubebuilder:default=false
	Force bool `json:"force,omitempty"`

	// RebootMode select the reboot mode during upgrade
	// +kubebuilder:validation:Enum=default;powercycle
	// +kubebuilder:default="default"
	RebootMode string `json:"rebootMode,omitempty"`

	// Timeout for the upgrade operation
	// +kubebuilder:default="30m"
	Timeout string `json:"timeout,omitempty"`

	// MaxRetries maximum number of retries before marking as failed
	// +kubebuilder:default=3
	MaxRetries int `json:"maxRetries,omitempty"`

	// NodeSelector to target specific nodes for upgrade
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// TalosPlanStatus defines the observed state of TalosPlan
type TalosPlanStatus struct {
	// Phase represents the current phase of the upgrade
	// +kubebuilder:validation:Enum=Pending;InProgress;Completed;Failed
	Phase string `json:"phase,omitempty"`

	// CurrentNode is the node currently being upgraded
	CurrentNode string `json:"currentNode,omitempty"`

	// CompletedNodes are nodes that have been successfully upgraded
	CompletedNodes []string `json:"completedNodes,omitempty"`

	// FailedNodes are nodes that failed to upgrade
	FailedNodes []NodeUpgradeStatus `json:"failedNodes,omitempty"`

	// LastUpdated timestamp of last status update
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// Message provides details about the current state
	Message string `json:"message,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed spec
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// NodeUpgradeStatus tracks the upgrade status of individual nodes
type NodeUpgradeStatus struct {
	// NodeName is the name of the node
	NodeName string `json:"nodeName"`

	// Retries is the number of times upgrade was attempted
	Retries int `json:"retries"`

	// LastError contains the last error message
	LastError string `json:"lastError,omitempty"`

	// JobName is the name of the job handling this node's upgrade
	JobName string `json:"jobName,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Current Node",type="string",JSONPath=".status.currentNode"
//+kubebuilder:printcolumn:name="Completed",type="integer",JSONPath=".status.completedNodes"
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
