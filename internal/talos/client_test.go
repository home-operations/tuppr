package talos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/siderolabs/talos/pkg/machinery/api/common"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	configpb "github.com/siderolabs/talos/pkg/machinery/api/resource/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type mockTalosClient struct {
	readCalls       int
	readErr         error
	readErrUntil    int
	readData        string
	versionCalls    int
	versionErr      error
	versionErrUntil int
	versionResp     *machine.VersionResponse
	cosiGetErr      error
	cosiGetErrUntil int
	cosiGetCalls    int
	closed          bool
	cosiResource    resource.Resource
	cosiListItems   []resource.Resource
	cosiListErr     error
	applyConfigReq  *machine.ApplyConfigurationRequest
	applyConfigErr  error
	imagePullReqs   []*machine.ImageServicePullRequest
	imagePullErr    error
	pullStreamErr   error
	pullStreamFunc  func(ctx context.Context) grpc.ServerStreamingClient[machine.ImageServicePullResponse]
}

func (m *mockTalosClient) Version(_ context.Context, _ ...grpc.CallOption) (*machine.VersionResponse, error) {
	m.versionCalls++
	if m.versionErrUntil > 0 && m.versionCalls <= m.versionErrUntil {
		return nil, m.versionErr
	}
	if m.versionErr != nil && m.versionCalls == 1 {
		return nil, m.versionErr
	}
	return m.versionResp, nil
}

func (m *mockTalosClient) COSIGetRawSpec(_ context.Context, _, _, _ string) ([]byte, error) {
	m.readCalls++
	if m.readErrUntil > 0 && m.readCalls <= m.readErrUntil {
		return nil, m.readErr
	}
	if m.readErr != nil && m.readCalls == 1 {
		return nil, m.readErr
	}
	return proto.Marshal(&configpb.MachineConfigSpec{YamlMarshalled: []byte(m.readData)})
}

func (m *mockTalosClient) COSIGet(_ context.Context, _, _, _ string) (resource.Resource, error) {
	m.cosiGetCalls++
	if m.cosiGetErrUntil > 0 && m.cosiGetCalls <= m.cosiGetErrUntil {
		return nil, m.cosiGetErr
	}
	if m.cosiGetErr != nil && m.cosiGetCalls == 1 {
		return nil, m.cosiGetErr
	}
	return m.cosiResource, nil
}

func (m *mockTalosClient) COSIList(_ context.Context, _, _ string) ([]resource.Resource, error) {
	if m.cosiListErr != nil {
		return nil, m.cosiListErr
	}
	return m.cosiListItems, nil
}

func (m *mockTalosClient) ApplyConfiguration(_ context.Context, req *machine.ApplyConfigurationRequest, _ ...grpc.CallOption) (*machine.ApplyConfigurationResponse, error) {
	m.applyConfigReq = req
	return &machine.ApplyConfigurationResponse{}, m.applyConfigErr
}

// fakePullStream replays a canned pull stream: one progress message, then
// pullStreamErr or a clean EOF.
type fakePullStream struct {
	grpc.ClientStream
	err  error
	recv int
}

func (f *fakePullStream) Recv() (*machine.ImageServicePullResponse, error) {
	f.recv++
	if f.recv == 1 {
		return &machine.ImageServicePullResponse{}, nil
	}
	if f.err != nil {
		return nil, f.err
	}
	return nil, io.EOF
}

func (m *mockTalosClient) ImagePull(ctx context.Context, req *machine.ImageServicePullRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[machine.ImageServicePullResponse], error) {
	m.imagePullReqs = append(m.imagePullReqs, req)
	if m.imagePullErr != nil {
		return nil, m.imagePullErr
	}
	if m.pullStreamFunc != nil {
		return m.pullStreamFunc(ctx), nil
	}
	return &fakePullStream{err: m.pullStreamErr}, nil
}

// pacedPullStream replays msgs with a fixed delay before each, then a clean
// EOF; used to exercise the stall watchdog against a live-but-slow stream.
type pacedPullStream struct {
	grpc.ClientStream
	msgs     []*machine.ImageServicePullResponse
	interval time.Duration
	i        int
}

