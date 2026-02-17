package talosupgrade

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabel "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/drain"
	"github.com/home-operations/tuppr/internal/controller/maintenance"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
	"github.com/home-operations/tuppr/internal/metrics"
)

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

	logger.V(1).Info("Recorded node failure", "node", nodeName, "failedNodes", failedCount)
	return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}

func (r *Reconciler) verifyNodeUpgrade(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (bool, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Verifying node upgrade using Talos client", "node", nodeName)

	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return false, fmt.Errorf("failed to get node: %w", err)
	}

	nodeIP, err := nodeutil.GetNodeIP(node)
	if err != nil {
		return false, fmt.Errorf("failed to get node IP for %s: %w", nodeName, err)
	}

	targetVersion := talosUpgrade.Spec.Talos.Version

	if err := r.TalosClient.CheckNodeReady(ctx, nodeIP, nodeName); err != nil {
		if isTransientError(err) {
			logger.V(1).Info("Node not ready yet, will retry", "node", nodeName, "error", err)
			return false, nil
		}
		return false, err
	}

	currentVersion, err := r.TalosClient.GetNodeVersion(ctx, nodeIP)
	if err != nil {
		if isTransientError(err) {
			logger.V(1).Info("Node not ready yet, will retry", "node", nodeName, "error", err)
			return false, nil
		}
		return false, fmt.Errorf("failed to get current version from Talos for %s: %w", nodeName, err)
	}

	if currentVersion != targetVersion {
		return false, fmt.Errorf("node %s version mismatch: current=%s, target=%s",
			nodeName, currentVersion, targetVersion)
	}

	logger.V(1).Info("Node upgrade verification successful",
		"node", nodeName,
		"version", currentVersion)
	return true, nil
}

