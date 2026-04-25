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

// findActiveJobs returns all active (non-completed, non-failed) upgrade jobs and their target node names.
func (r *Reconciler) findActiveJobs(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) ([]batchv1.Job, []string, error) {
	jobList, err := jobs.ListJobsByLabel(ctx, r.Client, r.ControllerNamespace, "talos-upgrade")
	if err != nil {
		return nil, nil, err
	}

	var activeJobs []batchv1.Job
	var activeNodes []string

	for _, job := range jobList {
		nodeName, ok := job.Labels["tuppr.home-operations.com/target-node"]
		if !ok || nodeName == "" {
			continue
		}
		instanceName := job.Labels["app.kubernetes.io/instance"]
		controllerOwner := metav1.GetControllerOf(&job)
		ownedByTalosUpgrade := controllerOwner != nil &&
			controllerOwner.Kind == "TalosUpgrade" &&
			controllerOwner.Name == talosUpgrade.Name &&
			controllerOwner.UID == talosUpgrade.UID
		if instanceName != talosUpgrade.Name && !ownedByTalosUpgrade {
			continue
		}

		if slices.Contains(talosUpgrade.Status.CompletedNodes, nodeName) {
			continue
		}

		if slices.ContainsFunc(talosUpgrade.Status.FailedNodes, func(n tupprv1alpha1.NodeUpgradeStatus) bool {
			return n.NodeName == nodeName
		}) {
			continue
		}

		activeJobs = append(activeJobs, job)
		activeNodes = append(activeNodes, nodeName)
	}

	return activeJobs, activeNodes, nil
}

