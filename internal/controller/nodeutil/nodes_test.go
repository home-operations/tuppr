package nodeutil

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
				{Type: corev1.NodeExternalIP, Address: "1.2.3.4"},
				{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
			},
			wantIP: "10.0.0.1",
		},
		{
			name: "external IP fallback",
			addresses: []corev1.NodeAddress{
				{Type: corev1.NodeExternalIP, Address: "1.2.3.4"},
				{Type: corev1.NodeHostName, Address: "node-1"},
			},
			wantIP: "1.2.3.4",
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
				{Type: corev1.NodeHostName, Address: "node-1"},
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
			prefix:     "upgrade",
			identifier: "node-1",
		},
		{
			name:       "long identifier gets truncated",
			prefix:     "upgrade",
			identifier: "very-long-node-name-that-exceeds-kubernetes-limits-significantly",
		},
		{
			name:       "empty identifier",
			prefix:     "upgrade",
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
