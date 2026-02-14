package controller

import (
	"context"
	"strings"
	"testing"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func TestGenerateSafeJobName_UniquePerCall(t *testing.T) {
	name1 := GenerateSafeJobName("test", "node")
	name2 := GenerateSafeJobName("test", "node")
	if name1 == name2 {
		t.Fatalf("expected unique names, got: %s and %s", name1, name2)
	}
}

func TestIsAnotherUpgradeActive(t *testing.T) {
	scheme := newScheme()

	tests := []struct {
		name            string
		currentType     string
		existingObjects []client.Object
		wantBlocked     bool
		wantMessagePart string
	}{
		// --- Scenarios where Talos is checking ---
		{
			name:            "Talos checks: No K8s upgrade exists",
			currentType:     "talos",
			existingObjects: []client.Object{},
			wantBlocked:     false,
		},
		{
			name:        "Talos checks: K8s upgrade is Pending (Should NOT block)",
			currentType: "talos",
			existingObjects: []client.Object{
				&tupprv1alpha1.KubernetesUpgrade{
					ObjectMeta: metav1.ObjectMeta{Name: "k8s-upgrade"},
					Status:     tupprv1alpha1.KubernetesUpgradeStatus{Phase: constants.PhasePending},
				},
			},
			wantBlocked: false,
		},
		{
			name:        "Talos checks: K8s upgrade is InProgress (SHOULD block)",
			currentType: "talos",
			existingObjects: []client.Object{
				&tupprv1alpha1.KubernetesUpgrade{
					ObjectMeta: metav1.ObjectMeta{Name: "k8s-upgrade"},
					Status:     tupprv1alpha1.KubernetesUpgradeStatus{Phase: constants.PhaseInProgress},
				},
			},
			wantBlocked:     true,
			wantMessagePart: "Waiting for KubernetesUpgrade",
		},
		{
			name:        "Talos checks: K8s upgrade is Completed (Should NOT block)",
			currentType: "talos",
			existingObjects: []client.Object{
				&tupprv1alpha1.KubernetesUpgrade{
					ObjectMeta: metav1.ObjectMeta{Name: "k8s-upgrade"},
					Status:     tupprv1alpha1.KubernetesUpgradeStatus{Phase: constants.PhaseCompleted},
				},
			},
			wantBlocked: false,
		},
		{
			name:        "Talos checks: K8s upgrade is Failed (Should NOT block)",
			currentType: "talos",
			existingObjects: []client.Object{
				&tupprv1alpha1.KubernetesUpgrade{
					ObjectMeta: metav1.ObjectMeta{Name: "k8s-upgrade"},
					Status:     tupprv1alpha1.KubernetesUpgradeStatus{Phase: constants.PhaseFailed},
				},
			},
			wantBlocked: false,
		},

		// --- Scenarios where Kubernetes is checking ---
		{
			name:            "K8s checks: No Talos upgrade exists",
			currentType:     "kubernetes",
			existingObjects: []client.Object{},
			wantBlocked:     false,
		},
		{
			name:        "K8s checks: Talos upgrade is InProgress (SHOULD block)",
			currentType: "kubernetes",
			existingObjects: []client.Object{
				&tupprv1alpha1.TalosUpgrade{
					ObjectMeta: metav1.ObjectMeta{Name: "talos-upgrade"},
					Status:     tupprv1alpha1.TalosUpgradeStatus{Phase: constants.PhaseInProgress},
				},
			},
			wantBlocked:     true,
			wantMessagePart: "Waiting for TalosUpgrade",
		},
		{
			name:        "K8s checks: Talos upgrade is Pending",
			currentType: "kubernetes",
			existingObjects: []client.Object{
				&tupprv1alpha1.TalosUpgrade{
					ObjectMeta: metav1.ObjectMeta{Name: "talos-upgrade"},
					Status:     tupprv1alpha1.TalosUpgradeStatus{Phase: constants.PhasePending},
				},
			},
			wantBlocked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.existingObjects...).Build()

			blocked, msg, err := IsAnotherUpgradeActive(context.Background(), cl, tt.currentType)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if blocked != tt.wantBlocked {
				t.Errorf("IsAnotherUpgradeActive() blocked = %v, want %v", blocked, tt.wantBlocked)
			}

			if tt.wantBlocked && !strings.Contains(msg, tt.wantMessagePart) {
				t.Errorf("expected message to contain %q, got %q", tt.wantMessagePart, msg)
			}
		})
	}
}
