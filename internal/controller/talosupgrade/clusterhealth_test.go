package talosupgrade

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
)

const testPoolLabel = "pool"

func newControlPlaneNode(name, ip string) *corev1.Node {
	n := newNode(name, ip)
	n.Labels = map[string]string{controlPlaneLabel: ""}
	return n
}

func newNotReady(n *corev1.Node) *corev1.Node {
	n.Status.Conditions = []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}
	return n
}

func newPoolNode(name, ip, pool string) *corev1.Node {
	n := newNode(name, ip)
	n.Labels = map[string]string{testPoolLabel: pool}
	return n
}

func withPoolSelector(pool string) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.NodeSelector = &metav1.LabelSelector{MatchLabels: map[string]string{testPoolLabel: pool}}
	}
}

func TestCheckClusterHealth(t *testing.T) {
	tests := []struct {
		name         string
		selectorPool string
		nodes        []client.Object
		machineErrs  map[string]error
		etcdErr      error
		wantReason   string
		wantMachine  []string
		wantEtcdIPs  []string
		wantNoEtcd   bool
	}{
		{
			name:        "all workers ready, no etcd to check",
			nodes:       []client.Object{newNode(fakeNodeA, testNodeIP1), newNode(fakeNodeB, testNodeIP2)},
			wantMachine: []string{testNodeIP1, testNodeIP2},
		},
		{
			name: "control-plane IPs passed in node-name order",
			nodes: []client.Object{
				newControlPlaneNode(fakeNodeC, testNodeIP3),
				newNode(fakeNodeB, testNodeIP2),
				newControlPlaneNode(fakeNodeA, testNodeIP1),
			},
			wantMachine: []string{testNodeIP1, testNodeIP2, testNodeIP3},
			wantEtcdIPs: []string{testNodeIP1, testNodeIP3},
		},
		{
			name:         "unselected worker is ignored, even when not Ready",
			selectorPool: "a",
			nodes: []client.Object{
				newPoolNode(fakeNodeA, testNodeIP1, "a"),
				newNotReady(newPoolNode(fakeNodeB, testNodeIP2, "b")),
			},
			wantMachine: []string{testNodeIP1},
		},
		{
			name:         "unselected control-plane node is still checked",
			selectorPool: "a",
			nodes: []client.Object{
				newPoolNode(fakeNodeA, testNodeIP1, "a"),
				newNotReady(newControlPlaneNode(fakeNodeB, testNodeIP2)),
			},
			wantReason: "node(s) not Ready: " + fakeNodeB,
			wantNoEtcd: true,
		},
		{
			name:       "not-ready node blocks before Talos is queried",
			nodes:      []client.Object{newNode(fakeNodeA, testNodeIP1), newNotReady(newNode(fakeNodeB, testNodeIP2))},
			wantReason: "node(s) not Ready: " + fakeNodeB,
			wantNoEtcd: true,
		},
		{
			name:        "Talos MachineStatus not ready blocks before etcd is queried",
			nodes:       []client.Object{newNode(fakeNodeA, testNodeIP1), newControlPlaneNode(fakeNodeB, testNodeIP2)},
			machineErrs: map[string]error{testNodeIP2: errors.New("node 10.0.0.2 not ready: services: etcd not healthy")},
			wantReason:  "node node-b: Talos machine not ready",
			wantMachine: []string{testNodeIP1, testNodeIP2},
			wantNoEtcd:  true,
		},
		{
			name:        "etcd failure is reported",
			nodes:       []client.Object{newControlPlaneNode(fakeNodeA, testNodeIP1)},
			etcdErr:     errors.New("etcd member node-a is still a learner"),
			wantReason:  "etcd not healthy",
			wantMachine: []string{testNodeIP1},
			wantEtcdIPs: []string{testNodeIP1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.nodes...).Build()
			tc := &mockTalosClient{machineReadyErrs: tt.machineErrs, etcdErr: tt.etcdErr}
			r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})
			var opts []func(*tupprv1alpha1.TalosUpgrade)
			if tt.selectorPool != "" {
				opts = append(opts, withPoolSelector(tt.selectorPool))
			}
			tu := newTalosUpgrade(testUpgradeName, opts...)

			err := r.checkClusterHealth(context.Background(), tu)
			if tt.wantReason == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			} else {
				unhealthy, ok := errors.AsType[*unhealthyError](err)
				if !ok {
					t.Fatalf("expected *unhealthyError, got: %v", err)
				}
				if unhealthy.reason != tt.wantReason {
					t.Fatalf("expected reason %q, got %q", tt.wantReason, unhealthy.reason)
				}
			}

			if !slices.Equal(tc.machineReadyCalls, tt.wantMachine) {
				t.Fatalf("expected MachineStatus checks on %v, got %v", tt.wantMachine, tc.machineReadyCalls)
			}
			if tt.wantNoEtcd {
				if len(tc.etcdCalls) != 0 {
					t.Fatalf("expected no etcd check, got calls: %v", tc.etcdCalls)
				}
				return
			}
			if len(tc.etcdCalls) != 1 {
				t.Fatalf("expected one etcd check, got %d", len(tc.etcdCalls))
			}
			if !slices.Equal(tc.etcdCalls[0], tt.wantEtcdIPs) {
				t.Fatalf("expected etcd check on %v, got %v", tt.wantEtcdIPs, tc.etcdCalls[0])
			}
		})
	}
}

func TestCheckClusterHealth_Timeout(t *testing.T) {
	scheme := newTestScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(newControlPlaneNode(fakeNodeA, testNodeIP1)).Build()
	tc := &mockTalosClient{etcdBlock: true}
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})
	r.clusterHealthTimeout = 50 * time.Millisecond

	start := time.Now()
	err := r.checkClusterHealth(context.Background(), newTalosUpgrade(testUpgradeName))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("expected the gate to give up near its timeout, took %s", elapsed)
	}
}

