package talos

import (
	"fmt"
	"net"
	"net/url"

	talosconfig "github.com/siderolabs/talos/pkg/machinery/config/config"
	"github.com/siderolabs/talos/pkg/machinery/resources/config"
	corev1 "k8s.io/api/core/v1"
)

// ResolveControlPlaneHostAlias derives a HostAlias to inject into upgrade pods so that
// `talosctl upgrade-k8s` can resolve cluster.controlPlaneEndpoint from inside the cluster.
//
// Returns nil if no alias is needed (endpoint is an IP literal, or the machine config doesn't
// expose a usable endpoint). Returns a non-nil HostAlias when the endpoint is a hostname:
//   - prefer an IP from a matching extraHostEntries / StaticHostConfig entry on the same machine config
//   - otherwise fall back to fallbackIP (the target node IP, which always serves the apiserver)
func ResolveControlPlaneHostAlias(mc *config.MachineConfig, fallbackIP string) (*corev1.HostAlias, error) {
	if mc == nil {
		return nil, fmt.Errorf("machine config is nil")
	}

	cfg := mc.Config()
	if cfg == nil || cfg.Cluster() == nil {
		return nil, nil
	}

	// cfg.Cluster().Endpoint() panics on a nil ControlPlane in the underlying v1alpha1
	// struct, so isolate it via recover. Treating any failure here as "no alias needed"
	// is the safe choice — we'd rather miss the optimization than break the upgrade.
	endpoint := safeClusterEndpoint(cfg)
	if endpoint == nil {
		return nil, nil
	}

	host := endpointHost(endpoint)
	if host == "" {
		return nil, nil
	}

	// IP literal — pods can connect directly, no alias needed.
	if net.ParseIP(host) != nil {
		return nil, nil
	}

	// Look for a matching alias in the live machine config first.
	if ip, hostnames := lookupStaticHost(mc, host); ip != "" {
		return &corev1.HostAlias{IP: ip, Hostnames: hostnames}, nil
	}

	// Fall back to the target node's own IP. Every control-plane node serves the apiserver
	// on :6443, and the apiserver cert covers the controlPlaneEndpoint hostname via certSANs.
	if fallbackIP == "" {
		return nil, nil
	}
	if net.ParseIP(fallbackIP) == nil {
		return nil, fmt.Errorf("fallback IP %q is not a valid IP", fallbackIP)
	}
	return &corev1.HostAlias{IP: fallbackIP, Hostnames: []string{host}}, nil
}

// safeClusterEndpoint returns Cluster().Endpoint() but recovers from nil-deref panics
// in the upstream v1alpha1 ClusterConfig.Endpoint when ControlPlane is unset.
func safeClusterEndpoint(cfg talosconfig.Config) (u *url.URL) {
	defer func() {
		if r := recover(); r != nil {
			u = nil
		}
	}()
	return cfg.Cluster().Endpoint()
}

// endpointHost extracts the bare host from a controlPlaneEndpoint URL (no port).
func endpointHost(u *url.URL) string {
	if u == nil {
		return ""
	}
	if h := u.Hostname(); h != "" {
		return h
	}
	return u.Host
}

// lookupStaticHost finds a static-host entry whose aliases include `host`. It checks both
// the legacy v1alpha1 machine.network.extraHostEntries and any StaticHostConfig documents
// (the post-deprecation replacement). If found, it returns the entry's IP and the full
// list of hostnames mapped to that IP, so all aliases are propagated to HostAlias.
func lookupStaticHost(mc *config.MachineConfig, host string) (string, []string) {
	// Legacy v1alpha1 extraHostEntries — still emitted by talhelper, hcloud-talos, etc.
	if raw := mc.Provider().RawV1Alpha1(); raw != nil &&
		raw.MachineConfig != nil && raw.MachineConfig.MachineNetwork != nil { //nolint:staticcheck // Need to read deprecated field for backward compatibility
		for _, e := range raw.MachineConfig.MachineNetwork.ExtraHosts() { //nolint:staticcheck // Need to read deprecated field for backward compatibility
			if matchesHost(e, host) {
				return e.IP(), e.Aliases()
			}
		}
	}

	// New-style StaticHostConfig documents.
	for _, doc := range mc.Provider().Documents() {
		shc, ok := doc.(talosconfig.NetworkStaticHostConfig)
		if !ok {
			continue
		}
		if matchesHost(shc, host) {
			return shc.IP(), shc.Aliases()
		}
	}

	return "", nil
}

func matchesHost(e talosconfig.NetworkStaticHostConfig, host string) bool {
	for _, alias := range e.Aliases() {
		if alias == host {
			return true
		}
	}
	return false
}
