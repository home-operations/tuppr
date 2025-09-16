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
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/distribution/reference"
	talosclient "github.com/siderolabs/talos/pkg/machinery/client"
	talosclientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/siderolabs/talos/pkg/machinery/resources/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	upgradev1alpha1 "github.com/home-operations/talup/api/v1alpha1"
)

const (
	PhasePending    = "Pending"
	PhaseInProgress = "InProgress"
	PhaseCompleted  = "Completed"
	PhaseFailed     = "Failed"

	TalosPlanFinalizer    = "upgrade.home-operations.com/talos-finalizer"
	SchematicAnnotation   = "extensions.talos.dev/schematic"
	TalosConfigSecretName = "talup"
	TalosConfigSecretKey  = "talosconfig"
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

	// Fetch the TalosPlan instance
	var talosPlan upgradev1alpha1.TalosPlan
	if err := r.Get(ctx, req.NamespacedName, &talosPlan); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get TalosPlan")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if talosPlan.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, &talosPlan)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&talosPlan, TalosPlanFinalizer) {
		controllerutil.AddFinalizer(&talosPlan, TalosPlanFinalizer)
		return ctrl.Result{}, r.Update(ctx, &talosPlan)
	}

	// Check if upgrade is needed
	upgradeNeeded, err := r.isUpgradeNeeded(ctx, &talosPlan)
	if err != nil {
		logger.Error(err, "Failed to check if upgrade is needed")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if !upgradeNeeded {
		if talosPlan.Status.Phase != PhaseCompleted {
			talosPlan.Status.Phase = PhaseCompleted
			talosPlan.Status.Message = "All nodes are already at target version"
			talosPlan.Status.LastUpdated = metav1.Now()
			talosPlan.Status.ObservedGeneration = talosPlan.Generation
			return ctrl.Result{}, r.Status().Update(ctx, &talosPlan)
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Initialize status if needed
	if talosPlan.Status.Phase == "" {
		talosPlan.Status.Phase = PhasePending
		talosPlan.Status.LastUpdated = metav1.Now()
		talosPlan.Status.ObservedGeneration = talosPlan.Generation
		return ctrl.Result{}, r.Status().Update(ctx, &talosPlan)
	}

	// Handle upgrade phases
	switch talosPlan.Status.Phase {
	case PhasePending:
		return r.handlePendingPhase(ctx, &talosPlan)
	case PhaseInProgress:
		return r.handleInProgressPhase(ctx, &talosPlan)
	case PhaseFailed:
		return r.handleFailedPhase(ctx, &talosPlan)
	case PhaseCompleted:
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	return ctrl.Result{}, nil
}

func (r *TalosPlanReconciler) handleDeletion(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Clean up any running jobs
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList, client.InNamespace(talosPlan.Namespace), client.MatchingLabels{
		"app.kubernetes.io/name":       "talos-upgrade",
		"app.kubernetes.io/instance":   talosPlan.Name,
		"app.kubernetes.io/managed-by": "talup",
	}); err != nil {
		logger.Error(err, "Failed to list jobs for cleanup")
		return ctrl.Result{}, err
	}

	for _, job := range jobList.Items {
		if err := r.Delete(ctx, &job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			logger.Error(err, "Failed to delete job", "job", job.Name)
			return ctrl.Result{}, err
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(talosPlan, TalosPlanFinalizer)
	return ctrl.Result{}, r.Update(ctx, talosPlan)
}

func (r *TalosPlanReconciler) isUpgradeNeeded(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (bool, error) {
	logger := log.FromContext(ctx)

	// Get all nodes that match the selector
	nodes, err := r.getTargetNodes(ctx, talosPlan)
	if err != nil {
		logger.Error(err, "Failed to get target nodes")
		return false, err
	}

	targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag)

	for _, node := range nodes {
		// Check if node needs upgrade using Talos SDK
		needsUpgrade, err := r.nodeNeedsUpgrade(ctx, talosPlan, &node, targetImage)
		if err != nil {
			logger.Error(err, "Failed to check if node needs upgrade", "node", node.Name)
			continue // Skip this node and check others
		}
		if needsUpgrade {
			return true, nil
		}
	}

	return false, nil
}

func (r *TalosPlanReconciler) nodeNeedsUpgrade(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, node *corev1.Node, targetImage string) (bool, error) {
	logger := log.FromContext(ctx)

	// Get Talos client
	talosClient, err := r.getTalosClient(ctx, talosPlan.Namespace)
	if err != nil {
		return false, fmt.Errorf("failed to get Talos client: %w", err)
	}
	defer talosClient.Close()

	// Get node IP address
	nodeIP, err := r.getNodeInternalIP(node)
	if err != nil {
		return false, fmt.Errorf("failed to get node IP: %w", err)
	}

	// Set the context to target this specific node
	nodeCtx := talosclient.WithNodes(ctx, nodeIP)

	// Get version information from the node
	versionResp, err := talosClient.Version(nodeCtx)
	if err != nil {
		return false, fmt.Errorf("failed to get version from node %s: %w", node.Name, err)
	}

	if len(versionResp.Messages) == 0 {
		return false, fmt.Errorf("no version response from node %s", node.Name)
	}

	currentVersion := versionResp.Messages[0].Version.Tag

	// Get machine config to extract current schematic
	machineConfig, err := r.getMachineConfig(nodeCtx, talosClient)
	if err != nil {
		return false, fmt.Errorf("failed to get machine config from node %s: %w", node.Name, err)
	}

	currentInstallImage := machineConfig.Config().Machine().Install().Image()
	currentSchematic, err := r.extractSchematicFromMachineConfig(currentInstallImage)
	if err != nil {
		logger.V(1).Info("Could not extract schematic from machine config", "image", currentInstallImage, "error", err)
		currentSchematic = ""
	}

	// Get node schematic annotation
	nodeSchematic := r.getSchematicFromNode(node)

	// Extract target version and schematic from target image
	targetVersion, targetSchematic := r.extractVersionAndSchematic(targetImage)
	if targetVersion == "" {
		logger.Info("Could not extract target version from image, assuming upgrade needed", "image", targetImage)
		return true, nil
	}

	logger.V(1).Info("Comparing upgrade status",
		"node", node.Name,
		"currentVersion", currentVersion,
		"targetVersion", targetVersion,
		"currentSchematic", currentSchematic,
		"nodeSchematic", nodeSchematic,
		"targetSchematic", targetSchematic,
	)

	// Need upgrade if:
	// 1. Versions don't match, OR
	// 2. Schematics don't match (comparing machine config schematic with node annotation)
	needsUpgrade := currentVersion != targetVersion ||
		(targetSchematic != "" && nodeSchematic != currentSchematic)

	return needsUpgrade, nil
}

func (r *TalosPlanReconciler) getMachineConfig(ctx context.Context, talosClient *talosclient.Client) (*config.MachineConfig, error) {
	// Get machine config using COSI client
	res, err := talosClient.COSI.Get(ctx, resource.NewMetadata(
		"config",
		"MachineConfigs.config.talos.dev",
		"v1alpha1",
		resource.VersionUndefined,
	))
	if err != nil {
		return nil, fmt.Errorf("failed to get machine config: %w", err)
	}

	machineConfig, ok := res.(*config.MachineConfig)
	if !ok {
		return nil, fmt.Errorf("unexpected resource type for machine config")
	}

	return machineConfig, nil
}

func (r *TalosPlanReconciler) extractSchematicFromMachineConfig(installImage string) (string, error) {
	// Parse the install image reference to extract schematic
	ref, err := reference.ParseAnyReference(installImage)
	if err != nil {
		return "", fmt.Errorf("failed to parse install image reference: %w", err)
	}

	namedRef, ok := ref.(reference.Named)
	if !ok {
		return "", fmt.Errorf("not a named reference")
	}

	// Extract schematic from the image name path
	imageName := namedRef.Name()
	if strings.Contains(imageName, "factory.talos.dev") {
		// For factory images, schematic is the last part of the path
		parts := strings.Split(imageName, "/")
		if len(parts) >= 3 {
			return parts[len(parts)-1], nil
		}
	}

	return "", fmt.Errorf("no schematic found in image: %s", installImage)
}

func (r *TalosPlanReconciler) getTalosClient(ctx context.Context, namespace string) (*talosclient.Client, error) {
	// Get the talosconfig secret
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      TalosConfigSecretName,
		Namespace: namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get talosconfig secret: %w", err)
	}

	// Get the config data
	configData, exists := secret.Data[TalosConfigSecretKey]
	if !exists {
		return nil, fmt.Errorf("talosconfig key not found in secret")
	}

	// Parse the config
	config, err := talosclientconfig.FromBytes(configData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse talosconfig: %w", err)
	}

	// Create client options
	opts := []talosclient.OptionFunc{
		talosclient.WithConfig(config),
		talosclient.WithGRPCDialOptions(
			grpc.WithTransportCredentials(
				credentials.NewTLS(&tls.Config{
					InsecureSkipVerify: false,
				}),
			),
		),
	}

	// Create and return the client
	return talosclient.New(ctx, opts...)
}

func (r *TalosPlanReconciler) getNodeInternalIP(node *corev1.Node) (string, error) {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address, nil
		}
	}
	return "", fmt.Errorf("no internal IP found for node %s", node.Name)
}

func (r *TalosPlanReconciler) extractVersionAndSchematic(image string) (version, schematic string) {
	// Parse image like: factory.talos.dev/metal-installer/05b4a47a70bc97786ed83d200567dcc8a13f731b164537ba59d5397d668851fa:v1.11.1
	// or ghcr.io/siderolabs/installer:v1.11.1

	parts := strings.Split(image, ":")
	if len(parts) < 2 {
		return "", ""
	}

	version = parts[len(parts)-1] // Get the tag (version)

	// Check if this is a factory image (contains schematic)
	if strings.Contains(image, "factory.talos.dev") {
		pathParts := strings.Split(parts[0], "/")
		if len(pathParts) >= 3 {
			schematic = pathParts[len(pathParts)-1] // Last part before the tag
		}
	}

	return version, schematic
}

func (r *TalosPlanReconciler) getSchematicFromNode(node *corev1.Node) string {
	if node.Annotations == nil {
		return ""
	}
	return node.Annotations[SchematicAnnotation]
}

func (r *TalosPlanReconciler) verifyNodeUpgrade(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, nodeName string) bool {
	// Get the node and check if it has the expected version using Talos SDK
	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return false
	}

	targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag)
	needsUpgrade, err := r.nodeNeedsUpgrade(ctx, talosPlan, node, targetImage)
	if err != nil {
		// If we can't verify, assume it failed
		return false
	}

	// If it doesn't need upgrade, then the upgrade was successful
	return !needsUpgrade
}

