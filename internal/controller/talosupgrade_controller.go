package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

const (
	// Phase constants
	PhasePending    = "Pending"
	PhaseInProgress = "InProgress"
	PhaseCompleted  = "Completed"
	PhaseFailed     = "Failed"

	// Annotation from Talos
	SchematicAnnotation   = "extensions.talos.dev/schematic"
	TalosUpgradeFinalizer = "tuppr.home-operations.com/talos-finalizer"
)

// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tuppr.home-operations.com,resources=talosupgrades/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// TalosUpgradeReconciler reconciles a TalosUpgrade object
type TalosUpgradeReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	TalosConfigSecret   string
	ControllerNamespace string
}

func (r *TalosUpgradeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting reconciliation", "talosupgrade", req.Name)

	var talosUpgrade upgradev1alpha1.TalosUpgrade
	// For cluster-scoped resources, use Name instead of NamespacedName
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name}, &talosUpgrade); err != nil {
		if errors.IsNotFound(err) {
			logger.V(1).Info("TalosUpgrade resource not found, likely deleted", "talosupgrade", req.Name)
		} else {
			logger.Error(err, "Failed to get TalosUpgrade resource", "talosupgrade", req.Name)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.V(1).Info("Retrieved TalosUpgrade",
		"name", talosUpgrade.Name,
		"phase", talosUpgrade.Status.Phase,
		"generation", talosUpgrade.Generation,
		"observedGeneration", talosUpgrade.Status.ObservedGeneration)

	// Handle deletion
	if talosUpgrade.DeletionTimestamp != nil {
		logger.Info("TalosUpgrade is being deleted, starting cleanup", "name", talosUpgrade.Name)
		return r.cleanup(ctx, &talosUpgrade)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&talosUpgrade, TalosUpgradeFinalizer) {
		logger.V(1).Info("Adding finalizer to TalosUpgrade", "name", talosUpgrade.Name, "finalizer", TalosUpgradeFinalizer)
		controllerutil.AddFinalizer(&talosUpgrade, TalosUpgradeFinalizer)
		return ctrl.Result{}, r.Update(ctx, &talosUpgrade)
	}

	// Simple state machine
	logger.V(1).Info("Processing upgrade for TalosUpgrade", "name", talosUpgrade.Name, "currentPhase", talosUpgrade.Status.Phase)
	return r.processUpgrade(ctx, &talosUpgrade)
}

func (r *TalosUpgradeReconciler) processUpgrade(ctx context.Context, talosUpgrade *upgradev1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"talosupgrade", talosUpgrade.Name,
		"generation", talosUpgrade.Generation,
	)

	logger.V(1).Info("Starting upgrade processing")

	// Check if spec has changed (generation mismatch) - if so, reset status and restart
	if talosUpgrade.Status.ObservedGeneration != 0 && talosUpgrade.Status.ObservedGeneration < talosUpgrade.Generation {
		logger.Info("Spec has changed, resetting status and restarting upgrade",
			"currentGeneration", talosUpgrade.Generation,
			"observedGeneration", talosUpgrade.Status.ObservedGeneration,
			"oldPhase", talosUpgrade.Status.Phase)

		// Reset status to start fresh
		objectKey := client.ObjectKey{Name: talosUpgrade.Name} // No namespace for cluster-scoped
		patch := map[string]interface{}{
			"status": map[string]interface{}{
				"phase":              PhasePending,
				"currentNode":        "",
				"message":            "Spec updated, restarting upgrade process",
				"lastUpdated":        metav1.Now(),
				"observedGeneration": talosUpgrade.Generation,
				"completedNodes":     []string{},
				"failedNodes":        []upgradev1alpha1.NodeUpgradeStatus{},
			},
		}

		if err := r.patchStatus(ctx, objectKey, patch); err != nil {
			logger.Error(err, "Failed to reset status after spec change")
			return ctrl.Result{RequeueAfter: time.Second * 30}, nil
		}

		logger.V(1).Info("Status reset, proceeding with new upgrade")
		// Continue processing with reset status
	} else {
		// Skip lock acquisition for completed/failed upgrades (only if generation matches)
		if talosUpgrade.Status.Phase == PhaseCompleted || talosUpgrade.Status.Phase == PhaseFailed {
			logger.V(1).Info("Upgrade already completed or failed, skipping lock acquisition",
				"phase", talosUpgrade.Status.Phase)
			// Just requeue less frequently for completed/failed upgrades
			return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
		}
	}

	// Check if we can acquire the upgrade lock
	canProceed, queuePosition, err := r.acquireUpgradeLock(ctx, talosUpgrade)
	if err != nil {
		logger.Error(err, "Failed to check upgrade lock")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if !canProceed {
		logger.Info("Waiting in upgrade queue", "position", queuePosition)

		// Update status to show we're waiting
		objectKey := client.ObjectKey{Name: talosUpgrade.Name} // No namespace for cluster-scoped
		if err := r.updatePhase(ctx, objectKey, PhasePending, "",
			fmt.Sprintf("Waiting in queue (position %d)", queuePosition)); err != nil {
			logger.Error(err, "Failed to update phase while waiting")
		}

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	objectKey := client.ObjectKey{Name: talosUpgrade.Name} // No namespace for cluster-scoped

	if len(talosUpgrade.Status.FailedNodes) > 0 {
		logger.Info("Upgrade plan has failed nodes, blocking further progress",
			"talosupgrade", talosUpgrade.Name,
			"failedNodes", len(talosUpgrade.Status.FailedNodes))

		if err := r.updatePhase(ctx, objectKey, PhaseFailed, "",
			fmt.Sprintf("Upgrade stopped due to %d failed nodes", len(talosUpgrade.Status.FailedNodes))); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	activeJob, activeNode, err := r.findActiveJob(ctx, talosUpgrade)
	if err != nil {
		logger.Error(err, "Failed to find active jobs", "talosupgrade", talosUpgrade.Name)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if activeJob != nil {
		logger.V(1).Info("Found active job, monitoring status",
			"job", activeJob.Name,
			"node", activeNode,
			"active", activeJob.Status.Active,
			"succeeded", activeJob.Status.Succeeded,
			"failed", activeJob.Status.Failed)

		return r.handleJobStatus(ctx, talosUpgrade, activeNode, activeJob)
	}

	nextNode, err := r.findNextNode(ctx, talosUpgrade)
	if err != nil {
		logger.Error(err, "Failed to find next node to upgrade", "talosupgrade", talosUpgrade.Name)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if nextNode == "" {
		logger.Info("No more nodes to upgrade, marking plan as completed",
			"talosupgrade", talosUpgrade.Name,
			"completedNodes", talosUpgrade.Status.CompletedNodes,
			"failedNodes", len(talosUpgrade.Status.FailedNodes))

		// Use the enhanced completion status update
		if err := r.updateCompletionStatus(ctx, objectKey, talosUpgrade); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	logger.Info("Found next node to upgrade", "talosupgrade", talosUpgrade.Name, "node", nextNode)

	logger.V(1).Info("Creating new upgrade job", "node", nextNode)
	if _, err := r.createJob(ctx, talosUpgrade, nextNode); err != nil {
		logger.Error(err, "Failed to create upgrade job", "node", nextNode)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	logger.Info("Successfully created upgrade job", "node", nextNode)
	if err := r.updatePhase(ctx, objectKey, PhaseInProgress, nextNode, fmt.Sprintf("Upgrading node %s", nextNode)); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

func (r *TalosUpgradeReconciler) acquireUpgradeLock(ctx context.Context, talosUpgrade *upgradev1alpha1.TalosUpgrade) (bool, int, error) {
	logger := log.FromContext(ctx)

	// Get all TalosUpgrade resources in the cluster
	upgradeList := &upgradev1alpha1.TalosUpgradeList{}
	if err := r.List(ctx, upgradeList); err != nil {
		return false, 0, err
	}

	// Sort by creation timestamp, then by name for deterministic ordering
	sort.Slice(upgradeList.Items, func(i, j int) bool {
		iTime := upgradeList.Items[i].CreationTimestamp.Time
		jTime := upgradeList.Items[j].CreationTimestamp.Time

		// If timestamps are equal, sort by name for deterministic ordering
		if iTime.Equal(jTime) {
			return upgradeList.Items[i].Name < upgradeList.Items[j].Name
		}

		return iTime.Before(jTime)
	})

	// Find our position in the queue
	var queuePosition int
	var currentUpgrade *upgradev1alpha1.TalosUpgrade

	for _, upgrade := range upgradeList.Items {
		// Skip completed/failed upgrades
		if upgrade.Status.Phase == PhaseCompleted || upgrade.Status.Phase == PhaseFailed {
			continue
		}

		queuePosition++
		if upgrade.Name == talosUpgrade.Name {
			currentUpgrade = &upgrade
			break
		}
	}

	if currentUpgrade == nil {
		return false, 0, fmt.Errorf("could not find current upgrade in list")
	}

	// Check if any upgrade is currently running
	for _, upgrade := range upgradeList.Items {
		// Skip ourselves and completed/failed upgrades
		if upgrade.Name == talosUpgrade.Name ||
			upgrade.Status.Phase == PhaseCompleted || upgrade.Status.Phase == PhaseFailed {
			continue
		}

		// If someone else is running, we need to wait
		if upgrade.Status.Phase == PhaseInProgress {
			logger.V(1).Info("Another upgrade is in progress, waiting",
				"runningUpgrade", upgrade.Name,
				"queuePosition", queuePosition)
			return false, queuePosition, nil
		}
	}

	// Check if we're next in line by comparing our sorted position
	for _, upgrade := range upgradeList.Items {
		if upgrade.Status.Phase == PhaseCompleted || upgrade.Status.Phase == PhaseFailed {
			continue
		}

		// If someone comes before us in the sorted order and is still pending, they go first
		if upgrade.Status.Phase == PhasePending {
			// Compare using the same logic as our sort function
			upgradeTime := upgrade.CreationTimestamp.Time
			currentTime := currentUpgrade.CreationTimestamp.Time

			var isEarlier bool
			if upgradeTime.Equal(currentTime) {
				isEarlier = upgrade.Name < currentUpgrade.Name
			} else {
				isEarlier = upgradeTime.Before(currentTime)
			}

			if isEarlier {
				logger.V(1).Info("Waiting for earlier upgrade to start",
					"earlierUpgrade", upgrade.Name,
					"queuePosition", queuePosition)
				return false, queuePosition, nil
			}
		}

		// If we reach ourselves, we're next
		if upgrade.Name == talosUpgrade.Name {
			break
		}
	}

	logger.V(1).Info("Acquired upgrade lock", "upgrade", talosUpgrade.Name, "queuePosition", queuePosition)
	return true, queuePosition, nil
}

// findActiveJob looks for any currently running job for this TalosUpgrade
func (r *TalosUpgradeReconciler) findActiveJob(ctx context.Context, talosUpgrade *upgradev1alpha1.TalosUpgrade) (*batchv1.Job, string, error) {
	logger := log.FromContext(ctx)

	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(r.ControllerNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     "talos-upgrade",
			"app.kubernetes.io/instance": talosUpgrade.Name,
		}); err != nil {
		return nil, "", err
	}

	logger.V(1).Info("Found jobs for TalosUpgrade", "count", len(jobList.Items))

	for _, job := range jobList.Items {
		nodeName := job.Labels["tuppr.home-operations.com/target-node"]

		// Skip jobs for nodes that are already completed
		if ContainsNode(nodeName, talosUpgrade.Status.CompletedNodes) {
			logger.V(1).Info("Skipping job for already completed node", "job", job.Name, "node", nodeName)
			continue
		}

		// Skip jobs for nodes that are already failed
		if ContainsFailedNode(nodeName, talosUpgrade.Status.FailedNodes) {
			logger.V(1).Info("Skipping job for already failed node", "job", job.Name, "node", nodeName)
			continue
		}

		// Return the FIRST job we find that's not completed/failed
		// This ensures only ONE job is processed at a time
		logger.V(1).Info("Found job that needs handling",
			"job", job.Name,
			"node", nodeName,
			"active", job.Status.Active,
			"succeeded", job.Status.Succeeded,
			"failed", job.Status.Failed)
		return &job, nodeName, nil
	}

	return nil, "", nil
}

func (r *TalosUpgradeReconciler) findNextNode(ctx context.Context, talosUpgrade *upgradev1alpha1.TalosUpgrade) (string, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Finding next node to upgrade", "talosupgrade", talosUpgrade.Name)

	nodes, err := r.getTargetNodes(ctx, talosUpgrade)
	if err != nil {
		logger.Error(err, "Failed to get target nodes")
		return "", err
	}

	logger.V(1).Info("Retrieved target nodes", "count", len(nodes))

	targetImage := fmt.Sprintf("%s:%s", talosUpgrade.Spec.Target.Image.Repository, talosUpgrade.Spec.Target.Image.Tag)
	logger.V(1).Info("Target image for upgrade", "image", targetImage)

	for _, node := range nodes {
		if ContainsNode(node.Name, talosUpgrade.Status.CompletedNodes) {
			logger.V(1).Info("Skipping already completed node", "node", node.Name)
			continue
		}

		if ContainsFailedNode(node.Name, talosUpgrade.Status.FailedNodes) {
			logger.V(1).Info("Skipping failed node", "node", node.Name)
			continue
		}

		logger.V(1).Info("Checking if node needs upgrade", "node", node.Name)

		if r.nodeNeedsUpgrade(ctx, &node, targetImage) {
			logger.Info("Node needs upgrade", "node", node.Name)
			return node.Name, nil
		}

		logger.V(1).Info("Node is already at target state", "node", node.Name)
	}

	logger.V(1).Info("No nodes need upgrade")
	return "", nil
}

func (r *TalosUpgradeReconciler) nodeNeedsUpgrade(ctx context.Context, node *corev1.Node, targetImage string) bool {
	logger := log.FromContext(ctx)

	currentVersion := GetTalosVersion(node.Status.NodeInfo.OSImage)
	currentSchematic := GetTalosSchematic(node)
	targetVersion, targetSchematic := GetTalosSchematicAndVersion(targetImage)

	logger.V(1).Info("Comparing node versions",
		"node", node.Name,
		"currentVersion", currentVersion,
		"targetVersion", targetVersion,
		"currentSchematic", currentSchematic,
		"targetSchematic", targetSchematic,
		"osImage", node.Status.NodeInfo.OSImage)

	if targetVersion == "" {
		logger.Info("Could not parse target version, assuming upgrade needed", "node", node.Name, "targetImage", targetImage)
		return true
	}

	versionMismatch := currentVersion != targetVersion
	schematicMismatch := targetSchematic != "" && currentSchematic != targetSchematic
	needsUpgrade := versionMismatch || schematicMismatch

	logger.V(1).Info("Upgrade decision",
		"node", node.Name,
		"versionMismatch", versionMismatch,
		"schematicMismatch", schematicMismatch,
		"needsUpgrade", needsUpgrade)

	return needsUpgrade
}

func (r *TalosUpgradeReconciler) createJob(ctx context.Context, talosUpgrade *upgradev1alpha1.TalosUpgrade, nodeName string) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Creating upgrade job", "node", nodeName)

	targetNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, targetNode); err != nil {
		logger.Error(err, "Failed to get target node", "node", nodeName)
		return nil, err
	}

	nodeIP, err := GetNodeInternalIP(targetNode)
	if err != nil {
		logger.Error(err, "Failed to get node internal IP", "node", nodeName)
		return nil, err
	}

	logger.V(1).Info("Node details", "node", nodeName, "ip", nodeIP)

	job := r.buildJob(ctx, talosUpgrade, nodeName, nodeIP) // Pass ctx instead of creating new one
	if err := controllerutil.SetControllerReference(talosUpgrade, job, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference", "job", job.Name)
		return nil, err
	}

	logger.V(1).Info("Creating job in cluster", "job", job.Name, "node", nodeName, "targetIP", nodeIP)

	if err := r.Create(ctx, job); err != nil {
		if errors.IsAlreadyExists(err) {
			logger.V(1).Info("Job already exists, this is expected in some cases", "job", job.Name)

			existingJob := &batchv1.Job{}
			if getErr := r.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, existingJob); getErr != nil {
				return nil, getErr
			}
			return existingJob, nil
		}
		logger.Error(err, "Failed to create job", "job", job.Name, "node", nodeName)
		return nil, err
	}

	logger.Info("Successfully created upgrade job", "job", job.Name, "node", nodeName)
	return job, nil
}

func (r *TalosUpgradeReconciler) buildJob(ctx context.Context, talosUpgrade *upgradev1alpha1.TalosUpgrade, nodeName, nodeIP string) *batchv1.Job {
	logger := log.FromContext(ctx)

	jobName := GenerateSafeJobName(talosUpgrade.Name, nodeName)

	labels := map[string]string{
		"app.kubernetes.io/name":                "talos-upgrade",
		"app.kubernetes.io/instance":            talosUpgrade.Name,
		"app.kubernetes.io/part-of":             "tuppr",
		"tuppr.home-operations.com/target-node": nodeName,
	}

	// Get talosctl image with defaults
	talosctlRepo := "ghcr.io/siderolabs/talosctl"
	if talosUpgrade.Spec.Talosctl.Image.Repository != "" {
		talosctlRepo = talosUpgrade.Spec.Talosctl.Image.Repository
	}

	talosctlTag := talosUpgrade.Spec.Talosctl.Image.Tag
	if talosctlTag == "" {
		talosctlTag = r.getTalosctlTagFromNode(ctx, nodeName) // Use passed context
		if talosctlTag == "" {
			logger.V(1).Info("Could not determine talosctl version from node, using latest", "node", nodeName)
			talosctlTag = "latest"
		} else {
			logger.V(1).Info("Using talosctl version from node osImage", "node", nodeName, "version", talosctlTag)
		}
	}

	talosctlImage := talosctlRepo + ":" + talosctlTag

	args := []string{
		"upgrade",
		"--nodes", nodeIP,
		"--image", fmt.Sprintf("%s:%s", talosUpgrade.Spec.Target.Image.Repository, talosUpgrade.Spec.Target.Image.Tag),
		"--timeout=15m",
	}

	if talosUpgrade.Spec.Target.Options.Debug {
		args = append(args, "--debug")
		logger.V(1).Info("Debug upgrade enabled", "node", nodeName)
	}

	if talosUpgrade.Spec.Target.Options.Force {
		args = append(args, "--force")
		logger.V(1).Info("Force upgrade enabled", "node", nodeName)
	}
	if talosUpgrade.Spec.Target.Options.RebootMode == "powercycle" {
		args = append(args, "--reboot-mode", "powercycle")
		logger.V(1).Info("Powercycle reboot mode enabled", "node", nodeName)
	}

	pullPolicy := corev1.PullIfNotPresent
	if talosUpgrade.Spec.Talosctl.Image.PullPolicy != "" {
		pullPolicy = talosUpgrade.Spec.Talosctl.Image.PullPolicy
	}

	logger.V(1).Info("Building job specification",
		"node", nodeName,
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
			BackoffLimit:            ptr.To(int32(3)),
			Completions:             ptr.To(int32(1)),
			TTLSecondsAfterFinished: ptr.To(int32(900)), // 15 minutes
			Parallelism:             ptr.To(int32(1)),
			ActiveDeadlineSeconds:   ptr.To(int64(4500)), // 1 hour 15 minutes, covers upgrade, health timeouts plus backoffLimit
			PodReplacementPolicy:    ptr.To(batchv1.TerminatingOrFailed),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(int64(65534)),
						RunAsGroup:   ptr.To(int64(65534)),
						FSGroup:      ptr.To(int64(65534)),
					},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "kubernetes.io/hostname",
										Operator: corev1.NodeSelectorOpNotIn,
										Values:   []string{nodeName},
									}},
								}},
							},
						},
					},
					Tolerations: []corev1.Toleration{{
						Key:      "node-role.kubernetes.io/control-plane",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					}},
					InitContainers: []corev1.Container{{
						Name:            "health",
						Image:           talosctlImage,
						Args:            []string{"health", "--nodes", nodeIP, "--wait-timeout=5m"},
						ImagePullPolicy: pullPolicy,
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							ReadOnlyRootFilesystem:   ptr.To(true),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "talosconfig",
							MountPath: "/var/run/secrets/talos.dev",
							ReadOnly:  true,
						}},
					}},
					Containers: []corev1.Container{{
						Name:            "upgrade",
						Image:           talosctlImage,
						Args:            args,
						ImagePullPolicy: pullPolicy,
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							ReadOnlyRootFilesystem:   ptr.To(true),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "talosconfig",
							MountPath: "/var/run/secrets/talos.dev",
							ReadOnly:  true,
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "talosconfig",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: r.TalosConfigSecret,
							},
						},
					}},
				},
			},
		},
	}
}