func (s *pacedPullStream) Recv() (*machine.ImageServicePullResponse, error) {
	if s.i >= len(s.msgs) {
		return nil, io.EOF
	}
	time.Sleep(s.interval)
	msg := s.msgs[s.i]
	s.i++
	return msg, nil
}

// stallingPullStream replays msgs, then blocks like a stalled transfer until
// the stream context ends, returning its error the way gRPC would.
type stallingPullStream struct {
	grpc.ClientStream
	ctx  context.Context
	msgs []*machine.ImageServicePullResponse
	i    int
}

func (s *stallingPullStream) Recv() (*machine.ImageServicePullResponse, error) {
	if s.i < len(s.msgs) {
		msg := s.msgs[s.i]
		s.i++
		return msg, nil
	}
	<-s.ctx.Done()
	return nil, status.FromContextError(s.ctx.Err()).Err()
}

func pullProgressMsg(layer string, st machine.ImageServicePullLayerProgress_Status, offset, total int64) *machine.ImageServicePullResponse {
	return &machine.ImageServicePullResponse{
		Response: &machine.ImageServicePullResponse_PullProgress{
			PullProgress: &machine.ImageServicePullProgress{
				LayerId: layer,
				Progress: &machine.ImageServicePullLayerProgress{
					Status: st,
					Offset: offset,
					Total:  total,
				},
			},
		},
	}
}

func (m *mockTalosClient) Close() error {
	m.closed = true
	return nil
}

func makeVersionResponse(tag string) *machine.VersionResponse {
	return &machine.VersionResponse{
		Messages: []*machine.Version{
			{
				Version: &machine.VersionInfo{
					Tag: tag,
				},
			},
		},
	}
}

