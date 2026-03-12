package kubernetesupgrade

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/jobs"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
	"github.com/home-operations/tuppr/internal/metrics"
)

func (r *Reconciler) findActiveJob(ctx context.Context) (*batchv1.Job, error) {
	return jobs.FindActiveJobByLabel(ctx, r.Client, r.ControllerNamespace, "kubernetes-upgrade")
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
		prevPhase := kubernetesUpgrade.Status.Phase
		if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
			"phase":   tupprv1alpha1.JobPhaseUpgrading,
			"message": message,
			"jobName": job.Name,
		}); err != nil {
			logger.Error(err, "Failed to update phase for active job", "job", job.Name)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}
		kubernetesUpgrade.Status.Phase = tupprv1alpha1.JobPhaseUpgrading
		r.recordPhaseTransition(kubernetesUpgrade, prevPhase, tupprv1alpha1.JobPhaseUpgrading)
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
	logger.V(1).Info("Kubernetes upgrade job completed, verifying", "job", job.Name)

	nodeName := job.Labels["tuppr.home-operations.com/target-node"]
	targetVersion := kubernetesUpgrade.Spec.Kubernetes.Version

	allUpgraded, err := r.areAllControlPlaneNodesUpgraded(ctx, targetVersion)
	if err != nil {
		logger.Error(err, "Failed to verify Kubernetes upgrade")
		return r.handleJobFailure(ctx, kubernetesUpgrade, job)
	}

	if allUpgraded {
		logger.Info("All control plane nodes at target version", "version", targetVersion)
		r.MetricsReporter.EndJobTiming(metrics.UpgradeTypeKubernetes, kubernetesUpgrade.Name, nodeName, "success")
		r.MetricsReporter.RecordActiveJobs(metrics.UpgradeTypeKubernetes, 0)
		if err := r.setPhase(ctx, kubernetesUpgrade, tupprv1alpha1.JobPhaseCompleted, "", fmt.Sprintf("Cluster successfully upgraded to %s", targetVersion)); err != nil {
			logger.Error(err, "Failed to update completion phase")
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}
		return ctrl.Result{RequeueAfter: time.Hour}, nil
	}

	if err := r.cleanupJob(ctx, job); err != nil {
		logger.Error(err, "Failed to cleanup job, but continuing", "job", job.Name)
	}

	logger.Info("Node upgraded, continuing to next control plane node", "version", targetVersion)
	prevPhase := kubernetesUpgrade.Status.Phase
	if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":          tupprv1alpha1.JobPhaseUpgrading,
		"controllerNode": "",
		"message":        fmt.Sprintf("Upgrading Kubernetes to %s, continuing to next node", targetVersion),
		"jobName":        "",
	}); err != nil {
		logger.Error(err, "Failed to update status after partial upgrade")
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}
	kubernetesUpgrade.Status.Phase = tupprv1alpha1.JobPhaseUpgrading
	r.recordPhaseTransition(kubernetesUpgrade, prevPhase, tupprv1alpha1.JobPhaseUpgrading)
	r.MetricsReporter.EndJobTiming(metrics.UpgradeTypeKubernetes, kubernetesUpgrade.Name, nodeName, "success")
	r.MetricsReporter.RecordActiveJobs(metrics.UpgradeTypeKubernetes, 0)

	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

func (r *Reconciler) handleJobFailure(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Kubernetes upgrade job failed", "job", job.Name)

	nodeName := job.Labels["tuppr.home-operations.com/target-node"]
	prevPhase := kubernetesUpgrade.Status.Phase
	if err := r.updateStatus(ctx, kubernetesUpgrade, map[string]any{
		"phase":              tupprv1alpha1.JobPhaseFailed,
		"message":            "Kubernetes upgrade job failed permanently",
		"lastError":          "Job failed permanently",
		"jobName":            job.Name,
		"observedGeneration": kubernetesUpgrade.Generation - 1,
	}); err != nil {
		logger.Error(err, "Failed to update failure status")
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}
	kubernetesUpgrade.Status.Phase = tupprv1alpha1.JobPhaseFailed
	r.recordPhaseTransition(kubernetesUpgrade, prevPhase, tupprv1alpha1.JobPhaseFailed)
	r.MetricsReporter.EndJobTiming(metrics.UpgradeTypeKubernetes, kubernetesUpgrade.Name, nodeName, "failure")
	r.MetricsReporter.RecordActiveJobs(metrics.UpgradeTypeKubernetes, 0)

	logger.V(1).Info("Recorded Kubernetes upgrade failure")
	return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}

func (r *Reconciler) cleanupJob(ctx context.Context, job *batchv1.Job) error {
	return jobs.DeleteJob(ctx, r.Client, job)
}

func (r *Reconciler) createJob(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, controllerNode, controllerIP string) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)

	job, err := r.buildJob(ctx, kubernetesUpgrade, controllerNode, controllerIP)
	if err != nil {
		return nil, fmt.Errorf("failed to build job: %w", err)
	}
	if err := controllerutil.SetControllerReference(kubernetesUpgrade, job, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	logger.V(1).Info("Creating Kubernetes upgrade job", "job", job.Name, "controllerNode", controllerNode)

	if err := r.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	r.MetricsReporter.RecordActiveJobs(metrics.UpgradeTypeKubernetes, 1)
	r.MetricsReporter.StartJobTiming(metrics.UpgradeTypeKubernetes, kubernetesUpgrade.Name, controllerNode)
	return job, nil
}

func (r *Reconciler) buildJob(ctx context.Context, kubernetesUpgrade *tupprv1alpha1.KubernetesUpgrade, controllerNode, controllerIP string) (*batchv1.Job, error) {
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
		currentVersion, err := r.TalosClient.GetNodeVersion(ctx, controllerIP)
		if err != nil || currentVersion == "" {
			return nil, fmt.Errorf("failed to detect talosctl version for node %s: %w", controllerNode, err)
		}
		talosctlTag = currentVersion
		logger.V(1).Info("Using current node version for talosctl compatibility",
			"node", controllerNode, "currentVersion", currentVersion)
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
				Spec: jobs.BuildTalosctlPodSpec(jobs.PodSpecOptions{
					ContainerName:     "upgrade-k8s",
					Image:             talosctlImage,
					PullPolicy:        pullPolicy,
					Args:              args,
					TalosConfigSecret: r.TalosConfigSecret,
					GracePeriod:       KubernetesJobGracePeriod,
					Affinity:          nil,
				}),
			},
		},
	}, nil
}