func (r *TalosUpgradeReconciler) handleJobStatus(ctx context.Context, talosUpgrade *upgradev1alpha1.TalosUpgrade, nodeName string, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	objectKey := client.ObjectKey{Name: talosUpgrade.Name} // No namespace for cluster-scoped

	logger.V(1).Info("Handling job status",
		"job", job.Name,
		"node", nodeName,
		"active", job.Status.Active,
		"succeeded", job.Status.Succeeded,
		"failed", job.Status.Failed,
		"backoffLimit", *job.Spec.BackoffLimit)

	switch {
	case job.Status.Succeeded > 0:
		logger.Info("Job completed successfully", "job", job.Name, "node", nodeName)

		targetNode := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, targetNode); err != nil {
			logger.Error(err, "Failed to get node for verification", "node", nodeName)
			// Treat verification failure as upgrade failure
			nodeStatus := upgradev1alpha1.NodeUpgradeStatus{
				NodeName:  nodeName,
				LastError: fmt.Sprintf("Verification failed: unable to get node: %v", err),
			}
			if err := r.addFailedNode(ctx, objectKey, nodeStatus); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.updatePhase(ctx, objectKey, PhaseFailed, "",
				fmt.Sprintf("Node %s upgrade verification failed", nodeName)); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
		}

		targetImage := fmt.Sprintf("%s:%s", talosUpgrade.Spec.Target.Image.Repository, talosUpgrade.Spec.Target.Image.Tag)
		if r.nodeNeedsUpgrade(ctx, targetNode, targetImage) {
			logger.Error(nil, "Job succeeded but node still needs upgrade", "node", nodeName)
			nodeStatus := upgradev1alpha1.NodeUpgradeStatus{
				NodeName:  nodeName,
				LastError: "Job succeeded but node not upgraded",
			}
			if err := r.addFailedNode(ctx, objectKey, nodeStatus); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.updatePhase(ctx, objectKey, PhaseFailed, "",
				fmt.Sprintf("Node %s upgrade verification failed", nodeName)); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
		}

		if err := r.addCompletedNode(ctx, objectKey, nodeName); err != nil {
			logger.Error(err, "Failed to add completed node", "node", nodeName)
			return ctrl.Result{}, err
		}

		// Enhanced status message with progress
		completedCount := len(talosUpgrade.Status.CompletedNodes) + 1 // +1 for the node we just completed
		message := fmt.Sprintf("Node %s upgraded successfully (%d completed)", nodeName, completedCount)

		if err := r.updatePhase(ctx, objectKey, PhasePending, "", message); err != nil {
			logger.Error(err, "Failed to update phase", "node", nodeName)
			return ctrl.Result{}, err
		}

		logger.Info("Successfully marked node as completed", "node", nodeName)
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil

	case job.Status.Failed > 0 && job.Status.Failed >= *job.Spec.BackoffLimit:
		logger.Info("Job failed permanently", "job", job.Name, "node", nodeName)

		nodeStatus := upgradev1alpha1.NodeUpgradeStatus{
			NodeName:  nodeName,
			LastError: "Job failed permanently",
		}
		if err := r.addFailedNode(ctx, objectKey, nodeStatus); err != nil {
			logger.Error(err, "Failed to add failed node", "node", nodeName)
			return ctrl.Result{}, err
		}

		// Enhanced failure message
		failedCount := len(talosUpgrade.Status.FailedNodes) + 1 // +1 for the node we just failed
		message := fmt.Sprintf("Node %s upgrade failed (%d failed nodes) - stopping further upgrades", nodeName, failedCount)

		if err := r.updatePhase(ctx, objectKey, PhaseFailed, "", message); err != nil {
			logger.Error(err, "Failed to update phase", "node", nodeName)
			return ctrl.Result{}, err
		}

		logger.Info("Successfully marked node as failed", "node", nodeName)
		return ctrl.Result{RequeueAfter: time.Minute * 10}, nil

	default:
		// Enhanced in-progress status
		message := fmt.Sprintf("Upgrading node %s (job: %s)", nodeName, job.Name)
		if err := r.updatePhase(ctx, objectKey, PhaseInProgress, nodeName, message); err != nil {
			logger.Error(err, "Failed to update in-progress phase", "node", nodeName)
		}

		logger.V(1).Info("Job is still active", "job", job.Name, "node", nodeName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}
}

