package talosupgrade

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

func newControlPlaneNode(name, ip string) *corev1.Node {
	n := newNode(name, ip)
	n.Labels = map[string]string{controlPlaneLabel: ""}
	return n
}

func newNotReadyNode(name, ip string) *corev1.Node {
	n := newNode(name, ip)
	n.Status.Conditions = []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}
	return n
}

func TestCheckClusterHealth(t *testing.T) {
	tests := []struct {
		name        string
		nodes       []client.Object
		machineErrs map[string]error
		etcdErr     error
		wantErr     string
		wantEtcdIPs []string
		wantNoEtcd  bool
	}{
		{
			name:        "all workers ready, no etcd to check",
			nodes:       []client.Object{newNode(fakeNodeA, testNodeIP1), newNode(fakeNodeB, testNodeIP2)},
			wantEtcdIPs: nil,
		},
		{
			name: "control-plane IPs passed in node-name order",
			nodes: []client.Object{
				newControlPlaneNode(fakeNodeC, testNodeIP3),
				newNode(fakeNodeB, testNodeIP2),
				newControlPlaneNode(fakeNodeA, testNodeIP1),
			},
			wantEtcdIPs: []string{testNodeIP1, testNodeIP3},
		},
		{
			name:       "not-ready node blocks before etcd is queried",
			nodes:      []client.Object{newNode(fakeNodeA, testNodeIP1), newNotReadyNode(fakeNodeB, testNodeIP2)},
			wantErr:    "node(s) not Ready: " + fakeNodeB,
			wantNoEtcd: true,
		},
		{
			name:        "Talos MachineStatus not ready blocks before etcd is queried",
			nodes:       []client.Object{newNode(fakeNodeA, testNodeIP1), newControlPlaneNode(fakeNodeB, testNodeIP2)},
			machineErrs: map[string]error{testNodeIP2: errors.New("node 10.0.0.2 not ready: services: etcd not healthy")},
			wantErr:     "node node-b: node 10.0.0.2 not ready: services: etcd not healthy",
			wantNoEtcd:  true,
		},
		{
			name:        "etcd failure is reported",
			nodes:       []client.Object{newControlPlaneNode(fakeNodeA, testNodeIP1)},
			etcdErr:     errors.New("etcd member node-a is still a learner"),
			wantErr:     "etcd not healthy: etcd member node-a is still a learner",
			wantEtcdIPs: []string{testNodeIP1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.nodes...).Build()
			tc := &mockTalosClient{machineReadyErrs: tt.machineErrs, etcdErr: tt.etcdErr}
			r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

			err := r.checkClusterHealth(context.Background())
			if tt.wantErr == "" && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr) {
				t.Fatalf("expected error %q, got: %v", tt.wantErr, err)
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

func TestTalosReconcile_ClusterHealthGatesBatch(t *testing.T) {
	tests := []struct {
		name        string
		extraNode   client.Object
		etcdErr     error
		wantMessage string
	}{
		{
			name:        "node outside the selection is not Ready",
			extraNode:   newNotReadyNode(fakeNodeB, testNodeIP2),
			wantMessage: "Waiting for cluster health: node(s) not Ready: " + fakeNodeB,
		},
		{
			name:        "etcd not healthy",
			extraNode:   newControlPlaneNode(fakeNodeB, testNodeIP2),
			etcdErr:     errors.New("etcd on 10.0.0.2 has no leader"),
			wantMessage: "Waiting for cluster health: etcd not healthy: etcd on 10.0.0.2 has no leader",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			tu := newTalosUpgrade(testUpgradeName,
				withFinalizer,
				withPhase(tupprv1alpha1.JobPhasePending),
			)
			tu.Spec.NodeSelector = nil
			target := newNode(fakeNodeA, testNodeIP1)
			tc := &mockTalosClient{
				nodeVersions: map[string]string{
					testNodeIP1: testV110Talos,
					testNodeIP2: fakeTalosVersion,
				},
				installImages: map[string]string{testNodeIP1: testFactoryInstaller},
				etcdErr:       tt.etcdErr,
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
			if !strings.Contains(updated.Status.Message, tt.wantMessage) {
				t.Fatalf("expected message %q, got: %s", tt.wantMessage, updated.Status.Message)
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
