package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Talos defines the talos configuration
type TalosSpec struct {
	// Version is the target Talos version to upgrade to (e.g., "v1.11.0")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9\-\.]+)?$`
	Version string `json:"version,omitempty"`
}

// Policy defines upgrade behavior options
type PolicySpec struct {
	// Debug enables debug mode for the upgrade
	// +kubebuilder:default=true
	// +optional
	Debug bool `json:"debug,omitempty"`

	// Force the upgrade (skip checks on etcd health and members)
	// +kubebuilder:default=false
	// +optional
	Force bool `json:"force,omitempty"`

	// Placement controls how strictly upgrade jobs avoid the target node
	// hard: required avoidance (job will fail if can't avoid target node)
	// soft: preferred avoidance (job prefers to avoid but can run on target node)
	// +kubebuilder:validation:Enum=hard;soft
	// +kubebuilder:default="soft"
	// +optional
	Placement string `json:"placement,omitempty"`

	// RebootMode select the reboot mode during upgrade
	// +kubebuilder:validation:Enum=default;powercycle
	// +kubebuilder:default="default"
	// +optional
	RebootMode string `json:"rebootMode,omitempty"`

	// Stage the upgrade to perform it after a reboot
	// +kubebuilder:default=false
	// +optional
	Stage bool `json:"stage,omitempty"`

	// Timeout for the per-node talosctl upgrade command
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern=`^([0-9]+[smh])+$`
	// +kubebuilder:default="30m"
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// TalosUpgradeSpec defines the desired state of TalosUpgrade
type TalosUpgradeSpec struct {
	// HealthChecks defines a list of CEL-based health checks to perform before each node upgrade
	// +optional
	HealthChecks []HealthCheckSpec `json:"healthChecks,omitempty"`

	// Talos specifies the talos configuration for upgrade operations
	// +optional
	Talos TalosSpec `json:"talos,omitempty"`

	// Talosctl specifies the talosctl configuration for upgrade operations
	// +optional
	Talosctl TalosctlSpec `json:"talosctl,omitempty"`

	// Policy configures upgrade behavior
	// +optional
	Policy PolicySpec `json:"policy,omitempty"`

	// Maintenance configuration behavior for upgrade operations
	// +optional
	Maintenance *MaintenanceSpec `json:"maintenance,omitempty"`
}

// TalosUpgradeStatus defines the observed state of TalosUpgrade
type TalosUpgradeStatus struct {
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

	// NextMaintenanceWindow reflect the next time a maintenance can happen
	// +optional
	NextMaintenanceWindow *metav1.Time `json:"nextMaintenanceWindow,omitempty"`
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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Current Node",type="string",JSONPath=".status.currentNode"
// +kubebuilder:printcolumn:name="Completed",type="integer",JSONPath=".status.completedNodes",priority=1
// +kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=".status.failedNodes",priority=1
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// TalosUpgrade is the Schema for the talosupgrades API
type TalosUpgrade struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TalosUpgradeSpec   `json:"spec,omitempty"`
	Status TalosUpgradeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TalosUpgradeList contains a list of TalosUpgrade
type TalosUpgradeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TalosUpgrade `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TalosUpgrade{}, &TalosUpgradeList{})
}
