package talosupgrade

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/home-operations/tuppr/internal/constants"
)

const (
	labelBarValue = "bar"
	labelFooKey   = "foo"
)

func TestAddNodeUpgradingLabel(t *testing.T) {
	scheme := newTestScheme()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fakeNodeA,
			Labels: map[string]string{labelFooKey: labelBarValue},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &Reconciler{Client: cl}

	if err := r.addNodeUpgradingLabel(context.Background(), fakeNodeA); err != nil {
		t.Fatalf("addNodeUpgradingLabel failed: %v", err)
	}

	var updated corev1.Node
	if err := cl.Get(context.Background(), types.NamespacedName{Name: fakeNodeA}, &updated); err != nil {
		t.Fatalf("failed to get updated node: %v", err)
	}

	if updated.Labels[constants.NodeUpgradingLabel] != upgradingLabelValue {
		t.Fatalf("expected upgrading label to be %q, got: %s", upgradingLabelValue, updated.Labels[constants.NodeUpgradingLabel])
	}

	if updated.Labels[labelFooKey] != labelBarValue {
		t.Fatal("expected existing labels to be preserved")
	}
}

func TestAddNodeUpgradingLabel_Idempotent(t *testing.T) {
	scheme := newTestScheme()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fakeNodeA,
			Labels: map[string]string{constants.NodeUpgradingLabel: upgradingLabelValue},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &Reconciler{Client: cl}

	if err := r.addNodeUpgradingLabel(context.Background(), fakeNodeA); err != nil {
		t.Fatalf("addNodeUpgradingLabel failed: %v", err)
	}

	var updated corev1.Node
	if err := cl.Get(context.Background(), types.NamespacedName{Name: fakeNodeA}, &updated); err != nil {
		t.Fatalf("failed to get updated node: %v", err)
	}

	if updated.Labels[constants.NodeUpgradingLabel] != upgradingLabelValue {
		t.Fatalf("expected upgrading label to still be %q, got: %s", upgradingLabelValue, updated.Labels[constants.NodeUpgradingLabel])
	}
}

func TestRemoveNodeUpgradingLabel(t *testing.T) {
	scheme := newTestScheme()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: fakeNodeA,
			Labels: map[string]string{
				constants.NodeUpgradingLabel: upgradingLabelValue,
				"foo":                        labelBarValue,
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &Reconciler{Client: cl}

	if err := r.removeNodeUpgradingLabel(context.Background(), fakeNodeA); err != nil {
		t.Fatalf("removeNodeUpgradingLabel failed: %v", err)
	}

	var updated corev1.Node
	if err := cl.Get(context.Background(), types.NamespacedName{Name: fakeNodeA}, &updated); err != nil {
		t.Fatalf("failed to get updated node: %v", err)
	}

	if updated.Labels[constants.NodeUpgradingLabel] != "" {
		t.Fatalf("expected upgrading label to be removed, got: %s", updated.Labels[constants.NodeUpgradingLabel])
	}

	if updated.Labels[labelFooKey] != labelBarValue {
		t.Fatal("expected other labels to be preserved")
	}
}

func TestRemoveNodeUpgradingLabel_Idempotent(t *testing.T) {
	scheme := newTestScheme()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fakeNodeA,
			Labels: map[string]string{labelFooKey: labelBarValue},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &Reconciler{Client: cl}

	if err := r.removeNodeUpgradingLabel(context.Background(), fakeNodeA); err != nil {
		t.Fatalf("removeNodeUpgradingLabel failed: %v", err)
	}

	var updated corev1.Node
	if err := cl.Get(context.Background(), types.NamespacedName{Name: fakeNodeA}, &updated); err != nil {
		t.Fatalf("failed to get updated node: %v", err)
	}

	if updated.Labels[constants.NodeUpgradingLabel] != "" {
		t.Fatalf("expected upgrading label to not exist, got: %s", updated.Labels[constants.NodeUpgradingLabel])
	}
}

func hasOutdatedTaint(node *corev1.Node) bool {
	for _, t := range node.Spec.Taints {
		if t.Key == constants.NodeOutdatedTaint {
			return t.Effect == corev1.TaintEffectPreferNoSchedule
		}
	}
	return false
}

func TestAddNodeOutdatedTaint(t *testing.T) {
	scheme := newTestScheme()
	other := corev1.Taint{Key: labelFooKey, Effect: corev1.TaintEffectNoSchedule}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: fakeNodeA},
		Spec:       corev1.NodeSpec{Taints: []corev1.Taint{other}},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &Reconciler{Client: cl}

	if err := r.addNodeOutdatedTaint(context.Background(), fakeNodeA); err != nil {
		t.Fatalf("addNodeOutdatedTaint failed: %v", err)
	}

	var updated corev1.Node
	if err := cl.Get(context.Background(), types.NamespacedName{Name: fakeNodeA}, &updated); err != nil {
		t.Fatalf("failed to get updated node: %v", err)
	}

	if !hasOutdatedTaint(&updated) {
		t.Fatal("expected outdated PreferNoSchedule taint to be present")
	}
	if len(updated.Spec.Taints) != 2 {
		t.Fatalf("expected existing taints to be preserved, got %d taints", len(updated.Spec.Taints))
	}
}

