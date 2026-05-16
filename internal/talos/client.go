package talos

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/siderolabs/go-retry/retry"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	"github.com/siderolabs/talos/pkg/machinery/config/configpatcher"
	"github.com/siderolabs/talos/pkg/machinery/resources/config"
	talosruntime "github.com/siderolabs/talos/pkg/machinery/resources/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type talosClient interface {
	Version(ctx context.Context, opts ...grpc.CallOption) (*machine.VersionResponse, error)
	COSIGet(ctx context.Context, namespace, typ, id string) (resource.Resource, error)
	COSIList(ctx context.Context, namespace, typ string) ([]resource.Resource, error)
	ApplyConfiguration(ctx context.Context, req *machine.ApplyConfigurationRequest, opts ...grpc.CallOption) (*machine.ApplyConfigurationResponse, error)
	Close() error
}

type realTalosClient struct {
	*client.Client
}

func (r *realTalosClient) COSIGet(ctx context.Context, namespace, typ, id string) (resource.Resource, error) {
	md := resource.NewMetadata(namespace, typ, id, resource.VersionUndefined)
	return r.COSI.Get(ctx, md)
}

func (r *realTalosClient) COSIList(ctx context.Context, namespace, typ string) ([]resource.Resource, error) {
	kind := resource.NewMetadata(namespace, typ, "", resource.VersionUndefined)
	list, err := r.COSI.List(ctx, kind)
	if err != nil {
		return nil, err
	}
	return list.Items, nil
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

type ExtensionInfo struct {
	Schematic  string
	Extensions []string
}

func (s *Client) GetNodeExtensions(ctx context.Context, nodeIP string) (ExtensionInfo, error) {
	nodeCtx := client.WithNode(ctx, nodeIP)
	var items []resource.Resource

	err := s.executeWithRetry(ctx, func() error {
		var err error
		items, err = s.talos.COSIList(nodeCtx, talosruntime.NamespaceName, talosruntime.ExtensionStatusType)
		return err
	})
	if err != nil {
		return ExtensionInfo{}, fmt.Errorf("failed to list extensions from node %s: %w", nodeIP, err)
	}

	var info ExtensionInfo
	for _, r := range items {
		es, ok := r.(*talosruntime.ExtensionStatus)
		if !ok {
			continue
		}
		name := es.TypedSpec().Metadata.Name
		if name == "schematic" {
			info.Schematic = es.TypedSpec().Metadata.Version
			continue
		}
		info.Extensions = append(info.Extensions, name)
	}
	return info, nil
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

func (s *Client) PatchNodeInstallImage(ctx context.Context, nodeIP, newImage string) error {
	mc, err := s.GetNodeMachineConfig(ctx, nodeIP)
	if err != nil {
		return fmt.Errorf("failed to patch install image on node %s: %w", nodeIP, err)
	}

	// JSON6902 patches reject multi-document configs; strategic merge does not.
	patchYAML := fmt.Sprintf("version: v1alpha1\nmachine:\n  install:\n    image: %q\n", newImage)

	patchProvider, err := configloader.NewFromBytes([]byte(patchYAML))
	if err != nil {
		return fmt.Errorf("failed to load config patch: %w", err)
	}

	patch := configpatcher.NewStrategicMergePatch(patchProvider)

	output, err := configpatcher.Apply(configpatcher.WithConfig(mc.Provider()), []configpatcher.Patch{patch})
	if err != nil {
		return fmt.Errorf("failed to apply config patch: %w", err)
	}

	patchedBytes, err := output.Bytes()
	if err != nil {
		return fmt.Errorf("failed to serialize patched config: %w", err)
	}

	nodeCtx := client.WithNode(ctx, nodeIP)

	err = s.executeWithRetry(ctx, func() error {
		_, err := s.talos.ApplyConfiguration(nodeCtx, &machine.ApplyConfigurationRequest{
			Data: patchedBytes,
			Mode: machine.ApplyConfigurationRequest_NO_REBOOT,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to apply configuration to node %s: %w", nodeIP, err)
	}

	return nil
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
		err := operation()
		if err == nil {
			return nil
		}
		if !IsTransientError(err) {
			return err
		}
		if refreshErr := s.refreshTalosClient(ctx); refreshErr != nil {
			return retry.ExpectedError(refreshErr)
		}
		return retry.ExpectedError(err)
	})
}

func IsTransientError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded:
			return true
		default:
			return false
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ETIMEDOUT, syscall.EPIPE:
			return true
		}
	}

	errStr := strings.ToLower(err.Error())
	for _, indicator := range []string{
		"connection refused",
		"connection reset",
		"i/o timeout",
		"eof",
	} {
		if strings.Contains(errStr, indicator) {
			return true
		}
	}

	return false
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
