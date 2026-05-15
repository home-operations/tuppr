package metrics

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabel "k8s.io/apimachinery/pkg/labels"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

const (
	labelControlPlane = "node-role.kubernetes.io/control-plane"
	labelWorker       = "node-role.kubernetes.io/worker"
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

func TestSelectorFor(t *testing.T) {
	got, err := selectorFor(nil)
	if err != nil {
		t.Fatalf("selectorFor(nil) error = %v", err)
	}
	if !got.Matches(k8slabel.Set{"any": "thing"}) {
		t.Errorf("nil selector should match everything")
	}

	ls := &metav1.LabelSelector{MatchLabels: map[string]string{labelWorker: ""}}
	sel, err := selectorFor(ls)
	if err != nil {
		t.Fatalf("selectorFor(worker) error = %v", err)
	}
	if !sel.Matches(k8slabel.Set{labelWorker: ""}) {
		t.Errorf("worker selector should match worker node")
	}
	if sel.Matches(k8slabel.Set{labelControlPlane: ""}) {
		t.Errorf("worker selector should not match control-plane node")
	}

	bad := &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "x", Operator: "BadOperator"}}}
	if _, err := selectorFor(bad); err == nil {
		t.Errorf("expected error for invalid selector operator")
	}
}

func TestAppendWindows(t *testing.T) {
	windows := []tupprv1alpha1.WindowSpec{
		{Start: testWindowStartA, Duration: metav1.Duration{Duration: 4 * time.Hour}, Timezone: defaultTimezone},
		{Start: testWindowStartB, Duration: metav1.Duration{Duration: 2*time.Hour + 30*time.Minute}},
	}

	got := appendWindows(nil, UpgradeTypeTalos, fleet, windows)
	want := []MaintenanceWindowSnapshot{
		{UpgradeType: UpgradeTypeTalos, Name: fleet, Index: "0", Start: testWindowStartA, Duration: testDuration4h, Timezone: defaultTimezone},
		{UpgradeType: UpgradeTypeTalos, Name: fleet, Index: "1", Start: testWindowStartB, Duration: "2h30m0s", Timezone: defaultTimezone},
	}
	if len(got) != len(want) {
		t.Fatalf("appendWindows length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("appendWindows[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	if out := appendWindows(nil, UpgradeTypeKubernetes, "cluster", nil); len(out) != 0 {
		t.Errorf("appendWindows(nil) = %v, want empty", out)
	}
}

func TestNodeRole(t *testing.T) {
	cases := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{"control-plane label", map[string]string{labelControlPlane: ""}, NodeRoleControlPlane},
		{"legacy master label", map[string]string{"node-role.kubernetes.io/master": ""}, NodeRoleControlPlane},
		{"both labels", map[string]string{labelControlPlane: "", "node-role.kubernetes.io/master": ""}, NodeRoleControlPlane},
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
