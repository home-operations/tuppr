package kubernetesupgrade

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
)

func (r *Reconciler) findActiveJob(ctx context.Context) (*batchv1.Job, error) {
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(r.ControllerNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/name": "kubernetes-upgrade",
		}); err != nil {
		return nil, err
	}

	for _, job := range jobList.Items {
		return &job, nil
	}

	return nil, nil
}

func (r *Reconciler) handleJobStatus(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.V(1).Info("Handling Kubernetes job status",
		"job", job.Name,
		"active", job.Status.Active,
		"succeeded", job.Status.Succeeded,
		"failed", job.Status.Failed,
		"backoffLimit", *job.Spec.BackoffLimit)

	if job.Status.Succeeded == 0 && (job.Status.Failed == 0 || job.Status.Failed < *job.Spec.BackoffLimit) {
		message := fmt.Sprintf("Upgrading Kubernetes to %s (job: %s)", kubernetesUpgrade.Spec.Kubernetes.Version, job.Name)
		if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
			"phase":   constants.PhaseInProgress,
			"message": message,
			"jobName": job.Name,
		}); err != nil {
			logger.Error(err, "Failed to update phase for active job", "job", job.Name)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}
		logger.V(1).Info("Kubernetes upgrade job is still active", "job", job.Name)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if job.Status.Succeeded > 0 {
		return r.handleJobSuccess(ctx, kubernetesUpgrade, job)
	}

	return r.handleJobFailure(ctx, kubernetesUpgrade, job)
}

func (r *Reconciler) handleJobSuccess(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Kubernetes upgrade job completed successfully", "job", job.Name)

	targetVersion := kubernetesUpgrade.Spec.Kubernetes.Version

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

	if err := r.cleanupJob(ctx, job); err != nil {
		logger.Error(err, "Failed to cleanup job, but continuing", "job", job.Name)
	}

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
	return ctrl.Result{RequeueAfter: time.Hour}, nil
}

func (r *Reconciler) handleJobFailure(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, job *batchv1.Job) (ctrl.Result, error) {
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

func (r *Reconciler) cleanupJob(ctx context.Context, job *batchv1.Job) error {
	logger := log.FromContext(ctx)
	logger.Info("Deleting successful Kubernetes upgrade job and its pods", "job", job.Name)

	deletePolicy := metav1.DeletePropagationForeground
	if err := r.Delete(ctx, job, &client.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		logger.Error(err, "Failed to delete successful job", "job", job.Name)
		return err
	}

	return nil
}

func (r *Reconciler) startUpgrade(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade) (ctrl.Result, error) {
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

func (r *Reconciler) findControllerNode(ctx context.Context, targetVersion string) (string, string, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return "", "", fmt.Errorf("failed to list nodes: %w", err)
	}

	for _, node := range nodeList.Items {
		if _, isController := node.Labels["node-role.kubernetes.io/control-plane"]; isController {
			if node.Status.NodeInfo.KubeletVersion != targetVersion {
				nodeIP, err := nodeutil.GetNodeIP(&node)
				if err != nil {
					continue
				}
				return node.Name, nodeIP, nil
			}
		}
	}

	return "", "", fmt.Errorf("no controller node found with node-role.kubernetes.io/control-plane label")
}

func (r *Reconciler) createJob(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, controllerNode, controllerIP string) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)

	job := r.buildJob(ctx, kubernetesUpgrade, controllerNode, controllerIP)
	if err := controllerutil.SetControllerReference(kubernetesUpgrade, job, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	logger.V(1).Info("Creating Kubernetes upgrade job", "job", job.Name, "controllerNode", controllerNode)

	if err := r.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return job, nil
}

func (r *Reconciler) buildJob(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, controllerNode, controllerIP string) *batchv1.Job {
	logger := log.FromContext(ctx)

	jobName := nodeutil.GenerateSafeJobName(kubernetesUpgrade.Name, controllerNode)

	labels := map[string]string{
		"app.kubernetes.io/name":                "kubernetes-upgrade",
		"app.kubernetes.io/instance":            kubernetesUpgrade.Name,
		"app.kubernetes.io/part-of":             "tuppr",
		"tuppr.home-operations.com/target-node": controllerNode,
	}

	talosctlRepo := constants.DefaultTalosctlImage
	if kubernetesUpgrade.Spec.Talosctl.Image.Repository != "" {
		talosctlRepo = kubernetesUpgrade.Spec.Talosctl.Image.Repository
	}

	talosctlTag := kubernetesUpgrade.Spec.Talosctl.Image.Tag
	if talosctlTag == "" {
		if currentVersion, err := r.TalosClient.GetNodeVersion(ctx, controllerIP); err == nil && currentVersion != "" {
			talosctlTag = currentVersion
			logger.V(1).Info("Using current node version for talosctl compatibility",
				"node", controllerNode, "currentVersion", currentVersion)
		} else {
			talosctlTag = constants.DefaultTalosctlTag
			logger.V(1).Info("Could not detect current version, using fallback version for talosctl",
				"node", controllerNode, "version", talosctlTag)
		}
	}

	talosctlImage := talosctlRepo + ":" + talosctlTag

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
