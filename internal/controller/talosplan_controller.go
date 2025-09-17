package controller

import (
	"context"
	"fmt"
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

	upgradev1alpha1 "github.com/home-operations/talup/api/v1alpha1"
)

const (
	TalosPlanFinalizer             = "upgrade.home-operations.com/talos-finalizer"
	SchematicAnnotation            = "extensions.talos.dev/schematic"
	TalosPort                      = "50000"
	DefaultTimeout                 = 30 * time.Minute
	DefaultBackoffLimit            = 3    // 3 retries
	DefaultActiveDeadlineSeconds   = 1800 // 30 minutes
	DefaultTTLSecondsAfterFinished = 900  // 15 minutes
)

// TalosPlanReconciler reconciles a TalosPlan object
type TalosPlanReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=upgrade.home-operations.com,resources=talosupgrades,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=upgrade.home-operations.com,resources=talosupgrades/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=upgrade.home-operations.com,resources=talosupgrades/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *TalosPlanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting reconciliation", "namespacedName", req.NamespacedName)

	// Fetch the TalosPlan instance
	var talosPlan upgradev1alpha1.TalosPlan
	if err := r.Get(ctx, req.NamespacedName, &talosPlan); err != nil {
		if errors.IsNotFound(err) {
			logger.V(1).Info("TalosPlan not found, likely deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get TalosPlan")
		return ctrl.Result{}, err
	}

	logger.Info("Retrieved TalosPlan",
		"name", talosPlan.Name,
		"namespace", talosPlan.Namespace,
		"currentPhase", talosPlan.Status.Phase,
		"generation", talosPlan.Generation,
		"observedGeneration", talosPlan.Status.ObservedGeneration)

	// Handle deletion
	if talosPlan.DeletionTimestamp != nil {
		logger.Info("TalosPlan is being deleted, handling cleanup")
		return r.handleDeletion(ctx, &talosPlan)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&talosPlan, TalosPlanFinalizer) {
		logger.V(1).Info("Adding finalizer to TalosPlan")
		controllerutil.AddFinalizer(&talosPlan, TalosPlanFinalizer)
		return ctrl.Result{}, r.Update(ctx, &talosPlan)
	}

	// Check if upgrade is needed
	logger.Info("Checking if upgrade is needed")
	upgradeNeeded, err := r.isUpgradeNeeded(ctx, &talosPlan)
	if err != nil {
		logger.Error(err, "Failed to check if upgrade is needed")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	logger.Info("Upgrade check completed", "upgradeNeeded", upgradeNeeded)

	if !upgradeNeeded {
		if talosPlan.Status.Phase != PhaseCompleted {
			logger.Info("No upgrade needed, marking as completed")
			talosPlan.Status.Phase = PhaseCompleted
			talosPlan.Status.Message = "All nodes are already at target version"
			talosPlan.Status.LastUpdated = metav1.Now()
			talosPlan.Status.ObservedGeneration = talosPlan.Generation
			return ctrl.Result{}, r.Status().Update(ctx, &talosPlan)
		}
		logger.V(1).Info("Already completed, requeuing for periodic check")
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// If upgrade is needed but we're in Completed/Failed phase, reset to start new upgrade
	if upgradeNeeded && (talosPlan.Status.Phase == PhaseCompleted || talosPlan.Status.Phase == PhaseFailed) {
		logger.Info("Upgrade needed but plan is completed/failed, resetting to start new upgrade")
		talosPlan.Status.Phase = PhasePending
		talosPlan.Status.CurrentNode = ""
		talosPlan.Status.Message = "Starting new upgrade"
		talosPlan.Status.LastUpdated = metav1.Now()
		talosPlan.Status.ObservedGeneration = talosPlan.Generation
		// Clear completed/failed nodes to start fresh
		talosPlan.Status.CompletedNodes = []string{}
		talosPlan.Status.FailedNodes = []upgradev1alpha1.NodeUpgradeStatus{}
		return ctrl.Result{}, r.Status().Update(ctx, &talosPlan)
	}

	// Initialize status if needed
	if talosPlan.Status.Phase == "" {
		logger.Info("Initializing TalosPlan status to Pending")
		talosPlan.Status.Phase = PhasePending
		talosPlan.Status.LastUpdated = metav1.Now()
		talosPlan.Status.ObservedGeneration = talosPlan.Generation
		return ctrl.Result{}, r.Status().Update(ctx, &talosPlan)
	}

	logger.Info("Handling upgrade phase", "phase", talosPlan.Status.Phase)

	// Handle upgrade phases
	switch talosPlan.Status.Phase {
	case PhasePending:
		return r.handlePendingPhase(ctx, &talosPlan)
	case PhaseInProgress:
		return r.handleInProgressPhase(ctx, &talosPlan)
	case PhaseFailed:
		return r.handleFailedPhase(ctx, &talosPlan)
	case PhaseCompleted:
		logger.V(1).Info("Plan completed, requeuing for periodic check")
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	logger.V(1).Info("Reconciliation completed")
	return ctrl.Result{}, nil
}

func (r *TalosPlanReconciler) handleDeletion(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting deletion cleanup", "talosPlan", talosPlan.Name)

	// Clean up any running jobs
	jobList := &batchv1.JobList{}
	listOpts := []client.ListOption{
		client.InNamespace(talosPlan.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":       "talos-upgrade",
			"app.kubernetes.io/instance":   talosPlan.Name,
			"app.kubernetes.io/managed-by": "talup",
		},
	}

	if err := r.List(ctx, jobList, listOpts...); err != nil {
		logger.Error(err, "Failed to list jobs for cleanup")
		return ctrl.Result{}, err
	}

	logger.Info("Found jobs to cleanup", "count", len(jobList.Items))

	for _, job := range jobList.Items {
		logger.Info("Deleting job", "job", job.Name)
		if err := r.Delete(ctx, &job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			logger.Error(err, "Failed to delete job", "job", job.Name)
			return ctrl.Result{}, err
		}
	}

	// Remove finalizer
	logger.Info("Removing finalizer")
	controllerutil.RemoveFinalizer(talosPlan, TalosPlanFinalizer)
	return ctrl.Result{}, r.Update(ctx, talosPlan)
}

func (r *TalosPlanReconciler) isUpgradeNeeded(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (bool, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Checking upgrade requirements")

	// Get all nodes that match the selector
	nodes, err := r.getTargetNodes(ctx, talosPlan)
	if err != nil {
		logger.Error(err, "Failed to get target nodes")
		return false, err
	}

	logger.Info("Found target nodes", "count", len(nodes))

	targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag)
	logger.Info("Target image", "image", targetImage)

	for _, node := range nodes {
		logger.Info("Checking node upgrade status", "node", node.Name)

		// Simple check using node status - no API calls needed!
		needsUpgrade, err := r.nodeNeedsUpgrade(ctx, talosPlan, &node, targetImage)
		if err != nil {
			logger.Error(err, "Failed to check if node needs upgrade", "node", node.Name)
			continue // Skip this node and check others
		}

		logger.Info("Node upgrade check result", "node", node.Name, "needsUpgrade", needsUpgrade)

		if needsUpgrade {
			logger.Info("At least one node needs upgrade", "node", node.Name)
			return true, nil
		}
	}

	logger.Info("No nodes need upgrade")
	return false, nil
}

func (r *TalosPlanReconciler) nodeNeedsUpgrade(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, node *corev1.Node, targetImage string) (bool, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Checking individual node upgrade status", "node", node.Name, "targetImage", targetImage)

	// Extract current version from node's OSImage field
	// Example: "Talos (v1.11.1)" -> "v1.11.1"
	currentVersion := ExtractVersionFromOSImage(node.Status.NodeInfo.OSImage)
	logger.Info("Retrieved current version from OSImage", "node", node.Name, "osImage", node.Status.NodeInfo.OSImage, "currentVersion", currentVersion)

	// Get current schematic from node annotation
	currentSchematic := GetSchematicFromNode(node)
	logger.Info("Current schematic from annotation", "node", node.Name, "currentSchematic", currentSchematic)

	// Extract target version and schematic from target image
	targetVersion, targetSchematic := ExtractVersionAndSchematic(targetImage)
	if targetVersion == "" {
		logger.Info("Could not extract target version from image, assuming upgrade needed", "image", targetImage)
		return true, nil
	}
	logger.Info("Target version and schematic", "targetVersion", targetVersion, "targetSchematic", targetSchematic)

	logger.Info("Comparing upgrade status",
		"node", node.Name,
		"currentVersion", currentVersion,
		"targetVersion", targetVersion,
		"currentSchematic", currentSchematic,
		"targetSchematic", targetSchematic,
	)

	// Need upgrade if:
	// 1. Versions don't match, OR
	// 2. Schematics don't match
	versionMismatch := currentVersion != targetVersion
	schematicMismatch := targetSchematic != "" && currentSchematic != targetSchematic

	needsUpgrade := versionMismatch || schematicMismatch

	logger.Info("Upgrade decision",
		"node", node.Name,
		"versionMismatch", versionMismatch,
		"schematicMismatch", schematicMismatch,
		"needsUpgrade", needsUpgrade)

	return needsUpgrade, nil
}

func (r *TalosPlanReconciler) verifyNodeUpgrade(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, nodeName string) bool {
	logger := log.FromContext(ctx)
	logger.Info("Verifying node upgrade", "node", nodeName)

	// Get the node and check if it has the expected version
	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		logger.Error(err, "Failed to get node for verification", "node", nodeName)
		return false
	}

	targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag)
	logger.V(1).Info("Checking if node still needs upgrade", "node", nodeName, "targetImage", targetImage)

	needsUpgrade, err := r.nodeNeedsUpgrade(ctx, talosPlan, node, targetImage)
	if err != nil {
		logger.Error(err, "Failed to verify node upgrade", "node", nodeName)
		// If we can't verify, assume it failed
		return false
	}

	// If it doesn't need upgrade, then the upgrade was successful
	upgradeSuccessful := !needsUpgrade
	logger.Info("Node upgrade verification result", "node", nodeName, "upgradeSuccessful", upgradeSuccessful)
	return upgradeSuccessful
}

func (r *TalosPlanReconciler) handlePendingPhase(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling pending phase", "talosPlan", talosPlan.Name)

	// **ATOMIC OPERATION: Update status first as a "lock"**
	// Only proceed if we successfully claim a node
	nodes, err := r.getTargetNodes(ctx, talosPlan)
	if err != nil {
		logger.Error(err, "Failed to get target nodes")
		return ctrl.Result{}, err
	}

	targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag)

	// Find first node that needs upgrade
	var targetNode *corev1.Node
	for _, node := range nodes {
		needsUpgrade, err := r.nodeNeedsUpgrade(ctx, talosPlan, &node, targetImage)
		if err != nil {
			logger.Error(err, "Failed to check if node needs upgrade", "node", node.Name)
			continue
		}

		if needsUpgrade &&
			!IsNodeInList(node.Name, talosPlan.Status.CompletedNodes) &&
			!IsNodeInFailedList(node.Name, talosPlan.Status.FailedNodes) {
			targetNode = &node
			break
		}
	}

	if targetNode == nil {
		logger.Info("All nodes are upgraded, marking plan as completed")
		talosPlan.Status.Phase = PhaseCompleted
		talosPlan.Status.Message = "All nodes successfully upgraded"
		talosPlan.Status.CurrentNode = ""
		talosPlan.Status.LastUpdated = metav1.Now()
		return ctrl.Result{}, r.Status().Update(ctx, talosPlan)
	}

	talosPlan.Status.Phase = PhaseInProgress
	talosPlan.Status.CurrentNode = targetNode.Name
	talosPlan.Status.Message = fmt.Sprintf("Claiming node %s for upgrade", targetNode.Name)
	talosPlan.Status.LastUpdated = metav1.Now()

	if err := r.Status().Update(ctx, talosPlan); err != nil {
		logger.Error(err, "Failed to claim node", "node", targetNode.Name)
		return ctrl.Result{}, err
	}

	logger.Info("Creating upgrade job for claimed node", "targetNode", targetNode.Name)
	job, err := r.createOrUpdateUpgradeJob(ctx, talosPlan, targetNode.Name)
	if err != nil {
		logger.Error(err, "Failed to create upgrade job", "node", targetNode.Name)
		// Reset status on failure
		talosPlan.Status.Phase = PhasePending
		talosPlan.Status.CurrentNode = ""
		talosPlan.Status.Message = fmt.Sprintf("Failed to create job for node %s", targetNode.Name)
		r.Status().Update(ctx, talosPlan)
		return ctrl.Result{}, err
	}

	// Update message to reflect job creation
	talosPlan.Status.Message = fmt.Sprintf("Upgrading node %s", targetNode.Name)
	talosPlan.Status.LastUpdated = metav1.Now()

	logger.Info("Successfully created upgrade job", "job", job.Name, "node", targetNode.Name)
	return ctrl.Result{RequeueAfter: time.Second * 30}, r.Status().Update(ctx, talosPlan)
}

