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
	TalosPlanFinalizer  = "upgrade.home-operations.com/talos-finalizer"
	SchematicAnnotation = "extensions.talos.dev/schematic"
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
	for _, node := range nodes {
		logger.V(1).Info("Target node", "name", node.Name)
	}

	targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag)
	logger.Info("Target image", "image", targetImage)

	for _, node := range nodes {
		logger.Info("Checking node upgrade status", "node", node.Name)

		// Check if node needs upgrade using Talos SDK
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

	// Get node IP address
	logger.V(1).Info("Getting node IP address", "node", node.Name)
	nodeIP, err := r.getNodeInternalIP(node)
	if err != nil {
		logger.Error(err, "Failed to get node IP", "node", node.Name)
		return false, fmt.Errorf("failed to get node IP: %w", err)
	}
	logger.Info("Using node IP", "node", node.Name, "ip", nodeIP)

	// Create a Talos client specifically for this node
	logger.V(1).Info("Creating node-specific Talos client")
	talosClient, err := r.getTalosClientForNode(ctx, talosPlan.Namespace, nodeIP)
	if err != nil {
		logger.Error(err, "Failed to get Talos client for node")
		return false, fmt.Errorf("failed to get Talos client: %w", err)
	}
	defer talosClient.Close()

	// Get version information from the node
	logger.V(1).Info("Getting version from node", "node", node.Name, "ip", nodeIP)
	versionResp, err := talosClient.Version(ctx)
	if err != nil {
		logger.Error(err, "Failed to get version from node", "node", node.Name, "ip", nodeIP)
		return false, fmt.Errorf("failed to get version from node %s: %w", node.Name, err)
	}

	if len(versionResp.Messages) == 0 {
		logger.Error(fmt.Errorf("no version response"), "No version response from node", "node", node.Name)
		return false, fmt.Errorf("no version response from node %s", node.Name)
	}

	currentVersion := versionResp.Messages[0].Version.Tag
	logger.Info("Retrieved current version", "node", node.Name, "currentVersion", currentVersion)

	// Get machine config to extract current schematic
	logger.V(1).Info("Getting machine config", "node", node.Name)
	machineConfig, err := r.getMachineConfig(ctx, talosClient)
	if err != nil {
		logger.Error(err, "Failed to get machine config", "node", node.Name)
		return false, fmt.Errorf("failed to get machine config from node %s: %w", node.Name, err)
	}

	currentInstallImage := machineConfig.Config().Machine().Install().Image()
	logger.Info("Retrieved current install image", "node", node.Name, "currentInstallImage", currentInstallImage)

	currentSchematic, err := r.extractSchematicFromMachineConfig(currentInstallImage)
	if err != nil {
		logger.V(1).Info("Could not extract schematic from machine config", "node", node.Name, "image", currentInstallImage, "error", err)
		currentSchematic = ""
	}
	logger.Info("Current schematic", "node", node.Name, "currentSchematic", currentSchematic)

	// Get node schematic annotation
	nodeSchematic := r.getSchematicFromNode(node)
	logger.V(1).Info("Node schematic annotation", "node", node.Name, "nodeSchematic", nodeSchematic)

	// Extract target version and schematic from target image
	targetVersion, targetSchematic := r.extractVersionAndSchematic(targetImage)
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
		"nodeSchematic", nodeSchematic,
		"targetSchematic", targetSchematic,
	)

	// Need upgrade if:
	// 1. Versions don't match, OR
	// 2. Schematics don't match (comparing machine config schematic with node annotation)
	versionMismatch := currentVersion != targetVersion
	schematicMismatch := targetSchematic != "" && nodeSchematic != currentSchematic

	needsUpgrade := versionMismatch || schematicMismatch

	logger.Info("Upgrade decision",
		"node", node.Name,
		"versionMismatch", versionMismatch,
		"schematicMismatch", schematicMismatch,
		"needsUpgrade", needsUpgrade)

	return needsUpgrade, nil
}

