package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubernetesSpec defines the target Kubernetes configuration
type KubernetesSpec struct {
	// Version is the target Kubernetes version to upgrade to (e.g., "v1.34.0")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9\-\.]+)?$`
	Version string `json:"version"`
}

// KubernetesUpgradeSpec defines the desired state of KubernetesUpgrade
type KubernetesUpgradeSpec struct {
	// Kubernetes defines the target Kubernetes configuration
	// +kubebuilder:validation:Required
	Kubernetes KubernetesSpec `json:"kubernetes"`

	// Talosctl specifies the talosctl configuration for upgrade operations
	// +optional
	Talosctl TalosctlSpec `json:"talosctl,omitempty"`

	// HealthChecks defines a list of CEL-based health checks to perform before the upgrade
	// +optional
	HealthChecks []HealthCheckSpec `json:"healthChecks,omitempty"`

	// Maintenance configuration behavior for upgrade operations
	// +optional
	Maintenance *MaintenanceSpec `json:"maintenance,omitempty"`
}

// KubernetesUpgradeStatus defines the observed state of KubernetesUpgrade
type KubernetesUpgradeStatus struct {
	// Phase represents the current phase of the upgrade
	// +kubebuilder:validation:Enum=Pending;Draining;Upgrading;Rebooting;Completed;Failed
	// +optional
	Phase JobPhase `json:"phase,omitempty"`

	// ControllerNode is the controller node being used for the upgrade
	// +optional
	ControllerNode string `json:"controllerNode,omitempty"`

	// CurrentVersion is the current Kubernetes version detected in the cluster
	// +optional
	CurrentVersion string `json:"currentVersion,omitempty"`

	// TargetVersion is the target version from the spec
	// +optional
	TargetVersion string `json:"targetVersion,omitempty"`

	// LastUpdated timestamp of last status update
	// +optional
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// Message provides details about the current state
	// +optional
	Message string `json:"message,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// JobName is the name of the job handling the upgrade
	// +optional
	JobName string `json:"jobName,omitempty"`

	// Retries is the number of times the upgrade was attempted
	// +kubebuilder:validation:Minimum=0
	// +optional
	Retries int `json:"retries,omitempty"`

	// LastError contains the last error message
	// +optional
	LastError string `json:"lastError,omitempty"`

	// NextMaintenanceWindow reflect the next time a maintenance can happen
	// +optional
	NextMaintenanceWindow *metav1.Time `json:"nextMaintenanceWindow,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Current",type="string",JSONPath=".status.currentVersion"
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".status.targetVersion"
// +kubebuilder:printcolumn:name="Controller Node",type="string",JSONPath=".status.controllerNode",priority=1
// +kubebuilder:printcolumn:name="Retries",type="integer",JSONPath=".status.retries",priority=1
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// KubernetesUpgrade is the Schema for the kubernetesupgrades API
type KubernetesUpgrade struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubernetesUpgradeSpec   `json:"spec,omitempty"`
	Status KubernetesUpgradeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubernetesUpgradeList contains a list of KubernetesUpgrade
type KubernetesUpgradeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubernetesUpgrade `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubernetesUpgrade{}, &KubernetesUpgradeList{})
}