func (r *TalosPlanReconciler) handleInProgressPhase(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling in-progress phase", "talosPlan", talosPlan.Name, "currentNode", talosPlan.Status.CurrentNode)

	if talosPlan.Status.CurrentNode == "" {
		logger.Info("No current node, moving to pending to start next upgrade")
		talosPlan.Status.Phase = PhasePending
		return ctrl.Result{}, r.Status().Update(ctx, talosPlan)
	}

	// Check job status
	jobName := fmt.Sprintf("talos-upgrade-%s-%s", talosPlan.Name, talosPlan.Status.CurrentNode)
	logger.V(1).Info("Checking job status", "job", jobName)

	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      jobName,
		Namespace: talosPlan.Namespace,
	}, job)

	if err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "Job not found", "job", jobName)
			return r.markNodeFailed(ctx, talosPlan, talosPlan.Status.CurrentNode, "Job not found")
		}
		logger.Error(err, "Failed to get upgrade job", "job", jobName)
		return ctrl.Result{}, err
	}

	logger.V(1).Info("Job status", "job", jobName,
		"succeeded", job.Status.Succeeded,
		"failed", job.Status.Failed,
		"active", job.Status.Active)

	switch {
	case job.Status.Succeeded > 0:
		logger.Info("Job succeeded, verifying node upgrade", "job", jobName, "node", talosPlan.Status.CurrentNode)
		if r.verifyNodeUpgrade(ctx, talosPlan, talosPlan.Status.CurrentNode) {
			logger.Info("Node upgrade verified successfully", "node", talosPlan.Status.CurrentNode)
			return r.markNodeCompleted(ctx, talosPlan, talosPlan.Status.CurrentNode)
		} else {
			logger.Error(fmt.Errorf("verification failed"), "Node upgrade verification failed", "node", talosPlan.Status.CurrentNode)
			return r.markNodeFailed(ctx, talosPlan, talosPlan.Status.CurrentNode, "Node upgrade verification failed")
		}

	case job.Status.Failed > 0:
		backoffLimit := int32(DefaultBackoffLimit)
		if job.Spec.BackoffLimit != nil {
			backoffLimit = *job.Spec.BackoffLimit
		}

		if job.Status.Failed > backoffLimit {
			logger.Error(fmt.Errorf("job failed after retries"), "Upgrade job failed after all retries",
				"job", jobName,
				"node", talosPlan.Status.CurrentNode,
				"failed", job.Status.Failed,
				"backoffLimit", backoffLimit)

			return r.markNodeFailed(ctx, talosPlan, talosPlan.Status.CurrentNode,
				fmt.Sprintf("Upgrade job failed after %d retries", job.Status.Failed))
		} else {
			logger.Info("Job has failures but still retrying", "job", jobName,
				"failed", job.Status.Failed,
				"backoffLimit", backoffLimit)
		}
	}

	// Check for timeout
	timeout := r.getTimeout(talosPlan.Spec.Timeout)
	jobAge := time.Since(job.CreationTimestamp.Time)
	logger.V(1).Info("Job timing", "job", jobName, "age", jobAge, "timeout", timeout)

	if jobAge > timeout {
		logger.Error(fmt.Errorf("timeout"), "Upgrade job timed out", "job", jobName, "age", jobAge, "timeout", timeout)
		return r.markNodeFailed(ctx, talosPlan, talosPlan.Status.CurrentNode, "Upgrade timeout")
	}

	// Job still running or retrying
	logger.V(1).Info("Job still running", "job", jobName)
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

