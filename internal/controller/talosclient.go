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
	configData, err := s.loadTalosConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load talosconfig: %w", err)
	}

	talosClient, err := s.createTalosClient(ctx, configData)
	if err != nil {
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}

	return talosClient, nil
}

// loadTalosConfig loads the talosconfig from the Kubernetes secret
func (s *TalosClient) loadTalosConfig(ctx context.Context) ([]byte, error) {
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      s.talosConfigSecret,
		Namespace: s.controllerNamespace,
	}

	if err := s.k8sClient.Get(ctx, secretKey, secret); err != nil {
		return nil, fmt.Errorf("failed to get talosconfig secret %s: %w", s.talosConfigSecret, err)
	}

	configData, exists := secret.Data[TalosConfigSecretKey]
	if !exists {
		return nil, fmt.Errorf("config key not found in secret %s", s.talosConfigSecret)
	}

	return configData, nil
}

// createTalosClient creates a new Talos client from config data
func (s *TalosClient) createTalosClient(ctx context.Context, configData []byte) (*talosclient.Client, error) {
	talosConfig, err := clientconfig.FromBytes(configData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse talosconfig: %w", err)
	}

	talosClient, err := talosclient.New(ctx, talosclient.WithConfig(talosConfig))
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return talosClient, nil
}

// GetNodeInfo retrieves detailed information about a Talos node
func (s *TalosClient) GetNodeInfo(ctx context.Context, nodeIP string) (*NodeInfo, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Getting node info", "nodeIP", nodeIP)

	// Use the existing client with proper node targeting
	talosClient, err := s.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get talos client: %w", err)
	}
	defer talosClient.Close()

	// Target the specific node using WithNodes
	nodeCtx := talosclient.WithNodes(ctx, nodeIP)

	// Get version info
	resp, err := talosClient.Version(nodeCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get node info from %s: %w", nodeIP, err)
	}

	if len(resp.Messages) == 0 {
		return nil, fmt.Errorf("no response from node %s", nodeIP)
	}

	msg := resp.Messages[0]
	version := msg.GetVersion()
	if version == nil {
		return nil, fmt.Errorf("version is nil for node %s", nodeIP)
	}

	nodeInfo := &NodeInfo{
		NodeIP:       nodeIP,
		TalosVersion: version.GetTag(),
	}

	// Handle platform info
	if platform := msg.GetPlatform(); platform != nil {
		nodeInfo.Platform = platform.GetName()
	}

	// For COSI operations, we need a direct connection (COSI doesn't support proxying)
	// Only create direct connection if we need machine config
	configData, err := s.loadTalosConfig(ctx)
	if err != nil {
		logger.V(1).Info("Failed to load config for direct connection, continuing without machine config", "nodeIP", nodeIP)
		return nodeInfo, nil
	}

	talosConfig, err := clientconfig.FromBytes(configData)
	if err != nil {
		logger.V(1).Info("Failed to parse config for direct connection, continuing without machine config", "nodeIP", nodeIP)
		return nodeInfo, nil
	}

	// Create direct connection for COSI
	originalContext := talosConfig.Contexts[talosConfig.Context]
	directConfig := &clientconfig.Config{
		Context:  talosConfig.Context,
		Contexts: make(map[string]*clientconfig.Context),
	}
	directConfig.Contexts[directConfig.Context] = &clientconfig.Context{
		Endpoints: []string{nodeIP},
		CA:        originalContext.CA,
		Crt:       originalContext.Crt,
		Key:       originalContext.Key,
	}

	directClient, err := talosclient.New(ctx, talosclient.WithConfig(directConfig))
	if err != nil {
		logger.V(1).Info("Failed to create direct client for COSI, continuing without machine config", "nodeIP", nodeIP)
		return nodeInfo, nil
	}
	defer directClient.Close()

	// Get machine config using direct connection
	r, err := directClient.COSI.Get(ctx, resource.NewMetadata("config", "MachineConfigs.config.talos.dev", "v1alpha1", resource.VersionUndefined))
	if err != nil {
		logger.V(1).Info("Failed to get machine config, continuing without it", "nodeIP", nodeIP, "error", err)
		return nodeInfo, nil
	}

	mc, ok := r.(*config.MachineConfig)
	if !ok {
		logger.V(1).Info("Unexpected resource type for machine config", "nodeIP", nodeIP)
		return nodeInfo, nil
	}

	// Extract install image
	if mcImage := mc.Config().Machine().Install().Image(); mcImage != "" {
		nodeInfo.Image = mcImage
		logger.V(1).Info("Install image found", "nodeIP", nodeIP, "image", mcImage)
	} else {
		logger.V(1).Info("Install image is empty", "nodeIP", nodeIP)
	}

	// Get Kubernetes version from kubelet image
	if kubeletImage := mc.Config().Machine().Kubelet().Image(); kubeletImage != "" {
		if kubeletRef, err := parseImageReference(kubeletImage); err == nil {
			nodeInfo.KubernetesVersion = kubeletRef.Tag()
		}
	}

	logger.V(1).Info("Node info collection complete", "nodeInfo", nodeInfo.String())
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
	_, err := s.GetClient(ctx)
	return err == nil
}
