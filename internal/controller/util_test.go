package controller

import (
	"context"
	"strings"
	"testing"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = tupprv1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = batchv1.AddToScheme(s)
	return s
}

func TestGenerateSafeJobName_UniquePerCall(t *testing.T) {
	name1 := GenerateSafeJobName("test", "node")
	name2 := GenerateSafeJobName("test", "node")
	if name1 == name2 {
		t.Fatalf("expected unique names, got: %s and %s", name1, name2)
	}
}

func TestIsAnotherUpgradeActive(t *testing.T) {
	tests := []struct {
		name        string
		upgradeType string
		objects     []interface{ GetName() string }
		wantBlocked bool
	}{
		{
			name:        "talos not blocked when no k8s upgrades",
			upgradeType: "talos",
			wantBlocked: false,
		},
		{
			name:        "kubernetes not blocked when no talos upgrades",
			upgradeType: "kubernetes",
			wantBlocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newScheme()
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()

			blocked, _, err := IsAnotherUpgradeActive(context.Background(), cl, tt.upgradeType)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if blocked != tt.wantBlocked {
				t.Fatalf("IsAnotherUpgradeActive() = %v, want %v", blocked, tt.wantBlocked)
			}
		})
	}
}

func TestIsAnotherUpgradeActive_TalosBlockedByK8s(t *testing.T) {
	scheme := newScheme()
	ku := &tupprv1alpha1.KubernetesUpgrade{
		ObjectMeta: metav1.ObjectMeta{Name: "k8s-upgrade"},
		Status: tupprv1alpha1.KubernetesUpgradeStatus{
			Phase: constants.PhaseInProgress,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ku).WithStatusSubresource(ku).Build()

	blocked, msg, err := IsAnotherUpgradeActive(context.Background(), cl, "talos")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked {
		t.Fatal("expected talos to be blocked by k8s upgrade")
	}
	if msg == "" {
		t.Fatal("expected non-empty message")
	}
}

func TestIsAnotherUpgradeActive_K8sBlockedByTalos(t *testing.T) {
	scheme := newScheme()
	tu := &tupprv1alpha1.TalosUpgrade{
		ObjectMeta: metav1.ObjectMeta{Name: "talos-upgrade"},
		Status: tupprv1alpha1.TalosUpgradeStatus{
			Phase: constants.PhaseInProgress,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tu).WithStatusSubresource(tu).Build()

	blocked, msg, err := IsAnotherUpgradeActive(context.Background(), cl, "kubernetes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked {
		t.Fatal("expected k8s to be blocked by talos upgrade")
	}
	if msg == "" {
		t.Fatal("expected non-empty message")
	}
}

func TestIsAnotherUpgradeActive_NotBlockedByCompleted(t *testing.T) {
	scheme := newScheme()
	ku := &tupprv1alpha1.KubernetesUpgrade{
		ObjectMeta: metav1.ObjectMeta{Name: "k8s-upgrade"},
		Status: tupprv1alpha1.KubernetesUpgradeStatus{
			Phase: constants.PhaseCompleted,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ku).WithStatusSubresource(ku).Build()

	blocked, _, err := IsAnotherUpgradeActive(context.Background(), cl, "talos")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Fatal("should not be blocked by completed upgrade")
	}
}
