package talosupgrade

import (
	"context"
	"fmt"
	"slices"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/jobs"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
	"github.com/home-operations/tuppr/internal/metrics"
)

func (r *Reconciler) findActiveJob(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (*batchv1.Job, string, error) {
	jobList, err := jobs.ListJobsByLabel(ctx, r.Client, r.ControllerNamespace, "talos-upgrade")
	if err != nil {
		return nil, "", err
	}

	for _, job := range jobList {
		nodeName := job.Labels["tuppr.home-operations.com/target-node"]

		if slices.Contains(talosUpgrade.Status.CompletedNodes, nodeName) {
			continue
		}

		if slices.ContainsFunc(talosUpgrade.Status.FailedNodes, func(n tupprv1alpha1.NodeUpgradeStatus) bool {
			return n.NodeName == nodeName
		}) {
			continue
		}

		return &job, nodeName, nil
	}

	return nil, "", nil
}

func (r *Reconciler) handleJobStatus(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.V(1).Info("Handling job status",
		"job", job.Name,
		"node", nodeName,
		"active", job.Status.Active,
		"succeeded", job.Status.Succeeded,
		"failed", job.Status.Failed,
		"backoffLimit", *job.Spec.BackoffLimit)

	if job.Status.Succeeded == 0 && (job.Status.Failed == 0 || job.Status.Failed < *job.Spec.BackoffLimit) {
		phase := tupprv1alpha1.JobPhaseUpgrading
		message := fmt.Sprintf("Upgrading node %s (job: %s)", nodeName, job.Name)

		// Check if the node is NotReady, which indicates it is rebooting
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err == nil {
			if !isNodeReady(node) {
				phase = tupprv1alpha1.JobPhaseRebooting
				message = fmt.Sprintf("Node %s is rebooting", nodeName)
			}
		}

		if err := r.setPhase(ctx, talosUpgrade, phase, nodeName, message); err != nil {
			logger.Error(err, "Failed to update phase for active job", "job", job.Name, "node", nodeName)
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		}
		logger.V(1).Info("Job is still active", "job", job.Name, "node", nodeName, "phase", phase)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	if job.Status.Succeeded > 0 {
		return r.handleJobSuccess(ctx, talosUpgrade, nodeName)
	}

	return r.handleJobFailure(ctx, talosUpgrade, nodeName)
}

func (r *Reconciler) handleJobSuccess(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Job completed, verifying node upgrade", "node", nodeName)

	isReady, err := r.verifyNodeUpgrade(ctx, talosUpgrade, nodeName)
	if err != nil {
		logger.Error(err, "Failed to verify node", "node", nodeName)
		return r.handleJobFailure(ctx, talosUpgrade, nodeName)
	}

	if !isReady {
		logger.V(1).Info("Node not yet ready after upgrade, waiting for reboot", "node", nodeName)
		if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseRebooting, nodeName, fmt.Sprintf("Node %s is rebooting", nodeName)); err != nil {
			logger.Error(err, "Failed to update phase for rebooting", "node", nodeName)
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	logger.Info("Node verified as upgraded and ready", "node", nodeName)

	if talosUpgrade.Spec.Drain != nil {
		logger.V(1).Info("Uncordoning node after successful upgrade", "node", nodeName)
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			logger.Error(err, "Failed to get node for uncordon", "node", nodeName)
		} else {
			// Only patch if currently unschedulable
			if node.Spec.Unschedulable {
				patch := []byte(`{"spec":{"unschedulable":false}}`)
				if err := r.Patch(ctx, node, client.RawPatch(types.MergePatchType, patch)); err != nil {
					logger.Error(err, "Failed to uncordon node", "node", nodeName)
				}
			}
		}
	}

	if err := r.cleanupJobForNode(ctx, nodeName); err != nil {
		logger.Error(err, "Failed to cleanup job, but continuing", "node", nodeName)
	}

	if err := r.removeNodeUpgradingLabel(ctx, nodeName); err != nil {
		logger.Error(err, "Failed to remove upgrading label from node", "node", nodeName)
	}

	if err := r.addCompletedNode(ctx, talosUpgrade, nodeName); err != nil {
		logger.Error(err, "Failed to add completed node", "node", nodeName)
		return ctrl.Result{}, err
	}

	completedCount := len(talosUpgrade.Status.CompletedNodes) + 1
	message := fmt.Sprintf("Node %s upgraded successfully, health checks passed (%d completed)", nodeName, completedCount)

	if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhasePending, "", message); err != nil {
		logger.Error(err, "Failed to update phase", "node", nodeName)
		return ctrl.Result{}, err
	}

	r.MetricsReporter.EndJobTiming(metrics.UpgradeTypeTalos, talosUpgrade.Name, nodeName, "success")
	r.MetricsReporter.RecordActiveJobs(metrics.UpgradeTypeTalos, 0)
	logger.Info("Node upgrade completed", "node", nodeName,
		"completedNodes", completedCount)
	return ctrl.Result{RequeueAfter: time.Second * 5}, nil
}

func (r *Reconciler) handleJobFailure(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Node upgrade failed", "node", nodeName)

	if err := r.removeNodeUpgradingLabel(ctx, nodeName); err != nil {
		logger.Error(err, "Failed to remove upgrading label from node", "node", nodeName)
	}

	nodeStatus := tupprv1alpha1.NodeUpgradeStatus{
		NodeName:  nodeName,
		LastError: "Job failed permanently",
	}

	if err := r.addFailedNode(ctx, talosUpgrade, nodeStatus); err != nil {
		logger.Error(err, "Failed to add failed node", "node", nodeName)
		return ctrl.Result{}, err
	}

	failedCount := len(talosUpgrade.Status.FailedNodes) + 1
	message := fmt.Sprintf("Node %s upgrade failed (%d failed) - stopping", nodeName, failedCount)

	if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseFailed, "", message); err != nil {
		logger.Error(err, "Failed to update phase", "node", nodeName)
		return ctrl.Result{}, err
	}

	if err := r.updateStatus(ctx, talosUpgrade, map[string]any{
		"observedGeneration": talosUpgrade.Generation - 1,
	}); err != nil {
		logger.Error(err, "Failed to reset observedGeneration for retry", "node", nodeName)
	}

	r.MetricsReporter.EndJobTiming(metrics.UpgradeTypeTalos, talosUpgrade.Name, nodeName, "failure")
	r.MetricsReporter.RecordActiveJobs(metrics.UpgradeTypeTalos, 0)
	logger.V(1).Info("Recorded node failure", "node", nodeName, "failedNodes", failedCount)
	return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}