func (r *TalosPlanReconciler) handleFailedPhase(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling failed phase", "talosPlan", talosPlan.Name)

	// In failed phase, manual intervention is needed
	// Update message and wait
	expectedMessage := "Manual intervention required - upgrade failed after max retries"
	if talosPlan.Status.Message != expectedMessage {
		logger.Info("Updating failed phase message")
		talosPlan.Status.Message = expectedMessage
		talosPlan.Status.LastUpdated = metav1.Now()
		return ctrl.Result{}, r.Status().Update(ctx, talosPlan)
	}

	logger.V(1).Info("Plan in failed state, waiting for manual intervention")
	return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}

func (r *TalosPlanReconciler) markNodeCompleted(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, nodeName string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Marking node as completed", "node", nodeName)

	talosPlan.Status.CompletedNodes = append(talosPlan.Status.CompletedNodes, nodeName)
	talosPlan.Status.CurrentNode = ""
	talosPlan.Status.Phase = PhasePending // Move to pending to process next node
	talosPlan.Status.Message = fmt.Sprintf("Node %s upgraded successfully", nodeName)
	talosPlan.Status.LastUpdated = metav1.Now()

	logger.Info("Node marked as completed", "node", nodeName, "totalCompleted", len(talosPlan.Status.CompletedNodes))
	return ctrl.Result{}, r.Status().Update(ctx, talosPlan)
}