func (r *TalosPlanReconciler) handlePendingPhase(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get nodes that need upgrade
	nodes, err := r.getTargetNodes(ctx, talosPlan)
	if err != nil {
		logger.Error(err, "Failed to get target nodes")
		return ctrl.Result{}, err
	}

	targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag)

	// Find first node that needs upgrade and isn't completed or failed
	var targetNode *corev1.Node
	for _, node := range nodes {
		needsUpgrade, err := r.nodeNeedsUpgrade(ctx, talosPlan, &node, targetImage)
		if err != nil {
			logger.Error(err, "Failed to check if node needs upgrade", "node", node.Name)
			continue
		}

		if needsUpgrade &&
			!r.isNodeInList(node.Name, talosPlan.Status.CompletedNodes) &&
			!r.isNodeInFailedList(node.Name, talosPlan.Status.FailedNodes) {
			targetNode = &node
			break
		}
	}

	if targetNode == nil {
		// All nodes are upgraded
		talosPlan.Status.Phase = PhaseCompleted
		talosPlan.Status.Message = "All nodes successfully upgraded"
		talosPlan.Status.CurrentNode = ""
		talosPlan.Status.LastUpdated = metav1.Now()
		return ctrl.Result{}, r.Status().Update(ctx, talosPlan)
	}

	// Create upgrade job for the target node
	job, err := r.createUpgradeJob(ctx, talosPlan, targetNode.Name)
	if err != nil {
		logger.Error(err, "Failed to create upgrade job", "node", targetNode.Name)
		return ctrl.Result{}, err
	}

	// Update status
	talosPlan.Status.Phase = PhaseInProgress
	talosPlan.Status.CurrentNode = targetNode.Name
	talosPlan.Status.Message = fmt.Sprintf("Upgrading node %s", targetNode.Name)
	talosPlan.Status.LastUpdated = metav1.Now()

	logger.Info("Created upgrade job", "job", job.Name, "node", targetNode.Name)
	return ctrl.Result{RequeueAfter: time.Second * 30}, r.Status().Update(ctx, talosPlan)
}

