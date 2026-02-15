/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
)

const (
	// KubernetesUpgrade-specific constants
	KubernetesUpgradeFinalizer = "tuppr.home-operations.com/kubernetes-finalizer"

	// Job constants for Kubernetes upgrades
	KubernetesJobBackoffLimit     = 2    // Retry up to 2 times on failure
	KubernetesJobActiveDeadline   = 3600 // 60 minutes for Kubernetes upgrade
	KubernetesJobGracePeriod      = 300  // 5 minutes
	KubernetesJobTTLAfterFinished = 300  // 5 minutes
)

type VersionGetter interface {
	GetCurrentKubernetesVersion(ctx context.Context) (string, error)
}

// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=kubernetesupgrades,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=kubernetesupgrades/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=kubernetesupgrades/finalizers,verbs=update
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// KubernetesUpgradeReconciler reconciles a KubernetesUpgrade object
type KubernetesUpgradeReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	TalosConfigSecret   string
	ControllerNamespace string
	HealthChecker       HealthCheckRunner
	TalosClient         TalosClientInterface
	MetricsReporter     *MetricsReporter
	VersionGetter       VersionGetter
	now                 Now
}

// Reconcile is part of the main kubernetes reconciliation loop
func (r *KubernetesUpgradeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting KubernetesUpgrade reconciliation", "kubernetesupgrade", req.Name)

	var kubernetesUpgrade tupprv1alpha1.KubernetesUpgrade
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name}, &kubernetesUpgrade); err != nil {
		if errors.IsNotFound(err) {
			logger.V(1).Info("KubernetesUpgrade resource not found, likely deleted")
		} else {
			logger.Error(err, "Failed to get KubernetesUpgrade resource")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if kubernetesUpgrade.DeletionTimestamp != nil {
		return r.cleanup(ctx, &kubernetesUpgrade)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&kubernetesUpgrade, KubernetesUpgradeFinalizer) {
		controllerutil.AddFinalizer(&kubernetesUpgrade, KubernetesUpgradeFinalizer)
		return ctrl.Result{}, r.Update(ctx, &kubernetesUpgrade)
	}

	return r.processUpgrade(ctx, &kubernetesUpgrade)
}

// processUpgrade contains the main logic for handling the Kubernetes upgrade process
func (r *KubernetesUpgradeReconciler) processUpgrade(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"kubernetesupgrade", kubernetesUpgrade.Name,
		"generation", kubernetesUpgrade.Generation,
	)

	logger.V(1).Info("Starting Kubernetes upgrade processing")
	now := r.now.Now()

	// Check for suspend annotation first
	if suspended, err := r.handleSuspendAnnotation(ctx, kubernetesUpgrade); err != nil || suspended {
		return ctrl.Result{RequeueAfter: time.Minute * 30}, err
	}

	// Check for reset annotation
	if resetRequested, err := r.handleResetAnnotation(ctx, kubernetesUpgrade); err != nil || resetRequested {
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// Handle generation changes
	if reset, err := r.handleGenerationChange(ctx, kubernetesUpgrade); err != nil || reset {
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// Skip if already completed/failed
	if kubernetesUpgrade.Status.Phase == constants.PhaseCompleted || kubernetesUpgrade.Status.Phase == constants.PhaseFailed {
		logger.V(1).Info("Kubernetes upgrade already completed or failed", "phase", kubernetesUpgrade.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Check maintenance window - only gate start of work, not in-progress upgrades
	if kubernetesUpgrade.Status.Phase != constants.PhaseInProgress {
		maintenanceRes, err := CheckMaintenanceWindow(kubernetesUpgrade.Spec.Maintenance, now)
		if err != nil {
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		}
		if !maintenanceRes.Allowed {
			requeueAfter := time.Until(*maintenanceRes.NextWindowStart)
			if requeueAfter > 5*time.Minute {
				requeueAfter = 5 * time.Minute
			}
			nextTimestamp := maintenanceRes.NextWindowStart.Unix()
			r.MetricsReporter.RecordMaintenanceWindow(UpgradeTypeKubernetes, kubernetesUpgrade.Name, false, &nextTimestamp)
			if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
				"phase":                 constants.PhasePending,
				"controllerNode":        "",
				"message":               fmt.Sprintf("Waiting for maintenance window (next: %s)", maintenanceRes.NextWindowStart.Format(time.RFC3339)),
				"nextMaintenanceWindow": metav1.NewTime(*maintenanceRes.NextWindowStart),
			}); err != nil {
				logger.Error(err, "Failed to update status for maintenance window")
			}
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		r.MetricsReporter.RecordMaintenanceWindow(UpgradeTypeKubernetes, kubernetesUpgrade.Name, true, nil)
	}

	// Coordination check - only when not already InProgress (first-applied wins)
	if kubernetesUpgrade.Status.Phase != constants.PhaseInProgress {
		if blocked, message, err := IsAnotherUpgradeActive(ctx, r.Client, kubernetesUpgrade.Name, "kubernetes"); err != nil {
			logger.Error(err, "Failed to check for other active upgrades")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		} else if blocked {
			logger.Info("Blocked by another upgrade", "reason", message)
			if err := r.setPhase(ctx, kubernetesUpgrade, constants.PhasePending, "", message); err != nil {
				logger.Error(err, "Failed to update phase for coordination wait")
			}
			return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
		}
	}

	// Continue with existing upgrade logic...
	if activeJob, err := r.findActiveJob(ctx); err != nil {
		logger.Error(err, "Failed to find active jobs")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if activeJob != nil {
		logger.V(1).Info("Found active job, handling its status", "job", activeJob.Name)
		return r.handleJobStatus(ctx, kubernetesUpgrade, activeJob)
	}
	targetVersion := kubernetesUpgrade.Spec.Kubernetes.Version

	currentVersion, err := r.VersionGetter.GetCurrentKubernetesVersion(ctx)
	if err == nil {
		if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
			"currentVersion": currentVersion,
			"targetVersion":  targetVersion,
		}); err != nil {
			logger.Error(err, "Failed to update version status")
		}
	}

	allUpgraded, err := r.areAllControlPlaneNodesUpgraded(ctx, targetVersion)
	if err != nil {
		logger.Error(err, "Failed to verify control plane node versions")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if allUpgraded {
		logger.Info("All control plane nodes verified at target version", "version", targetVersion)

		// VIP Cache check
		if currentVersion != targetVersion {
			logger.Info("Nodes are updated but API server (VIP) still reports old version. Waiting for cache/LB update.")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		if err := r.setPhase(ctx, kubernetesUpgrade, constants.PhaseCompleted, "", fmt.Sprintf("Kubernetes successfully upgraded to %s", targetVersion)); err != nil {
			logger.Error(err, "Failed to update completion phase")
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}
		return ctrl.Result{RequeueAfter: time.Hour}, nil
	}

	// Add upgrade info to context for health check metrics
	ctx = context.WithValue(ctx, ContextKeyUpgradeType, UpgradeTypeKubernetes)
	ctx = context.WithValue(ctx, ContextKeyUpgradeName, kubernetesUpgrade.Name)

	logger.Info("Kubernetes upgrade needed", "current", currentVersion, "target", targetVersion)

	if err := r.HealthChecker.CheckHealth(ctx, kubernetesUpgrade.Spec.HealthChecks); err != nil {
		logger.Info("Health checks failed, will retry", "error", err.Error())
		if err := r.setPhase(ctx, kubernetesUpgrade, constants.PhasePending, "", fmt.Sprintf("Waiting for health checks: %s", err.Error())); err != nil {
			logger.Error(err, "Failed to update phase for health check")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return r.startUpgrade(ctx, kubernetesUpgrade)
}

// handleGenerationChange resets the upgrade if the spec generation has changed
func (r *KubernetesUpgradeReconciler) handleGenerationChange(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (bool, error) {
	if kubernetesUpgrade.Status.ObservedGeneration >= kubernetesUpgrade.Generation {
		return false, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Spec generation changed, resetting Kubernetes upgrade process",
		"generation", kubernetesUpgrade.Generation,
		"observed", kubernetesUpgrade.Status.ObservedGeneration,
		"newVersion", kubernetesUpgrade.Spec.Kubernetes.Version)

	// Reset status for new generation
	return true, r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":          constants.PhasePending,
		"controllerNode": "",
		"message":        fmt.Sprintf("Spec updated to %s, restarting upgrade process", kubernetesUpgrade.Spec.Kubernetes.Version),
		"jobName":        "",
		"retries":        0,
		"lastError":      "",
	})
}

// handleResetAnnotation checks for the reset annotation and resets the upgrade if found
func (r *KubernetesUpgradeReconciler) handleResetAnnotation(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (bool, error) {
	if kubernetesUpgrade.Annotations == nil {
		return false, nil
	}

	resetValue, hasReset := kubernetesUpgrade.Annotations[constants.ResetAnnotation]
	if !hasReset {
		return false, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Reset annotation found, clearing Kubernetes upgrade state", "resetValue", resetValue)

	// Remove the reset annotation and reset status
	newAnnotations := maps.Clone(kubernetesUpgrade.Annotations)
	maps.DeleteFunc(newAnnotations, func(k, v string) bool {
		return k == constants.ResetAnnotation
	})

	// Update both annotations and status
	kubernetesUpgrade.Annotations = newAnnotations
	if err := r.Update(ctx, kubernetesUpgrade); err != nil {
		logger.Error(err, "Failed to remove reset annotation")
		return false, err
	}

	// Reset the status
	if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":          constants.PhasePending,
		"controllerNode": "",
		"message":        "Reset requested via annotation",
		"jobName":        "",
		"retries":        0,
		"lastError":      "",
	}); err != nil {
		logger.Error(err, "Failed to reset status after annotation")
		return false, err
	}

	return true, nil
}

// handleSuspendAnnotation checks for the suspend annotation and pauses the controller if found
func (r *KubernetesUpgradeReconciler) handleSuspendAnnotation(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (bool, error) {
	if kubernetesUpgrade.Annotations == nil {
		return false, nil
	}

	suspendValue, isSuspended := kubernetesUpgrade.Annotations[constants.SuspendAnnotation]
	if !isSuspended {
		return false, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Suspend annotation found, controller is suspended",
		"suspendValue", suspendValue,
		"kubernetesupgrade", kubernetesUpgrade.Name)

	// Update status to indicate suspension
	message := fmt.Sprintf("Controller suspended via annotation (value: %s) - remove annotation to resume", suspendValue)
	if err := r.setPhase(ctx, kubernetesUpgrade, constants.PhasePending, "", message); err != nil {
		logger.Error(err, "Failed to update phase for suspension")
		return true, err
	}

	logger.V(1).Info("Controller suspended, no further processing will occur",
		"kubernetesupgrade", kubernetesUpgrade.Name,
		"suspendValue", suspendValue)

	return true, nil // Return true to indicate we should stop processing
}

// DiscoveryVersionGetter gets the Kubernetes version via the discovery API
type DiscoveryVersionGetter struct{}

func (d *DiscoveryVersionGetter) GetCurrentKubernetesVersion(ctx context.Context) (string, error) {
	config, err := ctrl.GetConfig()
	if err != nil {
		return "", fmt.Errorf("failed to get REST config: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create discovery client: %w", err)
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	serverVersion, err := discoveryClient.ServerVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get server version: %w", err)
	}

	version := serverVersion.GitVersion
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	return version, nil
}

// findControllerNode finds the first available controller node
func (r *KubernetesUpgradeReconciler) findControllerNode(ctx context.Context, targetVersion string) (string, string, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return "", "", fmt.Errorf("failed to list nodes: %w", err)
	}

	// Find first controller node that needs upgrade
	for _, node := range nodeList.Items {
		if _, isController := node.Labels["node-role.kubernetes.io/control-plane"]; isController {
			// If this node is NOT at the target version, it is our priority candidate
			if node.Status.NodeInfo.KubeletVersion != targetVersion {
				nodeIP, err := GetNodeIP(&node)
				if err != nil {
					continue
				}
				return node.Name, nodeIP, nil
			}
		}
	}

	return "", "", fmt.Errorf("no controller node found with node-role.kubernetes.io/control-plane label")
}

// startUpgrade creates a job to upgrade Kubernetes
func (r *KubernetesUpgradeReconciler) startUpgrade(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	targetVersion := kubernetesUpgrade.Spec.Kubernetes.Version
	controllerNode, controllerIP, err := r.findControllerNode(ctx, targetVersion)
	if err != nil {
		logger.Error(err, "Failed to find controller node")
		if err := r.setPhase(ctx, kubernetesUpgrade, constants.PhaseFailed, "", fmt.Sprintf("Failed to find controller node: %s", err.Error())); err != nil {
			logger.Error(err, "Failed to update phase for controller node failure")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	logger.Info("Starting Kubernetes upgrade", "controllerNode", controllerNode, "controllerIP", controllerIP)

	job, err := r.createJob(ctx, kubernetesUpgrade, controllerNode, controllerIP)
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes upgrade job")
		if err := r.setPhase(ctx, kubernetesUpgrade, constants.PhaseFailed, controllerNode, fmt.Sprintf("Failed to create job: %s", err.Error())); err != nil {
			logger.Error(err, "Failed to update phase for job creation failure")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	logger.Info("Successfully created Kubernetes upgrade job", "job", job.Name, "controllerNode", controllerNode)

	// Update status with job info
	if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":          constants.PhaseInProgress,
		"controllerNode": controllerNode,
		"jobName":        job.Name,
		"message":        fmt.Sprintf("Upgrading Kubernetes to %s on controller node %s", kubernetesUpgrade.Spec.Kubernetes.Version, controllerNode),
	}); err != nil {
		logger.Error(err, "Failed to update status for job creation")
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// createJob creates a Kubernetes Job to perform the Kubernetes upgrade
func (r *KubernetesUpgradeReconciler) createJob(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, controllerNode, controllerIP string) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)

	job := r.buildJob(ctx, kubernetesUpgrade, controllerNode, controllerIP)
	if err := controllerutil.SetControllerReference(kubernetesUpgrade, job, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	logger.V(1).Info("Creating Kubernetes upgrade job", "job", job.Name, "controllerNode", controllerNode)

	if err := r.Create(ctx, job); err != nil {
		if errors.IsAlreadyExists(err) {
			// Job already exists, just return it
			existingJob := &batchv1.Job{}
			if getErr := r.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, existingJob); getErr != nil {
				return nil, fmt.Errorf("failed to get existing job: %w", getErr)
			}
			logger.V(1).Info("Kubernetes upgrade job already exists, reusing", "job", job.Name)
			return existingJob, nil
		}
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return job, nil
}

// buildJob constructs a Kubernetes Job object for upgrading Kubernetes
func (r *KubernetesUpgradeReconciler) buildJob(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, controllerNode, controllerIP string) *batchv1.Job {
	logger := log.FromContext(ctx)

	jobName := GenerateSafeJobName(kubernetesUpgrade.Name, controllerNode)

	labels := map[string]string{
		"app.kubernetes.io/name":                "kubernetes-upgrade",
		"app.kubernetes.io/instance":            kubernetesUpgrade.Name,
		"app.kubernetes.io/part-of":             "tuppr",
		"tuppr.home-operations.com/target-node": controllerNode,
	}

	// Get talosctl image with defaults
	talosctlRepo := constants.DefaultTalosctlImage
	if kubernetesUpgrade.Spec.Talosctl.Image.Repository != "" {
		talosctlRepo = kubernetesUpgrade.Spec.Talosctl.Image.Repository
	}

	talosctlTag := kubernetesUpgrade.Spec.Talosctl.Image.Tag
	if talosctlTag == "" {
		// Try to detect the current Talos version for talosctl compatibility
		if currentVersion, err := r.TalosClient.GetNodeVersion(ctx, controllerIP); err == nil && currentVersion != "" {
			talosctlTag = currentVersion
			logger.V(1).Info("Using current node version for talosctl compatibility",
				"node", controllerNode, "currentVersion", currentVersion)
		} else {
			// This should never happen but lets fallback to 'latest' just in case
			talosctlTag = constants.DefaultTalosctlTag
			logger.V(1).Info("Could not detect current version, using fallback version for talosctl",
				"node", controllerNode, "version", talosctlTag, "error", err)
		}
	}

	talosctlImage := talosctlRepo + ":" + talosctlTag

	// Build upgrade arguments
	args := []string{
		"upgrade-k8s",
		"--nodes=" + controllerIP,
		"--to=" + kubernetesUpgrade.Spec.Kubernetes.Version,
	}

	pullPolicy := corev1.PullIfNotPresent
	if kubernetesUpgrade.Spec.Talosctl.Image.PullPolicy != "" {
		pullPolicy = kubernetesUpgrade.Spec.Talosctl.Image.PullPolicy
	}

	logger.V(1).Info("Building Kubernetes upgrade job specification",
		"controllerNode", controllerNode,
		"talosctlImage", talosctlImage,
		"pullPolicy", pullPolicy,
		"args", args)

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: r.ControllerNamespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To(int32(KubernetesJobBackoffLimit)),
			Completions:             ptr.To(int32(1)),
			TTLSecondsAfterFinished: ptr.To(int32(KubernetesJobTTLAfterFinished)),
			Parallelism:             ptr.To(int32(1)),
			ActiveDeadlineSeconds:   ptr.To(int64(KubernetesJobActiveDeadline)),
			PodReplacementPolicy:    ptr.To(batchv1.Failed),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy:                 corev1.RestartPolicyNever,
					TerminationGracePeriodSeconds: ptr.To(int64(KubernetesJobGracePeriod)),
					PriorityClassName:             "system-node-critical",
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(int64(65532)),
						RunAsGroup:   ptr.To(int64(65532)),
						FSGroup:      ptr.To(int64(65532)),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Operator: corev1.TolerationOpExists,
						},
					},
					Containers: []corev1.Container{{
						Name:            "upgrade-k8s",
						Image:           talosctlImage,
						ImagePullPolicy: pullPolicy,
						Args:            args,
						Env: []corev1.EnvVar{{
							Name:  "TALOSCONFIG",
							Value: "/var/run/secrets/talos.dev/talosconfig",
						}},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							ReadOnlyRootFilesystem:   ptr.To(true),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      constants.TalosSecretName,
							MountPath: "/var/run/secrets/talos.dev",
							ReadOnly:  true,
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: constants.TalosSecretName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName:  r.TalosConfigSecret,
								DefaultMode: ptr.To(int32(0420)),
							},
						},
					}},
				},
			},
		},
	}
}

