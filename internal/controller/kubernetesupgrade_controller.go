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
	"slices"
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
	KubernetesJobBackoffLimit       = 2     // Retry up to 2 times on failure
	KubernetesJobActiveDeadline     = 3600  // 60 minutes for Kubernetes upgrade
	KubernetesJobGracePeriod        = 300   // 5 minutes
	KubernetesJobTTLAfterFinished   = 300   // 5 minutes
	KubernetesJobTalosHealthTimeout = "10m" // Health check timeout
)

// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=kubernetesupgrades,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=kubernetesupgrades/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=kubernetesupgrades/finalizers,verbs=update
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
	HealthChecker       *HealthChecker
	TalosClient         *TalosClient
}

// Reconcile is part of the main kubernetes reconciliation loop
func (r *KubernetesUpgradeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting KubernetesUpgrade reconciliation", "kubernetesupgrade", req.Name)

	var kubernetesUpgrade tupprv1alpha1.KubernetesUpgrade
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name}, &kubernetesUpgrade); err != nil {
		if errors.IsNotFound(err) {
			logger.V(1).Info("KubernetesUpgrade resource not found, likely deleted", "kubernetesupgrade", req.Name)
		} else {
			logger.Error(err, "Failed to get KubernetesUpgrade resource", "kubernetesupgrade", req.Name)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.V(1).Info("Retrieved KubernetesUpgrade",
		"name", kubernetesUpgrade.Name,
		"phase", kubernetesUpgrade.Status.Phase,
		"generation", kubernetesUpgrade.Generation,
		"observedGeneration", kubernetesUpgrade.Status.ObservedGeneration)

	// Handle deletion
	if kubernetesUpgrade.DeletionTimestamp != nil {
		logger.Info("KubernetesUpgrade is being deleted, starting cleanup", "name", kubernetesUpgrade.Name)
		return r.cleanup(ctx, &kubernetesUpgrade)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&kubernetesUpgrade, KubernetesUpgradeFinalizer) {
		logger.V(1).Info("Adding finalizer to KubernetesUpgrade", "name", kubernetesUpgrade.Name, "finalizer", KubernetesUpgradeFinalizer)
		controllerutil.AddFinalizer(&kubernetesUpgrade, KubernetesUpgradeFinalizer)
		return ctrl.Result{}, r.Update(ctx, &kubernetesUpgrade)
	}

	// Process upgrade
	logger.V(1).Info("Processing Kubernetes upgrade", "name", kubernetesUpgrade.Name, "currentPhase", kubernetesUpgrade.Status.Phase)
	return r.processKubernetesUpgrade(ctx, &kubernetesUpgrade)
}

// processKubernetesUpgrade contains the main logic for handling the Kubernetes upgrade process
func (r *KubernetesUpgradeReconciler) processKubernetesUpgrade(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"kubernetesupgrade", kubernetesUpgrade.Name,
		"generation", kubernetesUpgrade.Generation,
	)

	logger.V(1).Info("Starting Kubernetes upgrade processing")

	// Handle generation changes
	if reset, err := r.handleKubernetesGenerationChange(ctx, kubernetesUpgrade); err != nil || reset {
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// Skip if already completed/failed (and generation matches)
	if kubernetesUpgrade.Status.Phase == constants.PhaseCompleted || kubernetesUpgrade.Status.Phase == constants.PhaseFailed {
		logger.V(1).Info("Kubernetes upgrade already completed or failed", "phase", kubernetesUpgrade.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Check for active job FIRST - if there's a job running, handle it regardless of version
	if activeJob, err := r.findActiveKubernetesJob(ctx, kubernetesUpgrade); err != nil {
		logger.Error(err, "Failed to find active jobs")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if activeJob != nil {
		logger.V(1).Info("Found active job, handling its status", "job", activeJob.Name)
		return r.handleKubernetesJobStatus(ctx, kubernetesUpgrade, activeJob)
	}

	// Only check version after confirming no jobs are running
	// Check if upgrade is needed by comparing current vs target version
	currentVersion, err := r.getCurrentKubernetesVersion(ctx)
	if err != nil {
		logger.Error(err, "Failed to get current Kubernetes version")
		if setErr := r.setKubernetesPhase(ctx, kubernetesUpgrade, constants.PhaseFailed, "", fmt.Sprintf("Failed to get current version: %s", err.Error())); setErr != nil {
			logger.Error(setErr, "Failed to update phase for version detection failure")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	targetVersion := kubernetesUpgrade.Spec.Kubernetes.Version

	// Update status with current and target versions
	if err := r.updateKubernetesStatus(ctx, kubernetesUpgrade, map[string]any{
		"currentVersion": currentVersion,
		"targetVersion":  targetVersion,
	}); err != nil {
		logger.Error(err, "Failed to update version status")
	}

	// Check if upgrade is needed
	if currentVersion == targetVersion {
		logger.Info("Kubernetes is already at target version", "current", currentVersion, "target", targetVersion)
		if err := r.setKubernetesPhase(ctx, kubernetesUpgrade, constants.PhaseCompleted, "", fmt.Sprintf("Kubernetes already at target version %s", targetVersion)); err != nil {
			logger.Error(err, "Failed to update completion phase")
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}
		return ctrl.Result{RequeueAfter: time.Hour}, nil // Check again in an hour
	}

	logger.Info("Kubernetes upgrade needed", "current", currentVersion, "target", targetVersion)

	// Perform health checks before starting upgrade
	if err := r.HealthChecker.CheckHealth(ctx, kubernetesUpgrade.Spec.HealthChecks); err != nil {
		logger.Info("Health checks failed, will retry", "error", err.Error())
		if setErr := r.setKubernetesPhase(ctx, kubernetesUpgrade, constants.PhasePending, "", fmt.Sprintf("Waiting for health checks: %s", err.Error())); setErr != nil {
			logger.Error(setErr, "Failed to update phase for health check")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Find controller node and start upgrade
	return r.startKubernetesUpgrade(ctx, kubernetesUpgrade)
}

// handleKubernetesGenerationChange resets the upgrade if the spec generation has changed
func (r *KubernetesUpgradeReconciler) handleKubernetesGenerationChange(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (bool, error) {
	if kubernetesUpgrade.Status.ObservedGeneration == 0 || kubernetesUpgrade.Status.ObservedGeneration >= kubernetesUpgrade.Generation {
		return false, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Spec generation changed, resetting Kubernetes upgrade process",
		"generation", kubernetesUpgrade.Generation,
		"observed", kubernetesUpgrade.Status.ObservedGeneration,
		"newVersion", kubernetesUpgrade.Spec.Kubernetes.Version)

	// Reset status for new generation
	return true, r.updateKubernetesStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":          constants.PhasePending,
		"controllerNode": "",
		"message":        fmt.Sprintf("Spec updated to %s, restarting upgrade process", kubernetesUpgrade.Spec.Kubernetes.Version),
		"jobName":        "",
		"retries":        0,
		"lastError":      "",
	})
}

// getCurrentKubernetesVersion gets the current Kubernetes version from the cluster
func (r *KubernetesUpgradeReconciler) getCurrentKubernetesVersion(ctx context.Context) (string, error) {
	// Use the existing REST config from the manager instead of ctrl.GetConfigOrDie()
	config, err := ctrl.GetConfig()
	if err != nil {
		return "", fmt.Errorf("failed to get REST config: %w", err)
	}

	// Create discovery client
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create discovery client: %w", err)
	}

	// Check if context is cancelled before making the call
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// Get server version
	serverVersion, err := discoveryClient.ServerVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get server version: %w", err)
	}

	// Return version in format that matches our spec (with 'v' prefix)
	version := serverVersion.GitVersion
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	return version, nil
}

// Replace manual slice searching with slices functions
func (r *KubernetesUpgradeReconciler) findControllerNode(ctx context.Context) (string, string, error) {
	logger := log.FromContext(ctx)

	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return "", "", fmt.Errorf("failed to list nodes: %w", err)
	}

	// Use slices.IndexFunc for finding controller nodes
	controllerIdx := slices.IndexFunc(nodeList.Items, func(node corev1.Node) bool {
		_, isController := node.Labels["node-role.kubernetes.io/control-plane"]
		return isController
	})

	if controllerIdx == -1 {
		return "", "", fmt.Errorf("no controller node found with node-role.kubernetes.io/control-plane label")
	}

	node := nodeList.Items[controllerIdx]
	nodeIP, err := GetNodeInternalIP(&node)
	if err != nil {
		return "", "", fmt.Errorf("failed to get IP for controller node %s: %w", node.Name, err)
	}

	logger.Info("Found controller node", "node", node.Name, "ip", nodeIP)
	return node.Name, nodeIP, nil
}

// startKubernetesUpgrade creates a job to upgrade Kubernetes
func (r *KubernetesUpgradeReconciler) startKubernetesUpgrade(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	controllerNode, controllerIP, err := r.findControllerNode(ctx)
	if err != nil {
		logger.Error(err, "Failed to find controller node")
		if setErr := r.setKubernetesPhase(ctx, kubernetesUpgrade, constants.PhaseFailed, "", fmt.Sprintf("Failed to find controller node: %s", err.Error())); setErr != nil {
			logger.Error(setErr, "Failed to update phase for controller node failure")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	logger.Info("Starting Kubernetes upgrade", "controllerNode", controllerNode, "controllerIP", controllerIP)

	job, err := r.createKubernetesJob(ctx, kubernetesUpgrade, controllerNode, controllerIP)
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes upgrade job")
		if setErr := r.setKubernetesPhase(ctx, kubernetesUpgrade, constants.PhaseFailed, controllerNode, fmt.Sprintf("Failed to create job: %s", err.Error())); setErr != nil {
			logger.Error(setErr, "Failed to update phase for job creation failure")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	logger.Info("Successfully created Kubernetes upgrade job", "job", job.Name, "controllerNode", controllerNode)

	// Update status with job info
	if err := r.updateKubernetesStatus(ctx, kubernetesUpgrade, map[string]any{
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

// createKubernetesJob creates a Kubernetes Job to perform the Kubernetes upgrade
func (r *KubernetesUpgradeReconciler) createKubernetesJob(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, controllerNode, controllerIP string) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)

	job := r.buildKubernetesJob(ctx, kubernetesUpgrade, controllerNode, controllerIP)
	if err := controllerutil.SetControllerReference(kubernetesUpgrade, job, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	logger.V(1).Info("Creating Kubernetes upgrade job in cluster", "job", job.Name, "controllerNode", controllerNode)

	if err := r.Create(ctx, job); err != nil {
		if errors.IsAlreadyExists(err) {
			existingJob := &batchv1.Job{}
			if getErr := r.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, existingJob); getErr != nil {
				return nil, fmt.Errorf("failed to get existing job: %w", getErr)
			}

			existingGen := existingJob.Labels["tuppr.home-operations.com/generation"]
			currentGen := fmt.Sprintf("%d", kubernetesUpgrade.Generation)

			if existingGen == currentGen {
				logger.V(1).Info("Kubernetes upgrade job already exists for current generation, reusing", "job", job.Name)
				return existingJob, nil
			} else {
				return nil, fmt.Errorf("job generation conflict: existing=%s, current=%s", existingGen, currentGen)
			}
		}
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return job, nil
}

// buildKubernetesJob constructs a Kubernetes Job object for upgrading Kubernetes
func (r *KubernetesUpgradeReconciler) buildKubernetesJob(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, controllerNode, controllerIP string) *batchv1.Job {
	logger := log.FromContext(ctx)

	jobName := GenerateSafeJobName(kubernetesUpgrade.Name, controllerNode)

	labels := map[string]string{
		"app.kubernetes.io/name":                "kubernetes-upgrade",
		"app.kubernetes.io/instance":            kubernetesUpgrade.Name,
		"app.kubernetes.io/part-of":             "tuppr",
		"tuppr.home-operations.com/target-node": controllerNode,
		"tuppr.home-operations.com/generation":  fmt.Sprintf("%d", kubernetesUpgrade.Generation),
	}

	// Get talosctl image with defaults
	talosctlRepo := constants.DefaultTalosctlImage
	if kubernetesUpgrade.Spec.Talosctl.Image.Repository != "" {
		talosctlRepo = kubernetesUpgrade.Spec.Talosctl.Image.Repository
	}

	talosctlTag := kubernetesUpgrade.Spec.Talosctl.Image.Tag
	if talosctlTag == "" {
		// Auto-detect talosctl version from current Talos version
		if detectedTag := r.detectTalosctlVersion(ctx, controllerIP); detectedTag != "" {
			talosctlTag = detectedTag
			logger.Info("Auto-detected talosctl version from Talos node",
				"controllerNode", controllerNode,
				"controllerIP", controllerIP,
				"talosctlVersion", talosctlTag)
		} else {
			talosctlTag = constants.DefaultTalosctlTag
			logger.Info("Failed to auto-detect talosctl version, using fallback",
				"controllerNode", controllerNode,
				"fallbackVersion", talosctlTag)
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
					InitContainers: []corev1.Container{{
						Name:            "health",
						Image:           talosctlImage,
						ImagePullPolicy: pullPolicy,
						Args:            []string{"health", "--nodes=" + controllerIP, "--wait-timeout=" + KubernetesJobTalosHealthTimeout},
						Env: []corev1.EnvVar{
							{
								Name:  "TALOSCONFIG",
								Value: "/var/run/secrets/talos.dev/config",
							},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							ReadOnlyRootFilesystem:   ptr.To(true),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1m"),
								corev1.ResourceMemory: resource.MustParse("8Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      constants.TalosSecretName,
							MountPath: "/var/run/secrets/talos.dev",
							ReadOnly:  true,
						}},
					}},
					Containers: []corev1.Container{{
						Name:            "upgrade-k8s",
						Image:           talosctlImage,
						ImagePullPolicy: pullPolicy,
						Args:            args,
						Env: []corev1.EnvVar{
							{
								Name:  "TALOSCONFIG",
								Value: "/var/run/secrets/talos.dev/config",
							},
						},
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
								corev1.ResourceMemory: resource.MustParse("32Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
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

// detectTalosctlVersion attempts to detect the appropriate talosctl version from the controller node
func (r *KubernetesUpgradeReconciler) detectTalosctlVersion(ctx context.Context, controllerIP string) string {
	logger := log.FromContext(ctx)

	// Use a shorter timeout for version detection to avoid blocking the upgrade
	detectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	nodeInfo, err := r.TalosClient.GetNodeInfo(detectCtx, controllerIP)
	if err != nil {
		logger.V(1).Info("Failed to get node info for talosctl version detection",
			"controllerIP", controllerIP,
			"error", err.Error())
		return ""
	}

	if nodeInfo.TalosVersion == "" {
		logger.V(1).Info("Node info returned empty Talos version", "controllerIP", controllerIP)
		return ""
	}

	// Use the same version as the running Talos version
	// This ensures compatibility between talosctl and the Talos cluster
	talosVersion := nodeInfo.TalosVersion

	// Ensure the version has a 'v' prefix for consistency
	if !strings.HasPrefix(talosVersion, "v") {
		talosVersion = "v" + talosVersion
	}

	logger.V(1).Info("Successfully detected Talos version for talosctl",
		"controllerIP", controllerIP,
		"talosVersion", talosVersion)

	return talosVersion
}

// findActiveKubernetesJob looks for any currently running job for this KubernetesUpgrade
func (r *KubernetesUpgradeReconciler) findActiveKubernetesJob(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)

	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(r.ControllerNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     "kubernetes-upgrade",
			"app.kubernetes.io/instance": kubernetesUpgrade.Name,
		}); err != nil {
		return nil, err
	}

	logger.V(1).Info("Found jobs for KubernetesUpgrade", "count", len(jobList.Items))

	for _, job := range jobList.Items {
		// Return the first job we find - there should only be one active at a time
		logger.V(1).Info("Found job that needs handling",
			"job", job.Name,
			"active", job.Status.Active,
			"succeeded", job.Status.Succeeded,
			"failed", job.Status.Failed)
		return &job, nil
	}

	return nil, nil
}

// handleKubernetesJobStatus processes the status of an active Kubernetes upgrade job
func (r *KubernetesUpgradeReconciler) handleKubernetesJobStatus(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, job *batchv1.Job) (ctrl.Result, error) {
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
		if err := r.updateKubernetesStatus(ctx, kubernetesUpgrade, map[string]any{
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
		return r.handleKubernetesJobSuccess(ctx, kubernetesUpgrade, job)
	}

	// Job failed permanently
	return r.handleKubernetesJobFailure(ctx, kubernetesUpgrade, job)
}

// handleKubernetesJobSuccess verifies the upgrade and marks it as completed
func (r *KubernetesUpgradeReconciler) handleKubernetesJobSuccess(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Kubernetes upgrade job completed successfully", "job", job.Name)

	// Verify the upgrade worked by checking the current version
	currentVersion, err := r.getCurrentKubernetesVersion(ctx)
	if err != nil {
		logger.Error(err, "Failed to verify Kubernetes upgrade")
		return r.handleKubernetesJobFailure(ctx, kubernetesUpgrade, job)
	}

	targetVersion := kubernetesUpgrade.Spec.Kubernetes.Version
	if currentVersion != targetVersion {
		logger.Error(fmt.Errorf("version mismatch after upgrade"), "Kubernetes upgrade verification failed",
			"current", currentVersion, "target", targetVersion)
		return r.handleKubernetesJobFailure(ctx, kubernetesUpgrade, job)
	}

	// Clean up the successful job immediately
	if err := r.cleanupKubernetesJob(ctx, job); err != nil {
		logger.Error(err, "Failed to cleanup job, but continuing", "job", job.Name)
	}

	// Update status to completed
	if err := r.updateKubernetesStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":          constants.PhaseCompleted,
		"currentVersion": currentVersion,
		"message":        fmt.Sprintf("Successfully upgraded Kubernetes to %s", currentVersion),
		"jobName":        "",
	}); err != nil {
		logger.Error(err, "Failed to update completion status")
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}

	logger.Info("Successfully completed Kubernetes upgrade and cleaned up job", "version", currentVersion)
	return ctrl.Result{RequeueAfter: time.Hour}, nil // Check again in an hour
}

// handleKubernetesJobFailure marks the upgrade as failed
func (r *KubernetesUpgradeReconciler) handleKubernetesJobFailure(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Kubernetes upgrade job failed permanently", "job", job.Name)

	retries := kubernetesUpgrade.Status.Retries + 1
	message := fmt.Sprintf("Kubernetes upgrade job failed (attempt %d)", retries)

	if err := r.updateKubernetesStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":     constants.PhaseFailed,
		"message":   message,
		"retries":   retries,
		"lastError": "Job failed permanently",
		"jobName":   job.Name,
	}); err != nil {
		logger.Error(err, "Failed to update failure status")
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}

	logger.Info("Successfully marked Kubernetes upgrade as failed")
	return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}

// cleanupKubernetesJob removes the job after successful completion
func (r *KubernetesUpgradeReconciler) cleanupKubernetesJob(ctx context.Context, job *batchv1.Job) error {
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

// updateKubernetesStatus applies a partial update to the status subresource
func (r *KubernetesUpgradeReconciler) updateKubernetesStatus(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, updates map[string]any) error {
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

// setKubernetesPhase updates the phase, controller node, and message in status
func (r *KubernetesUpgradeReconciler) setKubernetesPhase(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, phase, controllerNode, message string) error {
	return r.updateKubernetesStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":          phase,
		"controllerNode": controllerNode,
		"message":        message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubernetesUpgradeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := ctrl.Log.WithName("setup")
	logger.Info("Setting up KubernetesUpgrade controller with manager")

	// Initialize shared components
	r.HealthChecker = &HealthChecker{Client: mgr.GetClient()}
	r.TalosClient = NewTalosClient(mgr.GetClient(), r.TalosConfigSecret, r.ControllerNamespace)

	return ctrl.NewControllerManagedBy(mgr).
		For(&tupprv1alpha1.KubernetesUpgrade{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
