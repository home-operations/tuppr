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

	talosclientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"

	upgradev1alpha1 "github.com/home-operations/talup/api/v1alpha1"
)

const (
	KubernetesPlanFinalizer = "upgrade.home-operations.com/kubernetes-finalizer"
)

// KubernetesPlanReconciler reconciles a KubernetesPlan object
type KubernetesPlanReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=upgrade.home-operations.com,resources=kubernetesplans,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=upgrade.home-operations.com,resources=kubernetesplans/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=upgrade.home-operations.com,resources=kubernetesplans/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *KubernetesPlanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the KubernetesPlan instance
	var kubernetesPlan upgradev1alpha1.KubernetesPlan
	if err := r.Get(ctx, req.NamespacedName, &kubernetesPlan); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get KubernetesPlan")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if kubernetesPlan.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, &kubernetesPlan)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&kubernetesPlan, KubernetesPlanFinalizer) {
		controllerutil.AddFinalizer(&kubernetesPlan, KubernetesPlanFinalizer)
		return ctrl.Result{}, r.Update(ctx, &kubernetesPlan)
	}

	// Validate prerequisites
	if err := r.validatePrerequisites(ctx, &kubernetesPlan); err != nil {
		logger.Error(err, "Prerequisites validation failed")

		kubernetesPlan.Status.Phase = PhaseFailed
		kubernetesPlan.Status.Message = fmt.Sprintf("Prerequisites validation failed: %v", err)
		kubernetesPlan.Status.LastUpdated = metav1.Now()

		if statusErr := r.Status().Update(ctx, &kubernetesPlan); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after validation failure")
		}

		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Check if upgrade is needed
	upgradeNeeded, err := r.isUpgradeNeeded(ctx, &kubernetesPlan)
	if err != nil {
		logger.Error(err, "Failed to check if upgrade is needed")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if !upgradeNeeded {
		if kubernetesPlan.Status.Phase != PhaseCompleted {
			kubernetesPlan.Status.Phase = PhaseCompleted
			kubernetesPlan.Status.Message = "Kubernetes is already at target version"
			kubernetesPlan.Status.LastUpdated = metav1.Now()
			kubernetesPlan.Status.ObservedGeneration = kubernetesPlan.Generation
			return ctrl.Result{}, r.Status().Update(ctx, &kubernetesPlan)
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Initialize status if needed
	if kubernetesPlan.Status.Phase == "" {
		kubernetesPlan.Status.Phase = PhasePending
		kubernetesPlan.Status.TargetVersion = kubernetesPlan.Spec.Version
		kubernetesPlan.Status.LastUpdated = metav1.Now()
		kubernetesPlan.Status.ObservedGeneration = kubernetesPlan.Generation
		return ctrl.Result{}, r.Status().Update(ctx, &kubernetesPlan)
	}

	// Handle upgrade phases
	switch kubernetesPlan.Status.Phase {
	case PhasePending:
		return r.handlePendingPhase(ctx, &kubernetesPlan)
	case PhaseInProgress:
		return r.handleInProgressPhase(ctx, &kubernetesPlan)
	case PhaseFailed:
		return r.handleFailedPhase(ctx, &kubernetesPlan)
	case PhaseCompleted:
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	return ctrl.Result{}, nil
}

func (r *KubernetesPlanReconciler) validatePrerequisites(ctx context.Context, kubernetesPlan *upgradev1alpha1.KubernetesPlan) error {
	// Validate Talos config secret exists and is valid
	if err := r.validateTalosConfigSecret(ctx, kubernetesPlan.Namespace); err != nil {
		return fmt.Errorf("talos config validation failed: %w", err)
	}

	// Validate TalosctlSpec
	if err := r.validateTalosctlSpec(&kubernetesPlan.Spec.Talosctl); err != nil {
		return fmt.Errorf("talosctl spec validation failed: %w", err)
	}

	// Validate that control plane nodes exist
	controlPlaneNodes, err := r.getControlPlaneNodes(ctx, kubernetesPlan)
	if err != nil {
		return fmt.Errorf("failed to get control plane nodes: %w", err)
	}

	if len(controlPlaneNodes) == 0 {
		return fmt.Errorf("no control plane nodes found")
	}

	return nil
}

func (r *KubernetesPlanReconciler) validateTalosctlSpec(talosctl *upgradev1alpha1.TalosctlSpec) error {
	if talosctl.Image.Repository == "" {
		return fmt.Errorf("talosctl image repository cannot be empty")
	}

	if talosctl.Image.Tag == "" {
		return fmt.Errorf("talosctl image tag cannot be empty")
	}

	return nil
}

func (r *KubernetesPlanReconciler) validateTalosConfigSecret(ctx context.Context, namespace string) error {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      TalosConfigSecretName,
		Namespace: namespace,
	}, secret); err != nil {
		return fmt.Errorf("talosconfig secret '%s' not found in namespace '%s': %w",
			TalosConfigSecretName, namespace, err)
	}

	configData, exists := secret.Data[TalosConfigSecretKey]
	if !exists {
		return fmt.Errorf("talosconfig secret missing required key '%s'", TalosConfigSecretKey)
	}

	if len(configData) == 0 {
		return fmt.Errorf("talosconfig secret data is empty")
	}

	// Optionally validate that the config can be parsed
	_, err := talosclientconfig.FromBytes(configData)
	if err != nil {
		return fmt.Errorf("talosconfig is invalid: %w", err)
	}

	return nil
}

func (r *KubernetesPlanReconciler) isUpgradeNeeded(ctx context.Context, kubernetesPlan *upgradev1alpha1.KubernetesPlan) (bool, error) {
	// Get current Kubernetes version from any control plane node
	controlPlaneNodes, err := r.getControlPlaneNodes(ctx, kubernetesPlan)
	if err != nil {
		return false, fmt.Errorf("failed to get control plane nodes: %w", err)
	}

	if len(controlPlaneNodes) == 0 {
		return false, fmt.Errorf("no control plane nodes found")
	}

	// Use the first control plane node to check the version
	currentVersion := controlPlaneNodes[0].Status.NodeInfo.KubeletVersion

	// Remove 'v' prefix if present for comparison
	currentVersion = strings.TrimPrefix(currentVersion, "v")
	targetVersion := strings.TrimPrefix(kubernetesPlan.Spec.Version, "v")

	// Update current version in status
	kubernetesPlan.Status.CurrentVersion = currentVersion

	return currentVersion != targetVersion, nil
}

func (r *KubernetesPlanReconciler) getControlPlaneNodes(ctx context.Context, kubernetesPlan *upgradev1alpha1.KubernetesPlan) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	listOpts := []client.ListOption{}

	// Apply node selector if specified
	if kubernetesPlan.Spec.NodeSelector != nil {
		listOpts = append(listOpts, client.MatchingLabels(kubernetesPlan.Spec.NodeSelector))
	}

	if err := r.List(ctx, nodeList, listOpts...); err != nil {
		return nil, err
	}

	var controlPlaneNodes []corev1.Node
	for _, node := range nodeList.Items {
		// Check if node is a control plane node
		if r.isControlPlaneNode(&node) && r.isNodeReady(&node) {
			controlPlaneNodes = append(controlPlaneNodes, node)
		}
	}

	return controlPlaneNodes, nil
}