func TestClient_ExecuteWithRetry_Success(t *testing.T) {
	ctx := context.Background()
	mock := &mockTalosClient{
		versionResp: makeVersionResponse("v1.10.0"),
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	callCount := 0
	err := c.executeWithRetry(ctx, func() error {
		callCount++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "operation should be called exactly once on success")
}

func TestClient_ExecuteWithRetry_RetriesOnError(t *testing.T) {
	ctx := context.Background()
	testErr := status.Error(codes.Unavailable, "test error")

	mock := &mockTalosClient{
		versionErr:      testErr,
		versionErrUntil: 1,
		versionResp:     makeVersionResponse("v1.10.0"),
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	callCount := 0
	err := c.executeWithRetry(ctx, func() error {
		callCount++
		if callCount == 1 {
			return testErr
		}
		return nil
	})

	require.NoError(t, err)
	assert.GreaterOrEqual(t, callCount, 2, "should retry after error")
}

func TestClient_ExecuteWithRetry_RefreshesClientOnError(t *testing.T) {
	ctx := context.Background()
	testErr := status.Error(codes.Unavailable, "connection error")

	mock := &mockTalosClient{
		versionErr:      testErr,
		versionErrUntil: 1,
		versionResp:     makeVersionResponse("v1.10.0"),
	}

	refreshCount := 0
	c := &Client{
		talos: mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) {
			refreshCount++
			return mock, nil
		},
	}

	callCount := 0
	err := c.executeWithRetry(ctx, func() error {
		callCount++
		if callCount == 1 {
			return testErr
		}
		return nil
	})

	require.NoError(t, err)
	assert.GreaterOrEqual(t, refreshCount, 1, "should refresh client on error")
}

func TestClient_ExecuteWithRetry_PersistentFailure(t *testing.T) {
	ctx := context.Background()
	persistentErr := errors.New("persistent error")

	mock := &mockTalosClient{
		versionErr:      persistentErr,
		versionErrUntil: 100,
		versionResp:     makeVersionResponse("v1.10.0"),
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	err := c.executeWithRetry(ctx, func() error {
		return persistentErr
	})

	require.Error(t, err, "should return error after retries exhausted")
}

func TestClient_GetNodeVersion_Success(t *testing.T) {
	ctx := context.Background()
	expectedVersion := "v1.10.0"

	mock := &mockTalosClient{
		versionResp: makeVersionResponse(expectedVersion),
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	version, err := c.GetNodeVersion(ctx, "10.0.0.1")

	require.NoError(t, err)
	assert.Equal(t, expectedVersion, version)
	assert.GreaterOrEqual(t, mock.versionCalls, 1, "version should be called")
}

func TestClient_GetNodeVersion_RetriesOnCertError(t *testing.T) {
	ctx := context.Background()
	certErr := status.Error(codes.Unavailable, "tls: expired certificate")

	mock := &mockTalosClient{
		versionErr:      certErr,
		versionErrUntil: 1,
		versionResp:     makeVersionResponse("v1.10.0"),
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	version, err := c.GetNodeVersion(ctx, "10.0.0.1")

	require.NoError(t, err)
	assert.Equal(t, "v1.10.0", version)
	assert.GreaterOrEqual(t, mock.versionCalls, 2, "should retry on cert error")
}

func TestClient_GetNodeVersion_EmptyResponse(t *testing.T) {
	ctx := context.Background()

	mock := &mockTalosClient{
		versionResp: &machine.VersionResponse{},
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	_, err := c.GetNodeVersion(ctx, "10.0.0.1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no response")
}

func TestClient_GetNodeVersion_NilVersion(t *testing.T) {
	ctx := context.Background()

	mock := &mockTalosClient{
		versionResp: &machine.VersionResponse{
			Messages: []*machine.Version{{}},
		},
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	_, err := c.GetNodeVersion(ctx, "10.0.0.1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "version is nil")
}

func TestClient_ReadMachineConfigRaw_Success(t *testing.T) {
	ctx := context.Background()
	raw := "version: v1alpha1\nmachine:\n  install:\n    image: img\n"

	mock := &mockTalosClient{readData: raw}
	c := &Client{talos: mock, newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil }}

	got, err := c.readMachineConfigRaw(ctx, "10.0.0.1")

	require.NoError(t, err)
	assert.Equal(t, raw, got)
}

func TestClient_ReadMachineConfigRaw_RetriesOnCertError(t *testing.T) {
	ctx := context.Background()

	mock := &mockTalosClient{
		readData:     "version: v1alpha1\n",
		readErr:      status.Error(codes.Unavailable, "tls: expired certificate"),
		readErrUntil: 1,
	}
	c := &Client{talos: mock, newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil }}

	_, err := c.readMachineConfigRaw(ctx, "10.0.0.1")

	require.NoError(t, err)
	assert.GreaterOrEqual(t, mock.readCalls, 2, "should retry on cert error")
}

// A config carrying document kinds this binary has never heard of must still be
// readable: the read path never decodes the whole config.
func TestClient_GetNodeInstallImage_UnknownDocumentKinds(t *testing.T) {
	ctx := context.Background()

	mock := &mockTalosClient{readData: "version: v1alpha1\nmachine:\n  install:\n    image: factory.talos.dev/installer/abc:v1.13.5\n---\napiVersion: v1alpha1\nkind: BGPPeerConfig\nname: peer-1\npeerAddress: 10.0.0.254\n"}
	c := &Client{talos: mock, newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil }}

	image, err := c.GetNodeInstallImage(ctx, "10.0.0.1")

	require.NoError(t, err)
	assert.Equal(t, "factory.talos.dev/installer/abc:v1.13.5", image)
}

func TestClient_RefreshTalosClient_Success(t *testing.T) {
	ctx := context.Background()

	oldMock := &mockTalosClient{
		versionResp: makeVersionResponse("v1.10.0"),
	}
	newMock := &mockTalosClient{
		versionResp: makeVersionResponse("v1.11.0"),
	}

	refreshCount := 0
	c := &Client{
		talos: oldMock,
		newClientFunc: func(ctx context.Context) (talosClient, error) {
			refreshCount++
			if refreshCount == 1 {
				return oldMock, nil
			}
			return newMock, nil
		},
	}

	err := c.refreshTalosClient(ctx)

	require.NoError(t, err)
	assert.True(t, oldMock.closed, "old client should be closed")
}

func TestClient_RefreshTalosClient_NewClientFailure(t *testing.T) {
	ctx := context.Background()

	oldMock := &mockTalosClient{
		versionResp: makeVersionResponse("v1.10.0"),
	}

	newClientErr := errors.New("failed to create client")
	c := &Client{
		talos: oldMock,
		newClientFunc: func(ctx context.Context) (talosClient, error) {
			return nil, newClientErr
		},
	}

	err := c.refreshTalosClient(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to reinitialize")
	assert.False(t, oldMock.closed, "old client should not be closed when new client creation fails")
}

func TestClient_RefreshTalosClient_CloseError(t *testing.T) {
	ctx := context.Background()

	oldMock := &mockTalosClient{
		versionResp: makeVersionResponse("v1.10.0"),
	}
	newMock := &mockTalosClient{
		versionResp: makeVersionResponse("v1.11.0"),
	}

	oldMock.closed = true

	c := &Client{
		talos: oldMock,
		newClientFunc: func(ctx context.Context) (talosClient, error) {
			return newMock, nil
		},
	}

	err := c.refreshTalosClient(ctx)

	assert.NoError(t, err)
}

func TestClient_ExecuteWithRetry_MultipleRetries(t *testing.T) {
	ctx := context.Background()
	certErr := status.Error(codes.Unavailable, "tls: expired certificate")

	mock := &mockTalosClient{
		versionErr:      certErr,
		versionErrUntil: 3,
		versionResp:     makeVersionResponse("v1.10.0"),
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	err := c.executeWithRetry(ctx, func() error {
		_, err := c.talos.Version(ctx)
		return err
	})

	require.NoError(t, err)
	assert.GreaterOrEqual(t, mock.versionCalls, 4, "should handle multiple retries")
}

func TestClient_CheckNodeReady_Integration(t *testing.T) {
	ctx := context.Background()

	mock := &mockTalosClient{
		versionResp: makeVersionResponse("v1.10.0"),
		readData:    "version: v1alpha1\n",
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	err := c.CheckNodeReady(ctx, "10.0.0.1", "node-1")

	require.NoError(t, err)
	assert.GreaterOrEqual(t, mock.versionCalls, 1)
	assert.GreaterOrEqual(t, mock.readCalls, 1)
}

func v1alpha1ConfigYAML(image string) string {
	return fmt.Sprintf("version: v1alpha1\nmachine:\n  install:\n    image: %q\n", image)
}

func TestClient_PatchNodeInstallImage_Success(t *testing.T) {
	ctx := context.Background()
	oldImage := "factory.talos.dev/installer/abc:v1.11.0"
	newImage := "factory.talos.dev/installer/abc:v1.12.0"

	mock := &mockTalosClient{
		versionResp: makeVersionResponse("v1.12.0"),
		readData:    v1alpha1ConfigYAML(oldImage),
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	err := c.PatchNodeInstallImage(ctx, "10.0.0.1", newImage)

	require.NoError(t, err)
	require.NotNil(t, mock.applyConfigReq)
	assert.Equal(t, machine.ApplyConfigurationRequest_NO_REBOOT, mock.applyConfigReq.Mode)
	assert.True(t, strings.Contains(string(mock.applyConfigReq.Data), newImage),
		"patched config should contain new image")
	assert.False(t, strings.Contains(string(mock.applyConfigReq.Data), oldImage),
		"patched config should not contain old image")
}

func TestClient_PatchNodeInstallImage_MultiDocumentConfig(t *testing.T) {
	ctx := context.Background()
	oldImage := "factory.talos.dev/installer/abc:v1.11.0"
	newImage := "factory.talos.dev/installer/abc:v1.12.0"

	cfgYAML := fmt.Sprintf(
		"version: v1alpha1\nmachine:\n  install:\n    image: %q\n"+
			"---\n"+
			"apiVersion: v1alpha1\nkind: SideroLinkConfig\napiUrl: https://siderolink.example/?jointoken=secret\n",
		oldImage,
	)

	mock := &mockTalosClient{
		versionResp: makeVersionResponse("v1.12.0"),
		readData:    cfgYAML,
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	err := c.PatchNodeInstallImage(ctx, "10.0.0.1", newImage)

	require.NoError(t, err)
	require.NotNil(t, mock.applyConfigReq)
	patched := string(mock.applyConfigReq.Data)
	assert.Contains(t, patched, newImage)
	assert.NotContains(t, patched, oldImage)
	assert.Contains(t, patched, "SideroLinkConfig")
	assert.Contains(t, patched, "siderolink.example")
}

func TestClient_PatchNodeInstallImage_GetConfigError(t *testing.T) {
	ctx := context.Background()

	mock := &mockTalosClient{
		versionResp: makeVersionResponse("v1.12.0"),
		readErr:     errors.New("connection refused"),
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	err := c.PatchNodeInstallImage(ctx, "10.0.0.1", "factory.talos.dev/installer/abc:v1.12.0")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to patch install image")
	assert.Nil(t, mock.applyConfigReq, "should not call ApplyConfiguration on config fetch error")
}

func TestClient_PatchNodeInstallImage_ApplyError(t *testing.T) {
	ctx := context.Background()
	mock := &mockTalosClient{
		versionResp:    makeVersionResponse("v1.12.0"),
		readData:       v1alpha1ConfigYAML("factory.talos.dev/installer/abc:v1.11.0"),
		applyConfigErr: errors.New("permission denied"),
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	err := c.PatchNodeInstallImage(ctx, "10.0.0.1", "factory.talos.dev/installer/abc:v1.12.0")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to apply configuration")
}

func TestClient_PullImage_Success(t *testing.T) {
	ctx := context.Background()
	mock := &mockTalosClient{}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	err := c.PullImage(ctx, "10.0.0.1", "factory.talos.dev/installer/abc:v1.13.7")

	require.NoError(t, err)
	require.Len(t, mock.imagePullReqs, 1)
	req := mock.imagePullReqs[0]
	assert.Equal(t, "factory.talos.dev/installer/abc:v1.13.7", req.ImageRef)
	// The installer runs from the system containerd instance's system
	// namespace; pulling anywhere else would be silently useless.
	assert.Equal(t, common.ContainerDriver_CONTAINERD, req.Containerd.Driver)
	assert.Equal(t, common.ContainerdNamespace_NS_SYSTEM, req.Containerd.Namespace)
}

func TestClient_PullImage_CallError(t *testing.T) {
	ctx := context.Background()
	mock := &mockTalosClient{
		imagePullErr: status.Error(codes.NotFound, "error pulling image: not found"),
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	err := c.PullImage(ctx, "10.0.0.1", "factory.talos.dev/installer/abc:v1.13.7")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to pull image factory.talos.dev/installer/abc:v1.13.7 on node 10.0.0.1")
	assert.False(t, IsUnimplementedError(err))
}

func TestClient_PullImage_StreamError(t *testing.T) {
	ctx := context.Background()
	mock := &mockTalosClient{
		pullStreamErr: status.Error(codes.Internal, "error pulling image: manifest unknown"),
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	err := c.PullImage(ctx, "10.0.0.1", "factory.talos.dev/installer/abc:v1.13.7")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest unknown")
}

func TestClient_PullImage_Unimplemented(t *testing.T) {
	ctx := context.Background()
	mock := &mockTalosClient{
		imagePullErr: status.Error(codes.Unimplemented, "unknown service machine.ImageService"),
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	err := c.PullImage(ctx, "10.0.0.1", "factory.talos.dev/installer/abc:v1.13.7")

	require.Error(t, err)
	assert.True(t, IsUnimplementedError(err), "Unimplemented must survive the error wrap")
}

func TestClient_PullImage_StallFailsFast(t *testing.T) {
	ctx := context.Background()
	// The stream stays open but transfers nothing — the shape of a registry
	// serving errors that containerd retries internally.
	mock := &mockTalosClient{
		pullStreamFunc: func(ctx context.Context) grpc.ServerStreamingClient[machine.ImageServicePullResponse] {
			return &stallingPullStream{ctx: ctx}
		},
	}

	c := &Client{
		talos:           mock,
		newClientFunc:   func(ctx context.Context) (talosClient, error) { return mock, nil },
		pullIdleTimeout: 100 * time.Millisecond,
	}

	start := time.Now()
	err := c.PullImage(ctx, "10.0.0.1", "factory.talos.dev/installer/abc:v1.13.7")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no pull progress after 100ms")
	assert.Contains(t, err.Error(), "0 layers, 0 bytes received")
	assert.Less(t, time.Since(start), 5*time.Second, "watchdog must fail fast, not wait for a deadline")
}

func TestClient_PullImage_IdenticalProgressReportsDoNotFeedWatchdog(t *testing.T) {
	ctx := context.Background()
	// A steady stream of identical reports is a stalled transfer, not a live
	// one: the watchdog must still fire.
	frozen := pullProgressMsg("layer-a", machine.ImageServicePullLayerProgress_DOWNLOADING, 1<<20, 8<<20)
	msgs := make([]*machine.ImageServicePullResponse, 50)
	for i := range msgs {
		msgs[i] = frozen
	}
	mock := &mockTalosClient{
		pullStreamFunc: func(ctx context.Context) grpc.ServerStreamingClient[machine.ImageServicePullResponse] {
			return &pacedPullStream{msgs: msgs, interval: 20 * time.Millisecond}
		},
	}

	c := &Client{
		talos:           mock,
		newClientFunc:   func(ctx context.Context) (talosClient, error) { return mock, nil },
		pullIdleTimeout: 150 * time.Millisecond,
	}

	err := c.PullImage(ctx, "10.0.0.1", "factory.talos.dev/installer/abc:v1.13.7")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no pull progress after 150ms")
	assert.Contains(t, err.Error(), "1 layer(s), 1.0/8.0 MiB received")
}

func TestClient_PullImage_AdvancingProgressFeedsWatchdog(t *testing.T) {
	ctx := context.Background()
	// Each message advances the offset; the pull outlives the idle timeout
	// several times over and must still succeed.
	msgs := make([]*machine.ImageServicePullResponse, 8)
	for i := range msgs {
		msgs[i] = pullProgressMsg("layer-a", machine.ImageServicePullLayerProgress_DOWNLOADING, int64(i)<<20, 8<<20)
	}
	mock := &mockTalosClient{
		pullStreamFunc: func(ctx context.Context) grpc.ServerStreamingClient[machine.ImageServicePullResponse] {
			return &pacedPullStream{msgs: msgs, interval: 50 * time.Millisecond}
		},
	}

	c := &Client{
		talos:           mock,
		newClientFunc:   func(ctx context.Context) (talosClient, error) { return mock, nil },
		pullIdleTimeout: 150 * time.Millisecond,
	}

	err := c.PullImage(ctx, "10.0.0.1", "factory.talos.dev/installer/abc:v1.13.7")

	require.NoError(t, err)
}

func TestClient_PullImage_DeadlineErrorIncludesProgress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	mock := &mockTalosClient{
		pullStreamFunc: func(ctx context.Context) grpc.ServerStreamingClient[machine.ImageServicePullResponse] {
			return &stallingPullStream{ctx: ctx, msgs: []*machine.ImageServicePullResponse{
				pullProgressMsg("layer-a", machine.ImageServicePullLayerProgress_DOWNLOADING, 384<<20, 512<<20),
			}}
		},
	}

	c := &Client{
		talos:           mock,
		newClientFunc:   func(ctx context.Context) (talosClient, error) { return mock, nil },
		pullIdleTimeout: time.Minute, // only the caller's deadline can end this pull
	}

	err := c.PullImage(ctx, "10.0.0.1", "factory.talos.dev/installer/abc:v1.13.7")

	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "pull timed out at 1 layer(s), 384.0/512.0 MiB received")
	assert.NotContains(t, err.Error(), "error(s) occurred", "a dead context must not produce a retry multierror")
}

func TestClient_ExecuteWithRetry_DeadContextSkipsRetryAndRefresh(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transientErr := status.Error(codes.Unavailable, "connection error")

	mock := &mockTalosClient{}
	refreshCount := 0
	c := &Client{
		talos: mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) {
			refreshCount++
			return mock, nil
		},
	}

	callCount := 0
	err := c.executeWithRetry(ctx, func() error {
		callCount++
		return transientErr
	})

	require.ErrorIs(t, err, transientErr)
	assert.Equal(t, 1, callCount, "a dead context must not be retried against")
	assert.Equal(t, 0, refreshCount, "a dead context must not trigger a client refresh")
}

func TestIsUnimplementedError(t *testing.T) {
	assert.False(t, IsUnimplementedError(nil))
	assert.False(t, IsUnimplementedError(errors.New("plain error")))
	assert.False(t, IsUnimplementedError(status.Error(codes.Unavailable, "down")))
	assert.True(t, IsUnimplementedError(status.Error(codes.Unimplemented, "nope")))
	assert.True(t, IsUnimplementedError(fmt.Errorf("wrapped: %w", status.Error(codes.Unimplemented, "nope"))))
}
