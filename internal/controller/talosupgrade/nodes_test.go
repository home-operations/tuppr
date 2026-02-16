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

const upgradingLabelValue = "true"

func TestAddNodeUpgradingLabel(t *testing.T) {
	scheme := newTestScheme()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fakeNodeA,
			Labels: map[string]string{"foo": "bar"},
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

	if updated.Labels["foo"] != "bar" {
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
				"foo":                        "bar",
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

	if updated.Labels["foo"] != "bar" {
		t.Fatal("expected other labels to be preserved")
	}
}

func TestRemoveNodeUpgradingLabel_Idempotent(t *testing.T) {
	scheme := newTestScheme()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fakeNodeA,
			Labels: map[string]string{"foo": "bar"},
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
