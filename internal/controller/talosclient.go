package controller

import (
	"bytes"
	"context"
	"fmt"
	"sync"
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

	// Cached client and mutex for thread safety
	mu          sync.RWMutex
	talosClient *talosclient.Client
	configData  []byte
	lastRefresh time.Time
}

// NewTalosClient creates a new Talos client service
func NewTalosClient(k8sClient ctrlclient.Client, talosConfigSecret, controllerNamespace string) *TalosClient {
	return &TalosClient{
		k8sClient:           k8sClient,
		talosConfigSecret:   talosConfigSecret,
		controllerNamespace: controllerNamespace,
	}
}

// GetClient returns a Talos client, creating or refreshing it as needed
func (s *TalosClient) GetClient(ctx context.Context) (*talosclient.Client, error) {
	logger := log.FromContext(ctx)

	s.mu.RLock()
	// Check if we have a valid cached client
	if s.talosClient != nil && time.Since(s.lastRefresh) < ClientRefreshInterval {
		defer s.mu.RUnlock()
		return s.talosClient, nil
	}
	s.mu.RUnlock()

	// Need to create or refresh client
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if s.talosClient != nil && time.Since(s.lastRefresh) < ClientRefreshInterval {
		return s.talosClient, nil
	}

	logger.V(1).Info("Creating/refreshing Talos client", "secret", s.talosConfigSecret)

	configData, err := s.loadTalosConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load talosconfig: %w", err)
	}

	// Check if config has changed
	if s.configData != nil && bytes.Equal(s.configData, configData) && s.talosClient != nil {
		s.lastRefresh = time.Now()
		return s.talosClient, nil
	}

	// Close old client if exists
	if s.talosClient != nil {
		if err := s.talosClient.Close(); err != nil {
			logger.V(1).Info("Error closing old Talos client", "error", err)
		}
	}

	// Create new client
	talosClient, err := s.createTalosClient(ctx, configData)
	if err != nil {
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}

	// Cache the new client and config
	s.talosClient = talosClient
	s.configData = make([]byte, len(configData))
	copy(s.configData, configData)
	s.lastRefresh = time.Now()

	logger.Info("Successfully created Talos client", "secret", s.talosConfigSecret)
	return s.talosClient, nil
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

// Close closes the Talos client connection
func (s *TalosClient) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.talosClient != nil {
		err := s.talosClient.Close()
		s.talosClient = nil
		s.configData = nil
		return err
	}
	return nil
}

// GetMachineConfig retrieves the machine config from a Talos node
func (s *TalosClient) GetMachineConfig(ctx context.Context, nodeIP string) (*config.MachineConfig, error) {
	talosClient, err := s.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get talos client: %w", err)
	}

	nodeCtx := talosclient.WithNodes(ctx, nodeIP)
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
	talosClient, err := s.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get talos client: %w", err)
	}

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

	// Safely handle platform info
	if platform := msg.GetPlatform(); platform != nil {
		nodeInfo.Platform = platform.GetName()
	}

	// Get machine config to extract additional info
	mc, err := s.GetMachineConfig(ctx, nodeIP)
	if err != nil {
		// Log error but don't fail the entire operation
		log.FromContext(ctx).V(1).Info("Failed to get machine config", "node", nodeIP, "error", err)
		return nodeInfo, nil
	}

	// Extract install image and schematic - same pattern as working example
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
