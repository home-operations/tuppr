package talos

import (
	"context"
	"errors"
	"testing"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/resources/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockTalosClient struct {
	versionCalls    int
	versionErr      error
	versionErrUntil int
	versionResp     *machine.VersionResponse
	cosiGetErr      error
	cosiGetErrUntil int
	cosiGetCalls    int
	closed          bool
	cosiResource    resource.Resource
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
	testErr := errors.New("test error")

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
	testErr := errors.New("connection error")

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
	certErr := errors.New("rpc error: code = Unavailable desc = tls: expired certificate")

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

func TestClient_GetNodeMachineConfig_Success(t *testing.T) {
	ctx := context.Background()
	expectedConfig := &config.MachineConfig{}

	mock := &mockTalosClient{
		versionResp:  makeVersionResponse("v1.10.0"),
		cosiResource: expectedConfig,
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	cfg, err := c.GetNodeMachineConfig(ctx, "10.0.0.1")

	require.NoError(t, err)
	assert.Equal(t, expectedConfig, cfg)
}

func TestClient_GetNodeMachineConfig_RetriesOnCertError(t *testing.T) {
	ctx := context.Background()
	certErr := errors.New("rpc error: code = Unavailable desc = tls: expired certificate")

	mc := &config.MachineConfig{}
	mock := &mockTalosClient{
		versionResp:     makeVersionResponse("v1.10.0"),
		cosiGetErr:      certErr,
		cosiGetErrUntil: 1,
		cosiResource:    mc,
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	result, err := c.GetNodeMachineConfig(ctx, "10.0.0.1")

	require.NoError(t, err)
	assert.Equal(t, mc, result)
	assert.GreaterOrEqual(t, mock.cosiGetCalls, 2, "should retry on cert error")
}

func TestClient_GetNodeMachineConfig_UnexpectedType(t *testing.T) {
	ctx := context.Background()

	mock := &mockTalosClient{
		versionResp:  makeVersionResponse("v1.10.0"),
		cosiResource: nil,
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	_, err := c.GetNodeMachineConfig(ctx, "10.0.0.1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected resource type")
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
	certErr := errors.New("tls: expired certificate")

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

	mc := &config.MachineConfig{}
	mock := &mockTalosClient{
		versionResp:  makeVersionResponse("v1.10.0"),
		cosiResource: mc,
	}

	c := &Client{
		talos:         mock,
		newClientFunc: func(ctx context.Context) (talosClient, error) { return mock, nil },
	}

	err := c.CheckNodeReady(ctx, "10.0.0.1", "node-1")

	require.NoError(t, err)
	assert.GreaterOrEqual(t, mock.versionCalls, 1)
	assert.GreaterOrEqual(t, mock.cosiGetCalls, 1)
}
