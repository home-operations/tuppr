package controller

import (
	"context"
	"encoding/json"
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
	TalosUpgradeFinalizer = "talup.home-operations.com/talos-finalizer"
)

//+kubebuilder:rbac:groups=talup.home-operations.com,resources=talosupgrades,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=talup.home-operations.com,resources=talosupgrades/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=talup.home-operations.com,resources=talosupgrades/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// TalosUpgradeReconciler reconciles a TalosUpgrade object
type TalosUpgradeReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	TalosConfigSecret string
}

func (r *TalosUpgradeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting reconciliation", "talosupgrade", req.NamespacedName)

	var talosPlan upgradev1alpha1.TalosUpgrade
	if err := r.Get(ctx, req.NamespacedName, &talosPlan); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("TalosUpgrade resource not found, likely deleted", "talosupgrade", req.NamespacedName)
		} else {
			logger.Error(err, "Failed to get TalosUpgrade resource", "talosupgrade", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Retrieved TalosUpgrade",
		"name", talosPlan.Name,
		"namespace", talosPlan.Namespace,
		"phase", talosPlan.Status.Phase,
		"generation", talosPlan.Generation,
		"observedGeneration", talosPlan.Status.ObservedGeneration)

	// Handle deletion
	if talosPlan.DeletionTimestamp != nil {
		logger.Info("TalosUpgrade is being deleted, starting cleanup", "name", talosPlan.Name)
		return r.cleanup(ctx, &talosPlan)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&talosPlan, TalosUpgradeFinalizer) {
		logger.Info("Adding finalizer to TalosUpgrade", "name", talosPlan.Name, "finalizer", TalosUpgradeFinalizer)
		controllerutil.AddFinalizer(&talosPlan, TalosUpgradeFinalizer)
		return ctrl.Result{}, r.Update(ctx, &talosPlan)
	}

	// Simple state machine
	logger.Info("Processing upgrade for TalosUpgrade", "name", talosPlan.Name, "currentPhase", talosPlan.Status.Phase)
	return r.processUpgrade(ctx, &talosPlan)
}

