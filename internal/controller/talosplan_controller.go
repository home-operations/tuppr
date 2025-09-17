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
	TalosPlanFinalizer = "upgrade.home-operations.com/talos-finalizer"
)

type TalosPlanReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	TalosctlImage     string
	TalosConfigSecret string
}

func (r *TalosPlanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting reconciliation", "talosplan", req.NamespacedName)

	var talosPlan upgradev1alpha1.TalosPlan
	if err := r.Get(ctx, req.NamespacedName, &talosPlan); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("TalosPlan resource not found, likely deleted", "talosplan", req.NamespacedName)
		} else {
			logger.Error(err, "Failed to get TalosPlan resource", "talosplan", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Retrieved TalosPlan",
		"name", talosPlan.Name,
		"namespace", talosPlan.Namespace,
		"phase", talosPlan.Status.Phase,
		"generation", talosPlan.Generation,
		"observedGeneration", talosPlan.Status.ObservedGeneration)

	// Handle deletion
	if talosPlan.DeletionTimestamp != nil {
		logger.Info("TalosPlan is being deleted, starting cleanup", "name", talosPlan.Name)
		return r.cleanup(ctx, &talosPlan)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&talosPlan, TalosPlanFinalizer) {
		logger.Info("Adding finalizer to TalosPlan", "name", talosPlan.Name, "finalizer", TalosPlanFinalizer)
		controllerutil.AddFinalizer(&talosPlan, TalosPlanFinalizer)
		return ctrl.Result{}, r.Update(ctx, &talosPlan)
	}

	// Simple state machine
	logger.Info("Processing upgrade for TalosPlan", "name", talosPlan.Name, "currentPhase", talosPlan.Status.Phase)
	return r.processUpgrade(ctx, &talosPlan)
}