func (r *TalosPlanReconciler) handleInProgressPhase(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if talosPlan.Status.CurrentNode == "" {
		// No current node, move to pending to start next upgrade
		talosPlan.Status.Phase = PhasePending
		return ctrl.Result{}, r.Status().Update(ctx, talosPlan)
	}

	// Check job status
	jobName := fmt.Sprintf("talos-upgrade-%s-%s", talosPlan.Name, talosPlan.Status.CurrentNode)
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      jobName,
		Namespace: talosPlan.Namespace,
	}, job)

	if err != nil {
		if errors.IsNotFound(err) {
			// Job doesn't exist, move to failed
			return r.markNodeFailed(ctx, talosPlan, talosPlan.Status.CurrentNode, "Job not found")
		}
		logger.Error(err, "Failed to get upgrade job")
		return ctrl.Result{}, err
	}

	// Check job conditions
	if job.Status.Succeeded > 0 {
		// Job succeeded, verify node upgrade using Talos SDK
		if r.verifyNodeUpgrade(ctx, talosPlan, talosPlan.Status.CurrentNode) {
			return r.markNodeCompleted(ctx, talosPlan, talosPlan.Status.CurrentNode)
		} else {
			return r.markNodeFailed(ctx, talosPlan, talosPlan.Status.CurrentNode, "Node upgrade verification failed")
		}
	}

	if job.Status.Failed > 0 {
		// Job failed
		return r.markNodeFailed(ctx, talosPlan, talosPlan.Status.CurrentNode, "Upgrade job failed")
	}

	// Check for timeout
	timeout, _ := time.ParseDuration(talosPlan.Spec.Timeout)
	if timeout == 0 {
		timeout = 30 * time.Minute
	}

	if time.Since(job.CreationTimestamp.Time) > timeout {
		return r.markNodeFailed(ctx, talosPlan, talosPlan.Status.CurrentNode, "Upgrade timeout")
	}

	// Job still running
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