func (r *TalosUpgradeReconciler) processUpgrade(ctx context.Context, talosPlan *upgradev1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"talosupgrade", talosPlan.Name,
		"namespace", talosPlan.Namespace,
		"generation", talosPlan.Generation,
	)

	logger.Info("Starting upgrade processing")

	namespacedName := client.ObjectKeyFromObject(talosPlan)

	if len(talosPlan.Status.FailedNodes) > 0 {
		logger.Info("Upgrade plan has failed nodes, blocking further progress",
			"talosupgrade", talosPlan.Name,
			"failedNodes", len(talosPlan.Status.FailedNodes))

		if err := r.updatePhase(ctx, namespacedName, PhaseFailed, "",
			fmt.Sprintf("Upgrade stopped due to %d failed nodes", len(talosPlan.Status.FailedNodes))); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	activeJob, activeNode, err := r.findActiveJob(ctx, talosPlan)
	if err != nil {
		logger.Error(err, "Failed to find active jobs", "talosupgrade", talosPlan.Name)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if activeJob != nil {
		logger.Info("Found active job, monitoring status",
			"job", activeJob.Name,
			"node", activeNode,
			"active", activeJob.Status.Active,
			"succeeded", activeJob.Status.Succeeded,
			"failed", activeJob.Status.Failed)

		return r.handleJobStatus(ctx, talosPlan, activeNode, activeJob)
	}

	nextNode, err := r.findNextNode(ctx, talosPlan)
	if err != nil {
		logger.Error(err, "Failed to find next node to upgrade", "talosupgrade", talosPlan.Name)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if nextNode == "" {
		logger.Info("No more nodes to upgrade, marking plan as completed",
			"talosupgrade", talosPlan.Name,
			"completedNodes", talosPlan.Status.CompletedNodes,
			"failedNodes", len(talosPlan.Status.FailedNodes))

		if err := r.updatePhase(ctx, namespacedName, PhaseCompleted, "", "All nodes upgraded successfully"); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	logger.Info("Found next node to upgrade", "talosupgrade", talosPlan.Name, "node", nextNode)

	logger.Info("Creating new upgrade job", "node", nextNode)
	if _, err := r.createJob(ctx, talosPlan, nextNode); err != nil {
		logger.Error(err, "Failed to create upgrade job", "node", nextNode)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	logger.Info("Successfully created upgrade job", "node", nextNode)
	if err := r.updatePhase(ctx, namespacedName, PhaseInProgress, nextNode, fmt.Sprintf("Upgrading node %s", nextNode)); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// findActiveJob looks for any currently running job for this TalosUpgrade
func (r *TalosUpgradeReconciler) findActiveJob(ctx context.Context, talosPlan *upgradev1alpha1.TalosUpgrade) (*batchv1.Job, string, error) {
	logger := log.FromContext(ctx)

	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(talosPlan.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     "talup-talos",
			"app.kubernetes.io/instance": talosPlan.Name,
		}); err != nil {
		return nil, "", err
	}

	logger.Info("Found jobs for TalosUpgrade", "count", len(jobList.Items))

	for _, job := range jobList.Items {
		nodeName := job.Labels["talup.home-operations.com/target-node"]

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

		// Return the FIRST job we find that's not completed/failed
		// This ensures only ONE job is processed at a time
		logger.Info("Found job that needs handling",
			"job", job.Name,
			"node", nodeName,
			"active", job.Status.Active,
			"succeeded", job.Status.Succeeded,
			"failed", job.Status.Failed)
		return &job, nodeName, nil
	}

	return nil, "", nil
}

func (r *TalosUpgradeReconciler) findNextNode(ctx context.Context, talosPlan *upgradev1alpha1.TalosUpgrade) (string, error) {
	logger := log.FromContext(ctx)
	logger.Info("Finding next node to upgrade", "talosupgrade", talosPlan.Name)

	nodes, err := r.getTargetNodes(ctx, talosPlan)
	if err != nil {
		logger.Error(err, "Failed to get target nodes")
		return "", err
	}

	logger.Info("Retrieved target nodes", "count", len(nodes))

	targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Target.Image.Repository, talosPlan.Spec.Target.Image.Tag)
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

func (r *TalosUpgradeReconciler) nodeNeedsUpgrade(ctx context.Context, talosPlan *upgradev1alpha1.TalosUpgrade, node *corev1.Node, targetImage string) (bool, error) {
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

func (r *TalosUpgradeReconciler) createJob(ctx context.Context, talosPlan *upgradev1alpha1.TalosUpgrade, nodeName string) (*batchv1.Job, error) {
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

func (r *TalosUpgradeReconciler) buildJob(talosPlan *upgradev1alpha1.TalosUpgrade, nodeName, nodeIP string) *batchv1.Job {
	logger := log.FromContext(context.Background())

	labels := map[string]string{
		"app.kubernetes.io/name":                "talup-talos",
		"app.kubernetes.io/instance":            talosPlan.Name,
		"talup.home-operations.com/target-node": nodeName,
	}

	// Get talosctl image with defaults
	talosctlRepo := "ghcr.io/siderolabs/talosctl"
	if talosPlan.Spec.Talosctl.Image.Repository != "" {
		talosctlRepo = talosPlan.Spec.Talosctl.Image.Repository
	}

	talosctlTag := talosPlan.Spec.Talosctl.Image.Tag
	if talosctlTag == "" {
		talosctlTag = r.getTalosctlTagFromNode(context.Background(), nodeName)
		if talosctlTag == "" {
			logger.Info("Could not determine talosctl version from node, using latest", "node", nodeName)
			talosctlTag = "latest"
		} else {
			logger.Info("Using talosctl version from node osImage", "node", nodeName, "version", talosctlTag)
		}
	}

	talosctlImage := talosctlRepo + ":" + talosctlTag

	args := []string{
		"upgrade",
		"--nodes", nodeIP,
		"--image", fmt.Sprintf("%s:%s", talosPlan.Spec.Target.Image.Repository, talosPlan.Spec.Target.Image.Tag),
		"--timeout=15m",
	}

	if talosPlan.Spec.Target.Options.Debug {
		args = append(args, "--debug")
		logger.Info("Debug upgrade enabled", "node", nodeName)
	}

	if talosPlan.Spec.Target.Options.Force {
		args = append(args, "--force")
		logger.Info("Force upgrade enabled", "node", nodeName)
	}
	if talosPlan.Spec.Target.Options.RebootMode == "powercycle" {
		args = append(args, "--reboot-mode", "powercycle")
		logger.Info("Powercycle reboot mode enabled", "node", nodeName)
	}

	pullPolicy := corev1.PullIfNotPresent
	if talosPlan.Spec.Talosctl.Image.PullPolicy != "" {
		pullPolicy = talosPlan.Spec.Talosctl.Image.PullPolicy
	}

	logger.Info("Building job specification",
		"node", nodeName,
		"talosctlImage", talosctlImage,
		"pullPolicy", pullPolicy,
		"args", args)

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("talup-talos-%s-%s", talosPlan.Name, nodeName),
			Namespace: talosPlan.Namespace,
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
							Name:      "talos",
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

func (r *TalosUpgradeReconciler) handleJobStatus(ctx context.Context, talosPlan *upgradev1alpha1.TalosUpgrade, nodeName string, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	namespacedName := client.ObjectKeyFromObject(talosPlan)

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

		targetNode := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, targetNode); err != nil {
			logger.Error(err, "Failed to get node for verification", "node", nodeName)
			// Treat verification failure as upgrade failure
			nodeStatus := upgradev1alpha1.NodeUpgradeStatus{
				NodeName:  nodeName,
				LastError: fmt.Sprintf("Verification failed: unable to get node: %v", err),
			}
			if err := r.addFailedNode(ctx, namespacedName, nodeStatus); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.updatePhase(ctx, namespacedName, PhaseFailed, "",
				fmt.Sprintf("Node %s upgrade verification failed", nodeName)); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
		}

		targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Target.Image.Repository, talosPlan.Spec.Target.Image.Tag)
		if needsUpgrade, err := r.nodeNeedsUpgrade(ctx, talosPlan, targetNode, targetImage); err != nil {
			logger.Error(err, "Failed to check if node needs upgrade", "node", nodeName)
			nodeStatus := upgradev1alpha1.NodeUpgradeStatus{
				NodeName:  nodeName,
				LastError: fmt.Sprintf("Verification failed: %v", err),
			}
			if err := r.addFailedNode(ctx, namespacedName, nodeStatus); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.updatePhase(ctx, namespacedName, PhaseFailed, "",
				fmt.Sprintf("Node %s upgrade verification failed", nodeName)); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
		} else if needsUpgrade {
			logger.Error(nil, "Job succeeded but node still needs upgrade", "node", nodeName)
			nodeStatus := upgradev1alpha1.NodeUpgradeStatus{
				NodeName:  nodeName,
				LastError: "Job succeeded but node not upgraded",
			}
			if err := r.addFailedNode(ctx, namespacedName, nodeStatus); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.updatePhase(ctx, namespacedName, PhaseFailed, "",
				fmt.Sprintf("Node %s upgrade verification failed", nodeName)); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
		}

		if err := r.addCompletedNode(ctx, namespacedName, nodeName); err != nil {
			logger.Error(err, "Failed to add completed node", "node", nodeName)
			return ctrl.Result{}, err
		}

		if err := r.updatePhase(ctx, namespacedName, PhasePending, "", fmt.Sprintf("Node %s upgraded successfully", nodeName)); err != nil {
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
		if err := r.addFailedNode(ctx, namespacedName, nodeStatus); err != nil {
			logger.Error(err, "Failed to add failed node", "node", nodeName)
			return ctrl.Result{}, err
		}

		if err := r.updatePhase(ctx, namespacedName, PhaseFailed, "", fmt.Sprintf("Node %s upgrade failed - stopping further upgrades", nodeName)); err != nil {
			logger.Error(err, "Failed to update phase", "node", nodeName)
			return ctrl.Result{}, err
		}

		logger.Info("Successfully marked node as failed", "node", nodeName)
		return ctrl.Result{RequeueAfter: time.Minute * 10}, nil

	default:
		logger.Info("Job is still active", "job", job.Name, "node", nodeName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}
}

// Separate functions for different status updates using patches
func (r *TalosUpgradeReconciler) updatePhase(ctx context.Context, namespacedName types.NamespacedName, phase string, currentNode, message string) error {
	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"phase":       phase,
			"currentNode": currentNode,
			"message":     message,
			"lastUpdated": metav1.Now(),
		},
	}

	return r.patchStatus(ctx, namespacedName, patch)
}

func (r *TalosUpgradeReconciler) addCompletedNode(ctx context.Context, namespacedName types.NamespacedName, nodeName string) error {
	// First check if node is already completed to avoid unnecessary patch
	talosPlan := &upgradev1alpha1.TalosUpgrade{}
	if err := r.Get(ctx, namespacedName, talosPlan); err != nil {
		return err
	}

	if ContainsNode(nodeName, talosPlan.Status.CompletedNodes) {
		return nil // Already completed
	}

	newCompletedNodes := append(talosPlan.Status.CompletedNodes, nodeName)
	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"completedNodes": newCompletedNodes,
			"lastUpdated":    metav1.Now(),
		},
	}

	return r.patchStatus(ctx, namespacedName, patch)
}

func (r *TalosUpgradeReconciler) addFailedNode(ctx context.Context, namespacedName types.NamespacedName, nodeStatus upgradev1alpha1.NodeUpgradeStatus) error {
	// First check if node is already failed to avoid unnecessary patch
	talosPlan := &upgradev1alpha1.TalosUpgrade{}
	if err := r.Get(ctx, namespacedName, talosPlan); err != nil {
		return err
	}

	if ContainsFailedNode(nodeStatus.NodeName, talosPlan.Status.FailedNodes) {
		return nil // Already failed
	}

	newFailedNodes := append(talosPlan.Status.FailedNodes, nodeStatus)
	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"failedNodes": newFailedNodes,
			"lastUpdated": metav1.Now(),
		},
	}

	return r.patchStatus(ctx, namespacedName, patch)
}