// findActiveJob looks for any currently running job for this KubernetesUpgrade
func (r *KubernetesUpgradeReconciler) findActiveJob(ctx context.Context) (*batchv1.Job, error) {
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(r.ControllerNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/name": "kubernetes-upgrade",
		}); err != nil {
		return nil, err
	}

	// Return the first job we find - there should only be one since there's only one CR
	for _, job := range jobList.Items {
		return &job, nil
	}

	return nil, nil
}

// handleJobStatus processes the status of an active Kubernetes upgrade job
func (r *KubernetesUpgradeReconciler) handleJobStatus(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.V(1).Info("Handling Kubernetes job status",
		"job", job.Name,
		"active", job.Status.Active,
		"succeeded", job.Status.Succeeded,
		"failed", job.Status.Failed,
		"backoffLimit", *job.Spec.BackoffLimit)

	// Job still running
	if job.Status.Succeeded == 0 && (job.Status.Failed == 0 || job.Status.Failed < *job.Spec.BackoffLimit) {
		message := fmt.Sprintf("Upgrading Kubernetes to %s (job: %s)", kubernetesUpgrade.Spec.Kubernetes.Version, job.Name)
		if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
			"phase":   constants.PhaseInProgress,
			"message": message,
			"jobName": job.Name,
		}); err != nil {
			logger.Error(err, "Failed to update phase for active job", "job", job.Name)
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		}
		logger.V(1).Info("Kubernetes upgrade job is still active", "job", job.Name)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	// Job succeeded - verify upgrade
	if job.Status.Succeeded > 0 {
		return r.handleJobSuccess(ctx, kubernetesUpgrade, job)
	}

	// Job failed permanently
	return r.handleJobFailure(ctx, kubernetesUpgrade, job)
}