func (r *TalosPlanReconciler) getTalosClientForNode(ctx context.Context, namespace, nodeIP string) (*talosclient.Client, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Creating Talos client for specific node", "namespace", namespace, "nodeIP", nodeIP)

	// Get the talosconfig secret
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      TalosConfigSecretName,
		Namespace: namespace,
	}

	logger.V(1).Info("Retrieving talosconfig secret", "secretName", TalosConfigSecretName, "namespace", namespace)
	if err := r.Get(ctx, secretKey, secret); err != nil {
		logger.Error(err, "Failed to get talosconfig secret", "secretName", TalosConfigSecretName, "namespace", namespace)
		return nil, fmt.Errorf("failed to get talosconfig secret: %w", err)
	}

	// Get the config data using the correct key
	configData, exists := secret.Data[TalosConfigSecretKey]
	if !exists {
		logger.Error(fmt.Errorf("key not found"), "Talosconfig key not found in secret", "key", TalosConfigSecretKey)
		return nil, fmt.Errorf("talosconfig key '%s' not found in secret", TalosConfigSecretKey)
	}

	logger.V(1).Info("Found talosconfig data", "dataLength", len(configData))

	// Parse the config
	config, err := talosclientconfig.FromBytes(configData)
	if err != nil {
		logger.Error(err, "Failed to parse talosconfig")
		return nil, fmt.Errorf("failed to parse talosconfig: %w", err)
	}

	// Override the endpoints in the config to target the specific node
	endpoint := nodeIP + ":50000"
	logger.Info("Setting Talos endpoint", "originalEndpoints", config.Contexts[config.Context].Endpoints, "newEndpoint", endpoint)
	config.Contexts[config.Context].Endpoints = []string{endpoint}

	// Add timeout context for client creation
	clientCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Create client options with more permissive settings
	opts := []talosclient.OptionFunc{
		talosclient.WithConfig(config),
		talosclient.WithGRPCDialOptions(
			grpc.WithTransportCredentials(
				credentials.NewTLS(&tls.Config{
					InsecureSkipVerify: true,
				}),
			),
		),
	}

	// Create and return the client
	logger.V(1).Info("Creating Talos client", "endpoint", endpoint)
	client, err := talosclient.New(clientCtx, opts...)
	if err != nil {
		logger.Error(err, "Failed to create Talos client", "endpoint", endpoint)
		return nil, fmt.Errorf("failed to create Talos client for %s: %w", endpoint, err)
	}

	// Test the connection with a quick version call
	logger.V(1).Info("Testing connection to Talos node", "endpoint", endpoint)
	testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
	defer testCancel()

	_, err = client.Version(testCtx)
	if err != nil {
		logger.Error(err, "Failed to connect to Talos node", "endpoint", endpoint)
		client.Close()
		return nil, fmt.Errorf("failed to connect to Talos node %s: %w", endpoint, err)
	}

	logger.Info("Successfully created and tested Talos client", "endpoint", endpoint)
	return client, nil
}

func (r *TalosPlanReconciler) getMachineConfig(ctx context.Context, talosClient *talosclient.Client) (*config.MachineConfig, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Retrieving machine config via COSI")

	// Get machine config using COSI client
	res, err := talosClient.COSI.Get(ctx, resource.NewMetadata(
		"config",
		"MachineConfigs.config.talos.dev",
		"v1alpha1",
		resource.VersionUndefined,
	))
	if err != nil {
		logger.Error(err, "Failed to get machine config via COSI")
		return nil, fmt.Errorf("failed to get machine config: %w", err)
	}

	machineConfig, ok := res.(*config.MachineConfig)
	if !ok {
		logger.Error(fmt.Errorf("type assertion failed"), "Unexpected resource type for machine config")
		return nil, fmt.Errorf("unexpected resource type for machine config")
	}

	logger.V(1).Info("Successfully retrieved machine config")
	return machineConfig, nil
}

