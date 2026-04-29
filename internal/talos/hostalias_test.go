package talos

import (
	"testing"

	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	"github.com/siderolabs/talos/pkg/machinery/resources/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadMachineConfig(t *testing.T, yaml string) *config.MachineConfig {
	t.Helper()
	provider, err := configloader.NewFromBytes([]byte(yaml))
	require.NoError(t, err)
	return config.NewMachineConfig(provider)
}

func TestResolveControlPlaneHostAlias_IPLiteral(t *testing.T) {
	cfg := `version: v1alpha1
machine: {}
cluster:
  controlPlane:
    endpoint: https://10.0.0.10:6443
`
	mc := loadMachineConfig(t, cfg)

	alias, err := ResolveControlPlaneHostAlias(mc, "10.0.1.101")
	require.NoError(t, err)
	assert.Nil(t, alias, "no alias needed when controlPlaneEndpoint is an IP literal")
}

func TestResolveControlPlaneHostAlias_HostnameWithMatchingExtraHost(t *testing.T) {
	cfg := `version: v1alpha1
machine:
  network:
    extraHostEntries:
      - ip: 10.0.1.100
        aliases:
          - kube.cluster.local
cluster:
  controlPlane:
    endpoint: https://kube.cluster.local:6443
`
	mc := loadMachineConfig(t, cfg)

	alias, err := ResolveControlPlaneHostAlias(mc, "10.0.1.101")
	require.NoError(t, err)
	require.NotNil(t, alias)
	assert.Equal(t, "10.0.1.100", alias.IP, "should pick up VIP from extraHostEntries")
	assert.Equal(t, []string{"kube.cluster.local"}, alias.Hostnames)
}

func TestResolveControlPlaneHostAlias_HostnameNoMatch_FallbackToNodeIP(t *testing.T) {
	cfg := `version: v1alpha1
machine: {}
cluster:
  controlPlane:
    endpoint: https://kube.example.internal:6443
`
	mc := loadMachineConfig(t, cfg)

	alias, err := ResolveControlPlaneHostAlias(mc, "10.0.1.101")
	require.NoError(t, err)
	require.NotNil(t, alias)
	assert.Equal(t, "10.0.1.101", alias.IP, "should fall back to target node IP")
	assert.Equal(t, []string{"kube.example.internal"}, alias.Hostnames)
}

func TestResolveControlPlaneHostAlias_MultipleAliasesPropagated(t *testing.T) {
	cfg := `version: v1alpha1
machine:
  network:
    extraHostEntries:
      - ip: 10.0.1.100
        aliases:
          - kube.cluster.local
          - api.cluster.local
          - cluster-vip
cluster:
  controlPlane:
    endpoint: https://kube.cluster.local:6443
`
	mc := loadMachineConfig(t, cfg)

	alias, err := ResolveControlPlaneHostAlias(mc, "10.0.1.101")
	require.NoError(t, err)
	require.NotNil(t, alias)
	assert.Equal(t, "10.0.1.100", alias.IP)
	assert.Equal(t, []string{"kube.cluster.local", "api.cluster.local", "cluster-vip"}, alias.Hostnames,
		"all aliases on the matching extraHostEntries IP should be propagated")
}

func TestResolveControlPlaneHostAlias_StaticHostConfigDocument(t *testing.T) {
	cfg := `version: v1alpha1
machine: {}
cluster:
  controlPlane:
    endpoint: https://kube.cluster.local:6443
---
apiVersion: v1alpha1
kind: StaticHostConfig
name: 10.0.1.100
hostnames:
  - kube.cluster.local
  - api.cluster.local
`
	mc := loadMachineConfig(t, cfg)

	alias, err := ResolveControlPlaneHostAlias(mc, "10.0.1.101")
	require.NoError(t, err)
	require.NotNil(t, alias)
	assert.Equal(t, "10.0.1.100", alias.IP, "should pick up IP from StaticHostConfig document")
	assert.ElementsMatch(t, []string{"kube.cluster.local", "api.cluster.local"}, alias.Hostnames)
}

func TestResolveControlPlaneHostAlias_NoEndpoint(t *testing.T) {
	cfg := `version: v1alpha1
machine: {}
cluster: {}
`
	mc := loadMachineConfig(t, cfg)

	alias, err := ResolveControlPlaneHostAlias(mc, "10.0.1.101")
	require.NoError(t, err)
	assert.Nil(t, alias, "no alias when controlPlaneEndpoint is unset")
}

func TestResolveControlPlaneHostAlias_NilMachineConfig(t *testing.T) {
	alias, err := ResolveControlPlaneHostAlias(nil, "10.0.1.101")
	require.Error(t, err)
	assert.Nil(t, alias)
}

func TestResolveControlPlaneHostAlias_InvalidFallbackIP(t *testing.T) {
	cfg := `version: v1alpha1
machine: {}
cluster:
  controlPlane:
    endpoint: https://kube.example.internal:6443
`
	mc := loadMachineConfig(t, cfg)

	alias, err := ResolveControlPlaneHostAlias(mc, "not-an-ip")
	require.Error(t, err)
	assert.Nil(t, alias)
}

func TestResolveControlPlaneHostAlias_EmptyFallbackIP(t *testing.T) {
	cfg := `version: v1alpha1
machine: {}
cluster:
  controlPlane:
    endpoint: https://kube.example.internal:6443
`
	mc := loadMachineConfig(t, cfg)

	alias, err := ResolveControlPlaneHostAlias(mc, "")
	require.NoError(t, err)
	assert.Nil(t, alias, "no alias when fallback IP is empty and no extraHostEntry matches")
}

func TestResolveControlPlaneHostAlias_IPv6Literal(t *testing.T) {
	cfg := `version: v1alpha1
machine: {}
cluster:
  controlPlane:
    endpoint: "https://[fd00::1]:6443"
`
	mc := loadMachineConfig(t, cfg)

	alias, err := ResolveControlPlaneHostAlias(mc, "10.0.1.101")
	require.NoError(t, err)
	assert.Nil(t, alias, "no alias needed for IPv6 literal endpoint")
}