// handleBatchJobStatus handles all active jobs in the current batch.
// It waits for all jobs to finish before processing results.
func (r *Reconciler) handleBatchJobStatus(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, activeJobs []batchv1.Job, activeNodes []string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var stillRunning []string
	var succeededNodes []string
	var failedNodes []string

	for i, job := range activeJobs {
		nodeName := activeNodes[i]

		logger.V(1).Info("Handling job status",
			"job", job.Name,
			"node", nodeName,
			"active", job.Status.Active,
			"succeeded", job.Status.Succeeded,
			"failed", job.Status.Failed,
			"backoffLimit", *job.Spec.BackoffLimit)

		if job.Status.Succeeded > 0 {
			succeededNodes = append(succeededNodes, nodeName)
		} else if job.Status.Failed >= *job.Spec.BackoffLimit {
			failedNodes = append(failedNodes, nodeName)
		} else {
			stillRunning = append(stillRunning, nodeName)
		}
	}

	// If any jobs are still running, wait
	if len(stillRunning) > 0 {
		phase := tupprv1alpha1.JobPhaseUpgrading
		message := fmt.Sprintf("Upgrading %d nodes (%d running, %d succeeded, %d failed)", len(activeJobs), len(stillRunning), len(succeededNodes), len(failedNodes))

		// Check if any running node is NotReady (rebooting)
		anyRebooting := false
		for _, nodeName := range stillRunning {
			node := &corev1.Node{}
			if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err == nil {
				if !isNodeReady(node) {
					anyRebooting = true
					break
				}
			}
		}

		if anyRebooting {
			phase = tupprv1alpha1.JobPhaseRebooting
			message = fmt.Sprintf("Nodes rebooting (%d running, %d succeeded, %d failed)", len(stillRunning), len(succeededNodes), len(failedNodes))
		}

		if err := r.setPhaseWithNodes(ctx, talosUpgrade, phase, activeNodes, message); err != nil {
			logger.Error(err, "Failed to update phase for active batch")
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		}

		r.MetricsReporter.RecordActiveJobs(metrics.UpgradeTypeTalos, len(stillRunning))
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	// All jobs are done — process results

	// Process succeeded jobs
	var rebootingNodes []string
	for _, nodeName := range succeededNodes {
		result, err := r.processSingleJobSuccess(ctx, talosUpgrade, nodeName)
		if err != nil {
			return ctrl.Result{}, err
		}
		switch result {
		case jobResultRebooting:
			rebootingNodes = append(rebootingNodes, nodeName)
		case jobResultFailed:
			// Verification failed — treat as failure
			failedNodes = append(failedNodes, nodeName)
		}
	}

	// If any nodes are still rebooting, wait for all of them before proceeding
	if len(rebootingNodes) > 0 {
		message := fmt.Sprintf("Waiting for %d node(s) to finish rebooting", len(rebootingNodes))
		if err := r.setPhaseWithNodes(ctx, talosUpgrade, tupprv1alpha1.JobPhaseRebooting, activeNodes, message); err != nil {
			logger.Error(err, "Failed to update phase for rebooting")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Process failed jobs
	for _, nodeName := range failedNodes {
		if err := r.processSingleJobFailure(ctx, talosUpgrade, nodeName); err != nil {
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}
	}

	// Determine final batch outcome
	if len(failedNodes) > 0 {
		failedCount := len(failedNodes)
		message := fmt.Sprintf("Batch upgrade stopped: %d nodes failed - stopping", failedCount)

		if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseFailed, "", message); err != nil {
			logger.Error(err, "Failed to update phase for batch failure")
			return ctrl.Result{}, err
		}

		if err := r.updateStatus(ctx, talosUpgrade, map[string]any{
			"observedGeneration": talosUpgrade.Generation - 1,
		}); err != nil {
			logger.Error(err, "Failed to reset observedGeneration for retry")
		}

		r.MetricsReporter.RecordActiveJobs(metrics.UpgradeTypeTalos, 0)
		return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
	}

	// All succeeded — ready for next batch
	completedCount := len(talosUpgrade.Status.CompletedNodes)
	message := fmt.Sprintf("Batch completed successfully (%d total completed)", completedCount)

	if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhasePending, "", message); err != nil {
		logger.Error(err, "Failed to update phase after batch completion")
		return ctrl.Result{}, err
	}

	r.MetricsReporter.RecordActiveJobs(metrics.UpgradeTypeTalos, 0)
	return ctrl.Result{RequeueAfter: time.Second * 5}, nil
}

type jobResult int

const (
	jobResultSuccess   jobResult = iota
	jobResultRebooting           // node not ready yet
	jobResultFailed              // verification failed
)

// processSingleJobSuccess handles a single succeeded job: verify, uncordon, cleanup.
// Returns the result without setting overall phase or metrics.
func (r *Reconciler) processSingleJobSuccess(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (jobResult, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Job completed, verifying node upgrade", "node", nodeName)

	isReady, err := r.verifyNodeUpgrade(ctx, talosUpgrade, nodeName)
	if err != nil {
		logger.Error(err, "Failed to verify node", "node", nodeName)
		return jobResultFailed, nil
	}

	if !isReady {
		logger.V(1).Info("Node not yet ready after upgrade, waiting for reboot", "node", nodeName)
		return jobResultRebooting, nil
	}

	logger.Info("Node verified as upgraded and ready", "node", nodeName)

	if err := r.syncNodeInstallImage(ctx, talosUpgrade, nodeName); err != nil {
		logger.Error(err, "Failed to sync install image in machine config, continuing", "node", nodeName)
	} else {
		logger.Info("Synced machine config install image", "node", nodeName)
	}

	if talosUpgrade.Spec.Drain != nil {
		logger.V(1).Info("Uncordoning node after successful upgrade", "node", nodeName)
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			logger.Error(err, "Failed to get node for uncordon", "node", nodeName)
		} else if node.Spec.Unschedulable {
			patch := []byte(`{"spec":{"unschedulable":false}}`)
			if err := r.Patch(ctx, node, client.RawPatch(types.MergePatchType, patch)); err != nil {
				logger.Error(err, "Failed to uncordon node", "node", nodeName)
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
		return jobResultSuccess, err
	}

	r.MetricsReporter.EndJobTiming(metrics.UpgradeTypeTalos, talosUpgrade.Name, nodeName, "success")
	logger.Info("Node upgrade completed", "node", nodeName)
	return jobResultSuccess, nil
}

// processSingleJobFailure handles a single failed job: cleanup labels, record failure.
func (r *Reconciler) processSingleJobFailure(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) error {
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
		return err
	}

	if err := r.cleanupJobForNode(ctx, nodeName); err != nil {
		logger.Error(err, "Failed to cleanup failed job, but continuing", "node", nodeName)
	}

	r.MetricsReporter.EndJobTiming(metrics.UpgradeTypeTalos, talosUpgrade.Name, nodeName, "failure")
	return nil
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
		if !jobs.IsTerminal(&job) {
			continue
		}
		logger.V(1).Info("Deleting terminal job", "job", job.Name, "node", nodeName)
		if err := jobs.DeleteJob(ctx, r.Client, &job); err != nil {
			return err
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

func (r *Reconciler) syncNodeInstallImage(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) error {
	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	nodeIP, err := nodeutil.GetNodeIP(node)
	if err != nil {
		return fmt.Errorf("failed to get node IP for %s: %w", nodeName, err)
	}

	targetImage, err := r.buildTalosUpgradeImage(ctx, talosUpgrade, nodeName)
	if err != nil {
		return fmt.Errorf("failed to build target image for %s: %w", nodeName, err)
	}

	return r.TalosClient.PatchNodeInstallImage(ctx, nodeIP, targetImage)
}

func getActiveDeadlineSeconds(timeout time.Duration) int64 {
	attempts := int64(TalosJobBackoffLimit + 1)
	timeoutSeconds := int64(timeout.Seconds())
	return attempts*timeoutSeconds + TalosJobActiveDeadlineBuffer
}