// isNodeReady returns true if the node has a Ready condition set to True.
func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	// Check Standard Go Context Timeouts
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check gRPC Status
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded:
			return true
		default:
			// If it's a valid gRPC error but NOT one of the above, it's likely permanent (e.g. Unauthenticated)
			// We return false here to stop falling through to string matching.
			return false
		}
	}

	// Check Network Errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
	}

	//  Check Syscall Errors (Connection issues)
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ETIMEDOUT, syscall.EPIPE:
			return true
		}
	}

	//  Fallback String Matching
	errStr := strings.ToLower(err.Error())
	transientIndicators := []string{
		"connection refused",
		"connection reset",
		"i/o timeout",
		"eof", // Often happens if server restarts mid-stream
	}

	for _, indicator := range transientIndicators {
		if strings.Contains(errStr, indicator) {
			return true
		}
	}

	return false
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

			deletePolicy := metav1.DeletePropagationForeground
			if err := r.Delete(ctx, &job, &client.DeleteOptions{
				PropagationPolicy: &deletePolicy,
			}); err != nil {
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
				Spec: corev1.PodSpec{
					RestartPolicy:                 corev1.RestartPolicyNever,
					TerminationGracePeriodSeconds: ptr.To(int64(TalosJobGracePeriod)),
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
					Affinity: &corev1.Affinity{
						NodeAffinity: nodeAffinity,
					},
					Tolerations: []corev1.Toleration{
						{
							Operator: corev1.TolerationOpExists,
						},
					},
					Containers: []corev1.Container{{
						Name:            "upgrade",
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

func getActiveDeadlineSeconds(timeout time.Duration) int64 {
	attempts := int64(TalosJobBackoffLimit + 1)
	timeoutSeconds := int64(timeout.Seconds())
	return attempts*timeoutSeconds + TalosJobActiveDeadlineBuffer
}

func (r *Reconciler) buildTalosUpgradeImage(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (string, error) {
	logger := log.FromContext(ctx)

	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	targetVersion := r.getTargetVersion(node, talosUpgrade.Spec.Talos.Version)

	var imageBase string

	if schematic, ok := node.Annotations[constants.SchematicAnnotation]; ok && schematic != "" {
		imageBase = fmt.Sprintf("%s/%s", constants.DefaultFactoryURL, schematic)
		logger.V(1).Info("Using schematic override from annotation", "node", nodeName, "schematic", schematic)
	} else {
		nodeIP, err := nodeutil.GetNodeIP(node)
		if err != nil {
			return "", fmt.Errorf("failed to get node IP for %s: %w", nodeName, err)
		}

		currentImage, err := r.TalosClient.GetNodeInstallImage(ctx, nodeIP)
		if err != nil {
			return "", fmt.Errorf("failed to get install image from Talos client for %s: %w", nodeName, err)
		}

		parts := strings.Split(currentImage, ":")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid current image format for node %s: %s", nodeName, currentImage)
		}
		imageBase = parts[0]
	}

	targetImage := fmt.Sprintf("%s:%s", imageBase, targetVersion)

	logger.V(1).Info("Built target image",
		"node", nodeName,
		"targetImage", targetImage,
		"version", targetVersion)

	return targetImage, nil
}

func (r *Reconciler) getTargetVersion(node *corev1.Node, crdTargetVersion string) string {
	if v, ok := node.Annotations[constants.VersionAnnotation]; ok && v != "" {
		return v
	}
	return crdTargetVersion
}

func (r *Reconciler) processNextNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	maintenanceRes, err := maintenance.CheckWindow(talosUpgrade.Spec.Maintenance, r.Now.Now())
	if err != nil {
		logger.Error(err, "Failed to check maintenance window")
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}
	if !maintenanceRes.Allowed {
		requeueAfter := time.Until(*maintenanceRes.NextWindowStart)
		if requeueAfter > 5*time.Minute {
			requeueAfter = 5 * time.Minute
		}
		nextTimestamp := maintenanceRes.NextWindowStart.Unix()
		r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeTalos, talosUpgrade.Name, false, &nextTimestamp)
		if err := r.updateStatus(ctx, talosUpgrade, map[string]any{
			"phase":                 tupprv1alpha1.JobPhasePending,
			"currentNode":           "",
			"message":               fmt.Sprintf("Maintenance window closed between nodes, waiting (next: %s)", maintenanceRes.NextWindowStart.Format(time.RFC3339)),
			"nextMaintenanceWindow": metav1.NewTime(*maintenanceRes.NextWindowStart),
		}); err != nil {
			logger.Error(err, "Failed to update status for maintenance window")
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeTalos, talosUpgrade.Name, true, nil)

	ctx = context.WithValue(ctx, metrics.ContextKeyUpgradeType, metrics.UpgradeTypeTalos)
	ctx = context.WithValue(ctx, metrics.ContextKeyUpgradeName, talosUpgrade.Name)

	if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseHealthChecking, "", "Running health checks"); err != nil {
		logger.Error(err, "Failed to update phase for health check")
	}
	if err := r.HealthChecker.CheckHealth(ctx, talosUpgrade.Spec.HealthChecks); err != nil {
		logger.Info("Waiting for health checks to pass", "error", err.Error())
		if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseHealthChecking, "", fmt.Sprintf("Waiting for health checks: %s", err.Error())); err != nil {
			logger.Error(err, "Failed to update phase for health check")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	nextNode, err := r.findNextNode(ctx, talosUpgrade)
	if err != nil {
		logger.Error(err, "Failed to find next node to upgrade")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if nextNode == "" {
		return r.completeUpgrade(ctx, talosUpgrade)
	}
	targetImage, err := r.buildTalosUpgradeImage(ctx, talosUpgrade, nextNode)
	if err != nil {
		logger.Error(err, "Failed to determine target image", "node", nextNode)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	logger.V(1).Info("Verifying target image availability", "image", targetImage)
	if err := r.ImageChecker.Check(ctx, targetImage); err != nil {
		logger.Info("Waiting for target image to become available", "node", nextNode, "image", targetImage, "error", err.Error())

		message := fmt.Sprintf("Waiting for image availability: %s", err.Error())
		if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhasePending, nextNode, message); err != nil {
			logger.Error(err, "Failed to update phase while waiting for image")
		}

		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	logger.Info("Found next node to upgrade", "node", nextNode, "image", targetImage)

	// Drain node before upgrade if drain spec is configured
	if talosUpgrade.Spec.Drain != nil {
		if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseDraining, nextNode, fmt.Sprintf("Draining node %s", nextNode)); err != nil {
			logger.Error(err, "Failed to update phase for draining", "node", nextNode)
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		}
		logger.Info("Draining node before upgrade", "node", nextNode)
		if err := r.drainNode(ctx, nextNode, talosUpgrade.Spec.Drain); err != nil {
			logger.Error(err, "Failed to drain node", "node", nextNode)
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		logger.V(1).Info("Node drained successfully", "node", nextNode)
	}

	if _, err := r.createJob(ctx, talosUpgrade, nextNode, targetImage); err != nil {
		logger.Error(err, "Failed to create upgrade job", "node", nextNode)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if err := r.addNodeUpgradingLabel(ctx, nextNode); err != nil {
		logger.Error(err, "Failed to add upgrading label to node", "node", nextNode)
	}

	if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseUpgrading, nextNode, fmt.Sprintf("Upgrading node %s", nextNode)); err != nil {
		logger.Error(err, "Failed to update phase for node upgrade", "node", nextNode)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

func (r *Reconciler) findNextNode(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (string, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Finding next node to upgrade", "talosupgrade", talosUpgrade.Name)

	nodes, err := r.getSortedNodes(ctx, talosUpgrade.Spec.NodeSelector)
	if err != nil {
		logger.Error(err, "Failed to get nodes")
		return "", err
	}

	crdTargetVersion := talosUpgrade.Spec.Talos.Version

	for i := range nodes {
		node := &nodes[i]

		if slices.Contains(talosUpgrade.Status.CompletedNodes, node.Name) {
			continue
		}

		if slices.ContainsFunc(talosUpgrade.Status.FailedNodes, func(fn tupprv1alpha1.NodeUpgradeStatus) bool {
			return fn.NodeName == node.Name
		}) {
			continue
		}

		needsUpgrade, err := r.nodeNeedsUpgrade(ctx, node, crdTargetVersion)
		if err != nil {
			logger.Error(err, "Failed to check if node needs upgrade", "node", node.Name)
			return "", fmt.Errorf("failed to check node %s: %w", node.Name, err)
		}

		if needsUpgrade {
			logger.V(1).Info("Node needs upgrade", "node", node.Name)
			return node.Name, nil
		}
	}

	logger.V(1).Info("All nodes are up to date")
	return "", nil
}

func (r *Reconciler) getSortedNodes(ctx context.Context, nodeSelector *metav1.LabelSelector) ([]corev1.Node, error) {
	var selector k8slabel.Selector
	var err error
	nodeList := &corev1.NodeList{}
	if nodeSelector != nil {
		selector, err = metav1.LabelSelectorAsSelector(nodeSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to parse nodeSelector: %w", err)
		}
	} else {
		selector = k8slabel.Everything()
	}

	listOpts := &client.ListOptions{LabelSelector: selector}
	if err := r.List(ctx, nodeList, listOpts); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	nodes := nodeList.Items
	slices.SortFunc(nodes, func(a, b corev1.Node) int {
		return strings.Compare(a.Name, b.Name)
	})

	return nodes, nil
}

func (r *Reconciler) nodeNeedsUpgrade(ctx context.Context, node *corev1.Node, crdTargetVersion string) (bool, error) {
	logger := log.FromContext(ctx)

	nodeIP, err := nodeutil.GetNodeIP(node)
	if err != nil {
		return false, fmt.Errorf("failed to get node IP for %s: %w", node.Name, err)
	}

	currentVersion, err := r.TalosClient.GetNodeVersion(ctx, nodeIP)
	if err != nil {
		return false, fmt.Errorf("failed to get current version for node %s (%s): %w", node.Name, nodeIP, err)
	}

	targetVersion := r.getTargetVersion(node, crdTargetVersion)

	if currentVersion != targetVersion {
		logger.V(1).Info("Node version mismatch detected",
			"node", node.Name,
			"current", currentVersion,
			"target", targetVersion)
		return true, nil
	}
	if targetSchematic, ok := node.Annotations[constants.SchematicAnnotation]; ok && targetSchematic != "" {
		currentImage, err := r.TalosClient.GetNodeInstallImage(ctx, nodeIP)
		if err != nil {
			logger.Error(err, "Failed to get install image to verify schematic", "node", node.Name)
			return false, err
		}

		if !strings.Contains(currentImage, targetSchematic) {
			logger.V(1).Info("Node schematic mismatch detected",
				"node", node.Name,
				"currentImage", currentImage,
				"targetSchematic", targetSchematic)
			return true, nil
		}
	}

	return false, nil
}

func (r *Reconciler) drainNode(ctx context.Context, nodeName string, drainSpec *tupprv1alpha1.DrainSpec) error {
	drainer := drain.NewDrainer(r.Client)

	// Cordon the node first
	if err := drainer.CordonNode(ctx, nodeName); err != nil {
		return fmt.Errorf("failed to cordon node %s: %w", nodeName, err)
	}

	opts := drain.DrainOptions{
		RespectPDBs: drainSpec.DisableEviction == nil || !*drainSpec.DisableEviction,
		Timeout:     10 * time.Minute,
		GracePeriod: nil,
	}

	// Drain the node
	if err := drainer.DrainNode(ctx, nodeName, opts); err != nil {
		return fmt.Errorf("failed to drain node %s: %w", nodeName, err)
	}

	return nil
}