func (r *TalosPlanReconciler) markNodeFailed(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, nodeName, reason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Marking node as failed", "node", nodeName, "reason", reason)

	// Add to failed nodes list (for tracking purposes)
	talosPlan.Status.FailedNodes = append(talosPlan.Status.FailedNodes, upgradev1alpha1.NodeUpgradeStatus{
		NodeName:  nodeName,
		LastError: reason,
		Retries:   0, // Job handled the retries
	})

	// Mark plan as failed - manual intervention needed
	logger.Info("Node failed, marking plan as failed", "node", nodeName)
	talosPlan.Status.Phase = PhaseFailed
	talosPlan.Status.CurrentNode = ""
	talosPlan.Status.Message = fmt.Sprintf("Node %s failed: %s", nodeName, reason)
	talosPlan.Status.LastUpdated = metav1.Now()

	return ctrl.Result{RequeueAfter: time.Second * 30}, r.Status().Update(ctx, talosPlan)
}

func (r *TalosPlanReconciler) createOrUpdateUpgradeJob(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, nodeName string) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)
	logger.Info("Creating or updating upgrade job", "node", nodeName)

	// Get target node IP for the talosctl command
	targetNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, targetNode); err != nil {
		logger.Error(err, "Failed to get target node", "node", nodeName)
		return nil, fmt.Errorf("failed to get target node: %w", err)
	}

	targetNodeIP, err := GetNodeInternalIP(targetNode)
	if err != nil {
		logger.Error(err, "Failed to get target node IP", "node", nodeName)
		return nil, fmt.Errorf("failed to get target node IP: %w", err)
	}

	// Prepare job name
	jobName := fmt.Sprintf("talos-upgrade-%s-%s", talosPlan.Name, nodeName)

	// Build labels
	labels := map[string]string{
		"app.kubernetes.io/name":       "talos-upgrade",
		"app.kubernetes.io/instance":   talosPlan.Name,
		"app.kubernetes.io/managed-by": "talup",
		"talup.io/target-node":         nodeName,
	}

	talosctlImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Talosctl.Image.Repository, talosPlan.Spec.Talosctl.Image.Tag)
	targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag)

	// Use IP address for talosctl command
	args := []string{"upgrade", "--nodes", targetNodeIP, "--image", targetImage, "--debug"}
	if talosPlan.Spec.Timeout != "" {
		args = append(args, "--timeout", talosPlan.Spec.Timeout)
	}
	if talosPlan.Spec.Force {
		args = append(args, "--force")
	}
	if talosPlan.Spec.RebootMode == "powercycle" {
		args = append(args, "--reboot-mode", "powercycle")
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: talosPlan.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, job, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(talosPlan, job, r.Scheme); err != nil {
			return err
		}

		// Only set labels and spec if this is a new job - Jobs are immutable once created!
		if job.CreationTimestamp.IsZero() {
			job.Labels = labels
			job.Spec = batchv1.JobSpec{
				BackoffLimit:            ptr.To(int32(DefaultBackoffLimit)),
				Completions:             ptr.To(int32(1)),
				TTLSecondsAfterFinished: ptr.To(int32(DefaultTTLSecondsAfterFinished)),
				Parallelism:             ptr.To(int32(1)),
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
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
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "kubernetes.io/hostname",
													Operator: corev1.NodeSelectorOpNotIn,
													Values:   []string{nodeName},
												},
											},
										},
									},
								},
							},
						},
						Tolerations: []corev1.Toleration{{
							Key:      "node-role.kubernetes.io/control-plane",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						}},
						Containers: []corev1.Container{
							{
								Name:            "talosctl",
								Image:           talosctlImage,
								ImagePullPolicy: corev1.PullIfNotPresent,
								Args:            args,
								SecurityContext: &corev1.SecurityContext{
									AllowPrivilegeEscalation: ptr.To(false),
									ReadOnlyRootFilesystem:   ptr.To(true),
									Capabilities: &corev1.Capabilities{
										Drop: []corev1.Capability{"ALL"},
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "talos",
										MountPath: "/var/run/secrets/talos.dev",
										ReadOnly:  true,
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "talos",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: TalosConfigSecretName,
									},
								},
							},
						},
					},
				},
			}
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "Failed to create/update job", "job", jobName)
		return nil, err
	}

	logger.Info("Successfully created/updated upgrade job",
		"operation", op,
		"job", jobName,
		"targetNode", nodeName,
		"targetIP", targetNodeIP)
	return job, nil
}

func (r *TalosPlanReconciler) getTimeout(timeoutStr string) time.Duration {
	if timeoutStr == "" {
		return DefaultTimeout
	}

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return DefaultTimeout
	}
	return timeout
}

func (r *TalosPlanReconciler) getTargetNodes(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) ([]corev1.Node, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Getting target nodes", "nodeSelector", talosPlan.Spec.NodeSelector)

	nodeList := &corev1.NodeList{}
	listOpts := []client.ListOption{}

	if talosPlan.Spec.NodeSelector != nil {
		listOpts = append(listOpts, client.MatchingLabels(talosPlan.Spec.NodeSelector))
		logger.V(1).Info("Using node selector", "selector", talosPlan.Spec.NodeSelector)
	}

	if err := r.List(ctx, nodeList, listOpts...); err != nil {
		logger.Error(err, "Failed to list nodes")
		return nil, err
	}

	logger.Info("Retrieved target nodes", "count", len(nodeList.Items))
	for _, node := range nodeList.Items {
		logger.V(1).Info("Target node", "name", node.Name)
	}

	return nodeList.Items, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TalosPlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&upgradev1alpha1.TalosPlan{}).
		Owns(&batchv1.Job{}).
		Named("talosupgrade").
		Complete(r)
}
