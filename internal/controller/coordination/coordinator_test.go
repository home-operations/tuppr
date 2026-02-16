package coordination

import (
	"context"
	"strings"
	"testing"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = tupprv1alpha1.AddToScheme(scheme)
	return scheme
}

func TestIsAnotherUpgradeActive(t *testing.T) {
	scheme := newTestScheme()

	tests := []struct {
		name            string
		currentName     string // Name of the resource being reconciled
		currentType     string
		existingObjects []client.Object
		wantBlocked     bool
		wantMessagePart string
	}{
		// --- Scenarios where Talos is checking (Cross-Type: K8s) ---
		{
			name:            "Talos checks: No K8s upgrade exists",
			currentName:     "talos-upgrade",
			currentType:     "talos",
			existingObjects: []client.Object{},
			wantBlocked:     false,
		},
		{
			name:        "Talos checks: K8s upgrade is Pending (Should NOT block)",
			currentName: "talos-upgrade",
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
			currentName: "talos-upgrade",
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

		// --- Scenarios where Talos is checking (Same-Type: Queuing) ---
		{
			name:        "Talos checks: Another Talos plan is InProgress (SHOULD block)",
			currentName: "talos-worker-west",
			currentType: "talos",
			existingObjects: []client.Object{
				&tupprv1alpha1.TalosUpgrade{
					ObjectMeta: metav1.ObjectMeta{Name: "talos-worker-east"},
					Status:     tupprv1alpha1.TalosUpgradeStatus{Phase: constants.PhaseInProgress},
				},
			},
			wantBlocked:     true,
			wantMessagePart: "Waiting for another TalosUpgrade plan",
		},
		{
			name:        "Talos checks: Another Talos plan is Pending (Should NOT block - race resolved by controller order)",
			currentName: "talos-worker-west",
			currentType: "talos",
			existingObjects: []client.Object{
				&tupprv1alpha1.TalosUpgrade{
					ObjectMeta: metav1.ObjectMeta{Name: "talos-worker-east"},
					Status:     tupprv1alpha1.TalosUpgradeStatus{Phase: constants.PhasePending},
				},
			},
			wantBlocked: false,
		},
		{
			name:        "Talos checks: Self is InProgress (Should NOT block)",
			currentName: "talos-worker-west",
			currentType: "talos",
			existingObjects: []client.Object{
				&tupprv1alpha1.TalosUpgrade{
					ObjectMeta: metav1.ObjectMeta{Name: "talos-worker-west"},
					Status:     tupprv1alpha1.TalosUpgradeStatus{Phase: constants.PhaseInProgress},
				},
			},
			wantBlocked: false,
		},

		// --- Scenarios where Kubernetes is checking ---
		{
			name:            "K8s checks: No Talos upgrade exists",
			currentName:     "k8s-upgrade",
			currentType:     "kubernetes",
			existingObjects: []client.Object{},
			wantBlocked:     false,
		},
		{
			name:        "K8s checks: Talos upgrade is InProgress (SHOULD block)",
			currentName: "k8s-upgrade",
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
			name:        "K8s checks: Talos upgrade is Pending (SHOULD block)",
			currentName: "k8s-upgrade",
			currentType: "kubernetes",
			existingObjects: []client.Object{
				&tupprv1alpha1.TalosUpgrade{
					ObjectMeta: metav1.ObjectMeta{Name: "talos-upgrade"},
					Status:     tupprv1alpha1.TalosUpgradeStatus{Phase: constants.PhasePending},
				},
			},
			wantBlocked:     true,
			wantMessagePart: "Waiting for TalosUpgrade",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.existingObjects...).Build()

			// Updated call with currentName
			blocked, msg, err := IsAnotherUpgradeActive(context.Background(), cl, tt.currentName, tt.currentType)
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