func (r *TalosPlanReconciler) processUpgrade(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting upgrade processing",
		"talosplan", talosPlan.Name,
		"targetImage", fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag),
		"completedNodes", len(talosPlan.Status.CompletedNodes),
		"failedNodes", len(talosPlan.Status.FailedNodes))

	// First, check if there's any active job running for this plan
	activeJob, activeNode, err := r.findActiveJob(ctx, talosPlan)
	if err != nil {
		logger.Error(err, "Failed to find active jobs", "talosplan", talosPlan.Name)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if activeJob != nil {
		logger.Info("Found active job, monitoring status",
			"job", activeJob.Name,
			"node", activeNode,
			"active", activeJob.Status.Active,
			"succeeded", activeJob.Status.Succeeded,
			"failed", activeJob.Status.Failed)

		// Monitor the active job
		return r.handleJobStatus(ctx, talosPlan, activeNode, activeJob)
	}

	// No active job, check if we have failed nodes that should block progress
	if len(talosPlan.Status.FailedNodes) > 0 {
		logger.Info("Upgrade plan has failed nodes, blocking further progress",
			"talosplan", talosPlan.Name,
			"failedNodes", len(talosPlan.Status.FailedNodes))
		return r.updateStatus(ctx, talosPlan, PhaseFailed, "",
			fmt.Sprintf("Upgrade stopped due to %d failed nodes", len(talosPlan.Status.FailedNodes)),
			time.Minute*5)
	}

	// Find next node to upgrade
	nextNode, err := r.findNextNode(ctx, talosPlan)
	if err != nil {
		logger.Error(err, "Failed to find next node to upgrade", "talosplan", talosPlan.Name)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if nextNode == "" {
		logger.Info("No more nodes to upgrade, marking plan as completed",
			"talosplan", talosPlan.Name,
			"completedNodes", talosPlan.Status.CompletedNodes,
			"failedNodes", len(talosPlan.Status.FailedNodes))
		return r.updateStatus(ctx, talosPlan, PhaseCompleted, "", "All nodes upgraded successfully", time.Minute*5)
	}

	logger.Info("Found next node to upgrade", "talosplan", talosPlan.Name, "node", nextNode)

	// Create job for the next node
	logger.Info("Creating new upgrade job", "node", nextNode)
	if _, err := r.createJob(ctx, talosPlan, nextNode); err != nil {
		logger.Error(err, "Failed to create upgrade job", "node", nextNode)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	logger.Info("Successfully created upgrade job", "node", nextNode)
	return r.updateStatus(ctx, talosPlan, PhaseInProgress, nextNode, fmt.Sprintf("Upgrading node %s", nextNode), time.Second*30)
}

// findActiveJob looks for any currently running job for this TalosPlan
func (r *TalosPlanReconciler) findActiveJob(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (*batchv1.Job, string, error) {
	logger := log.FromContext(ctx)

	// List all jobs owned by this TalosPlan
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(talosPlan.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     "talos-upgrade",
			"app.kubernetes.io/instance": talosPlan.Name,
		}); err != nil {
		return nil, "", err
	}

	logger.Info("Found jobs for TalosPlan", "count", len(jobList.Items))

	for _, job := range jobList.Items {
		nodeName := job.Labels["talup.io/target-node"]

		// Skip jobs for nodes that are already completed
		if ContainsNode(nodeName, talosPlan.Status.CompletedNodes) {
			logger.V(1).Info("Skipping job for already completed node", "job", job.Name, "node", nodeName)
			continue
		}

		// Skip jobs for nodes that are already failed
		if ContainsFailedNode(nodeName, talosPlan.Status.FailedNodes) {
			logger.V(1).Info("Skipping job for already failed node", "job", job.Name, "node", nodeName)
			continue
		}

		// Check if job needs attention (running, succeeded, or permanently failed)
		if r.jobNeedsHandling(&job) {
			logger.Info("Found job that needs handling",
				"job", job.Name,
				"node", nodeName,
				"active", job.Status.Active,
				"succeeded", job.Status.Succeeded,
				"failed", job.Status.Failed)
			return &job, nodeName, nil
		}
	}

	return nil, "", nil
}

// jobNeedsHandling returns true if the job needs to be handled (running, completed, or failed)
func (r *TalosPlanReconciler) jobNeedsHandling(job *batchv1.Job) bool {
	// Handle jobs that are:
	// 1. Still running (active > 0), OR
	// 2. Have succeeded, OR
	// 3. Have failed permanently (failed >= backoffLimit), OR
	// 4. Are still retrying (failed < backoffLimit and succeeded = 0)
	return job.Status.Active > 0 ||
		job.Status.Succeeded > 0 ||
		job.Status.Failed >= *job.Spec.BackoffLimit ||
		(job.Status.Succeeded == 0 && job.Status.Failed < *job.Spec.BackoffLimit && job.Status.Failed > 0)
}

func (r *TalosPlanReconciler) findNextNode(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (string, error) {
	logger := log.FromContext(ctx)
	logger.Info("Finding next node to upgrade", "talosplan", talosPlan.Name)

	nodes, err := r.getTargetNodes(ctx, talosPlan)
	if err != nil {
		logger.Error(err, "Failed to get target nodes")
		return "", err
	}

	logger.Info("Retrieved target nodes", "count", len(nodes))

	targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag)
	logger.Info("Target image for upgrade", "image", targetImage)

	for _, node := range nodes {
		if ContainsNode(node.Name, talosPlan.Status.CompletedNodes) {
			logger.V(1).Info("Skipping already completed node", "node", node.Name)
			continue
		}

		if ContainsFailedNode(node.Name, talosPlan.Status.FailedNodes) {
			logger.V(1).Info("Skipping failed node", "node", node.Name)
			continue
		}

		logger.Info("Checking if node needs upgrade", "node", node.Name)

		if needsUpgrade, _ := r.nodeNeedsUpgrade(ctx, talosPlan, &node, targetImage); needsUpgrade {
			logger.Info("Node needs upgrade", "node", node.Name)
			return node.Name, nil
		}

		logger.Info("Node is already at target state", "node", node.Name)
	}

	logger.Info("No nodes need upgrade")
	return "", nil
}

func (r *TalosPlanReconciler) nodeNeedsUpgrade(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, node *corev1.Node, targetImage string) (bool, error) {
	logger := log.FromContext(ctx)

	currentVersion := GetTalosVersion(node.Status.NodeInfo.OSImage)
	currentSchematic := GetTalosSchematic(node)
	targetVersion, targetSchematic := GetTalosSchematicAndVersion(targetImage)

	logger.Info("Comparing node versions",
		"node", node.Name,
		"currentVersion", currentVersion,
		"targetVersion", targetVersion,
		"currentSchematic", currentSchematic,
		"targetSchematic", targetSchematic,
		"osImage", node.Status.NodeInfo.OSImage)

	if targetVersion == "" {
		logger.Info("Could not parse target version, assuming upgrade needed", "node", node.Name, "targetImage", targetImage)
		return true, nil
	}

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

func (r *TalosPlanReconciler) createJob(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, nodeName string) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)
	logger.Info("Creating upgrade job", "node", nodeName)

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

	logger.Info("Node details", "node", nodeName, "ip", nodeIP)

	job := r.buildJob(talosPlan, nodeName, nodeIP)
	if err := controllerutil.SetControllerReference(talosPlan, job, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference", "job", job.Name)
		return nil, err
	}

	logger.Info("Creating job in cluster", "job", job.Name, "node", nodeName, "targetIP", nodeIP)

	if err := r.Create(ctx, job); err != nil {
		if errors.IsAlreadyExists(err) {
			logger.Info("Job already exists, this is expected in some cases", "job", job.Name)
			// Return the existing job
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

func (r *TalosPlanReconciler) buildJob(talosPlan *upgradev1alpha1.TalosPlan, nodeName, nodeIP string) *batchv1.Job {
	logger := log.FromContext(context.Background())

	labels := map[string]string{
		"app.kubernetes.io/name":     "talos-upgrade",
		"app.kubernetes.io/instance": talosPlan.Name,
		"talup.io/target-node":       nodeName,
	}

	args := []string{
		"upgrade",
		"--nodes", nodeIP,
		"--image", fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag),
		"--timeout=10m",
	}

	if talosPlan.Spec.Force {
		args = append(args, "--force")
		logger.Info("Force upgrade enabled", "node", nodeName)
	}
	if talosPlan.Spec.RebootMode == "powercycle" {
		args = append(args, "--reboot-mode", "powercycle")
		logger.Info("Powercycle reboot mode enabled", "node", nodeName)
	}

	talosctlImage := r.TalosctlImage

	logger.Info("Building job specification",
		"node", nodeName,
		"talosctlImage", talosctlImage,
		"args", args)

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("talos-upgrade-%s-%s", talosPlan.Name, nodeName),
			Namespace: talosPlan.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To(int32(3)),
			Completions:             ptr.To(int32(1)),
			TTLSecondsAfterFinished: ptr.To(int32(900)), // 15 minutes
			Parallelism:             ptr.To(int32(1)),
			ActiveDeadlineSeconds:   ptr.To(int64(3600)), // 1 hour
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
						Name:  "health-check",
						Image: talosctlImage,
						Args:  []string{"health", "--nodes", nodeIP, "--wait-timeout=5m"},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							ReadOnlyRootFilesystem:   ptr.To(true),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "talos",
							MountPath: "/var/run/secrets/talos.dev",
							ReadOnly:  true,
						}},
					}},
					Containers: []corev1.Container{{
						Name:  "talosctl",
						Image: talosctlImage,
						Args:  args,
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							ReadOnlyRootFilesystem:   ptr.To(true),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "talos",
							MountPath: "/var/run/secrets/talos.dev",
							ReadOnly:  true,
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "talos",
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

func (r *TalosPlanReconciler) handleJobStatus(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, nodeName string, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("Handling job status",
		"job", job.Name,
		"node", nodeName,
		"active", job.Status.Active,
		"succeeded", job.Status.Succeeded,
		"failed", job.Status.Failed,
		"backoffLimit", *job.Spec.BackoffLimit)

	switch {
	case job.Status.Succeeded > 0:
		logger.Info("Job completed successfully", "job", job.Name, "node", nodeName)

		// Verify the node actually upgraded before marking as complete
		targetNode := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, targetNode); err != nil {
			logger.Error(err, "Failed to get node for verification", "node", nodeName)
			// Continue anyway, job succeeded
		} else {
			targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag)
			if needsUpgrade, _ := r.nodeNeedsUpgrade(ctx, talosPlan, targetNode, targetImage); needsUpgrade {
				logger.Error(nil, "Job succeeded but node still needs upgrade", "node", nodeName)
				// Mark as failed since the upgrade didn't actually work
				talosPlan.Status.FailedNodes = append(talosPlan.Status.FailedNodes,
					upgradev1alpha1.NodeUpgradeStatus{NodeName: nodeName, LastError: "Job succeeded but node not upgraded"})
				return r.updateStatus(ctx, talosPlan, PhaseFailed, "",
					fmt.Sprintf("Node %s upgrade verification failed", nodeName), time.Minute*5)
			}
		}

		// Add to completed list and move on
		if !ContainsNode(nodeName, talosPlan.Status.CompletedNodes) {
			talosPlan.Status.CompletedNodes = append(talosPlan.Status.CompletedNodes, nodeName)
		}
		logger.Info("Added node to completed list",
			"node", nodeName,
			"totalCompleted", len(talosPlan.Status.CompletedNodes),
			"completedNodes", talosPlan.Status.CompletedNodes)
		return r.updateStatus(ctx, talosPlan, PhasePending, "", fmt.Sprintf("Node %s upgraded successfully", nodeName), time.Second*5)

	case job.Status.Failed > 0 && job.Status.Failed >= *job.Spec.BackoffLimit:
		logger.Info("Job failed permanently",
			"job", job.Name,
			"node", nodeName,
			"failures", job.Status.Failed,
			"backoffLimit", *job.Spec.BackoffLimit)

		// Add to failed list - this will block further progress
		if !ContainsFailedNode(nodeName, talosPlan.Status.FailedNodes) {
			talosPlan.Status.FailedNodes = append(talosPlan.Status.FailedNodes,
				upgradev1alpha1.NodeUpgradeStatus{NodeName: nodeName, LastError: "Job failed permanently"})
		}
		logger.Info("Added node to failed list",
			"node", nodeName,
			"totalFailed", len(talosPlan.Status.FailedNodes))
		return r.updateStatus(ctx, talosPlan, PhaseFailed, "",
			fmt.Sprintf("Node %s upgrade failed - stopping further upgrades", nodeName), time.Minute*10)

	default:
		// Still running or retrying
		logger.Info("Job is still active",
			"job", job.Name,
			"node", nodeName,
			"active", job.Status.Active,
			"failed", job.Status.Failed,
			"backoffLimit", *job.Spec.BackoffLimit)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}
}