// handleJobSuccess verifies the upgrade and marks it as completed
func (r *KubernetesUpgradeReconciler) handleJobSuccess(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Kubernetes upgrade job completed successfully", "job", job.Name)

	targetVersion := kubernetesUpgrade.Spec.Kubernetes.Version

	// Verify the upgrade worked by checking the current version
	allUpgraded, err := r.areAllControlPlaneNodesUpgraded(ctx, targetVersion)
	if err != nil {
		logger.Error(err, "Failed to verify Kubernetes upgrade")
		return r.handleJobFailure(ctx, kubernetesUpgrade, job)
	}

	if allUpgraded {
		logger.Info("All control plane nodes are at target version", "version", targetVersion)
		if err := r.setPhase(ctx, kubernetesUpgrade, constants.PhaseCompleted, "", fmt.Sprintf("Cluster successfully upgraded to %s", targetVersion)); err != nil {
			logger.Error(err, "Failed to update completion phase")
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}
		return ctrl.Result{RequeueAfter: time.Hour}, nil
	}

	// Clean up the successful job immediately
	if err := r.cleanupJob(ctx, job); err != nil {
		logger.Error(err, "Failed to cleanup job, but continuing", "job", job.Name)
	}

	// Update status to completed
	if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":          constants.PhaseCompleted,
		"currentVersion": targetVersion,
		"message":        fmt.Sprintf("Successfully upgraded Kubernetes to %s", targetVersion),
		"jobName":        "",
	}); err != nil {
		logger.Error(err, "Failed to update completion status")
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}

	logger.Info("Successfully completed Kubernetes upgrade and cleaned up job", "version", targetVersion)
	return ctrl.Result{RequeueAfter: time.Hour}, nil // Check again in an hour
}

