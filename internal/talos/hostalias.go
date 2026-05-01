package talos

import (
	"net"
	"strings"

	talosconfig "github.com/siderolabs/talos/pkg/machinery/config/config"
	corev1 "k8s.io/api/core/v1"
)

// ResolveControlPlaneHostAlias returns a HostAlias mapping the
// cluster.controlPlane.endpoint hostname to either the IP that the Talos host
// uses for it (extraHostEntries / StaticHostConfig) or, if no entry matches,
// fallbackIP — every control-plane node serves the apiserver on :6443 and the
// apiserver cert covers the endpoint via certSANs.
//
// Returns nil when no alias is needed (endpoint missing or an IP literal).
func ResolveControlPlaneHostAlias(cfg talosconfig.Config, fallbackIP string) *corev1.HostAlias {
	endpoint := cfg.Cluster().Endpoint()
	if endpoint == nil {
		return nil
	}

	host := endpoint.Hostname()
	if net.ParseIP(host) != nil {
		return nil
	}

	for _, entry := range cfg.NetworkStaticHostConfig() {
		for _, alias := range entry.Aliases() {
			if strings.EqualFold(alias, host) {
				return &corev1.HostAlias{
					IP:        entry.IP(),
					Hostnames: []string{host},
				}
			}
		}
	}

	return &corev1.HostAlias{
		IP:        fallbackIP,
		Hostnames: []string{host},
	}
}