func (r *KubernetesPlanReconciler) isControlPlaneNode(node *corev1.Node) bool {
	// Check for control plane labels
	labels := node.Labels
	if labels == nil {
		return false
	}

	// Common control plane labels
	controlPlaneLabels := []string{
		"node-role.kubernetes.io/control-plane",
		"node-role.kubernetes.io/master",
	}

	for _, label := range controlPlaneLabels {
		if _, exists := labels[label]; exists {
			return true
		}
	}

	return false
}

func (r *KubernetesPlanReconciler) handlePendingPhase(ctx context.Context, kubernetesPlan *upgradev1alpha1.KubernetesPlan) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get control plane nodes
	controlPlaneNodes, err := r.getControlPlaneNodes(ctx, kubernetesPlan)
	if err != nil {
		logger.Error(err, "Failed to get control plane nodes")
		return ctrl.Result{}, err
	}

	// For Kubernetes upgrades, we only need to run on one control plane node
	// Find a suitable node that hasn't been upgraded yet
	var targetNode *corev1.Node
	for _, node := range controlPlaneNodes {
		if !r.isNodeInList(node.Name, kubernetesPlan.Status.UpgradedNodes) &&
			!r.isNodeInFailedList(node.Name, kubernetesPlan.Status.FailedNodes) {
			targetNode = &node
			break
		}
	}

	if targetNode == nil {
		// All nodes processed, mark as completed
		kubernetesPlan.Status.Phase = PhaseCompleted
		kubernetesPlan.Status.Message = "Kubernetes upgrade completed successfully"
		kubernetesPlan.Status.CurrentNode = ""
		kubernetesPlan.Status.LastUpdated = metav1.Now()
		return ctrl.Result{}, r.Status().Update(ctx, kubernetesPlan)
	}

	// Create upgrade job for Kubernetes
	job, err := r.createKubernetesUpgradeJob(ctx, kubernetesPlan, targetNode.Name)
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes upgrade job", "node", targetNode.Name)
		return ctrl.Result{}, err
	}

	// Update status
	kubernetesPlan.Status.Phase = PhaseInProgress
	kubernetesPlan.Status.CurrentNode = targetNode.Name
	kubernetesPlan.Status.Message = fmt.Sprintf("Upgrading Kubernetes on control plane node %s", targetNode.Name)
	kubernetesPlan.Status.LastUpdated = metav1.Now()

	logger.Info("Created Kubernetes upgrade job", "job", job.Name, "node", targetNode.Name)
	return ctrl.Result{RequeueAfter: time.Second * 30}, r.Status().Update(ctx, kubernetesPlan)
}