func TestTalosReconcile_ClusterHealthGatesBatch(t *testing.T) {
	tests := []struct {
		name        string
		extraNode   client.Object
		machineErrs map[string]error
		etcdErr     error
		wantMessage string
	}{
		{
			name:        "control-plane node outside the selection is not Ready",
			extraNode:   newNotReady(newControlPlaneNode(fakeNodeB, testNodeIP2)),
			wantMessage: "Waiting for cluster health: node(s) not Ready: " + fakeNodeB,
		},
		{
			name:        "volatile Talos detail stays out of the status message",
			extraNode:   newControlPlaneNode(fakeNodeB, testNodeIP2),
			machineErrs: map[string]error{testNodeIP2: errors.New("rpc error: code = Unavailable; 3 attempts")},
			wantMessage: "Waiting for cluster health: node node-b: Talos machine not ready",
		},
		{
			name:        "etcd not healthy",
			extraNode:   newControlPlaneNode(fakeNodeB, testNodeIP2),
			etcdErr:     errors.New("etcd on 10.0.0.2 has no leader"),
			wantMessage: "Waiting for cluster health: etcd not healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			tu := newTalosUpgrade(testUpgradeName,
				withFinalizer,
				withPhase(tupprv1alpha1.JobPhasePending),
				withPoolSelector("a"),
			)
			target := newPoolNode(fakeNodeA, testNodeIP1, "a")
			tc := &mockTalosClient{
				nodeVersions: map[string]string{
					testNodeIP1: testV110Talos,
					testNodeIP2: fakeTalosVersion,
				},
				installImages:    map[string]string{testNodeIP1: testFactoryInstaller},
				machineReadyErrs: tt.machineErrs,
				etcdErr:          tt.etcdErr,
			}
			hc := &mockHealthChecker{}
			cl := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(tu, target, tt.extraNode).WithStatusSubresource(tu).Build()
			r := newTalosReconciler(cl, scheme, tc, hc)

			reconcileTalos(t, r, testUpgradeName)

			updated := getTalosUpgrade(t, cl, testUpgradeName)
			if updated.Status.Phase != tupprv1alpha1.JobPhaseHealthChecking {
				t.Fatalf("expected phase HealthChecking, got: %s", updated.Status.Phase)
			}
			if updated.Status.Message != tt.wantMessage {
				t.Fatalf("expected message %q, got: %q", tt.wantMessage, updated.Status.Message)
			}
			if hc.calls != 0 {
				t.Fatalf("expected user health checks not to run while the cluster is unhealthy, got %d calls", hc.calls)
			}
			jobList := &batchv1.JobList{}
			if err := cl.List(context.Background(), jobList); err != nil {
				t.Fatalf("list jobs: %v", err)
			}
			if len(jobList.Items) != 0 {
				t.Fatalf("expected no upgrade job while the cluster is unhealthy, got %d", len(jobList.Items))
			}
		})
	}
}

func TestTalosReconcile_ClusterHealthGatesCompletion(t *testing.T) {
	tests := []struct {
		name      string
		completed []string
		upgrading bool
		wantPhase tupprv1alpha1.JobPhase
	}{
		{
			name:      "run that upgraded a node waits before completing",
			completed: []string{fakeNodeA},
			wantPhase: tupprv1alpha1.JobPhaseHealthChecking,
		},
		{
			name:      "upgrade whose Job was garbage collected unobserved still waits",
			upgrading: true,
			wantPhase: tupprv1alpha1.JobPhaseHealthChecking,
		},
		{
			name:      "run with nothing to upgrade completes regardless",
			wantPhase: tupprv1alpha1.JobPhaseCompleted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			tu := newTalosUpgrade(testUpgradeName,
				withFinalizer,
				withPhase(tupprv1alpha1.JobPhasePending),
				withCompletedNodes(tt.completed...),
			)
			node := newControlPlaneNode(fakeNodeA, testNodeIP1)
			if tt.upgrading {
				node.Labels[constants.NodeUpgradingLabel] = upgradingLabelValue
			}
			tc := &mockTalosClient{
				nodeVersions: map[string]string{testNodeIP1: fakeTalosVersion},
				etcdErr:      errors.New("etcd member node-a is still a learner"),
			}
			cl := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(tu, node).WithStatusSubresource(tu).Build()
			r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

			reconcileTalos(t, r, testUpgradeName)

			updated := getTalosUpgrade(t, cl, testUpgradeName)
			if updated.Status.Phase != tt.wantPhase {
				t.Fatalf("expected phase %s, got: %s (%s)", tt.wantPhase, updated.Status.Phase, updated.Status.Message)
			}
			if tt.wantPhase == tupprv1alpha1.JobPhaseHealthChecking &&
				!strings.Contains(updated.Status.Message, "etcd not healthy") {
				t.Fatalf("expected etcd wait message, got: %s", updated.Status.Message)
			}
			if tt.upgrading {
				if !slices.Contains(updated.Status.CompletedNodes, fakeNodeA) {
					t.Fatalf("expected %s recorded as completed, got %v", fakeNodeA, updated.Status.CompletedNodes)
				}
				got := &corev1.Node{}
				if err := cl.Get(context.Background(), client.ObjectKey{Name: fakeNodeA}, got); err != nil {
					t.Fatalf("get node: %v", err)
				}
				if _, ok := got.Labels[constants.NodeUpgradingLabel]; ok {
					t.Fatalf("expected the stale upgrading label to be removed")
				}
			}
		})
	}
}