func (r *KubernetesUpgradeReconciler) areAllControlPlaneNodesUpgraded(ctx context.Context, targetVersion string) (bool, error) {
	nodeList := &corev1.NodeList{}
	opts := []client.ListOption{
		client.MatchingLabels{"node-role.kubernetes.io/control-plane": ""},
	}

	if err := r.List(ctx, nodeList, opts...); err != nil {
		return false, fmt.Errorf("failed to list control plane nodes: %w", err)
	}

	if len(nodeList.Items) == 0 {
		return false, fmt.Errorf("no control plane nodes found")
	}

	for _, node := range nodeList.Items {
		// KubeletVersion includes the 'v' prefix usually, ensuring we match the spec format
		if node.Status.NodeInfo.KubeletVersion != targetVersion {
			log.FromContext(ctx).Info("Control plane node not yet upgraded",
				"node", node.Name,
				"current", node.Status.NodeInfo.KubeletVersion,
				"target", targetVersion)
			return false, nil
		}
	}

	return true, nil
}

// handleJobFailure marks the upgrade as failed
func (r *KubernetesUpgradeReconciler) handleJobFailure(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Kubernetes upgrade job failed permanently", "job", job.Name)

	if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":     constants.PhaseFailed,
		"message":   "Kubernetes upgrade job failed permanently",
		"lastError": "Job failed permanently",
		"jobName":   job.Name,
	}); err != nil {
		logger.Error(err, "Failed to update failure status")
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}

	logger.Info("Successfully marked Kubernetes upgrade as failed")
	return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}