func (r *KubernetesPlanReconciler) createKubernetesUpgradeJob(ctx context.Context, kubernetesPlan *upgradev1alpha1.KubernetesPlan, targetNodeName string) (*batchv1.Job, error) {
	// Find an executor node (different from target if possible)
	executorNode, err := r.findExecutorNode(ctx, kubernetesPlan, targetNodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to find executor node: %w", err)
	}

	// Get target node IP
	targetNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: targetNodeName}, targetNode); err != nil {
		return nil, fmt.Errorf("failed to get target node: %w", err)
	}

	targetNodeIP, err := r.getNodeInternalIP(targetNode)
	if err != nil {
		return nil, fmt.Errorf("failed to get target node IP: %w", err)
	}

	jobName := fmt.Sprintf("k8s-upgrade-%s-%s", kubernetesPlan.Name, targetNodeName)

	// Build talosctl image name from spec
	talosctlImage := fmt.Sprintf("%s:%s",
		kubernetesPlan.Spec.Talosctl.Image.Repository,
		kubernetesPlan.Spec.Talosctl.Image.Tag)

	timeout, _ := time.ParseDuration(kubernetesPlan.Spec.Timeout)
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	ttlSeconds := int32(timeout.Seconds())

	// Build talosctl upgrade-k8s command args
	args := []string{
		"upgrade-k8s",
		"--nodes", targetNodeIP,
		"--to", kubernetesPlan.Spec.Version,
	}

	// Add pre-pull images flag
	prePullImages := true
	if kubernetesPlan.Spec.PrePullImages != nil {
		prePullImages = *kubernetesPlan.Spec.PrePullImages
	}
	if !prePullImages {
		args = append(args, "--pre-pull-images=false")
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: kubernetesPlan.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "kubernetes-upgrade",
				"app.kubernetes.io/instance":   kubernetesPlan.Name,
				"app.kubernetes.io/managed-by": "talup",
				"talup.io/target-node":         targetNodeName,
				"talup.io/upgrade-type":        "kubernetes",
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
									SecretName: "talup",
								},
							},
						},
					},
				},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(kubernetesPlan, job, r.Scheme); err != nil {
		return nil, err
	}

	if err := r.Create(ctx, job); err != nil {
		return nil, err
	}

	return job, nil
}