// Enhanced status update function with progress information
func (r *TalosUpgradeReconciler) updatePhase(ctx context.Context, objectKey client.ObjectKey, phase string, currentNode, message string) error {
	// Get current TalosUpgrade to calculate additional status fields
	talosUpgrade := &upgradev1alpha1.TalosUpgrade{}
	if err := r.Get(ctx, objectKey, talosUpgrade); err != nil {
		return err
	}

	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"phase":              phase,
			"currentNode":        currentNode,
			"message":            message,
			"lastUpdated":        metav1.Now(),
			"observedGeneration": talosUpgrade.Generation,
		},
	}

	return r.patchStatus(ctx, objectKey, patch)
}

// Add a comprehensive status update when upgrades complete
func (r *TalosUpgradeReconciler) updateCompletionStatus(ctx context.Context, objectKey client.ObjectKey, talosUpgrade *upgradev1alpha1.TalosUpgrade) error {
	completedCount := len(talosUpgrade.Status.CompletedNodes)
	failedCount := len(talosUpgrade.Status.FailedNodes)
	totalCount := completedCount + failedCount

	var message string
	var phase string

	if failedCount > 0 {
		phase = PhaseFailed
		message = fmt.Sprintf("Upgrade completed with failures: %d successful, %d failed out of %d nodes",
			completedCount, failedCount, totalCount)
	} else {
		phase = PhaseCompleted
		message = fmt.Sprintf("Successfully upgraded %d nodes", completedCount)
	}

	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"phase":              phase,
			"currentNode":        "",
			"message":            message,
			"lastUpdated":        metav1.Now(),
			"observedGeneration": talosUpgrade.Generation,
		},
	}

	return r.patchStatus(ctx, objectKey, patch)
}