func (r *TalosPlanReconciler) extractSchematicFromMachineConfig(installImage string) (string, error) {
	logger := log.FromContext(context.Background())
	logger.V(1).Info("Extracting schematic from install image", "image", installImage)

	// Parse the install image reference to extract schematic
	ref, err := reference.ParseAnyReference(installImage)
	if err != nil {
		logger.Error(err, "Failed to parse install image reference", "image", installImage)
		return "", fmt.Errorf("failed to parse install image reference: %w", err)
	}

	namedRef, ok := ref.(reference.Named)
	if !ok {
		logger.Error(fmt.Errorf("not a named reference"), "Install image is not a named reference", "image", installImage)
		return "", fmt.Errorf("not a named reference")
	}

	// Extract schematic from the image name path
	imageName := namedRef.Name()
	logger.V(1).Info("Parsed image name", "imageName", imageName)

	if strings.Contains(imageName, "factory.talos.dev") {
		// For factory images, schematic is the last part of the path
		parts := strings.Split(imageName, "/")
		if len(parts) >= 3 {
			schematic := parts[len(parts)-1]
			logger.V(1).Info("Extracted schematic from factory image", "schematic", schematic, "image", installImage)
			return schematic, nil
		}
	}

	logger.V(1).Info("No schematic found in image", "image", installImage)
	return "", fmt.Errorf("no schematic found in image: %s", installImage)
}

func (r *TalosPlanReconciler) getTalosClient(ctx context.Context, namespace string) (*talosclient.Client, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Creating Talos client", "namespace", namespace)

	// Get the talosconfig secret
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      TalosConfigSecretName,
		Namespace: namespace,
	}

	logger.V(1).Info("Retrieving talosconfig secret", "secretName", TalosConfigSecretName, "namespace", namespace)
	if err := r.Get(ctx, secretKey, secret); err != nil {
		logger.Error(err, "Failed to get talosconfig secret", "secretName", TalosConfigSecretName, "namespace", namespace)
		return nil, fmt.Errorf("failed to get talosconfig secret: %w", err)
	}

	// Get the config data using the correct key
	configData, exists := secret.Data[TalosConfigSecretKey]
	if !exists {
		logger.Error(fmt.Errorf("key not found"), "Talosconfig key not found in secret", "key", TalosConfigSecretKey)
		return nil, fmt.Errorf("talosconfig key '%s' not found in secret", TalosConfigSecretKey)
	}

	logger.V(1).Info("Found talosconfig data", "dataLength", len(configData))

	// Parse the config
	config, err := talosclientconfig.FromBytes(configData)
	if err != nil {
		logger.Error(err, "Failed to parse talosconfig")
		return nil, fmt.Errorf("failed to parse talosconfig: %w", err)
	}

	// Create client options with more permissive settings for development
	opts := []talosclient.OptionFunc{
		talosclient.WithConfig(config),
		talosclient.WithGRPCDialOptions(
			grpc.WithTransportCredentials(
				credentials.NewTLS(&tls.Config{
					InsecureSkipVerify: true,
				}),
			),
		),
	}

	// Create and return the client
	logger.V(1).Info("Creating Talos client with config")
	client, err := talosclient.New(ctx, opts...)
	if err != nil {
		logger.Error(err, "Failed to create Talos client")
		return nil, err
	}

	logger.V(1).Info("Successfully created Talos client")
	return client, nil
}

func (r *TalosPlanReconciler) getNodeInternalIP(node *corev1.Node) (string, error) {
	logger := log.FromContext(context.Background())
	logger.V(1).Info("Getting internal IP for node", "node", node.Name)

	for _, addr := range node.Status.Addresses {
		logger.V(1).Info("Node address", "node", node.Name, "type", addr.Type, "address", addr.Address)
		if addr.Type == corev1.NodeInternalIP {
			logger.V(1).Info("Found internal IP", "node", node.Name, "ip", addr.Address)
			return addr.Address, nil
		}
	}

	logger.Error(fmt.Errorf("no internal IP found"), "No internal IP found for node", "node", node.Name)
	return "", fmt.Errorf("no internal IP found for node %s", node.Name)
}