func (r *TalosPlanReconciler) handleFailedPhase(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) (ctrl.Result, error) {
	// In failed phase, manual intervention is needed
	// Update message and wait
	if talosPlan.Status.Message != "Manual intervention required - upgrade failed after max retries" {
		talosPlan.Status.Message = "Manual intervention required - upgrade failed after max retries"
		talosPlan.Status.LastUpdated = metav1.Now()
		return ctrl.Result{}, r.Status().Update(ctx, talosPlan)
	}

	return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}

func (r *TalosPlanReconciler) markNodeCompleted(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, nodeName string) (ctrl.Result, error) {
	talosPlan.Status.CompletedNodes = append(talosPlan.Status.CompletedNodes, nodeName)
	talosPlan.Status.CurrentNode = ""
	talosPlan.Status.Phase = PhasePending // Move to pending to process next node
	talosPlan.Status.Message = fmt.Sprintf("Node %s upgraded successfully", nodeName)
	talosPlan.Status.LastUpdated = metav1.Now()

	return ctrl.Result{}, r.Status().Update(ctx, talosPlan)
}

func (r *TalosPlanReconciler) markNodeFailed(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, nodeName, reason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Find existing failed node entry or create new one
	var nodeStatus *upgradev1alpha1.NodeUpgradeStatus
	for i := range talosPlan.Status.FailedNodes {
		if talosPlan.Status.FailedNodes[i].NodeName == nodeName {
			nodeStatus = &talosPlan.Status.FailedNodes[i]
			break
		}
	}

	if nodeStatus == nil {
		talosPlan.Status.FailedNodes = append(talosPlan.Status.FailedNodes, upgradev1alpha1.NodeUpgradeStatus{
			NodeName:  nodeName,
			Retries:   1,
			LastError: reason,
		})
		nodeStatus = &talosPlan.Status.FailedNodes[len(talosPlan.Status.FailedNodes)-1]
	} else {
		nodeStatus.Retries++
		nodeStatus.LastError = reason
	}

	maxRetries := talosPlan.Spec.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	if nodeStatus.Retries >= maxRetries {
		// Max retries reached, mark as failed
		talosPlan.Status.Phase = PhaseFailed
		talosPlan.Status.CurrentNode = ""
		talosPlan.Status.Message = fmt.Sprintf("Node %s failed after %d retries: %s", nodeName, nodeStatus.Retries, reason)
		logger.Info("Node upgrade failed after max retries", "node", nodeName, "retries", nodeStatus.Retries)
	} else {
		// Retry
		talosPlan.Status.Phase = PhasePending
		talosPlan.Status.CurrentNode = ""
		talosPlan.Status.Message = fmt.Sprintf("Node %s failed (retry %d/%d): %s", nodeName, nodeStatus.Retries, maxRetries, reason)
		logger.Info("Node upgrade failed, will retry", "node", nodeName, "retries", nodeStatus.Retries)
	}

	talosPlan.Status.LastUpdated = metav1.Now()
	return ctrl.Result{RequeueAfter: time.Second * 30}, r.Status().Update(ctx, talosPlan)
}

