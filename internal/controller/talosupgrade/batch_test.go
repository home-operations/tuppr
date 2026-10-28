package talosupgrade

import (
	"context"
	"slices"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/upgradeaudit"
)

const (
	fakeNodeD   = "node-d"
	testNodeIP4 = "10.0.0.4"
)

// upgradeJob is an upgrade Job from another plan targeting node; finished
// marks it Complete.
func upgradeJob(plan, node string, finished bool) *batchv1.Job {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      plan + "-" + node,
			Namespace: testNamespace,
			Labels: map[string]string{
				appLabelKey:         talosUpgradeAppName,
				appInstanceLabelKey: plan,
				targetNodeLabelKey:  node,
			},
		},
	}
	if finished {
		job.Status.Succeeded = 1
		job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
	}
	return job
}

func withUpgradingLabel(n *corev1.Node) *corev1.Node {
	n.Labels[constants.NodeUpgradingLabel] = upgradingLabelValue
	return n
}

func TestNextBatch(t *testing.T) {
	nodes := []client.Object{
		newControlPlaneNode(fakeNodeA, testNodeIP1),
		newNode(fakeNodeB, testNodeIP2),
		newControlPlaneNode(fakeNodeC, testNodeIP3),
		newNode(fakeNodeD, testNodeIP4),
	}

	tests := []struct {
		name        string
		pending     []string
		parallelism int
		want        []string
	}{
		{
			name:        "control-plane node at the front goes alone",
			pending:     []string{fakeNodeA, fakeNodeB, fakeNodeD},
			parallelism: 3,
			want:        []string{fakeNodeA},
		},
		{
			name:        "worker at the front fills with workers, skipping control-plane nodes",
			pending:     []string{fakeNodeB, fakeNodeC, fakeNodeD},
			parallelism: 3,
			want:        []string{fakeNodeB, fakeNodeD},
		},
		{
			name:        "worker batch capped at parallelism",
			pending:     []string{fakeNodeB, fakeNodeD},
			parallelism: 1,
			want:        []string{fakeNodeB},
		},
		{
			name:        "parallelism 1 keeps queue order",
			pending:     []string{fakeNodeC, fakeNodeB},
			parallelism: 1,
			want:        []string{fakeNodeC},
		},
		{
			name:        "only control-plane nodes left",
			pending:     []string{fakeNodeA, fakeNodeC},
			parallelism: 2,
			want:        []string{fakeNodeA},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodes...).Build()
			r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

			got, err := r.nextBatch(context.Background(), tt.pending, tt.parallelism)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("expected batch %v, got %v", tt.want, got)
			}
		})
	}
}

func TestNextBatch_MissingNode(t *testing.T) {
	scheme := newTestScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	if _, err := r.nextBatch(context.Background(), []string{fakeNodeA}, 1); err == nil {
		t.Fatal("expected an error for a node that no longer exists")
	}
}

func TestTalosReconcile_BatchSerializesControlPlane(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade(testUpgradeName,
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePending),
		withParallelism(3),
	)
	tc := &mockTalosClient{
		nodeVersions: map[string]string{
			testNodeIP1: testV110Talos,
			testNodeIP2: testV110Talos,
			testNodeIP3: testV110Talos,
		},
		installImages: map[string]string{
			testNodeIP1: testFactoryInstaller,
			testNodeIP2: testFactoryInstaller,
			testNodeIP3: testFactoryInstaller,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu,
			newControlPlaneNode(fakeNodeA, testNodeIP1),
			newControlPlaneNode(fakeNodeB, testNodeIP2),
			newControlPlaneNode(fakeNodeC, testNodeIP3),
		).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, testUpgradeName)

	var jobList batchv1.JobList
	if err := cl.List(context.Background(), &jobList, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Fatalf("expected 1 job for three control-plane nodes at parallelism=3, got: %d", len(jobList.Items))
	}
	if got := jobList.Items[0].Labels[targetNodeLabelKey]; got != fakeNodeA {
		t.Fatalf("expected the job to target %s, got %s", fakeNodeA, got)
	}
	updated := getTalosUpgrade(t, cl, testUpgradeName)
	if !slices.Equal(updated.Status.CurrentNodes, []string{fakeNodeA}) {
		t.Fatalf("expected currentNodes [%s], got %v", fakeNodeA, updated.Status.CurrentNodes)
	}
}