func (r *TalosUpgradeReconciler) addCompletedNode(ctx context.Context, objectKey client.ObjectKey, nodeName string) error {
	// First check if node is already completed to avoid unnecessary patch
	talosUpgrade := &upgradev1alpha1.TalosUpgrade{}
	if err := r.Get(ctx, objectKey, talosUpgrade); err != nil {
		return err
	}

	if ContainsNode(nodeName, talosUpgrade.Status.CompletedNodes) {
		return nil // Already completed
	}

	newCompletedNodes := append(talosUpgrade.Status.CompletedNodes, nodeName)
	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"completedNodes": newCompletedNodes,
			"lastUpdated":    metav1.Now(),
		},
	}

	return r.patchStatus(ctx, objectKey, patch)
}

func (r *TalosUpgradeReconciler) addFailedNode(ctx context.Context, objectKey client.ObjectKey, nodeStatus upgradev1alpha1.NodeUpgradeStatus) error {
	// First check if node is already failed to avoid unnecessary patch
	talosUpgrade := &upgradev1alpha1.TalosUpgrade{}
	if err := r.Get(ctx, objectKey, talosUpgrade); err != nil {
		return err
	}

	if ContainsFailedNode(nodeStatus.NodeName, talosUpgrade.Status.FailedNodes) {
		return nil // Already failed
	}

	newFailedNodes := append(talosUpgrade.Status.FailedNodes, nodeStatus)
	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"failedNodes": newFailedNodes,
			"lastUpdated": metav1.Now(),
		},
	}

	return r.patchStatus(ctx, objectKey, patch)
}

