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
	logger.V(1).Info("Creating fresh Talos client", "secret", s.talosConfigSecret)

	configData, err := s.loadTalosConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load talosconfig: %w", err)
	}

	talosClient, err := s.createTalosClient(ctx, configData)
	if err != nil {
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}

	logger.V(1).Info("Successfully created fresh Talos client", "secret", s.talosConfigSecret)
	return talosClient, nil
}

// loadTalosConfig loads the talosconfig from the Kubernetes secret
func (s *TalosClient) loadTalosConfig(ctx context.Context) ([]byte, error) {
	secret := &corev1.Secret{}
	if err := s.k8sClient.Get(ctx, types.NamespacedName{
		Name:      s.talosConfigSecret,
		Namespace: s.controllerNamespace,
	}, secret); err != nil {
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

// GetMachineConfig retrieves the machine config from a Talos node
func (s *TalosClient) GetMachineConfig(ctx context.Context, nodeIP string) (*config.MachineConfig, error) {
	// Create a node-specific client context FIRST
	nodeCtx := talosclient.WithNodes(ctx, nodeIP)

	// Load config and create client with node context
	configData, err := s.loadTalosConfig(ctx) // Use ctx, not nodeCtx here
	if err != nil {
		return nil, fmt.Errorf("failed to load talosconfig: %w", err)
	}

	talosClient, err := s.createTalosClient(nodeCtx, configData)
	if err != nil {
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() {
		if closeErr := talosClient.Close(); closeErr != nil {
			log.FromContext(ctx).V(1).Info("Failed to close Talos client", "error", closeErr)
		}
	}()

	timeoutCtx, cancel := context.WithTimeout(nodeCtx, DefaultTimeout)
	defer cancel()

	r, err := talosClient.COSI.Get(timeoutCtx, resource.NewMetadata("config", "MachineConfigs.config.talos.dev", "v1alpha1", resource.VersionUndefined))
	if err != nil {
		return nil, fmt.Errorf("failed to get machine config from node %s: %w", nodeIP, err)
	}

	mc, ok := r.(*config.MachineConfig)
	if !ok {
		return nil, fmt.Errorf("unexpected resource type for machine config from node %s", nodeIP)
	}

	return mc, nil
}

// GetNodeInfo retrieves detailed information about a Talos node
func (s *TalosClient) GetNodeInfo(ctx context.Context, nodeIP string) (*NodeInfo, error) {
	// Create a node-specific client context FIRST
	nodeCtx := talosclient.WithNodes(ctx, nodeIP)

	// Load config and create client with node context
	configData, err := s.loadTalosConfig(ctx) // Fix: use ctx, not nodeCtx
	if err != nil {
		return nil, fmt.Errorf("failed to load talosconfig: %w", err)
	}

	talosClient, err := s.createTalosClient(nodeCtx, configData)
	if err != nil {
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() {
		if closeErr := talosClient.Close(); closeErr != nil {
			log.FromContext(ctx).V(1).Info("Failed to close Talos client", "error", closeErr)
		}
	}()

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

	// Safely handle platform info
	if platform := msg.GetPlatform(); platform != nil {
		nodeInfo.Platform = platform.GetName()
	}

	// Get machine config using the SAME client instead of calling GetMachineConfig
	timeoutCtx, cancel := context.WithTimeout(nodeCtx, DefaultTimeout)
	defer cancel()

	r, err := talosClient.COSI.Get(timeoutCtx, resource.NewMetadata("config", "MachineConfigs.config.talos.dev", "v1alpha1", resource.VersionUndefined))
	if err != nil {
		// Log error but don't fail the entire operation
		log.FromContext(ctx).V(1).Info("Failed to get machine config", "node", nodeIP, "error", err)
		return nodeInfo, nil
	}

	mc, ok := r.(*config.MachineConfig)
	if !ok {
		log.FromContext(ctx).V(1).Info("Unexpected resource type for machine config", "node", nodeIP)
		return nodeInfo, nil
	}

	// Extract install image and schematic
	if mcImage := mc.Config().Machine().Install().Image(); mcImage != "" {
		nodeInfo.Image = mcImage
	}

	// Get Kubernetes version from kubelet image
	if kubeletImage := mc.Config().Machine().Kubelet().Image(); kubeletImage != "" {
		if kubeletRef, err := parseImageReference(kubeletImage); err == nil {
			nodeInfo.KubernetesVersion = kubeletRef.Tag()
		}
	}

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