func (r *Reconciler) cleanupJobForNode(ctx context.Context, nodeName string) error {
	logger := log.FromContext(ctx)

	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(r.ControllerNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":                "talos-upgrade",
			"tuppr.home-operations.com/target-node": nodeName,
		}); err != nil {
		return fmt.Errorf("failed to list jobs for node %s: %w", nodeName, err)
	}

	for _, job := range jobList.Items {
		if job.Status.Succeeded > 0 {
			logger.V(1).Info("Deleting completed job", "job", job.Name, "node", nodeName)

			if err := jobs.DeleteJob(ctx, r.Client, &job); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reconciler) createJob(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName, targetImage string) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Creating upgrade job", "node", nodeName)

	targetNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, targetNode); err != nil {
		logger.Error(err, "Failed to get target node", "node", nodeName)
		return nil, err
	}

	nodeIP, err := nodeutil.GetNodeIP(targetNode)
	if err != nil {
		logger.Error(err, "Failed to get InternalIP or ExternalIP", "node", nodeName)
		return nil, err
	}

	job := r.buildJob(ctx, talosUpgrade, nodeName, nodeIP, targetImage)
	if err := controllerutil.SetControllerReference(talosUpgrade, job, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference", "job", job.Name)
		return nil, err
	}

	if err := r.Create(ctx, job); err != nil {
		if err.Error() == "already exists" {
			existingJob := &batchv1.Job{}
			if getErr := r.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, existingJob); getErr != nil {
				logger.Error(getErr, "Failed to get existing job", "job", job.Name)
				return nil, getErr
			}
			logger.V(1).Info("Job already exists, reusing", "job", job.Name)
			return existingJob, nil
		}
		logger.Error(err, "Failed to create job", "job", job.Name, "node", nodeName)
		return nil, err
	}

	logger.Info("Successfully created upgrade job", "job", job.Name, "node", nodeName)
	if r.Notifier != nil {
		message := fmt.Sprintf("Starting upgrade for node %s", nodeName)
		targetVersion := r.getTargetVersion(targetNode, talosUpgrade.Spec.Talos.Version)
		currentVersion, err := r.TalosClient.GetNodeVersion(ctx, nodeIP)
		if err != nil {
			logger.V(1).Info("Failed to determine current Talos version for notification", "error", err, "job", job.Name, "node", nodeName)
		} else {
			message = fmt.Sprintf(
				"Node %s is upgrading Talos from %s -> %s",
				nodeName,
				currentVersion,
				targetVersion,
			)
		}
		if err := r.Notifier.Send(
			"Tuppr Upgrade Started",
			message,
		); err != nil {
			logger.V(1).Info("Failed to send start notification", "error", err, "job", job.Name, "node", nodeName)
		}
	}
	r.MetricsReporter.RecordActiveJobs(metrics.UpgradeTypeTalos, 1)
	r.MetricsReporter.StartJobTiming(metrics.UpgradeTypeTalos, talosUpgrade.Name, nodeName)
	return job, nil
}

