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

// NodeSelector defines how to select nodes for upgrade
type NodeSelectorSpec struct {
	// MatchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
	// map is equivalent to an element of matchExpressions, whose key field is "key", the
	// operator is "In", and the values array contains only "value".
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`

	// MatchExpressions is a list of label selector requirements. The requirements are ANDed.
	// +optional
	MatchExpressions []metav1.LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// Talos defines the talos configuration
type TalosSpec struct {
	// Version is the target Talos version to upgrade to (e.g., "v1.11.0")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9\-\.]+)?$`
	Version string `json:"version,omitempty"`
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
	Placement string `json:"placementPreset,omitempty"`

	// RebootMode select the reboot mode during upgrade
	// +kubebuilder:validation:Enum=default;powercycle
	// +kubebuilder:default="default"
	// +optional
	RebootMode string `json:"rebootMode,omitempty"`
}

// TalosUpgradeSpec defines the desired state of TalosUpgrade
type TalosUpgradeSpec struct {
	// HealthChecks defines a list of CEL-based health checks to perform before each node upgrade
	// +optional
	HealthChecks []HealthCheckSpec `json:"healthChecks,omitempty"`

	// NodeSelector specifies which nodes to target for the upgrade
	// If empty, all nodes will be targeted
	// +optional
	NodeSelector NodeSelectorSpec `json:"nodeSelector,omitempty"`

	// Talosctl specifies the talosctl configuration for upgrade operations
	// +optional
	Talos TalosSpec `json:"talos,omitempty"`

	// Talosctl specifies the talosctl configuration for upgrade operations
	// +optional
	Talosctl TalosctlSpec `json:"talosctl,omitempty"`

	// Policy configures upgrade behavior
	// +optional
	Policy PolicySpec `json:"policy,omitempty"`
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
