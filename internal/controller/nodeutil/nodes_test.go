package nodeutil

import (
	"context"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const controlPlaneLabel = "node-role.kubernetes.io/control-plane"

const (
	testExternalIP   = "1.2.3.4"
	testHostnameNode = "node-1"
	testJobPrefix    = "upgrade"
)

func TestGetNodeIP(t *testing.T) {
	tests := []struct {
		name      string
		addresses []corev1.NodeAddress
		wantIP    string
		wantErr   bool
	}{
		{
			name: "internal IP preferred",
			addresses: []corev1.NodeAddress{
				{Type: corev1.NodeExternalIP, Address: testExternalIP},
				{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
			},
			wantIP: "10.0.0.1",
		},
		{
			name: "external IP fallback",
			addresses: []corev1.NodeAddress{
				{Type: corev1.NodeExternalIP, Address: testExternalIP},
				{Type: corev1.NodeHostName, Address: testHostnameNode},
			},
			wantIP: testExternalIP,
		},
		{
			name: "only internal IP",
			addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "192.168.1.1"},
			},
			wantIP: "192.168.1.1",
		},
		{
			name: "no IP addresses",
			addresses: []corev1.NodeAddress{
				{Type: corev1.NodeHostName, Address: testHostnameNode},
			},
			wantErr: true,
		},
		{
			name:      "empty addresses",
			addresses: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
				Status:     corev1.NodeStatus{Addresses: tt.addresses},
			}
			ip, err := GetNodeIP(node)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetNodeIP() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && ip != tt.wantIP {
				t.Fatalf("GetNodeIP() = %q, want %q", ip, tt.wantIP)
			}
		})
	}
}

func TestGenerateSafeJobName(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		identifier string
		maxLen     int
	}{
		{
			name:       "normal names",
			prefix:     testJobPrefix,
			identifier: "node-1",
		},
		{
			name:       "long identifier gets truncated",
			prefix:     testJobPrefix,
			identifier: "very-long-node-name-that-exceeds-kubernetes-limits-significantly",
		},
		{
			name:       "empty identifier",
			prefix:     testJobPrefix,
			identifier: "",
		},
		{
			name:       "long prefix with identifier",
			prefix:     strings.Repeat("a", 40),
			identifier: "node-with-a-long-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSafeJobName(tt.prefix, tt.identifier)
			if len(result) > 63 {
				t.Fatalf("job name too long: %d chars (%s)", len(result), result)
			}
			if result == "" {
				t.Fatal("expected non-empty job name")
			}
			if !strings.HasPrefix(result, tt.prefix) {
				t.Fatalf("expected prefix %q, got %q", tt.prefix, result)
			}
		})
	}
}

func TestGenerateSafeJobName_UniquePerCall(t *testing.T) {
	name1 := GenerateSafeJobName("test", "node")
	name2 := GenerateSafeJobName("test", "node")
	if name1 == name2 {
		t.Fatalf("expected unique names, got: %s and %s", name1, name2)
	}
}

func TestControlPlaneEndpointIPs(t *testing.T) {
	node := func(name, ip, role string, ready bool) *corev1.Node {
		status := corev1.ConditionFalse
		if ready {
			status = corev1.ConditionTrue
		}
		labels := map[string]string{}
		if role == "cp" {
			labels[controlPlaneLabel] = ""
		}
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
			Status: corev1.NodeStatus{
				Addresses:  []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: ip}},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: status}},
			},
		}
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		node("cp-b", "10.0.0.2", "cp", true),
		node("cp-a", "10.0.0.1", "cp", true),
		node("cp-down", "10.0.0.3", "cp", false),   // not Ready -> skipped
		node("worker", "10.0.0.9", "worker", true), // not control-plane -> skipped
	).Build()

	got := ControlPlaneEndpointIPs(context.Background(), cl, controlPlaneLabel)
	want := []string{"10.0.0.1", "10.0.0.2"} // sorted by node name (cp-a, cp-b)
	if !slices.Equal(got, want) {
		t.Fatalf("ControlPlaneEndpointIPs() = %v, want %v", got, want)
	}
}