func (r *TalosPlanReconciler) createUpgradeJob(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, nodeName string) (*batchv1.Job, error) {
	// Find a node different from the target node to run the job
	executorNode, err := r.findExecutorNode(ctx, talosPlan, nodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to find executor node: %w", err)
	}

	// Get target node IP for the talosctl command
	targetNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, targetNode); err != nil {
		return nil, fmt.Errorf("failed to get target node: %w", err)
	}

	targetNodeIP, err := r.getNodeInternalIP(targetNode)
	if err != nil {
		return nil, fmt.Errorf("failed to get target node IP: %w", err)
	}

	jobName := fmt.Sprintf("talos-upgrade-%s-%s", talosPlan.Name, nodeName)
	talosctlImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Talosctl.Image.Repository, talosPlan.Spec.Talosctl.Image.Tag)
	targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag)

	timeout, _ := time.ParseDuration(talosPlan.Spec.Timeout)
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	ttlSeconds := int32(timeout.Seconds())

	// Build talosctl upgrade command args similar to your script
	args := []string{
		"upgrade",
		"--nodes", targetNodeIP, // Use IP instead of hostname
		"--image", targetImage,
		"--preserve", "true", // Always preserve data
		"--debug",
	}

	if talosPlan.Spec.Timeout != "" {
		args = append(args, "--timeout", talosPlan.Spec.Timeout)
	}

	if talosPlan.Spec.Force {
		args = append(args, "--force")
	}

	// Add reboot mode
	rebootMode := "default"
	if talosPlan.Spec.RebootMode != "" {
		rebootMode = talosPlan.Spec.RebootMode
	}
	if rebootMode == "powercycle" {
		args = append(args, "--reboot-mode", "powercycle")
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: talosPlan.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "talos-upgrade",
				"app.kubernetes.io/instance":   talosPlan.Name,
				"app.kubernetes.io/managed-by": "talup",
				"talup.io/target-node":         nodeName,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttlSeconds,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": executorNode,
					},
					Containers: []corev1.Container{
						{
							Name:  "talosctl",
							Image: talosctlImage,
							Args:  args,
							Env: []corev1.EnvVar{
								{
									Name:  "TALOSCONFIG",
									Value: "/var/run/secrets/talos.dev/talosconfig",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "talup",
									MountPath: "/var/run/secrets/talos.dev",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "talup",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: TalosConfigSecretName,
								},
							},
						},
					},
				},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(talosPlan, job, r.Scheme); err != nil {
		return nil, err
	}

	if err := r.Create(ctx, job); err != nil {
		return nil, err
	}

	return job, nil
}

func (r *TalosPlanReconciler) findExecutorNode(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, targetNode string) (string, error) {
	nodes, err := r.getTargetNodes(ctx, talosPlan)
	if err != nil {
		return "", err
	}

	// Find a node that's not the target node and is ready
	for _, node := range nodes {
		if node.Name != targetNode && r.isNodeReady(&node) {
			return node.Name, nil
		}
	}

	return "", fmt.Errorf("no suitable executor node found")
}

func (r *TalosPlanReconciler) isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (r *TalosPlanReconciler) getTargetNodes(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	listOpts := []client.ListOption{}

	if talosPlan.Spec.NodeSelector != nil {
		listOpts = append(listOpts, client.MatchingLabels(talosPlan.Spec.NodeSelector))
	}

	if err := r.List(ctx, nodeList, listOpts...); err != nil {
		return nil, err
	}

	return nodeList.Items, nil
}

func (r *TalosPlanReconciler) isNodeInList(nodeName string, nodeList []string) bool {
	for _, name := range nodeList {
		if name == nodeName {
			return true
		}
	}
	return false
}

func (r *TalosPlanReconciler) isNodeInFailedList(nodeName string, failedNodes []upgradev1alpha1.NodeUpgradeStatus) bool {
	for _, node := range failedNodes {
		if node.NodeName == nodeName {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *TalosPlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&upgradev1alpha1.TalosPlan{}).
		Owns(&batchv1.Job{}).
		Named("talosupgrade").
		Complete(r)
}
