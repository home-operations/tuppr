package talos

import (
	"testing"

	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
)

const baseHostnameEndpoint = `version: v1alpha1
machine:
  type: controlplane
cluster:
  controlPlane:
    endpoint: https://kube.cluster.local:6443
`

func TestResolveControlPlaneHostAlias_IPLiteralEndpoint(t *testing.T) {
	yaml := `version: v1alpha1
machine:
  type: controlplane
cluster:
  controlPlane:
    endpoint: https://10.0.1.100:6443
`
	p, err := configloader.NewFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := ResolveControlPlaneHostAlias(p, "10.0.0.1"); got != nil {
		t.Fatalf("expected nil for IP literal, got: %+v", got)
	}
}

func TestResolveControlPlaneHostAlias_IPv6LiteralEndpoint(t *testing.T) {
	yaml := `version: v1alpha1
machine:
  type: controlplane
cluster:
  controlPlane:
    endpoint: https://[fd00::1]:6443
`
	p, err := configloader.NewFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := ResolveControlPlaneHostAlias(p, "10.0.0.1"); got != nil {
		t.Fatalf("expected nil for IPv6 literal, got: %+v", got)
	}
}

func TestResolveControlPlaneHostAlias_MatchesExtraHostEntries(t *testing.T) {
	yaml := `version: v1alpha1
machine:
  type: controlplane
  network:
    extraHostEntries:
      - ip: 10.0.1.100
        aliases:
          - kube.cluster.local
          - cluster.local
cluster:
  controlPlane:
    endpoint: https://kube.cluster.local:6443
`
	p, err := configloader.NewFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := ResolveControlPlaneHostAlias(p, "10.0.0.1")
	if got == nil {
		t.Fatal("expected alias")
	}
	if got.IP != "10.0.1.100" {
		t.Fatalf("expected IP 10.0.1.100, got: %s", got.IP)
	}
	// Only the endpoint hostname should be in the alias — propagating sibling
	// aliases like `cluster.local` would hijack the cluster's DNS suffix in the pod.
	if len(got.Hostnames) != 1 || got.Hostnames[0] != "kube.cluster.local" {
		t.Fatalf("expected only the endpoint hostname, got: %v", got.Hostnames)
	}
}

func TestResolveControlPlaneHostAlias_MatchesStaticHostConfigDoc(t *testing.T) {
	yaml := baseHostnameEndpoint + `---
apiVersion: v1alpha1
kind: StaticHostConfig
name: 10.0.1.100
hostnames:
  - kube.cluster.local
`
	p, err := configloader.NewFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := ResolveControlPlaneHostAlias(p, "10.0.0.1")
	if got == nil || got.IP != "10.0.1.100" {
		t.Fatalf("expected match against StaticHostConfig doc, got: %+v", got)
	}
}

func TestResolveControlPlaneHostAlias_NoMatchUsesFallback(t *testing.T) {
	p, err := configloader.NewFromBytes([]byte(baseHostnameEndpoint))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := ResolveControlPlaneHostAlias(p, "10.0.0.1")
	if got == nil {
		t.Fatal("expected fallback alias")
	}
	if got.IP != "10.0.0.1" {
		t.Fatalf("expected fallback IP, got: %s", got.IP)
	}
	if len(got.Hostnames) != 1 || got.Hostnames[0] != "kube.cluster.local" {
		t.Fatalf("expected endpoint hostname, got: %v", got.Hostnames)
	}
}