// cleanupJob removes the job after successful completion
func (r *KubernetesUpgradeReconciler) cleanupJob(ctx context.Context, job *batchv1.Job) error {
	logger := log.FromContext(ctx)
	logger.Info("Deleting successful Kubernetes upgrade job and its pods", "job", job.Name)

	// Use foreground deletion to ensure pods are cleaned up
	deletePolicy := metav1.DeletePropagationForeground
	if err := r.Delete(ctx, job, &client.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "Failed to delete successful job", "job", job.Name)
		return err
	}

	return nil
}

// cleanup handles finalization logic when a KubernetesUpgrade is deleted
func (r *KubernetesUpgradeReconciler) cleanup(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Cleaning up KubernetesUpgrade", "name", kubernetesUpgrade.Name)

	// Jobs will be cleaned up automatically via owner references
	logger.V(1).Info("Removing finalizer", "name", kubernetesUpgrade.Name, "finalizer", KubernetesUpgradeFinalizer)
	controllerutil.RemoveFinalizer(kubernetesUpgrade, KubernetesUpgradeFinalizer)

	if err := r.Update(ctx, kubernetesUpgrade); err != nil {
		logger.Error(err, "Failed to remove finalizer", "name", kubernetesUpgrade.Name)
		return ctrl.Result{}, err
	}

	logger.Info("Successfully cleaned up KubernetesUpgrade", "name", kubernetesUpgrade.Name)
	return ctrl.Result{}, nil
}