// Helper function to apply status patches
func (r *TalosUpgradeReconciler) patchStatus(ctx context.Context, namespacedName types.NamespacedName, patch map[string]interface{}) error {
	logger := log.FromContext(ctx)

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	talosPlan := &upgradev1alpha1.TalosUpgrade{}
	talosPlan.Name = namespacedName.Name
	talosPlan.Namespace = namespacedName.Namespace

	logger.V(1).Info("Applying status patch", "patch", string(patchBytes))

	return r.Status().Patch(ctx, talosPlan, client.RawPatch(types.MergePatchType, patchBytes))
}

func (r *TalosUpgradeReconciler) cleanup(ctx context.Context, talosPlan *upgradev1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Cleaning up TalosUpgrade", "name", talosPlan.Name)

	// Jobs will be cleaned up automatically via owner references
	logger.Info("Removing finalizer", "name", talosPlan.Name, "finalizer", TalosUpgradeFinalizer)
	controllerutil.RemoveFinalizer(talosPlan, TalosUpgradeFinalizer)

	if err := r.Update(ctx, talosPlan); err != nil {
		logger.Error(err, "Failed to remove finalizer", "name", talosPlan.Name)
		return ctrl.Result{}, err
	}

	logger.Info("Successfully cleaned up TalosUpgrade", "name", talosPlan.Name)
	return ctrl.Result{}, nil
}

