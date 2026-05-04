package integration

import (
	"context"
	"fmt"
	"sync"

	talosconfigresource "github.com/siderolabs/talos/pkg/machinery/resources/config"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

// mockTalosClient implements TalosClient interface for testing
type mockTalosClient struct {
	mu             sync.RWMutex
	nodeVersions   map[string]string
	installImages  map[string]string
	platforms      map[string]string
	waitReadyErr   error
	getVersionErr  error
	getInstallErr  error
	getPlatformErr error
}

func (m *mockTalosClient) GetNodeVersion(ctx context.Context, nodeIP string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getVersionErr != nil {
		return "", m.getVersionErr
	}
	if v, ok := m.nodeVersions[nodeIP]; ok {
		return v, nil
	}
	return "", fmt.Errorf("node %s not found", nodeIP)
}

func (m *mockTalosClient) SetNodeVersion(nodeIP, version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodeVersions[nodeIP] = version
}

func (m *mockTalosClient) CheckNodeReady(ctx context.Context, nodeIP, nodeName string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.waitReadyErr
}

func (m *mockTalosClient) GetNodeInstallImage(ctx context.Context, nodeIP string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getInstallErr != nil {
		return "", m.getInstallErr
	}
	if img, ok := m.installImages[nodeIP]; ok {
		return img, nil
	}
	return "", fmt.Errorf("install image not found for %s", nodeIP)
}

func (m *mockTalosClient) GetNodePlatform(ctx context.Context, nodeIP string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getPlatformErr != nil {
		return "", m.getPlatformErr
	}
	if p, ok := m.platforms[nodeIP]; ok {
		return p, nil
	}
	return "", fmt.Errorf("platform not found for %s", nodeIP)
}

func (m *mockTalosClient) SetNodePlatform(nodeIP, platform string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.platforms == nil {
		m.platforms = make(map[string]string)
	}
	m.platforms[nodeIP] = platform
}

func (m *mockTalosClient) SetNodeInstallImage(nodeIP, image string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.installImages[nodeIP] = image
}

func (m *mockTalosClient) PatchNodeInstallImage(ctx context.Context, nodeIP, newImage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.installImages[nodeIP] = newImage
	return nil
}

func (m *mockTalosClient) GetNodeMachineConfig(
	ctx context.Context, nodeIP string,
) (*talosconfigresource.MachineConfig, error) {
	return nil, fmt.Errorf("machine config not configured for %s", nodeIP)
}

func (m *mockTalosClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodeVersions = make(map[string]string)
	m.installImages = make(map[string]string)
	m.platforms = make(map[string]string)
	m.getVersionErr = nil
	m.getInstallErr = nil
	m.getPlatformErr = nil
}

// mockHealthChecker implements HealthCheckRunner interface for testing
type mockHealthChecker struct {
	mu  sync.RWMutex
	err error
}

func (m *mockHealthChecker) CheckHealth(ctx context.Context, healthChecks []tupprv1alpha1.HealthCheckSpec) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.err
}

func (m *mockHealthChecker) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// mockVersionGetter implements VersionGetter interface for testing
type mockVersionGetter struct {
	mu      sync.RWMutex
	version string
	err     error
}

func (m *mockVersionGetter) GetCurrentKubernetesVersion(ctx context.Context) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version, m.err
}

func (m *mockVersionGetter) SetVersion(version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.version = version
}

func (m *mockVersionGetter) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}