func (r *TalosPlanReconciler) extractVersionAndSchematic(image string) (version, schematic string) {
	logger := log.FromContext(context.Background())
	logger.V(1).Info("Extracting version and schematic from image", "image", image)

	// Parse image like: factory.talos.dev/metal-installer/05b4a47a70bc97786ed83d200567dcc8a13f731b164537ba59d5397d668851fa:v1.11.1
	// or ghcr.io/siderolabs/installer:v1.11.1

	parts := strings.Split(image, ":")
	if len(parts) < 2 {
		logger.Error(fmt.Errorf("invalid image format"), "Image does not contain version tag", "image", image)
		return "", ""
	}

	version = parts[len(parts)-1]
	logger.V(1).Info("Extracted version", "version", version)

	// Check if this is a factory image (contains schematic)
	if strings.Contains(image, "factory.talos.dev") {
		pathParts := strings.Split(parts[0], "/")
		if len(pathParts) >= 3 {
			schematic = pathParts[len(pathParts)-1]
			logger.V(1).Info("Extracted schematic from factory image", "schematic", schematic)
		}
	}

	logger.V(1).Info("Extraction complete", "image", image, "version", version, "schematic", schematic)
	return version, schematic
}

func (r *TalosPlanReconciler) getSchematicFromNode(node *corev1.Node) string {
	logger := log.FromContext(context.Background())

	if node.Annotations == nil {
		logger.V(1).Info("Node has no annotations", "node", node.Name)
		return ""
	}

	schematic := node.Annotations[SchematicAnnotation]
	logger.V(1).Info("Retrieved schematic from node annotation", "node", node.Name, "schematic", schematic)
	return schematic
}