func (r *Reconciler) buildJob(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName, nodeIP, targetImage string) *batchv1.Job {
	logger := log.FromContext(ctx)

	jobName := nodeutil.GenerateSafeJobName(talosUpgrade.Name, nodeName)

	labels := map[string]string{
		"app.kubernetes.io/name":                "talos-upgrade",
		"app.kubernetes.io/instance":            talosUpgrade.Name,
		"app.kubernetes.io/part-of":             "tuppr",
		"tuppr.home-operations.com/target-node": nodeName,
	}

	placement := talosUpgrade.Spec.Policy.Placement
	if placement == "" {
		placement = PlacementSoft
	}

	nodeSelector := corev1.NodeSelectorRequirement{
		Key:      "kubernetes.io/hostname",
		Operator: corev1.NodeSelectorOpNotIn,
		Values:   []string{nodeName},
	}

	var nodeAffinity *corev1.NodeAffinity
	if placement == "hard" {
		nodeAffinity = &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{nodeSelector},
				}},
			},
		}
		logger.V(1).Info("Using hard placement preset - required node avoidance", "node", nodeName)
	} else {
		nodeAffinity = &corev1.NodeAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
				Weight: 100,
				Preference: corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{nodeSelector},
				},
			}},
		}
		if placement != PlacementSoft {
			logger.V(1).Info("Unknown placement preset, using soft placement as fallback", "preset", placement, "node", nodeName)
		} else {
			logger.V(1).Info("Using soft placement preset - preferred node avoidance", "node", nodeName)
		}
	}

	talosctlRepo := constants.DefaultTalosctlImage
	if talosUpgrade.Spec.Talosctl.Image.Repository != "" {
		talosctlRepo = talosUpgrade.Spec.Talosctl.Image.Repository
	}

	talosctlTag := talosUpgrade.Spec.Talosctl.Image.Tag
	if talosctlTag == "" {
		if currentVersion, err := r.TalosClient.GetNodeVersion(ctx, nodeIP); err == nil && currentVersion != "" {
			talosctlTag = currentVersion
			logger.V(1).Info("Using current node version for talosctl compatibility",
				"node", nodeName, "currentVersion", currentVersion)
		} else {
			talosctlTag = talosUpgrade.Spec.Talos.Version
			logger.V(1).Info("Could not detect current version, using target version for talosctl",
				"node", nodeName, "version", talosctlTag)
		}
	}

	talosctlImage := talosctlRepo + ":" + talosctlTag

	timeout := TalosJobDefaultTimeout
	if talosUpgrade.Spec.Policy.Timeout != nil {
		timeout = talosUpgrade.Spec.Policy.Timeout.Duration
	}

	args := []string{
		"upgrade",
		"--nodes=" + nodeIP,
		"--image=" + targetImage,
		"--timeout=" + timeout.String(),
		"--wait=true",
	}

	if talosUpgrade.Spec.Policy.Debug {
		args = append(args, "--debug=true")
		logger.V(1).Info("Debug upgrade enabled", "node", nodeName)
	}

	if talosUpgrade.Spec.Policy.Force {
		args = append(args, "--force=true")
		logger.V(1).Info("Force upgrade enabled", "node", nodeName)
	}

	if talosUpgrade.Spec.Policy.RebootMode == "powercycle" {
		args = append(args, "--reboot-mode=powercycle")
		logger.V(1).Info("Powercycle reboot mode enabled", "node", nodeName)
	}

	if talosUpgrade.Spec.Policy.Stage {
		args = append(args, "--stage")
		logger.V(1).Info("Stage upgrade enabled", "node", nodeName)
	}

	pullPolicy := corev1.PullIfNotPresent
	if talosUpgrade.Spec.Talosctl.Image.PullPolicy != "" {
		pullPolicy = talosUpgrade.Spec.Talosctl.Image.PullPolicy
	}

	logger.V(1).Info("Building job specification",
		"node", nodeName,
		"talosctlImage", talosctlImage,
		"targetImage", targetImage,
		"pullPolicy", pullPolicy,
		"args", args)

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: r.ControllerNamespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To(int32(TalosJobBackoffLimit)),
			Completions:             ptr.To(int32(1)),
			TTLSecondsAfterFinished: ptr.To(int32(TalosJobTTLAfterFinished)),
			Parallelism:             ptr.To(int32(1)),
			ActiveDeadlineSeconds:   ptr.To(getActiveDeadlineSeconds(timeout)),
			PodReplacementPolicy:    ptr.To(batchv1.Failed),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: jobs.BuildTalosctlPodSpec(jobs.PodSpecOptions{
					ContainerName:     "upgrade",
					Image:             talosctlImage,
					PullPolicy:        pullPolicy,
					Args:              args,
					TalosConfigSecret: r.TalosConfigSecret,
					GracePeriod:       TalosJobGracePeriod,
					Affinity:          &corev1.Affinity{NodeAffinity: nodeAffinity},
				}),
			},
		},
	}
}

func getActiveDeadlineSeconds(timeout time.Duration) int64 {
	attempts := int64(TalosJobBackoffLimit + 1)
	timeoutSeconds := int64(timeout.Seconds())
	return attempts*timeoutSeconds + TalosJobActiveDeadlineBuffer
}
