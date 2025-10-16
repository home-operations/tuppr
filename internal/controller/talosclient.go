package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/siderolabs/go-retry/retry"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/resources/config"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cosi-project/runtime/pkg/resource"
)

// TalosClient provides Talos SDK operations for gathering node information
type TalosClient struct {
	talos *client.Client
}

// NewTalosClient creates a new Talos client service using the mounted configuration
func NewTalosClient(ctx context.Context) (*TalosClient, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Creating new Talos client")

	talosClient, err := client.New(ctx, client.WithDefaultConfig())
	if err != nil {
		logger.Error(err, "Failed to create Talos client")
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}

	logger.V(1).Info("Successfully created Talos client")
	return &TalosClient{talos: talosClient}, nil
}

// GetNodeVersion retrieves the Talos version from a specific node
func (s *TalosClient) GetNodeVersion(ctx context.Context, nodeIP string) (string, error) {
	nodeCtx := client.WithNode(ctx, nodeIP)
	var resp *machine.VersionResponse

	err := s.executeWithRetry(ctx, func() error {
		var err error
		resp, err = s.talos.Version(nodeCtx)
		return err
	})

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
	nodeCtx := client.WithNode(ctx, nodeIP)
	var r resource.Resource

	err := s.executeWithRetry(ctx, func() error {
		var err error
		r, err = s.talos.COSI.Get(nodeCtx, resource.NewMetadata("config", "MachineConfigs.config.talos.dev", "v1alpha1", resource.VersionUndefined))
		return err
	})

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

// WaitForNodeReady waits for a Talos node to be ready after an upgrade
func (s *TalosClient) WaitForNodeReady(ctx context.Context, nodeIP, nodeName string) error {
	logger := log.FromContext(ctx)

	const (
		maxRetries    = 30
		retryInterval = 30 * time.Second
	)

	logger.V(1).Info("Waiting for Talos node readiness after upgrade",
		"node", nodeName,
		"nodeIP", nodeIP,
		"timeout", time.Duration(maxRetries)*retryInterval)

	startTime := time.Now()

	for i := range maxRetries {
		if err := s.checkNodeReady(ctx, nodeIP); err != nil {
			if i == maxRetries-1 {
				elapsed := time.Since(startTime)
				logger.Error(err, "Talos node failed to become ready within timeout",
					"node", nodeName,
					"nodeIP", nodeIP,
					"totalAttempts", maxRetries,
					"totalElapsed", elapsed.Round(time.Second))
				return fmt.Errorf("talos node not ready after %d attempts for node %s: %w", maxRetries, nodeName, err)
			}
			time.Sleep(retryInterval)
			continue
		}

		elapsed := time.Since(startTime)
		logger.V(1).Info("Talos node is ready after upgrade",
			"node", nodeName,
			"nodeIP", nodeIP,
			"attempts", i+1,
			"totalElapsed", elapsed.Round(time.Second))
		return nil
	}

	return fmt.Errorf("timeout waiting for Talos node %s to be ready", nodeName)
}

// refreshTalosClient recreates the client if the current one is stale
func (s *TalosClient) refreshTalosClient(ctx context.Context) error {
	if _, err := s.talos.Version(ctx); err != nil {
		logger := log.FromContext(ctx)
		logger.V(2).Info("Refreshing stale Talos client")

		newClient, err := NewTalosClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to reinitialize talos client: %w", err)
		}

		s.talos.Close() //nolint:errcheck
		s.talos = newClient.talos
	}
	return nil
}

// executeWithRetry executes a function with retry logic and client refresh
func (s *TalosClient) executeWithRetry(ctx context.Context, operation func() error) error {
	return retry.Constant(10*time.Second, retry.WithUnits(100*time.Millisecond)).Retry(func() error {
		if err := operation(); err != nil {
			if refreshErr := s.refreshTalosClient(ctx); refreshErr != nil {
				return retry.ExpectedError(refreshErr)
			}
			return err
		}
		return nil
	})
}

// checkNodeReady performs comprehensive readiness checks
func (s *TalosClient) checkNodeReady(ctx context.Context, nodeIP string) error {
	nodeCtx := client.WithNode(ctx, nodeIP)
	checkCtx, cancel := context.WithTimeout(nodeCtx, 10*time.Second)
	defer cancel()

	// Check API connectivity
	if _, err := s.talos.Version(checkCtx); err != nil {
		if refreshErr := s.refreshTalosClient(ctx); refreshErr != nil {
			return fmt.Errorf("API check failed and client refresh failed: %w", err)
		}
		return fmt.Errorf("API not ready: %w", err)
	}

	// Verify machine config is accessible
	if _, err := s.GetNodeMachineConfig(ctx, nodeIP); err != nil {
		return fmt.Errorf("machine config not accessible: %w", err)
	}

	return nil
}