func (r *TalosPlanReconciler) verifyNodeUpgrade(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, nodeName string) bool {
	logger := log.FromContext(ctx)
	logger.Info("Verifying node upgrade", "node", nodeName)

	// Get the node and check if it has the expected version using Talos SDK
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

	// Get nodes that need upgrade
	nodes, err := r.getTargetNodes(ctx, talosPlan)
	if err != nil {
		logger.Error(err, "Failed to get target nodes")
		return ctrl.Result{}, err
	}

	logger.Info("Evaluating nodes for upgrade", "totalNodes", len(nodes))

	targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag)

	// Find first node that needs upgrade and isn't completed or failed
	var targetNode *corev1.Node
	for _, node := range nodes {
		logger.Info("Evaluating node", "node", node.Name,
			"inCompleted", r.isNodeInList(node.Name, talosPlan.Status.CompletedNodes),
			"inFailed", r.isNodeInFailedList(node.Name, talosPlan.Status.FailedNodes))

		needsUpgrade, err := r.nodeNeedsUpgrade(ctx, talosPlan, &node, targetImage)
		if err != nil {
			logger.Error(err, "Failed to check if node needs upgrade", "node", node.Name)
			continue
		}

		if needsUpgrade &&
			!r.isNodeInList(node.Name, talosPlan.Status.CompletedNodes) &&
			!r.isNodeInFailedList(node.Name, talosPlan.Status.FailedNodes) {
			targetNode = &node
			logger.Info("Selected node for upgrade", "node", node.Name)
			break
		}
	}

	if targetNode == nil {
		// All nodes are upgraded
		logger.Info("All nodes are upgraded, marking plan as completed")
		talosPlan.Status.Phase = PhaseCompleted
		talosPlan.Status.Message = "All nodes successfully upgraded"
		talosPlan.Status.CurrentNode = ""
		talosPlan.Status.LastUpdated = metav1.Now()
		return ctrl.Result{}, r.Status().Update(ctx, talosPlan)
	}

	// Create upgrade job for the target node
	logger.Info("Creating upgrade job", "targetNode", targetNode.Name)
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
	logger.Info("Handling in-progress phase", "talosPlan", talosPlan.Name, "currentNode", talosPlan.Status.CurrentNode)

	if talosPlan.Status.CurrentNode == "" {
		logger.Info("No current node, moving to pending to start next upgrade")
		// No current node, move to pending to start next upgrade
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
			// Job doesn't exist, move to failed
			return r.markNodeFailed(ctx, talosPlan, talosPlan.Status.CurrentNode, "Job not found")
		}
		logger.Error(err, "Failed to get upgrade job", "job", jobName)
		return ctrl.Result{}, err
	}

	logger.V(1).Info("Job status", "job", jobName,
		"succeeded", job.Status.Succeeded,
		"failed", job.Status.Failed,
		"active", job.Status.Active)

	// Check job conditions
	if job.Status.Succeeded > 0 {
		logger.Info("Job succeeded, verifying node upgrade", "job", jobName, "node", talosPlan.Status.CurrentNode)
		// Job succeeded, verify node upgrade using Talos SDK
		if r.verifyNodeUpgrade(ctx, talosPlan, talosPlan.Status.CurrentNode) {
			logger.Info("Node upgrade verified successfully", "node", talosPlan.Status.CurrentNode)
			return r.markNodeCompleted(ctx, talosPlan, talosPlan.Status.CurrentNode)
		} else {
			logger.Error(fmt.Errorf("verification failed"), "Node upgrade verification failed", "node", talosPlan.Status.CurrentNode)
			return r.markNodeFailed(ctx, talosPlan, talosPlan.Status.CurrentNode, "Node upgrade verification failed")
		}
	}

	if job.Status.Failed > 0 {
		logger.Error(fmt.Errorf("job failed"), "Upgrade job failed", "job", jobName, "node", talosPlan.Status.CurrentNode)
		// Job failed
		return r.markNodeFailed(ctx, talosPlan, talosPlan.Status.CurrentNode, "Upgrade job failed")
	}

	// Check for timeout
	timeout, _ := time.ParseDuration(talosPlan.Spec.Timeout)
	if timeout == 0 {
		timeout = 30 * time.Minute
	}

	jobAge := time.Since(job.CreationTimestamp.Time)
	logger.V(1).Info("Job timing", "job", jobName, "age", jobAge, "timeout", timeout)

	if jobAge > timeout {
		logger.Error(fmt.Errorf("timeout"), "Upgrade job timed out", "job", jobName, "age", jobAge, "timeout", timeout)
		return r.markNodeFailed(ctx, talosPlan, talosPlan.Status.CurrentNode, "Upgrade timeout")
	}

	// Job still running
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

	// Find existing failed node entry or create new one
	var nodeStatus *upgradev1alpha1.NodeUpgradeStatus
	for i := range talosPlan.Status.FailedNodes {
		if talosPlan.Status.FailedNodes[i].NodeName == nodeName {
			nodeStatus = &talosPlan.Status.FailedNodes[i]
			break
		}
	}

	if nodeStatus == nil {
		logger.Info("Creating new failed node status", "node", nodeName)
		talosPlan.Status.FailedNodes = append(talosPlan.Status.FailedNodes, upgradev1alpha1.NodeUpgradeStatus{
			NodeName:  nodeName,
			Retries:   1,
			LastError: reason,
		})
		nodeStatus = &talosPlan.Status.FailedNodes[len(talosPlan.Status.FailedNodes)-1]
	} else {
		logger.Info("Updating existing failed node status", "node", nodeName, "currentRetries", nodeStatus.Retries)
		nodeStatus.Retries++
		nodeStatus.LastError = reason
	}

	maxRetries := talosPlan.Spec.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	logger.Info("Node failure status", "node", nodeName, "retries", nodeStatus.Retries, "maxRetries", maxRetries)

	if nodeStatus.Retries >= maxRetries {
		// Max retries reached, mark as failed
		logger.Info("Max retries reached, marking plan as failed", "node", nodeName, "retries", nodeStatus.Retries)
		talosPlan.Status.Phase = PhaseFailed
		talosPlan.Status.CurrentNode = ""
		talosPlan.Status.Message = fmt.Sprintf("Node %s failed after %d retries: %s", nodeName, nodeStatus.Retries, reason)
	} else {
		// Retry
		logger.Info("Will retry node upgrade", "node", nodeName, "retries", nodeStatus.Retries, "maxRetries", maxRetries)
		talosPlan.Status.Phase = PhasePending
		talosPlan.Status.CurrentNode = ""
		talosPlan.Status.Message = fmt.Sprintf("Node %s failed (retry %d/%d): %s", nodeName, nodeStatus.Retries, maxRetries, reason)
	}

	talosPlan.Status.LastUpdated = metav1.Now()
	return ctrl.Result{RequeueAfter: time.Second * 30}, r.Status().Update(ctx, talosPlan)
}