func (r *TalosPlanReconciler) updateStatus(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, phase, currentNode, message string, requeue time.Duration) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	oldPhase := talosPlan.Status.Phase

	talosPlan.Status.Phase = phase
	talosPlan.Status.CurrentNode = currentNode
	talosPlan.Status.Message = message
	talosPlan.Status.LastUpdated = metav1.Now()
	talosPlan.Status.ObservedGeneration = talosPlan.Generation

	logger.Info("Updating TalosPlan status",
		"name", talosPlan.Name,
		"oldPhase", oldPhase,
		"newPhase", phase,
		"currentNode", currentNode,
		"message", message,
		"requeue", requeue)

	if err := r.Status().Update(ctx, talosPlan); err != nil {
		logger.Error(err, "Failed to update TalosPlan status", "name", talosPlan.Name)
		return ctrl.Result{}, err
	}

	logger.Info("Successfully updated TalosPlan status", "name", talosPlan.Name, "phase", phase)
	return ctrl.Result{RequeueAfter: requeue}, nil
}

func (r *TalosPlanReconciler) cleanup(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Cleaning up TalosPlan", "name", talosPlan.Name)

	// Jobs will be cleaned up automatically via owner references
	logger.Info("Removing finalizer", "name", talosPlan.Name, "finalizer", TalosPlanFinalizer)
	controllerutil.RemoveFinalizer(talosPlan, TalosPlanFinalizer)

	if err := r.Update(ctx, talosPlan); err != nil {
		logger.Error(err, "Failed to remove finalizer", "name", talosPlan.Name)
		return ctrl.Result{}, err
	}

	logger.Info("Successfully cleaned up TalosPlan", "name", talosPlan.Name)
	return ctrl.Result{}, nil
}

