package controller

import (
	"context"
	"fmt"

	"github.com/distribution/reference"
	talosclient "github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/resources/config"

	"github.com/cosi-project/runtime/pkg/resource"
)

// TalosClient provides Talos SDK operations for gathering node information
type TalosClient struct {
	talos *talosclient.Client
}

// NewTalosClient creates a new Talos client service using the mounted configuration
func NewTalosClient(ctx context.Context) (*TalosClient, error) {
	talosClient, err := talosclient.New(ctx, talosclient.WithDefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}

	return &TalosClient{
		talos: talosClient,
	}, nil
}

// Close closes the underlying Talos client
func (s *TalosClient) Close() error {
	if s.talos != nil {
		return s.talos.Close()
	}
	return nil
}

// GetNodeVersion retrieves the Talos version from a specific node
func (s *TalosClient) GetNodeVersion(ctx context.Context, nodeIP string) (string, error) {
	nodeCtx := talosclient.WithNodes(ctx, nodeIP)

	resp, err := s.talos.Version(nodeCtx)
	if err != nil {
		return "", fmt.Errorf("failed to get node version from %s: %w", nodeIP, err)
	}

	if len(resp.Messages) == 0 {
		return "", fmt.Errorf("no response from node %s", nodeIP)
	}

	version := resp.Messages[0].GetVersion()
	if version == nil {
		return "", fmt.Errorf("version is nil for node %s", nodeIP)
	}

	return version.GetTag(), nil
}

// GetNodeMachineConfig retrieves the machine configuration from a specific node
func (s *TalosClient) GetNodeMachineConfig(ctx context.Context, nodeIP string) (*config.MachineConfig, error) {
	// Create a direct client to the node for COSI operations
	directClient, err := talosclient.New(ctx,
		talosclient.WithDefaultConfig(),
		talosclient.WithEndpoints(nodeIP),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create direct client for node %s: %w", nodeIP, err)
	}
	defer directClient.Close() //nolint:errcheck

	// Get machine config using direct connection
	r, err := directClient.COSI.Get(ctx, resource.NewMetadata("config", "MachineConfigs.config.talos.dev", "v1alpha1", resource.VersionUndefined))
	if err != nil {
		return nil, fmt.Errorf("failed to get machine config from node %s: %w", nodeIP, err)
	}

	mc, ok := r.(*config.MachineConfig)
	if !ok {
		return nil, fmt.Errorf("unexpected resource type for machine config from node %s", nodeIP)
	}

	return mc, nil
}

// GetNodeInstallImage retrieves the install image from a specific node's machine config
func (s *TalosClient) GetNodeInstallImage(ctx context.Context, nodeIP string) (string, error) {
	mc, err := s.GetNodeMachineConfig(ctx, nodeIP)
	if err != nil {
		return "", err
	}

	image := mc.Config().Machine().Install().Image()
	if image == "" {
		return "", fmt.Errorf("install image is empty for node %s", nodeIP)
	}

	return image, nil
}

// GetNodeKubernetesVersion retrieves the Kubernetes version from a specific node's machine config
func (s *TalosClient) GetNodeKubernetesVersion(ctx context.Context, nodeIP string) (string, error) {
	mc, err := s.GetNodeMachineConfig(ctx, nodeIP)
	if err != nil {
		return "", err
	}

	kubeletImage := mc.Config().Machine().Kubelet().Image()
	if kubeletImage == "" {
		return "", fmt.Errorf("kubelet image is empty for node %s", nodeIP)
	}

	kubeletRef, err := parseImageReference(kubeletImage)
	if err != nil {
		return "", fmt.Errorf("failed to parse kubelet image %s: %w", kubeletImage, err)
	}

	return kubeletRef.Tag(), nil
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
