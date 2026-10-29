package talosupgrade

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
)

const (
	testTalos114    = "v1.14.0"
	testTalos1115   = "v1.11.5"
	testKube133     = "v1.33.1"
	testTooOldError = "host version 1.11.5 is too old to upgrade to Talos 1.14.0"
)

func newTalosNode(name, talosVersion, kubeletVersion string) *corev1.Node {
	node := newWebhookNode(name, nil)
	if talosVersion != "" {
		node.Status.NodeInfo.OSImage = "Talos (" + talosVersion + ")"
	}
	node.Status.NodeInfo.KubeletVersion = kubeletVersion
	return node
}

func withVersionAnnotation(node *corev1.Node, version string) *corev1.Node {
	node.Annotations = map[string]string{constants.VersionAnnotation: version}
	return node
}

// upgradePathWarnings returns the warnings validateUpgradePath can emit, so a
// case can assert none appeared without matching the webhook's other warnings.
func upgradePathWarnings(warnings []string) []string {
	var out []string
	for _, w := range warnings {
		if strings.Contains(w, "will be downgraded") || strings.Contains(w, "compatibility data") {
			out = append(out, w)
		}
	}
	return out
}

func TestTalosUpgrade_ValidateCreate_UpgradePath(t *testing.T) {
	nonTalos := newWebhookNode(testNode1, nil)
	nonTalos.Status.NodeInfo.OSImage = "Ubuntu 22.04 LTS"
	nonTalos.Status.NodeInfo.KubeletVersion = "v1.20.0"

	tooOld := func(names ...string) []*corev1.Node {
		nodes := make([]*corev1.Node, 0, len(names))
		for _, n := range names {
			nodes = append(nodes, newTalosNode(n, testTalos1115, testKube133))
		}
		return nodes
	}

	tests := []struct {
		name        string
		target      string
		nodes       []*corev1.Node
		wantErr     []string
		wantWarning string
	}{
		{
			name:   "supported upgrade",
			target: testTalos114,
			nodes:  []*corev1.Node{newTalosNode(testNode1, "v1.12.3", testKube133)},
		},
		{
			name:   "node already at the target",
			target: testTalos114,
			nodes:  []*corev1.Node{newTalosNode(testNode1, testTalos114, "v1.31.0")},
		},
		{
			name:   "node without a reported osImage is skipped",
			target: testTalos114,
			nodes:  []*corev1.Node{newTalosNode(testNode1, "", "")},
		},
		{
			name:   "non-Talos osImage is skipped",
			target: testTalos114,
			nodes:  []*corev1.Node{nonTalos},
		},
		{
			name:   "pre-release target",
			target: "v1.14.0-beta.0",
			nodes:  []*corev1.Node{newTalosNode(testNode1, "v1.12.3", testKube133)},
		},
		{
			name:    "host too old for the target",
			target:  testTalos114,
			nodes:   append(tooOld(testNode1, testNode2), newTalosNode(testNode3, "v1.13.2", testKube133)),
			wantErr: []string{testTooOldError, "(node(s) node-1, node-2)"},
		},
		{
			name:    "long node lists are truncated",
			target:  testTalos114,
			nodes:   tooOld("node-a", "node-b", "node-c", "node-d", "node-e"),
			wantErr: []string{"(node(s) node-a, node-b, node-c and 2 more)"},
		},
		{
			name:    "running Kubernetes too old for the target",
			target:  testTalos114,
			nodes:   []*corev1.Node{newTalosNode(testNode1, "v1.13.2", "v1.31.4")},
			wantErr: []string{"Kubernetes 1.31.4 is too old to be used with Talos 1.14.0", "(node(s) node-1)"},
		},
		{
			name:    "running Kubernetes too new for the target",
			target:  "v1.13.5",
			nodes:   []*corev1.Node{newTalosNode(testNode1, "v1.12.3", "v1.37.0")},
			wantErr: []string{"Kubernetes 1.37.0 is too new to be used with Talos 1.13.5"},
		},
		{
			name:   "several reasons are listed in order",
			target: testTalos114,
			nodes: []*corev1.Node{
				newTalosNode(testNode1, "v1.13.2", "v1.31.4"),
				newTalosNode(testNode2, testTalos1115, testKube133),
			},
			wantErr: []string{
				testTooOldError + " (node(s) node-2); version of Kubernetes 1.31.4 is too old to be used with Talos 1.14.0 (node(s) node-1)",
			},
		},
		{
			name:        "supported downgrade warns",
			target:      "v1.13.5",
			nodes:       []*corev1.Node{newTalosNode(testNode1, "v1.14.1", testKube133)},
			wantWarning: "Node(s) node-1 will be downgraded from Talos v1.14.1 to v1.13.5",
		},
		{
			name:    "downgrade past the supported window",
			target:  "v1.12.0",
			nodes:   []*corev1.Node{newTalosNode(testNode1, "v1.14.1", testKube133)},
			wantErr: []string{"host version 1.14.1 is too new to downgrade to Talos 1.12.0"},
		},
		{
			name:        "target outside the compatibility data warns",
			target:      "v1.99.0",
			nodes:       []*corev1.Node{newTalosNode(testNode1, "v1.12.3", testKube133)},
			wantWarning: "Talos v1.99.0 is not in the compatibility data",
		},
		{
			name:        "target outside the compatibility data skips the Kubernetes check",
			target:      "v1.99.0",
			nodes:       []*corev1.Node{newTalosNode(testNode1, "v1.12.3", "v1.20.0")},
			wantWarning: "Talos v1.99.0 is not in the compatibility data",
		},
		{
			name:   "per-node version annotation is the node's target",
			target: testTalos114,
			nodes:  []*corev1.Node{withVersionAnnotation(newTalosNode(testNode1, testTalos1115, testKube133), "v1.13.0")},
		},
		{
			name:   "annotation without the v prefix matches the running version",
			target: testTalos114,
			nodes:  []*corev1.Node{withVersionAnnotation(newTalosNode(testNode1, "v1.13.0", testKube133), "1.13.0")},
		},
		{
			name:    "rejection names the pinning annotation",
			target:  "v1.13.0",
			nodes:   []*corev1.Node{withVersionAnnotation(newTalosNode(testNode1, testTalos1115, testKube133), testTalos114)},
			wantErr: []string{testTooOldError, "(node(s) node-1 (pinned by " + constants.VersionAnnotation + "))"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []runtime.Object{talosConfigSecretWithKey(testNamespace, validTalosConfig())}
			for _, n := range tt.nodes {
				objects = append(objects, n)
			}
			v := newTalosValidator(objects...)

			warnings, err := v.ValidateCreate(context.Background(), newTalosUpgrade("test", withTalosVersion(tt.target)))
			if len(tt.wantErr) == 0 && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			for _, want := range tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("expected error containing %q, got: %v", want, err)
				}
			}

			pathWarnings := upgradePathWarnings(warnings)
			switch {
			case tt.wantWarning == "" && len(pathWarnings) > 0:
				t.Fatalf("expected no upgrade path warnings, got: %v", pathWarnings)
			case tt.wantWarning != "" && !containsWarning(pathWarnings, tt.wantWarning):
				t.Fatalf("expected warning containing %q, got: %v", tt.wantWarning, pathWarnings)
			}
		})
	}
}

