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
	return &TalosClient{
		talos: talosClient,
	}, nil
}

// refreshTalosClient recreates the client if the current one is stale
func (s *TalosClient) refreshTalosClient(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Checking if Talos client needs refresh")

	if _, err := s.talos.Version(ctx); err != nil {
		logger.Info("Talos client is stale, refreshing", "error", err.Error())

		newClient, err := NewTalosClient(ctx)
		if err != nil {
			logger.Error(err, "Failed to reinitialize Talos client")
			return fmt.Errorf("failed to reinitialize talos client: %w", err)
		}

		s.talos.Close() //nolint:errcheck
		s.talos = newClient.talos
		logger.Info("Successfully refreshed Talos client")
	} else {
		logger.V(2).Info("Talos client is healthy, no refresh needed")
	}

	return nil
}

// GetNodeVersion retrieves the Talos version from a specific node
func (s *TalosClient) GetNodeVersion(ctx context.Context, nodeIP string) (string, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Getting node version from Talos", "nodeIP", nodeIP)

	nodeCtx := client.WithNode(ctx, nodeIP)
	var resp *machine.VersionResponse
	var retryCount int

	err := retry.Constant(10*time.Second, retry.WithUnits(100*time.Millisecond)).Retry(func() error {
		retryCount++
		logger.V(2).Info("Attempting to get version from node", "nodeIP", nodeIP, "attempt", retryCount)

		var versionErr error
		resp, versionErr = s.talos.Version(nodeCtx)
		if versionErr != nil {
			logger.V(1).Info("Version request failed, refreshing client", "nodeIP", nodeIP, "error", versionErr.Error())
			err := s.refreshTalosClient(ctx) //nolint:errcheck
			if err != nil {
				logger.Error(err, "Failed to refresh client during version retrieval", "nodeIP", nodeIP)
				return retry.ExpectedError(err)
			}
			return versionErr
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "Failed to get node version after retries", "nodeIP", nodeIP, "attempts", retryCount)
		return "", fmt.Errorf("failed to get node version from %s: %w", nodeIP, err)
	}

	if len(resp.Messages) == 0 {
		logger.Error(nil, "No response messages from node", "nodeIP", nodeIP)
		return "", fmt.Errorf("no response from node %s", nodeIP)
	}

	version := resp.Messages[0].GetVersion()
	if version == nil {
		logger.Error(nil, "Version is nil in response", "nodeIP", nodeIP)
		return "", fmt.Errorf("version is nil for node %s", nodeIP)
	}

	versionTag := version.GetTag()
	logger.Info("Successfully retrieved node version", "nodeIP", nodeIP, "version", versionTag, "attempts", retryCount)
	return versionTag, nil
}

// GetNodeMachineConfig retrieves the machine configuration from a specific node
func (s *TalosClient) GetNodeMachineConfig(ctx context.Context, nodeIP string) (*config.MachineConfig, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Getting machine config from node", "nodeIP", nodeIP)

	nodeCtx := client.WithNode(ctx, nodeIP)
	var r resource.Resource
	var retryCount int

	err := retry.Constant(10*time.Second, retry.WithUnits(100*time.Millisecond)).Retry(func() error {
		retryCount++
		logger.V(2).Info("Attempting to get machine config", "nodeIP", nodeIP, "attempt", retryCount)

		var getErr error
		r, getErr = s.talos.COSI.Get(nodeCtx, resource.NewMetadata("config", "MachineConfigs.config.talos.dev", "v1alpha1", resource.VersionUndefined))
		if getErr != nil {
			logger.V(1).Info("Machine config request failed, refreshing client", "nodeIP", nodeIP, "error", getErr.Error())
			err := s.refreshTalosClient(ctx) //nolint:errcheck
			if err != nil {
				logger.Error(err, "Failed to refresh client during machine config retrieval", "nodeIP", nodeIP)
				return retry.ExpectedError(err)
			}
			return getErr
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "Failed to get machine config after retries", "nodeIP", nodeIP, "attempts", retryCount)
		return nil, fmt.Errorf("failed to get machine config from node %s: %w", nodeIP, err)
	}

	mc, ok := r.(*config.MachineConfig)
	if !ok {
		logger.Error(nil, "Unexpected resource type for machine config", "nodeIP", nodeIP, "type", fmt.Sprintf("%T", r))
		return nil, fmt.Errorf("unexpected resource type for machine config from node %s", nodeIP)
	}

	logger.V(1).Info("Successfully retrieved machine config", "nodeIP", nodeIP, "attempts", retryCount)
	return mc, nil
}

// GetNodeInstallImage retrieves the install image from a specific node's machine config
func (s *TalosClient) GetNodeInstallImage(ctx context.Context, nodeIP string) (string, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Getting install image from node", "nodeIP", nodeIP)

	mc, err := s.GetNodeMachineConfig(ctx, nodeIP)
	if err != nil {
		logger.Error(err, "Failed to get machine config for install image", "nodeIP", nodeIP)
		return "", err
	}

	image := mc.Config().Machine().Install().Image()
	if image == "" {
		logger.Error(nil, "Install image is empty", "nodeIP", nodeIP)
		return "", fmt.Errorf("install image is empty for node %s", nodeIP)
	}

	logger.Info("Successfully retrieved install image", "nodeIP", nodeIP, "image", image)
	return image, nil
}

// WaitForNodeReady waits for a Talos node to be ready after an upgrade
func (s *TalosClient) WaitForNodeReady(ctx context.Context, nodeIP, nodeName string) error {
	logger := log.FromContext(ctx)

	maxRetries := 30                  // 30 retries
	retryInterval := 30 * time.Second // 30 seconds between retries = 15 minutes total
	totalTimeout := time.Duration(maxRetries) * retryInterval

	logger.Info("Starting wait for Talos node readiness after upgrade",
		"node", nodeName,
		"nodeIP", nodeIP,
		"maxRetries", maxRetries,
		"retryInterval", retryInterval,
		"totalTimeout", totalTimeout)

	startTime := time.Now()

	for i := 0; i < maxRetries; i++ {
		elapsed := time.Since(startTime)
		remaining := totalTimeout - elapsed

		// Check if the Talos API is responding with basic health
		if err := s.checkNodeAPIReady(ctx, nodeIP); err != nil {
			logger.V(1).Info("Talos API not ready yet, retrying",
				"node", nodeName,
				"nodeIP", nodeIP,
				"attempt", i+1,
				"maxRetries", maxRetries,
				"elapsed", elapsed.Round(time.Second),
				"remaining", remaining.Round(time.Second),
				"error", err.Error())

			if i == maxRetries-1 {
				logger.Error(err, "Talos API failed to become ready within timeout",
					"node", nodeName,
					"nodeIP", nodeIP,
					"totalAttempts", maxRetries,
					"totalElapsed", elapsed.Round(time.Second))
				return fmt.Errorf("talos API not ready after %d attempts for node %s: %w", maxRetries, nodeName, err)
			}

			time.Sleep(retryInterval)
			continue
		}

		logger.V(1).Info("Talos API is ready, checking services", "node", nodeName, "nodeIP", nodeIP, "attempt", i+1)

		// API is ready, now check if essential services are running
		if err := s.checkNodeServicesReady(ctx, nodeIP); err != nil {
			logger.V(1).Info("Talos services not ready yet, retrying",
				"node", nodeName,
				"nodeIP", nodeIP,
				"attempt", i+1,
				"maxRetries", maxRetries,
				"elapsed", elapsed.Round(time.Second),
				"remaining", remaining.Round(time.Second),
				"error", err.Error())

			if i == maxRetries-1 {
				logger.Error(err, "Talos services failed to become ready within timeout",
					"node", nodeName,
					"nodeIP", nodeIP,
					"totalAttempts", maxRetries,
					"totalElapsed", elapsed.Round(time.Second))
				return fmt.Errorf("talos services not ready after %d attempts for node %s: %w", maxRetries, nodeName, err)
			}

			time.Sleep(retryInterval)
			continue
		}

		finalElapsed := time.Since(startTime)
		logger.Info("Talos node is ready after upgrade",
			"node", nodeName,
			"nodeIP", nodeIP,
			"attempts", i+1,
			"totalElapsed", finalElapsed.Round(time.Second))
		return nil
	}

	return fmt.Errorf("timeout waiting for Talos node %s to be ready", nodeName)
}

// checkNodeAPIReady checks if the Talos API is responding with basic connectivity
func (s *TalosClient) checkNodeAPIReady(ctx context.Context, nodeIP string) error {
	logger := log.FromContext(ctx)
	logger.V(2).Info("Checking Talos API readiness", "nodeIP", nodeIP)

	nodeCtx := client.WithNode(ctx, nodeIP)

	// Use a shorter timeout for readiness checks
	checkCtx, cancel := context.WithTimeout(nodeCtx, 10*time.Second)
	defer cancel()

	// Simple version check to see if API is responding
	_, err := s.talos.Version(checkCtx)
	if err != nil {
		logger.V(2).Info("API check failed, attempting client refresh", "nodeIP", nodeIP, "error", err.Error())
		// Try to refresh the client on error
		if refreshErr := s.refreshTalosClient(ctx); refreshErr != nil {
			logger.V(1).Info("Both API check and client refresh failed", "nodeIP", nodeIP, "apiError", err.Error(), "refreshError", refreshErr.Error())
			return fmt.Errorf("API check failed and client refresh failed: %w", err)
		}
		return fmt.Errorf("API not ready: %w", err)
	}

	logger.V(2).Info("Talos API is responding", "nodeIP", nodeIP)
	return nil
}

// checkNodeServicesReady checks if essential Talos services are running
func (s *TalosClient) checkNodeServicesReady(ctx context.Context, nodeIP string) error {
	logger := log.FromContext(ctx)
	logger.V(2).Info("Checking Talos services readiness", "nodeIP", nodeIP)

	// Check if we can get machine config (indicates node is fully configured)
	logger.V(2).Info("Checking machine config availability", "nodeIP", nodeIP)
	if _, err := s.GetNodeMachineConfig(ctx, nodeIP); err != nil {
		logger.V(2).Info("Machine config not available", "nodeIP", nodeIP, "error", err.Error())
		return fmt.Errorf("machine config not available: %w", err)
	}

	// Check if we can get install image (indicates node configuration is accessible)
	logger.V(2).Info("Checking install image availability", "nodeIP", nodeIP)
	if _, err := s.GetNodeInstallImage(ctx, nodeIP); err != nil {
		logger.V(2).Info("Install image not available", "nodeIP", nodeIP, "error", err.Error())
		return fmt.Errorf("install image not available: %w", err)
	}

	logger.V(2).Info("All Talos services are ready", "nodeIP", nodeIP)
	return nil
}
