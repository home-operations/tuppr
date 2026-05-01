package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubernetesSpec defines the target Kubernetes configuration
type KubernetesSpec struct {
	// Version is the target Kubernetes version to upgrade to (e.g., "v1.34.0")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9\-\.]+)?$`
	Version string `json:"version"`

	// ImageRepository overrides the registry+path prefix for Kubernetes component
	// images. When set, each component (kube-apiserver, kube-controller-manager,
	// kube-scheduler, kube-proxy, kubelet) is pulled from
	// "<imageRepository>/<component>:<version>".
	// +optional
	ImageRepository string `json:"imageRepository,omitempty"`

	// HostAliases are appended to the upgrade Job pod's /etc/hosts. The controller
	// auto-discovers an alias for cluster.controlPlane.endpoint from the live
	// machine config; entries here override that for any hostname they cover.
	// +optional
	HostAliases []corev1.HostAlias `json:"hostAliases,omitempty"`
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

	// StartedAt is the time the current upgrade attempt began
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// CompletedAt is the time the upgrade reached a terminal phase
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// History records past version transitions on this CR, newest first
	// +optional
	// +kubebuilder:validation:MaxItems=10
	History []UpgradeHistoryEntry `json:"history,omitempty"`
}

// UpgradeHistoryEntry records a single completed version transition
type UpgradeHistoryEntry struct {
	// FromVersion is the cluster version detected at the start of the run
	// +optional
	FromVersion string `json:"fromVersion,omitempty"`

	// ToVersion is the spec-target version at the time of completion
	// +kubebuilder:validation:Required
	ToVersion string `json:"toVersion"`

	// StartedAt is when the run began
	// +kubebuilder:validation:Required
	StartedAt metav1.Time `json:"startedAt"`

	// CompletedAt is when the run reached its terminal phase
	// +kubebuilder:validation:Required
	CompletedAt metav1.Time `json:"completedAt"`

	// Phase is the terminal phase reached (Completed or Failed)
	// +kubebuilder:validation:Required
	Phase JobPhase `json:"phase"`

	// Retries is the number of retries recorded during the run
	// +kubebuilder:validation:Minimum=0
	// +optional
	Retries int `json:"retries,omitempty"`

	// LastError is the final error message when Phase is Failed
	// +optional
	LastError string `json:"lastError,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Current",type="string",JSONPath=".status.currentVersion"
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".status.targetVersion"
// +kubebuilder:printcolumn:name="Controller Node",type="string",JSONPath=".status.controllerNode",priority=1
// +kubebuilder:printcolumn:name="Retries",type="integer",JSONPath=".status.retries",priority=1
// +kubebuilder:printcolumn:name="Completed",type="date",JSONPath=".status.completedAt",priority=1
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