func TestAddAndRemoveNodeUpgradingTaint(t *testing.T) {
	scheme := newTestScheme()
	other := corev1.Taint{Key: labelFooKey, Effect: corev1.TaintEffectNoSchedule}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: fakeNodeA},
		Spec:       corev1.NodeSpec{Taints: []corev1.Taint{other}},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &Reconciler{Client: cl}

	if err := r.addNodeUpgradingTaint(context.Background(), fakeNodeA); err != nil {
		t.Fatalf("addNodeUpgradingTaint failed: %v", err)
	}

	var tainted corev1.Node
	if err := cl.Get(context.Background(), types.NamespacedName{Name: fakeNodeA}, &tainted); err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	found := false
	for _, tt := range tainted.Spec.Taints {
		if tt.Key == constants.NodeUpgradingTaint {
			if tt.Effect != corev1.TaintEffectPreferNoSchedule {
				t.Fatalf("expected PreferNoSchedule, got %s", tt.Effect)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("expected upgrading taint to be present")
	}

	if err := r.removeNodeUpgradingTaint(context.Background(), fakeNodeA); err != nil {
		t.Fatalf("removeNodeUpgradingTaint failed: %v", err)
	}

	var cleared corev1.Node
	if err := cl.Get(context.Background(), types.NamespacedName{Name: fakeNodeA}, &cleared); err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	for _, tt := range cleared.Spec.Taints {
		if tt.Key == constants.NodeUpgradingTaint {
			t.Fatal("expected upgrading taint to be removed")
		}
	}
	if len(cleared.Spec.Taints) != 1 {
		t.Fatalf("expected the unrelated taint to be preserved, got %d", len(cleared.Spec.Taints))
	}
}

func TestAddNodeOutdatedTaint_Idempotent(t *testing.T) {
	scheme := newTestScheme()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: fakeNodeA},
		Spec: corev1.NodeSpec{Taints: []corev1.Taint{
			{Key: constants.NodeOutdatedTaint, Effect: corev1.TaintEffectPreferNoSchedule},
		}},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &Reconciler{Client: cl}

	if err := r.addNodeOutdatedTaint(context.Background(), fakeNodeA); err != nil {
		t.Fatalf("addNodeOutdatedTaint failed: %v", err)
	}

	var updated corev1.Node
	if err := cl.Get(context.Background(), types.NamespacedName{Name: fakeNodeA}, &updated); err != nil {
		t.Fatalf("failed to get updated node: %v", err)
	}

	if len(updated.Spec.Taints) != 1 {
		t.Fatalf("expected exactly one taint, got %d", len(updated.Spec.Taints))
	}
}

func TestRemoveNodeOutdatedTaint(t *testing.T) {
	scheme := newTestScheme()
	other := corev1.Taint{Key: labelFooKey, Effect: corev1.TaintEffectNoSchedule}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: fakeNodeA},
		Spec: corev1.NodeSpec{Taints: []corev1.Taint{
			other,
			{Key: constants.NodeOutdatedTaint, Effect: corev1.TaintEffectPreferNoSchedule},
		}},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &Reconciler{Client: cl}

	if err := r.removeNodeOutdatedTaint(context.Background(), fakeNodeA); err != nil {
		t.Fatalf("removeNodeOutdatedTaint failed: %v", err)
	}

	var updated corev1.Node
	if err := cl.Get(context.Background(), types.NamespacedName{Name: fakeNodeA}, &updated); err != nil {
		t.Fatalf("failed to get updated node: %v", err)
	}

	if hasOutdatedTaint(&updated) {
		t.Fatal("expected outdated taint to be removed")
	}
	if len(updated.Spec.Taints) != 1 || updated.Spec.Taints[0].Key != labelFooKey {
		t.Fatal("expected unrelated taint to be preserved")
	}
}

func TestRemoveNodeOutdatedTaint_Idempotent(t *testing.T) {
	scheme := newTestScheme()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: fakeNodeA}}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &Reconciler{Client: cl}

	if err := r.removeNodeOutdatedTaint(context.Background(), fakeNodeA); err != nil {
		t.Fatalf("removeNodeOutdatedTaint failed: %v", err)
	}
}
