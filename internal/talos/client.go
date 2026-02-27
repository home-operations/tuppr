package talos

import (
	"context"
	"fmt"
	"time"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/siderolabs/go-retry/retry"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/resources/config"
	"google.golang.org/grpc"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type talosClient interface {
	Version(ctx context.Context, opts ...grpc.CallOption) (*machine.VersionResponse, error)
	COSIGet(ctx context.Context, namespace, typ, id string) (resource.Resource, error)
	Close() error
}

type realTalosClient struct {
	*client.Client
}

func (r *realTalosClient) COSIGet(ctx context.Context, namespace, typ, id string) (resource.Resource, error) {
	md := resource.NewMetadata(namespace, typ, id, resource.VersionUndefined)
	return r.COSI.Get(ctx, md)
}

type Client struct {
	talos         talosClient
	newClientFunc func(ctx context.Context) (talosClient, error)
}

type ClientOption func(*Client)

func WithNewClientFunc(fn func(ctx context.Context) (talosClient, error)) ClientOption {
	return func(c *Client) {
		c.newClientFunc = fn
	}
}

func NewClient(ctx context.Context, opts ...ClientOption) (*Client, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Creating new Talos client")

	c := &Client{
		newClientFunc: defaultNewClient,
	}
	for _, opt := range opts {
		opt(c)
	}

	talosClient, err := c.newClientFunc(ctx)
	if err != nil {
		logger.Error(err, "Failed to create Talos client")
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}
	c.talos = talosClient

	logger.V(1).Info("Successfully created Talos client")
	return c, nil
}

func defaultNewClient(ctx context.Context) (talosClient, error) {
	c, err := client.New(ctx, client.WithDefaultConfig())
	if err != nil {
		return nil, err
	}
	return &realTalosClient{Client: c}, nil
}

func (s *Client) GetNodeVersion(ctx context.Context, nodeIP string) (string, error) {
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

func (s *Client) GetNodeMachineConfig(ctx context.Context, nodeIP string) (*config.MachineConfig, error) {
	nodeCtx := client.WithNode(ctx, nodeIP)
	var r resource.Resource

	err := s.executeWithRetry(ctx, func() error {
		var err error
		r, err = s.talos.COSIGet(nodeCtx, "config", "MachineConfigs.config.talos.dev", "v1alpha1")
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

func (s *Client) GetNodeInstallImage(ctx context.Context, nodeIP string) (string, error) {
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

func (s *Client) CheckNodeReady(ctx context.Context, nodeIP, nodeName string) error {
	logger := log.FromContext(ctx)

	logger.V(1).Info("Verifying Talos node readiness",
		"node", nodeName,
		"nodeIP", nodeIP,
	)

	if err := s.checkNodeReady(ctx, nodeIP); err != nil {
		return fmt.Errorf("node not ready: %w", err)
	}

	return nil
}

func (s *Client) refreshTalosClient(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.V(2).Info("Refreshing Talos client")

	newClient, err := s.newClientFunc(ctx)
	if err != nil {
		return fmt.Errorf("failed to reinitialize talos client: %w", err)
	}

	err = s.talos.Close()
	if err != nil {
		return err
	}
	s.talos = newClient
	return nil
}

func (s *Client) executeWithRetry(ctx context.Context, operation func() error) error {
	return retry.Constant(10*time.Second, retry.WithUnits(100*time.Millisecond)).Retry(func() error {
		if err := operation(); err != nil {
			if refreshErr := s.refreshTalosClient(ctx); refreshErr != nil {
				return retry.ExpectedError(refreshErr)
			}
			return retry.ExpectedError(err)
		}
		return nil
	})
}

func (s *Client) checkNodeReady(ctx context.Context, nodeIP string) error {
	nodeCtx := client.WithNode(ctx, nodeIP)
	checkCtx, cancel := context.WithTimeout(nodeCtx, 10*time.Second)
	defer cancel()

	if _, err := s.talos.Version(checkCtx); err != nil {
		if refreshErr := s.refreshTalosClient(ctx); refreshErr != nil {
			return fmt.Errorf("API check failed and client refresh failed: %w", err)
		}
		return fmt.Errorf("API not ready: %w", err)
	}

	if _, err := s.GetNodeMachineConfig(ctx, nodeIP); err != nil {
		return fmt.Errorf("machine config not accessible: %w", err)
	}

	return nil
}