// Helper function to apply status patches
func (r *TalosUpgradeReconciler) patchStatus(ctx context.Context, objectKey client.ObjectKey, patch map[string]interface{}) error {
	logger := log.FromContext(ctx)

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	talosUpgrade := &upgradev1alpha1.TalosUpgrade{}
	talosUpgrade.Name = objectKey.Name
	// No namespace for cluster-scoped resources

	logger.V(1).Info("Applying status patch", "patch", string(patchBytes))

	return r.Status().Patch(ctx, talosUpgrade, client.RawPatch(types.MergePatchType, patchBytes))
}

func (r *TalosUpgradeReconciler) cleanup(ctx context.Context, talosUpgrade *upgradev1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Cleaning up TalosUpgrade", "name", talosUpgrade.Name)

	// Jobs will be cleaned up automatically via owner references
	logger.V(1).Info("Removing finalizer", "name", talosUpgrade.Name, "finalizer", TalosUpgradeFinalizer)
	controllerutil.RemoveFinalizer(talosUpgrade, TalosUpgradeFinalizer)

	if err := r.Update(ctx, talosUpgrade); err != nil {
		logger.Error(err, "Failed to remove finalizer", "name", talosUpgrade.Name)
		return ctrl.Result{}, err
	}

	logger.Info("Successfully cleaned up TalosUpgrade", "name", talosUpgrade.Name)
	return ctrl.Result{}, nil
}

