package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/distribution/reference"
	talosclient "github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/siderolabs/talos/pkg/machinery/resources/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cosi-project/runtime/pkg/resource"
)

const (
	// ClientRefreshInterval defines how often to refresh the Talos client
	ClientRefreshInterval = 5 * time.Minute
	// DefaultTimeout for Talos operations
	DefaultTimeout = 30 * time.Second
	// TalosConfigSecretKey is the key for talosconfig in the secret
	TalosConfigSecretKey = "config"
)

// NodeInfo contains information about a Talos node
type NodeInfo struct {
	NodeIP            string `json:"nodeIP"`
	TalosVersion      string `json:"talosVersion"`
	KubernetesVersion string `json:"kubernetesVersion"`
	Platform          string `json:"platform"`
	Image             string `json:"image"`
}

// String returns a string representation of NodeInfo
func (ni *NodeInfo) String() string {
	return fmt.Sprintf("Node{IP: %s, TalosVersion: %s, K8sVersion: %s, Platform: %s, Image: %s}",
		ni.NodeIP, ni.TalosVersion, ni.KubernetesVersion, ni.Platform, ni.Image)
}

// TalosClient provides Talos SDK operations for gathering node information
type TalosClient struct {
	k8sClient           ctrlclient.Client
	talosConfigSecret   string
	controllerNamespace string
}

// NewTalosClient creates a new Talos client service
func NewTalosClient(k8sClient ctrlclient.Client, talosConfigSecret, controllerNamespace string) *TalosClient {
	return &TalosClient{
		k8sClient:           k8sClient,
		talosConfigSecret:   talosConfigSecret,
		controllerNamespace: controllerNamespace,
	}
}

func (s *TalosClient) GetClient(ctx context.Context) (*talosclient.Client, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Creating fresh Talos client",
		"secret", s.talosConfigSecret,
		"namespace", s.controllerNamespace)

	configData, err := s.loadTalosConfig(ctx)
	if err != nil {
		logger.Error(err, "Failed to load talosconfig")
		return nil, fmt.Errorf("failed to load talosconfig: %w", err)
	}

	logger.V(2).Info("Talosconfig loaded successfully", "configSize", len(configData))

	talosClient, err := s.createTalosClient(ctx, configData)
	if err != nil {
		logger.Error(err, "Failed to create talos client")
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}

	logger.V(1).Info("Successfully created fresh Talos client",
		"secret", s.talosConfigSecret,
		"client", fmt.Sprintf("%p", talosClient))
	return talosClient, nil
}

// loadTalosConfig loads the talosconfig from the Kubernetes secret
func (s *TalosClient) loadTalosConfig(ctx context.Context) ([]byte, error) {
	logger := log.FromContext(ctx)
	logger.V(2).Info("Loading talosconfig from secret",
		"secret", s.talosConfigSecret,
		"namespace", s.controllerNamespace)

	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      s.talosConfigSecret,
		Namespace: s.controllerNamespace,
	}

	if err := s.k8sClient.Get(ctx, secretKey, secret); err != nil {
		logger.Error(err, "Failed to get talosconfig secret",
			"secretName", s.talosConfigSecret,
			"namespace", s.controllerNamespace)
		return nil, fmt.Errorf("failed to get talosconfig secret %s: %w", s.talosConfigSecret, err)
	}

	logger.V(2).Info("Secret retrieved successfully",
		"secret", s.talosConfigSecret,
		"dataKeys", fmt.Sprintf("%v", getMapKeys(secret.Data)))

	configData, exists := secret.Data[TalosConfigSecretKey]
	if !exists {
		logger.Error(nil, "Config key not found in secret",
			"secret", s.talosConfigSecret,
			"expectedKey", TalosConfigSecretKey,
			"availableKeys", fmt.Sprintf("%v", getMapKeys(secret.Data)))
		return nil, fmt.Errorf("config key not found in secret %s", s.talosConfigSecret)
	}

	logger.V(2).Info("Talosconfig data extracted",
		"secret", s.talosConfigSecret,
		"configSize", len(configData))

	return configData, nil
}