func TestTalosUpgrade_ValidateCreate_UpgradePathFailsOpen(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = tupprv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(talosConfigSecretWithKey(testNamespace, validTalosConfig()), newTalosNode(testNode1, testTalos1115, testKube133)).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*corev1.NodeList); ok {
					return errors.New("apiserver unavailable")
				}
				return cl.List(ctx, list, opts...)
			},
		}).
		Build()
	v := &Validator{Client: c, TalosConfigSecret: testTalosConfigKey, Namespace: testNamespace}

	if _, err := v.ValidateCreate(context.Background(), newTalosUpgrade("test", withTalosVersion(testTalos114))); err != nil {
		t.Fatalf("expected the check to fail open when nodes can't be listed, got: %v", err)
	}
}

func TestTalosUpgrade_ValidateUpdate_UpgradePathOnlyOnTargetChange(t *testing.T) {
	secret := talosConfigSecretWithKey(testNamespace, validTalosConfig())
	tooOld := newTalosNode(testNode1, testTalos1115, testKube133)
	tooOld.Labels = map[string]string{testLabelRole: testRoleWorker}

	tests := []struct {
		name    string
		old     *tupprv1alpha1.TalosUpgrade
		update  func(*tupprv1alpha1.TalosUpgrade)
		wantErr bool
	}{
		{
			name: "metadata-only update is not re-checked",
			old:  newTalosUpgrade("test", withTalosVersion(testTalos114)),
			update: func(tu *tupprv1alpha1.TalosUpgrade) {
				tu.Finalizers = nil
			},
		},
		{
			name:    "changed target is checked",
			old:     newTalosUpgrade("test", withTalosVersion("v1.13.0"), withTalosPhase(tupprv1alpha1.JobPhaseCompleted)),
			update:  func(tu *tupprv1alpha1.TalosUpgrade) { tu.Spec.Talos.Version = testTalos114 },
			wantErr: true,
		},
		{
			name: "changed nodeSelector is checked",
			old:  newTalosUpgrade("test", withTalosVersion(testTalos114), withTalosPhase(tupprv1alpha1.JobPhaseCompleted)),
			update: func(tu *tupprv1alpha1.TalosUpgrade) {
				tu.Spec.NodeSelector = &metav1.LabelSelector{MatchLabels: map[string]string{testLabelRole: testRoleWorker}}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTalosValidator(secret.DeepCopy(), tooOld.DeepCopy())
			tt.old.Finalizers = []string{"tuppr.home-operations.com/finalizer"}
			updated := tt.old.DeepCopy()
			tt.update(updated)

			_, err := v.ValidateUpdate(context.Background(), tt.old, updated)
			if tt.wantErr && (err == nil || !strings.Contains(err.Error(), "too old to upgrade")) {
				t.Fatalf("expected upgrade path rejection, got: %v", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected update to be allowed, got: %v", err)
			}
		})
	}
}