func (r *KubernetesPlanReconciler) handleInProgressPhase(ctx context.Context, kubernetesPlan *upgradev1alpha1.KubernetesPlan) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if kubernetesPlan.Status.CurrentNode == "" {
		kubernetesPlan.Status.Phase = PhasePending
		return ctrl.Result{}, r.Status().Update(ctx, kubernetesPlan)
	}

	// Check job status
	jobName := fmt.Sprintf("k8s-upgrade-%s-%s", kubernetesPlan.Name, kubernetesPlan.Status.CurrentNode)
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      jobName,
		Namespace: kubernetesPlan.Namespace,
	}, job)

	if err != nil {
		if errors.IsNotFound(err) {
			return r.markNodeFailed(ctx, kubernetesPlan, kubernetesPlan.Status.CurrentNode, "Job not found")
		}
		logger.Error(err, "Failed to get upgrade job")
		return ctrl.Result{}, err
	}

	// Check job conditions
	if job.Status.Succeeded > 0 {
		// Job succeeded, verify upgrade
		if r.verifyKubernetesUpgrade(ctx, kubernetesPlan) {
			return r.markNodeCompleted(ctx, kubernetesPlan, kubernetesPlan.Status.CurrentNode)
		} else {
			return r.markNodeFailed(ctx, kubernetesPlan, kubernetesPlan.Status.CurrentNode, "Kubernetes upgrade verification failed")
		}
	}

	if job.Status.Failed > 0 {
		return r.markNodeFailed(ctx, kubernetesPlan, kubernetesPlan.Status.CurrentNode, "Upgrade job failed")
	}

	// Check for timeout
	timeout, _ := time.ParseDuration(kubernetesPlan.Spec.Timeout)
	if timeout == 0 {
		timeout = 30 * time.Minute
	}

	if time.Since(job.CreationTimestamp.Time) > timeout {
		return r.markNodeFailed(ctx, kubernetesPlan, kubernetesPlan.Status.CurrentNode, "Upgrade timeout")
	}

	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

func (r *KubernetesPlanReconciler) verifyKubernetesUpgrade(ctx context.Context, kubernetesPlan *upgradev1alpha1.KubernetesPlan) bool {
	// Check if Kubernetes version matches target
	upgradeNeeded, err := r.isUpgradeNeeded(ctx, kubernetesPlan)
	if err != nil {
		return false
	}
	return !upgradeNeeded
}