// Helper function to get map keys for logging
func getMapKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// createTalosClient creates a new Talos client from config data
func (s *TalosClient) createTalosClient(ctx context.Context, configData []byte) (*talosclient.Client, error) {
	logger := log.FromContext(ctx)
	logger.V(2).Info("Parsing talosconfig", "configSize", len(configData))

	talosConfig, err := clientconfig.FromBytes(configData)
	if err != nil {
		logger.Error(err, "Failed to parse talosconfig")
		return nil, fmt.Errorf("failed to parse talosconfig: %w", err)
	}

	logger.V(2).Info("Talosconfig parsed successfully",
		"context", talosConfig.Context,
		"endpoints", fmt.Sprintf("%v", talosConfig.Contexts[talosConfig.Context].Endpoints))

	talosClient, err := talosclient.New(ctx, talosclient.WithConfig(talosConfig))
	if err != nil {
		logger.Error(err, "Failed to create talos client")
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	logger.V(2).Info("Talos client created successfully", "client", fmt.Sprintf("%p", talosClient))

	return talosClient, nil
}

// GetNodeInfo retrieves detailed information about a Talos node
func (s *TalosClient) GetNodeInfo(ctx context.Context, nodeIP string) (*NodeInfo, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Getting node info", "nodeIP", nodeIP)

	// Create a node-specific client context
	nodeCtx := talosclient.WithNodes(ctx, nodeIP)
	logger.V(2).Info("Created node context", "nodeIP", nodeIP)

	// Load config and create client
	configData, err := s.loadTalosConfig(ctx)
	if err != nil {
		logger.Error(err, "Failed to load talosconfig for node", "nodeIP", nodeIP)
		return nil, fmt.Errorf("failed to load talosconfig: %w", err)
	}

	talosClient, err := s.createTalosClient(ctx, configData)
	if err != nil {
		logger.Error(err, "Failed to create talos client for node", "nodeIP", nodeIP)
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() {
		if closeErr := talosClient.Close(); closeErr != nil {
			logger.V(1).Info("Failed to close Talos client", "error", closeErr, "nodeIP", nodeIP)
		} else {
			logger.V(2).Info("Talos client closed successfully", "nodeIP", nodeIP)
		}
	}()

	logger.V(2).Info("Calling talos Version API", "nodeIP", nodeIP)

	// Get version info
	resp, err := talosClient.Version(nodeCtx)
	if err != nil {
		logger.Error(err, "Failed to get version from node", "nodeIP", nodeIP)
		return nil, fmt.Errorf("failed to get node info from %s: %w", nodeIP, err)
	}

	logger.V(2).Info("Version API response received",
		"nodeIP", nodeIP,
		"messageCount", len(resp.Messages))

	if len(resp.Messages) == 0 {
		logger.Error(nil, "No response messages from node", "nodeIP", nodeIP)
		return nil, fmt.Errorf("no response from node %s", nodeIP)
	}

	msg := resp.Messages[0]
	version := msg.GetVersion()

	if version == nil {
		logger.Error(nil, "Version is nil in response", "nodeIP", nodeIP)
		return nil, fmt.Errorf("version is nil for node %s", nodeIP)
	}

	logger.V(2).Info("Version info extracted",
		"nodeIP", nodeIP,
		"talosVersion", version.GetTag(),
		"platform", func() string {
			if p := msg.GetPlatform(); p != nil {
				return p.GetName()
			}
			return "unknown"
		}())

	nodeInfo := &NodeInfo{
		NodeIP:       nodeIP,
		TalosVersion: version.GetTag(),
	}

	// Safely handle platform info
	if platform := msg.GetPlatform(); platform != nil {
		nodeInfo.Platform = platform.GetName()
		logger.V(2).Info("Platform info set", "nodeIP", nodeIP, "platform", platform.GetName())
	} else {
		logger.V(2).Info("No platform info available", "nodeIP", nodeIP)
	}

	// Now get the machine config to extract image and kubelet version
	logger.V(2).Info("Fetching machine config from node", "nodeIP", nodeIP)

	timeoutCtx, cancel := context.WithTimeout(nodeCtx, DefaultTimeout)
	defer cancel()

	r, err := talosClient.COSI.Get(timeoutCtx, resource.NewMetadata("config", "MachineConfigs.config.talos.dev", "v1alpha1", resource.VersionUndefined))
	if err != nil {
		logger.Error(err, "Failed to get machine config from node", "nodeIP", nodeIP)
		logger.V(1).Info("Continuing without machine config", "nodeIP", nodeIP)
		return nodeInfo, nil
	}

	logger.V(2).Info("Machine config resource retrieved",
		"nodeIP", nodeIP,
		"resourceType", fmt.Sprintf("%T", r))

	mc, ok := r.(*config.MachineConfig)
	if !ok {
		logger.Error(nil, "Unexpected resource type for machine config",
			"nodeIP", nodeIP,
			"expectedType", "*config.MachineConfig",
			"actualType", fmt.Sprintf("%T", r))
		return nodeInfo, nil
	}

	logger.V(2).Info("Machine config cast successful", "nodeIP", nodeIP)

	// Debug: log the entire machine config structure
	logger.V(3).Info("Machine config details",
		"nodeIP", nodeIP,
		"machineConfig", fmt.Sprintf("%+v", mc.Config().Machine().Install()))

	// Extract install image
	if mcImage := mc.Config().Machine().Install().Image(); mcImage != "" {
		nodeInfo.Image = mcImage
		logger.V(1).Info("Install image found", "nodeIP", nodeIP, "image", mcImage)
	} else {
		logger.V(1).Info("Install image is empty", "nodeIP", nodeIP)

		// Try to get the image from the machine config's installer disk image
		if diskImage := mc.Config().Machine().Install().Disk(); diskImage != "" {
			logger.V(2).Info("Found disk image instead of install image", "nodeIP", nodeIP, "disk", diskImage)
		} else {
			logger.V(2).Info("Disk image is also empty", "nodeIP", nodeIP)
		}

		// Alternative: try to get from the extensions or system extensions
		if extensions := mc.Config().Machine().Install().Extensions(); len(extensions) > 0 {
			logger.V(2).Info("Found extensions", "nodeIP", nodeIP, "extensionCount", len(extensions))
			for i, ext := range extensions {
				logger.V(3).Info("Extension details", "nodeIP", nodeIP, "index", i, "extension", ext)
			}
		} else {
			logger.V(2).Info("No extensions found", "nodeIP", nodeIP)
		}
	}

	// Get Kubernetes version from kubelet image
	logger.V(2).Info("Extracting kubelet image", "nodeIP", nodeIP)

	if kubeletImage := mc.Config().Machine().Kubelet().Image(); kubeletImage != "" {
		logger.V(2).Info("Kubelet image found", "nodeIP", nodeIP, "kubeletImage", kubeletImage)

		if kubeletRef, err := parseImageReference(kubeletImage); err == nil {
			nodeInfo.KubernetesVersion = kubeletRef.Tag()
			logger.V(2).Info("Kubernetes version extracted from kubelet image",
				"nodeIP", nodeIP,
				"kubernetesVersion", kubeletRef.Tag())
		} else {
			logger.Error(err, "Failed to parse kubelet image reference",
				"nodeIP", nodeIP,
				"kubeletImage", kubeletImage)
		}
	} else {
		logger.V(1).Info("Kubelet image is empty", "nodeIP", nodeIP)
	}

	logger.V(1).Info("Node info collection complete",
		"nodeIP", nodeIP,
		"nodeInfo", nodeInfo.String())

	return nodeInfo, nil
}

// parseImageReference parses an image reference to extract components
func parseImageReference(image string) (reference.NamedTagged, error) {
	ref, err := reference.ParseAnyReference(image)
	if err != nil {
		return nil, err
	}
	ntref, ok := ref.(reference.NamedTagged)
	if !ok {
		return nil, fmt.Errorf("not a NamedTagged reference")
	}
	return ntref, nil
}

// IsAvailable checks if the Talos client service is available
func (s *TalosClient) IsAvailable(ctx context.Context) bool {
	logger := log.FromContext(ctx)
	logger.V(2).Info("Checking if Talos client is available")

	_, err := s.GetClient(ctx)
	isAvailable := err == nil

	logger.V(2).Info("Talos client availability check complete",
		"available", isAvailable,
		"error", err)

	return isAvailable
}