func (r *TalosPlanReconciler) createUpgradeJob(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, nodeName string) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)
	logger.Info("Creating upgrade job", "node", nodeName)

	// Find a node different from the target node to run the job
	executorNode, err := r.findExecutorNode(ctx, talosPlan, nodeName)
	if err != nil {
		logger.Error(err, "Failed to find executor node", "targetNode", nodeName)
		return nil, fmt.Errorf("failed to find executor node: %w", err)
	}
	logger.Info("Selected executor node", "executorNode", executorNode, "targetNode", nodeName)

	// Get target node IP for the talosctl command
	targetNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, targetNode); err != nil {
		logger.Error(err, "Failed to get target node", "node", nodeName)
		return nil, fmt.Errorf("failed to get target node: %w", err)
	}

	targetNodeIP, err := r.getNodeInternalIP(targetNode)
	if err != nil {
		logger.Error(err, "Failed to get target node IP", "node", nodeName)
		return nil, fmt.Errorf("failed to get target node IP: %w", err)
	}
	logger.Info("Target node details", "node", nodeName, "ip", targetNodeIP)

	jobName := fmt.Sprintf("talos-upgrade-%s-%s", talosPlan.Name, nodeName)
	talosctlImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Talosctl.Image.Repository, talosPlan.Spec.Talosctl.Image.Tag)
	targetImage := fmt.Sprintf("%s:%s", talosPlan.Spec.Image.Repository, talosPlan.Spec.Image.Tag)

	logger.Info("Job configuration",
		"jobName", jobName,
		"talosctlImage", talosctlImage,
		"targetImage", targetImage)

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

	logger.Info("Command args", "args", args)

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
	if err := controllerutil.SetControllerReference(talosPlan, job, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference")
		return nil, err
	}

	logger.V(1).Info("Creating job", "job", jobName)
	if err := r.Create(ctx, job); err != nil {
		logger.Error(err, "Failed to create job", "job", jobName)
		return nil, err
	}

	logger.Info("Successfully created upgrade job", "job", jobName, "executorNode", executorNode, "targetNode", nodeName)
	return job, nil
}

func (r *TalosPlanReconciler) findExecutorNode(ctx context.Context, talosPlan *upgradev1alpha1.TalosPlan, targetNode string) (string, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Finding executor node", "targetNode", targetNode)

	nodes, err := r.getTargetNodes(ctx, talosPlan)
	if err != nil {
		logger.Error(err, "Failed to get target nodes for executor selection")
		return "", err
	}

	logger.V(1).Info("Evaluating nodes for executor role", "totalNodes", len(nodes))

	// Find a node that's not the target node and is ready
	for _, node := range nodes {
		isReady := r.isNodeReady(&node)
		logger.V(1).Info("Evaluating node", "node", node.Name, "isTarget", node.Name == targetNode, "isReady", isReady)

		if node.Name != targetNode && isReady {
			logger.Info("Selected executor node", "executorNode", node.Name, "targetNode", targetNode)
			return node.Name, nil
		}
	}

	logger.Error(fmt.Errorf("no suitable executor found"), "No suitable executor node found", "targetNode", targetNode)
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