func (r *TalosPlanReconciler) getTargetNodes(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) ([]corev1.Node, error) {
	logger := log.FromContext(ctx)
	logger.Info("Getting target nodes", "talosplan", talosPlan.Name, "nodeSelector", talosPlan.Spec.NodeSelector)

	nodeList := &corev1.NodeList{}
	listOpts := []client.ListOption{}

	if talosPlan.Spec.NodeSelector != nil {
		listOpts = append(listOpts, client.MatchingLabels(talosPlan.Spec.NodeSelector))
		logger.Info("Using node selector", "selector", talosPlan.Spec.NodeSelector)
	} else {
		logger.Info("No node selector specified, selecting all nodes")
	}

	if err := r.List(ctx, nodeList, listOpts...); err != nil {
		logger.Error(err, "Failed to list nodes", "talosplan", talosPlan.Name)
		return nil, err
	}

	nodeNames := make([]string, len(nodeList.Items))
	for i, node := range nodeList.Items {
		nodeNames[i] = node.Name
	}

	logger.Info("Found target nodes",
		"talosplan", talosPlan.Name,
		"count", len(nodeList.Items),
		"nodes", nodeNames)

	return nodeList.Items, nil
}

func (r *TalosPlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := ctrl.Log.WithName("setup")
	logger.Info("Setting up TalosPlan controller with manager")

	return ctrl.NewControllerManagedBy(mgr).
		For(&upgradev1alpha1.TalosPlan{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
