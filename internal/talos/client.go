package talos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	cosiv1alpha1 "github.com/cosi-project/runtime/api/v1alpha1"
	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/siderolabs/go-retry/retry"
	"github.com/siderolabs/talos/pkg/machinery/api/common"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	configpb "github.com/siderolabs/talos/pkg/machinery/api/resource/config"
	"github.com/siderolabs/talos/pkg/machinery/client"
	talosruntime "github.com/siderolabs/talos/pkg/machinery/resources/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type talosClient interface {
	Version(ctx context.Context, opts ...grpc.CallOption) (*machine.VersionResponse, error)
	COSIGet(ctx context.Context, namespace, typ, id string) (resource.Resource, error)
	COSIList(ctx context.Context, namespace, typ string) ([]resource.Resource, error)
	ApplyConfiguration(ctx context.Context, req *machine.ApplyConfigurationRequest, opts ...grpc.CallOption) (*machine.ApplyConfigurationResponse, error)
	ImagePull(ctx context.Context, req *machine.ImageServicePullRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[machine.ImageServicePullResponse], error)
	COSIGetRawSpec(ctx context.Context, namespace, typ, id string) ([]byte, error)
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

func (r *realTalosClient) ImagePull(ctx context.Context, req *machine.ImageServicePullRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[machine.ImageServicePullResponse], error) {
	return r.ImageClient.Pull(ctx, req, opts...)
}

// COSIGetRawSpec fetches a resource over the raw COSI state API and returns its
// wire spec bytes untouched. The typed COSI client decodes specs through the
// resource registry, which for MachineConfig runs the whole config through the
// machinery decoder; the raw bytes carry no such requirement.
func (r *realTalosClient) COSIGetRawSpec(ctx context.Context, namespace, typ, id string) ([]byte, error) {
	resp, err := cosiv1alpha1.NewStateClient(r.Conn()).Get(ctx, &cosiv1alpha1.GetRequest{
		Namespace: namespace,
		Type:      typ,
		Id:        id,
	})
	if err != nil {
		return nil, err
	}

	return resp.GetResource().GetSpec().GetProtoSpec(), nil
}

type Client struct {
	talos            talosClient
	newClientFunc    func(ctx context.Context) (talosClient, error)
	endpointResolver func(ctx context.Context) []string
	pullIdleTimeout  time.Duration
}

type ClientOption func(*Client)

func WithNewClientFunc(fn func(ctx context.Context) (talosClient, error)) ClientOption {
	return func(c *Client) {
		c.newClientFunc = fn
	}
}

// WithEndpointResolver overrides the talosconfig endpoints with the IPs fn returns,
// so the client keeps reaching Talos when cluster DNS is drained mid-upgrade. An
// empty result falls back to the talosconfig.
func WithEndpointResolver(fn func(ctx context.Context) []string) ClientOption {
	return func(c *Client) {
		c.endpointResolver = fn
	}
}

