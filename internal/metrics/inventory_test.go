package metrics

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseTalosVersion(t *testing.T) {
	cases := []struct {
		osImage string
		want    string
	}{
		{"Talos (v1.13.0)", "v1.13.0"},
		{"Talos (v1.7.5-pre.0)", "v1.7.5-pre.0"},
		{"Talos v1.5.0", "v1.5.0"},
		{"Talos (v1.10.6-amd64)", "v1.10.6-amd64"},
		{"Ubuntu 22.04 LTS", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := parseTalosVersion(c.osImage); got != c.want {
			t.Errorf("parseTalosVersion(%q) = %q, want %q", c.osImage, got, c.want)
		}
	}
}

func TestNodeRole(t *testing.T) {
	cases := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{"control-plane label", map[string]string{"node-role.kubernetes.io/control-plane": ""}, NodeRoleControlPlane},
		{"legacy master label", map[string]string{"node-role.kubernetes.io/master": ""}, NodeRoleControlPlane},
		{"both labels", map[string]string{"node-role.kubernetes.io/control-plane": "", "node-role.kubernetes.io/master": ""}, NodeRoleControlPlane},
		{"no role labels", map[string]string{"kubernetes.io/hostname": "worker-01"}, NodeRoleWorker},
		{"nil labels", nil, NodeRoleWorker},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: c.labels}}
			if got := nodeRole(n); got != c.want {
				t.Errorf("nodeRole(%v) = %q, want %q", c.labels, got, c.want)
			}
		})
	}
}