// updateStatus applies a partial update to the status subresource
func (r *KubernetesUpgradeReconciler) updateStatus(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, updates map[string]any) error {
	logger := log.FromContext(ctx)

	// Always include generation and timestamp
	updates["observedGeneration"] = kubernetesUpgrade.Generation
	updates["lastUpdated"] = metav1.Now()

	patch := map[string]any{
		"status": updates,
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	logger.V(1).Info("Applying Kubernetes status patch", "patch", string(patchBytes))

	statusObj := &tupprv1alpha1.KubernetesUpgrade{}
	statusObj.Name = kubernetesUpgrade.Name

	return r.Status().Patch(ctx, statusObj, client.RawPatch(types.MergePatchType, patchBytes))
}

// setPhase updates the phase, controller node, and message in status
func (r *KubernetesUpgradeReconciler) setPhase(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, phase, controllerNode, message string) error {
	// Record phase change metrics
	r.MetricsReporter.RecordKubernetesUpgradePhase(kubernetesUpgrade.Name, phase)

	return r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":          phase,
		"controllerNode": controllerNode,
		"message":        message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubernetesUpgradeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := ctrl.Log.WithName("setup")
	logger.Info("Setting up KubernetesUpgrade controller with manager")

	// Only create dependencies if not already set (allows mock injection for testing)
	if r.MetricsReporter == nil {
		r.MetricsReporter = NewMetricsReporter()
	}
	if r.HealthChecker == nil {
		r.HealthChecker = &HealthChecker{Client: mgr.GetClient(), MetricsReporter: r.MetricsReporter}
	}
	if r.VersionGetter == nil {
		r.VersionGetter = &DiscoveryVersionGetter{}
	}
	if r.TalosClient == nil {
		talosClient, err := NewTalosClient(context.Background())
		if err != nil {
			return fmt.Errorf("failed to create talos client: %w", err)
		}
		r.TalosClient = talosClient
	}
	r.now = &realClock{}

	return ctrl.NewControllerManagedBy(mgr).
		For(&tupprv1alpha1.KubernetesUpgrade{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