func (r *TalosUpgradeReconciler) getTargetNodes(ctx context.Context, talosPlan *upgradev1alpha1.TalosUpgrade) ([]corev1.Node, error) {
	logger := log.FromContext(ctx)
	logger.Info("Getting target nodes", "talosupgrade", talosPlan.Name, "nodeSelector", talosPlan.Spec.Target.NodeSelector)

	nodeList := &corev1.NodeList{}
	listOpts := []client.ListOption{}

	if talosPlan.Spec.Target.NodeSelector != nil {
		listOpts = append(listOpts, client.MatchingLabels(talosPlan.Spec.Target.NodeSelector))
		logger.Info("Using node selector", "selector", talosPlan.Spec.Target.NodeSelector)
	} else {
		logger.Info("No node selector specified, selecting all nodes")
	}

	if err := r.List(ctx, nodeList, listOpts...); err != nil {
		logger.Error(err, "Failed to list nodes", "talosupgrade", talosPlan.Name)
		return nil, err
	}

	nodeNames := make([]string, len(nodeList.Items))
	for i, node := range nodeList.Items {
		nodeNames[i] = node.Name
	}

	logger.Info("Found target nodes",
		"talosupgrade", talosPlan.Name,
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

	logger.Info("Extracted version from node osImage",
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