func (r *TalosUpgradeReconciler) getTargetNodes(ctx context.Context, talosUpgrade *upgradev1alpha1.TalosUpgrade) ([]corev1.Node, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Getting target nodes", "talosupgrade", talosUpgrade.Name, "nodeSelector", talosUpgrade.Spec.Target.NodeSelector)

	nodeList := &corev1.NodeList{}
	listOpts := []client.ListOption{}

	if talosUpgrade.Spec.Target.NodeSelector != nil {
		listOpts = append(listOpts, client.MatchingLabels(talosUpgrade.Spec.Target.NodeSelector))
		logger.V(1).Info("Using node selector", "selector", talosUpgrade.Spec.Target.NodeSelector)
	} else {
		logger.V(1).Info("No node selector specified, selecting all nodes")
	}

	if err := r.List(ctx, nodeList, listOpts...); err != nil {
		logger.Error(err, "Failed to list nodes", "talosupgrade", talosUpgrade.Name)
		return nil, err
	}

	nodeNames := make([]string, len(nodeList.Items))
	for i, node := range nodeList.Items {
		nodeNames[i] = node.Name
	}

	logger.V(1).Info("Found target nodes",
		"talosupgrade", talosUpgrade.Name,
		"count", len(nodeList.Items),
		"nodes", nodeNames)

	return nodeList.Items, nil
}

// getTalosctlTagFromNode extracts the Talos version from the node's osImage to use as talosctl tag
func (r *TalosUpgradeReconciler) getTalosctlTagFromNode(ctx context.Context, nodeName string) string {
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	logger := log.FromContext(timeoutCtx)

	node := &corev1.Node{}
	if err := r.Get(timeoutCtx, types.NamespacedName{Name: nodeName}, node); err != nil {
		logger.Error(err, "Failed to get node for talosctl version extraction", "node", nodeName)
		return ""
	}

	osImage := node.Status.NodeInfo.OSImage
	version := GetTalosVersion(osImage)

	logger.V(1).Info("Extracted version from node osImage",
		"node", nodeName,
		"osImage", osImage,
		"extractedVersion", version)

	return version
}

func (r *TalosUpgradeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := ctrl.Log.WithName("setup")
	logger.Info("Setting up TalosUpgrade controller with manager")

	return ctrl.NewControllerManagedBy(mgr).
		For(&upgradev1alpha1.TalosUpgrade{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