func NewClient(ctx context.Context, opts ...ClientOption) (*Client, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Creating new Talos client")

	c := &Client{}
	for _, opt := range opts {
		opt(c)
	}
	if c.newClientFunc == nil {
		c.newClientFunc = c.defaultNewClient
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

func (c *Client) defaultNewClient(ctx context.Context) (talosClient, error) {
	opts := []client.OptionFunc{client.WithDefaultConfig()}
	if c.endpointResolver != nil {
		if endpoints := c.endpointResolver(ctx); len(endpoints) > 0 {
			opts = append(opts, client.WithEndpoints(endpoints...))
		}
	}

	cl, err := client.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &realTalosClient{Client: cl}, nil
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

// readMachineConfigRaw reads the node's machine config as the raw bytes inside
// the MachineConfig resource's wire spec, never decoding the documents: a
// config can carry kinds newer than any machinery this binary links, and the
// typed decoders hard-error on unknown kinds (see machineconfig.go).
func (s *Client) readMachineConfigRaw(ctx context.Context, nodeIP string) (string, error) {
	nodeCtx := client.WithNode(ctx, nodeIP)
	var protoBytes []byte

	err := s.executeWithRetry(ctx, func() error {
		var err error
		protoBytes, err = s.talos.COSIGetRawSpec(nodeCtx, "config", "MachineConfigs.config.talos.dev", "v1alpha1")
		return err
	})
	if err != nil {
		return "", fmt.Errorf("failed to get machine config from node %s: %w", nodeIP, err)
	}

	var spec configpb.MachineConfigSpec
	if err := proto.Unmarshal(protoBytes, &spec); err != nil {
		return "", fmt.Errorf("failed to unmarshal machine config spec from node %s: %w", nodeIP, err)
	}

	return string(spec.YamlMarshalled), nil
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

func (s *Client) GetNodePlatform(ctx context.Context, nodeIP string) (string, error) {
	nodeCtx := client.WithNode(ctx, nodeIP)
	var r resource.Resource

	err := s.executeWithRetry(ctx, func() error {
		var err error
		r, err = s.talos.COSIGet(nodeCtx, talosruntime.NamespaceName, talosruntime.PlatformMetadataType, talosruntime.PlatformMetadataID)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("failed to get platform metadata from node %s: %w", nodeIP, err)
	}

	platform, ok := r.(*talosruntime.PlatformMetadata)
	if !ok {
		return "", fmt.Errorf("unexpected resource type for platform metadata from node %s", nodeIP)
	}

	return platform.TypedSpec().Platform, nil
}

func (s *Client) GetNodeInstallImage(ctx context.Context, nodeIP string) (string, error) {
	raw, err := s.readMachineConfigRaw(ctx, nodeIP)
	if err != nil {
		return "", err
	}

	image, err := installImageFromConfig(raw)
	if err != nil {
		return "", fmt.Errorf("%w for node %s", err, nodeIP)
	}

	return image, nil
}

func (s *Client) PatchNodeInstallImage(ctx context.Context, nodeIP, newImage string) error {
	raw, err := s.readMachineConfigRaw(ctx, nodeIP)
	if err != nil {
		return fmt.Errorf("failed to patch install image on node %s: %w", nodeIP, err)
	}

	patched, err := setInstallImage(raw, newImage)
	if err != nil {
		return fmt.Errorf("failed to patch install image on node %s: %w", nodeIP, err)
	}

	nodeCtx := client.WithNode(ctx, nodeIP)

	err = s.executeWithRetry(ctx, func() error {
		_, err := s.talos.ApplyConfiguration(nodeCtx, &machine.ApplyConfigurationRequest{
			Data: []byte(patched),
			Mode: machine.ApplyConfigurationRequest_NO_REBOOT,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to apply configuration to node %s: %w", nodeIP, err)
	}

	return nil
}

// defaultPullIdleTimeout is how long a pull stream may go without progress
// before it is failed. Long enough for manifest resolution and slow layer
// starts; far shorter than the caller's overall pull budget.
const defaultPullIdleTimeout = 90 * time.Second

// pullLayerState is one layer's last observed pull progress.
type pullLayerState struct {
	status machine.ImageServicePullLayerProgress_Status
	offset int64
	total  int64
}

// pullProgress accumulates per-layer state from the pull stream so stall
// detection and error messages can report how far a pull got.
type pullProgress struct {
	layers map[string]pullLayerState
}

// observe folds one stream message in and reports whether it advanced the
// pull: a new layer, a status change, or a byte-offset change. Identical
// re-reports do not count as progress, so a transfer containerd is silently
// retrying (e.g. a registry serving 5xx) cannot keep the watchdog fed.
func (p *pullProgress) observe(resp *machine.ImageServicePullResponse) bool {
	pp := resp.GetPullProgress()
	if pp == nil {
		// The terminal message carrying the pulled image name.
		return true
	}
	lp := pp.GetProgress()
	cur := pullLayerState{status: lp.GetStatus(), offset: lp.GetOffset(), total: lp.GetTotal()}
	prev, seen := p.layers[pp.GetLayerId()]
	if seen && prev.status == cur.status && prev.offset == cur.offset {
		return false
	}
	p.layers[pp.GetLayerId()] = cur
	return true
}

func (p *pullProgress) summary() string {
	if len(p.layers) == 0 {
		return "0 layers, 0 bytes received"
	}
	var offset, total int64
	for _, l := range p.layers {
		offset += l.offset
		total += l.total
	}
	return fmt.Sprintf("%d layer(s), %.1f/%.1f MiB received",
		len(p.layers), float64(offset)/(1<<20), float64(total)/(1<<20))
}

// PullImage pulls imageRef into the node's system containerd image store — the
// store machined runs the installer from — so the pull at upgrade time is a
// skip. A pull whose stream reports no progress for pullIdleTimeout fails fast
// instead of burning the caller's whole budget: containerd retries registry
// 5xx internally, so a broken registry otherwise looks identical to a slow
// link until the deadline. Requires Talos >= v1.13 (ImageService); older
// nodes return Unimplemented.
func (s *Client) PullImage(ctx context.Context, nodeIP, imageRef string) error {
	nodeCtx := client.WithNode(ctx, nodeIP)

	err := s.executeWithRetry(ctx, func() error {
		return s.pullImageOnce(nodeCtx, imageRef)
	})
	if err != nil {
		return fmt.Errorf("failed to pull image %s on node %s: %w", imageRef, nodeIP, err)
	}
	return nil
}

func (s *Client) pullImageOnce(ctx context.Context, imageRef string) error {
	// Cancelled on return so a watchdog-abandoned stream (and its Recv
	// goroutine) is torn down rather than left pulling.
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := s.talos.ImagePull(streamCtx, &machine.ImageServicePullRequest{
		Containerd: &common.ContainerdInstance{
			Driver:    common.ContainerDriver_CONTAINERD,
			Namespace: common.ContainerdNamespace_NS_SYSTEM,
		},
		ImageRef: imageRef,
	})
	if err != nil {
		return err
	}

	type recvResult struct {
		resp *machine.ImageServicePullResponse
		err  error
	}
	recvCh := make(chan recvResult)
	go func() {
		for {
			resp, err := stream.Recv()
			select {
			case recvCh <- recvResult{resp: resp, err: err}:
			case <-streamCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	idleTimeout := s.pullIdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = defaultPullIdleTimeout
	}
	idle := time.NewTimer(idleTimeout)
	defer idle.Stop()

	progress := pullProgress{layers: map[string]pullLayerState{}}
	for {
		select {
		case r := <-recvCh:
			if r.err != nil {
				if errors.Is(r.err, io.EOF) {
					return nil
				}
				if ctx.Err() != nil {
					return fmt.Errorf("pull timed out at %s: %w", progress.summary(), ctx.Err())
				}
				return r.err
			}
			if progress.observe(r.resp) {
				idle.Reset(idleTimeout)
			}
		case <-ctx.Done():
			// Watched directly: the Recv goroutine's error delivery races
			// its own shutdown once the context dies.
			return fmt.Errorf("pull timed out at %s: %w", progress.summary(), ctx.Err())
		case <-idle.C:
			return fmt.Errorf("no pull progress after %s (%s); registry may be failing requests", idleTimeout, progress.summary())
		}
	}
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
	var ctxDeadErr error
	err := retry.Constant(10*time.Second, retry.WithUnits(100*time.Millisecond)).Retry(func() error {
		err := operation()
		if err == nil {
			return nil
		}
		// A dead local context is not a transient server condition: every
		// retry against it fails instantly, the client refresh is wasted
		// work, and go-retry misreads the DeadlineExceeded as its own
		// attempt timeout (the confusing "…; timeout" multierror). Stop the
		// retryer and surface the failure as-is.
		if ctx.Err() != nil {
			ctxDeadErr = err
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
	if ctxDeadErr != nil {
		return ctxDeadErr
	}
	return err
}

// IsUnimplementedError reports whether the node's Talos API lacks the called
// RPC (e.g. ImageService on Talos < v1.13).
func IsUnimplementedError(err error) bool {
	if st, ok := status.FromError(err); ok {
		return st.Code() == codes.Unimplemented
	}
	return false
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

	if _, err := s.readMachineConfigRaw(ctx, nodeIP); err != nil {
		return fmt.Errorf("machine config not accessible: %w", err)
	}

	return nil
}