func (r *KubernetesPlanReconciler) handleDeletion(ctx context.Context, kubernetesPlan *upgradev1alpha1.KubernetesPlan) (ctrl.Result, error) {
	// Clean up jobs and remove finalizer
	logger := log.FromContext(ctx)

	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList, client.InNamespace(kubernetesPlan.Namespace), client.MatchingLabels{
		"app.kubernetes.io/name":       "kubernetes-upgrade",
		"app.kubernetes.io/instance":   kubernetesPlan.Name,
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

	controllerutil.RemoveFinalizer(kubernetesPlan, KubernetesPlanFinalizer)
	return ctrl.Result{}, r.Update(ctx, kubernetesPlan)
}

func (r *KubernetesPlanReconciler) handleFailedPhase(ctx context.Context, kubernetesPlan *upgradev1alpha1.KubernetesPlan) (ctrl.Result, error) {
	if kubernetesPlan.Status.Message != "Manual intervention required - upgrade failed after max retries" {
		kubernetesPlan.Status.Message = "Manual intervention required - upgrade failed after max retries"
		kubernetesPlan.Status.LastUpdated = metav1.Now()
		return ctrl.Result{}, r.Status().Update(ctx, kubernetesPlan)
	}
	return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}

func (r *KubernetesPlanReconciler) markNodeCompleted(ctx context.Context, kubernetesPlan *upgradev1alpha1.KubernetesPlan, nodeName string) (ctrl.Result, error) {
	kubernetesPlan.Status.UpgradedNodes = append(kubernetesPlan.Status.UpgradedNodes, nodeName)
	kubernetesPlan.Status.CurrentNode = ""
	kubernetesPlan.Status.Phase = PhaseCompleted // For K8s, typically only one node upgrade needed
	kubernetesPlan.Status.Message = fmt.Sprintf("Kubernetes upgraded successfully on node %s", nodeName)
	kubernetesPlan.Status.LastUpdated = metav1.Now()

	return ctrl.Result{}, r.Status().Update(ctx, kubernetesPlan)
}

func (r *KubernetesPlanReconciler) markNodeFailed(ctx context.Context, kubernetesPlan *upgradev1alpha1.KubernetesPlan, nodeName, reason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var nodeStatus *upgradev1alpha1.NodeUpgradeStatus
	for i := range kubernetesPlan.Status.FailedNodes {
		if kubernetesPlan.Status.FailedNodes[i].NodeName == nodeName {
			nodeStatus = &kubernetesPlan.Status.FailedNodes[i]
			break
		}
	}

	if nodeStatus == nil {
		kubernetesPlan.Status.FailedNodes = append(kubernetesPlan.Status.FailedNodes, upgradev1alpha1.NodeUpgradeStatus{
			NodeName:  nodeName,
			Retries:   1,
			LastError: reason,
		})
		nodeStatus = &kubernetesPlan.Status.FailedNodes[len(kubernetesPlan.Status.FailedNodes)-1]
	} else {
		nodeStatus.Retries++
		nodeStatus.LastError = reason
	}

	if nodeStatus.Retries >= DefaultBackoffLimit {
		kubernetesPlan.Status.Phase = PhaseFailed
		kubernetesPlan.Status.CurrentNode = ""
		kubernetesPlan.Status.Message = fmt.Sprintf("Kubernetes upgrade failed on node %s after %d retries: %s", nodeName, nodeStatus.Retries, reason)
		logger.Info("Kubernetes upgrade failed after max retries", "node", nodeName, "retries", nodeStatus.Retries)
	} else {
		kubernetesPlan.Status.Phase = PhasePending
		kubernetesPlan.Status.CurrentNode = ""
		kubernetesPlan.Status.Message = fmt.Sprintf("Kubernetes upgrade failed on node %s (retry %d/%d): %s", nodeName, nodeStatus.Retries, DefaultBackoffLimit, reason)
		logger.Info("Kubernetes upgrade failed, will retry", "node", nodeName, "retries", nodeStatus.Retries)
	}

	kubernetesPlan.Status.LastUpdated = metav1.Now()
	return ctrl.Result{RequeueAfter: time.Second * 30}, r.Status().Update(ctx, kubernetesPlan)
}

func (r *KubernetesPlanReconciler) findExecutorNode(ctx context.Context, kubernetesPlan *upgradev1alpha1.KubernetesPlan, targetNode string) (string, error) {
	controlPlaneNodes, err := r.getControlPlaneNodes(ctx, kubernetesPlan)
	if err != nil {
		return "", err
	}

	for _, node := range controlPlaneNodes {
		if node.Name != targetNode && r.isNodeReady(&node) {
			return node.Name, nil
		}
	}

	// If no other control plane node available, use the target node itself
	return targetNode, nil
}

func (r *KubernetesPlanReconciler) isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (r *KubernetesPlanReconciler) getNodeInternalIP(node *corev1.Node) (string, error) {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address, nil
		}
	}
	return "", fmt.Errorf("no internal IP found for node %s", node.Name)
}

func (r *KubernetesPlanReconciler) isNodeInList(nodeName string, nodeList []string) bool {
	for _, name := range nodeList {
		if name == nodeName {
			return true
		}
	}
	return false
}

func (r *KubernetesPlanReconciler) isNodeInFailedList(nodeName string, failedNodes []upgradev1alpha1.NodeUpgradeStatus) bool {
	for _, node := range failedNodes {
		if node.NodeName == nodeName {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubernetesPlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&upgradev1alpha1.KubernetesPlan{}).
		Owns(&batchv1.Job{}).
		Named("kubernetesplan").
		Complete(r)
}