func TestControlPlaneUpgradeInFlight(t *testing.T) {
	tests := []struct {
		name    string
		objects []client.Object
		batch   []string
		want    string
	}{
		{
			name:    "worker batch is never held",
			objects: []client.Object{newControlPlaneNode(fakeNodeA, testNodeIP1), newNode(fakeNodeB, testNodeIP2), upgradeJob("other", fakeNodeA, false)},
			batch:   []string{fakeNodeB},
		},
		{
			name:    "unfinished Job on another control-plane node holds the batch",
			objects: []client.Object{newControlPlaneNode(fakeNodeA, testNodeIP1), newControlPlaneNode(fakeNodeB, testNodeIP2), upgradeJob("other", fakeNodeB, false)},
			batch:   []string{fakeNodeA},
			want:    fakeNodeB,
		},
		{
			name:    "finished Job does not hold the batch",
			objects: []client.Object{newControlPlaneNode(fakeNodeA, testNodeIP1), newControlPlaneNode(fakeNodeB, testNodeIP2), upgradeJob("other", fakeNodeB, true)},
			batch:   []string{fakeNodeA},
		},
		{
			name:    "Job on a worker does not hold a control-plane batch",
			objects: []client.Object{newControlPlaneNode(fakeNodeA, testNodeIP1), newNode(fakeNodeB, testNodeIP2), upgradeJob("other", fakeNodeB, false)},
			batch:   []string{fakeNodeA},
		},
		{
			name:    "Job on the batch node itself does not hold it",
			objects: []client.Object{newControlPlaneNode(fakeNodeA, testNodeIP1), upgradeJob("other", fakeNodeA, false)},
			batch:   []string{fakeNodeA},
		},
		{
			name:    "another control-plane node still labeled upgrading holds the batch",
			objects: []client.Object{newControlPlaneNode(fakeNodeA, testNodeIP1), withUpgradingLabel(newControlPlaneNode(fakeNodeB, testNodeIP2))},
			batch:   []string{fakeNodeA},
			want:    fakeNodeB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build()
			r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})
			r.APIReader = cl

			got, err := r.controlPlaneUpgradeInFlight(context.Background(), tt.batch)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestTalosReconcile_WaitsForOtherPlansControlPlaneUpgrade(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade(testUpgradeName,
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePending),
	)
	tu.Spec.NodeSelector = &metav1.LabelSelector{MatchLabels: map[string]string{controlPlaneLabel: ""}}
	tc := &mockTalosClient{
		nodeVersions:  map[string]string{testNodeIP1: testV110Talos, testNodeIP2: fakeTalosVersion},
		installImages: map[string]string{testNodeIP1: testFactoryInstaller},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu,
			newControlPlaneNode(fakeNodeA, testNodeIP1),
			newControlPlaneNode(fakeNodeB, testNodeIP2),
			upgradeJob("other-plan", fakeNodeB, false),
		).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})
	r.APIReader = cl

	reconcileTalos(t, r, testUpgradeName)

	updated := getTalosUpgrade(t, cl, testUpgradeName)
	if updated.Status.Phase != tupprv1alpha1.JobPhasePending {
		t.Fatalf("expected phase Pending, got: %s", updated.Status.Phase)
	}
	if !strings.Contains(updated.Status.Message, "Waiting for control-plane node "+fakeNodeB) {
		t.Fatalf("expected wait message naming %s, got: %s", fakeNodeB, updated.Status.Message)
	}
	cond := findProgressing(updated.Status.Conditions)
	if cond == nil || cond.Reason != upgradeaudit.ReasonWaitingForOtherUpgrade {
		t.Fatalf("expected Progressing reason %s, got %+v", upgradeaudit.ReasonWaitingForOtherUpgrade, cond)
	}
	var jobList batchv1.JobList
	if err := cl.List(context.Background(), &jobList, client.MatchingLabels{appInstanceLabelKey: testUpgradeName}); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	if len(jobList.Items) != 0 {
		t.Fatalf("expected no job for %s while %s upgrades, got %d", fakeNodeA, fakeNodeB, len(jobList.Items))
	}
}

func TestTalosReconcile_MixedBatchKeepsControllerNodeFirst(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade(testUpgradeName,
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePending),
		withParallelism(3),
	)
	tc := &mockTalosClient{
		nodeVersions: map[string]string{
			testNodeIP1: testV110Talos, testNodeIP2: testV110Talos,
			testNodeIP3: testV110Talos, testNodeIP4: testV110Talos,
		},
		installImages: map[string]string{
			testNodeIP1: testFactoryInstaller, testNodeIP2: testFactoryInstaller,
			testNodeIP3: testFactoryInstaller, testNodeIP4: testFactoryInstaller,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu,
			newControlPlaneNode(fakeNodeA, testNodeIP1),
			newNode(fakeNodeB, testNodeIP2),
			newControlPlaneNode(fakeNodeC, testNodeIP3),
			newNode(fakeNodeD, testNodeIP4),
		).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})
	r.APIReader = cl
	// The controller runs on worker node-d, so the queue is d, a, b, c: d at the
	// front makes a worker batch, which skips control-plane node-a for node-b.
	r.ControllerNodeName = fakeNodeD

	reconcileTalos(t, r, testUpgradeName)

	updated := getTalosUpgrade(t, cl, testUpgradeName)
	if want := []string{fakeNodeD, fakeNodeB}; !slices.Equal(updated.Status.CurrentNodes, want) {
		t.Fatalf("expected currentNodes %v, got %v", want, updated.Status.CurrentNodes)
	}
}
