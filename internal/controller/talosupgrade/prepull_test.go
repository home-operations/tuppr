package talosupgrade

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

func withPrePullDisabled(tu *tupprv1alpha1.TalosUpgrade) {
	disabled := false
	tu.Spec.Talos.PrePull = &disabled
}

// prePullTestImage is what the standard test fixtures resolve to: the
// version-swapped testFactoryInstaller repo at the spec target version.
const prePullTestImage = "factory.talos.dev/installer:" + fakeTalosVersion

func withPrePulledNodes(nodes ...tupprv1alpha1.PrePulledNode) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Status.PrePulledNodes = nodes
	}
}

func hasPrePulledNode(records []tupprv1alpha1.PrePulledNode, nodeName string) bool {
	return slices.ContainsFunc(records, func(e tupprv1alpha1.PrePulledNode) bool {
		return e.NodeName == nodeName
	})
}

func newPrePullMockClient() *mockTalosClient {
	return &mockTalosClient{
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
}

func listJobs(t *testing.T, cl client.Client) []batchv1.Job {
	t.Helper()
	var jobList batchv1.JobList
	if err := cl.List(context.Background(), &jobList, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	return jobList.Items
}

func TestTalosReconcile_PrePull_FleetPulledBeforeFirstJob(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade(testUpgradeName, withFinalizer, withPhase(tupprv1alpha1.JobPhasePending))
	nodeA := newNode(fakeNodeA, testNodeIP1)
	nodeB := newNode(fakeNodeB, testNodeIP2)
	nodeC := newNode(fakeNodeC, testNodeIP3)

	tc := newPrePullMockClient()
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA, nodeB, nodeC).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, testUpgradeName)

	// Every pending node is pulled — not just the parallelism=1 batch.
	for _, ip := range []string{testNodeIP1, testNodeIP2, testNodeIP3} {
		if !slices.Contains(tc.pullCalls, ip) {
			t.Fatalf("expected pre-pull for %s, got: %v", ip, tc.pullCalls)
		}
	}
	if got := tc.pullImageRefs[testNodeIP2]; got != prePullTestImage {
		t.Fatalf("expected pre-pull of %s, got: %s", prePullTestImage, got)
	}

	if jobs := listJobs(t, cl); len(jobs) != 1 {
		t.Fatalf("expected 1 upgrade job after pre-pull, got: %d", len(jobs))
	}

	updated := getTalosUpgrade(t, cl, testUpgradeName)
	for _, n := range []string{fakeNodeA, fakeNodeB, fakeNodeC} {
		if !slices.Contains(updated.Status.PrePulledNodes, tupprv1alpha1.PrePulledNode{NodeName: n, Image: prePullTestImage}) {
			t.Fatalf("expected %s recorded with %s in status.prePulledNodes, got: %v", n, prePullTestImage, updated.Status.PrePulledNodes)
		}
	}
}

func TestTalosReconcile_PrePull_Disabled(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade(testUpgradeName, withFinalizer, withPhase(tupprv1alpha1.JobPhasePending), withPrePullDisabled)
	nodeA := newNode(fakeNodeA, testNodeIP1)

	tc := newPrePullMockClient()
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, testUpgradeName)

	if len(tc.pullCalls) != 0 {
		t.Fatalf("expected no pre-pull calls with prePull disabled, got: %v", tc.pullCalls)
	}
	if jobs := listJobs(t, cl); len(jobs) != 1 {
		t.Fatalf("expected the upgrade to proceed without pre-pull, got %d jobs", len(jobs))
	}
	updated := getTalosUpgrade(t, cl, testUpgradeName)
	if len(updated.Status.PrePulledNodes) != 0 {
		t.Fatalf("expected empty status.prePulledNodes when disabled, got: %v", updated.Status.PrePulledNodes)
	}
}

func TestTalosReconcile_PrePull_FailureParksRunBeforeDisruption(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade(testUpgradeName, withFinalizer, withPhase(tupprv1alpha1.JobPhasePending))
	nodeA := newNode(fakeNodeA, testNodeIP1)
	nodeB := newNode(fakeNodeB, testNodeIP2)
	nodeC := newNode(fakeNodeC, testNodeIP3)

	tc := newPrePullMockClient()
	tc.pullErrs = map[string]error{testNodeIP2: errors.New("manifest unknown")}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA, nodeB, nodeC).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	result := reconcileTalos(t, r, testUpgradeName)

	if jobs := listJobs(t, cl); len(jobs) != 0 {
		t.Fatalf("expected no upgrade jobs after pre-pull failure, got: %d", len(jobs))
	}
	if result.RequeueAfter == 0 {
		t.Fatal("expected a requeue after pre-pull failure")
	}
	// Fail-fast: node-c is not pulled once node-b fails.
	if slices.Contains(tc.pullCalls, testNodeIP3) {
		t.Fatalf("expected no pre-pull for node-c after node-b failed, got: %v", tc.pullCalls)
	}

	updated := getTalosUpgrade(t, cl, testUpgradeName)
	if updated.Status.Phase != tupprv1alpha1.JobPhasePending {
		t.Fatalf("expected phase Pending (parked), got: %s", updated.Status.Phase)
	}
	// Progress up to the failure is persisted; the failing node is not.
	if !hasPrePulledNode(updated.Status.PrePulledNodes, fakeNodeA) {
		t.Fatalf("expected node-a recorded as pre-pulled, got: %v", updated.Status.PrePulledNodes)
	}
	if hasPrePulledNode(updated.Status.PrePulledNodes, fakeNodeB) {
		t.Fatalf("expected node-b not recorded after failure, got: %v", updated.Status.PrePulledNodes)
	}
	// The message names the node and the image ref.
	expectedImage := "factory.talos.dev/installer:" + fakeTalosVersion
	if !strings.Contains(updated.Status.Message, fakeNodeB) || !strings.Contains(updated.Status.Message, expectedImage) {
		t.Fatalf("expected message naming node and image, got: %q", updated.Status.Message)
	}
	for _, cond := range updated.Status.Conditions {
		if cond.Type == tupprv1alpha1.ConditionTypeProgressing && cond.Reason != "PrePullFailed" {
			t.Fatalf("expected Progressing reason PrePullFailed, got: %s", cond.Reason)
		}
	}
}

func TestTalosReconcile_PrePull_UnimplementedNodeSkipped(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade(testUpgradeName, withFinalizer, withPhase(tupprv1alpha1.JobPhasePending))
	nodeA := newNode(fakeNodeA, testNodeIP1)
	nodeB := newNode(fakeNodeB, testNodeIP2)

	tc := newPrePullMockClient()
	// Talos < v1.13 has no ImageService; the run must proceed regardless.
	tc.pullErrs = map[string]error{
		testNodeIP1: status.Error(codes.Unimplemented, "unknown service machine.ImageService"),
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA, nodeB).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, testUpgradeName)

	if !slices.Contains(tc.pullCalls, testNodeIP2) {
		t.Fatalf("expected pre-pull to continue past the unsupported node, got: %v", tc.pullCalls)
	}
	if jobs := listJobs(t, cl); len(jobs) != 1 {
		t.Fatalf("expected the upgrade to proceed, got %d jobs", len(jobs))
	}
	updated := getTalosUpgrade(t, cl, testUpgradeName)
	// The unsupported node is recorded as handled so it isn't re-attempted.
	for _, n := range []string{fakeNodeA, fakeNodeB} {
		if !hasPrePulledNode(updated.Status.PrePulledNodes, n) {
			t.Fatalf("expected %s in status.prePulledNodes, got: %v", n, updated.Status.PrePulledNodes)
		}
	}
}

func TestTalosReconcile_PrePull_RecordedNodesNotRepeated(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade(testUpgradeName, withFinalizer, withPhase(tupprv1alpha1.JobPhasePending),
		withPrePulledNodes(tupprv1alpha1.PrePulledNode{NodeName: fakeNodeA, Image: prePullTestImage}))
	nodeA := newNode(fakeNodeA, testNodeIP1)

	tc := newPrePullMockClient()
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, testUpgradeName)

	if len(tc.pullCalls) != 0 {
		t.Fatalf("expected no pre-pull calls for already-pulled nodes, got: %v", tc.pullCalls)
	}
	if jobs := listJobs(t, cl); len(jobs) != 1 {
		t.Fatalf("expected the upgrade to proceed, got %d jobs", len(jobs))
	}
}

func TestTalosReconcile_PrePull_RePullsWhenResolvedImageChanges(t *testing.T) {
	scheme := newTestScheme()
	// node-a was pre-pulled earlier in the run, but its resolved installer
	// ref has changed since (e.g. a version or factory-url annotation edit).
	tu := newTalosUpgrade(testUpgradeName, withFinalizer, withPhase(tupprv1alpha1.JobPhasePending),
		withPrePulledNodes(tupprv1alpha1.PrePulledNode{NodeName: fakeNodeA, Image: "factory.talos.dev/installer:v1.11.0"}))
	nodeA := newNode(fakeNodeA, testNodeIP1)

	tc := newPrePullMockClient()
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, testUpgradeName)

	if got := tc.pullImageRefs[testNodeIP1]; got != prePullTestImage {
		t.Fatalf("expected re-pull of the newly resolved image %s, got: %q (calls: %v)", prePullTestImage, got, tc.pullCalls)
	}
	updated := getTalosUpgrade(t, cl, testUpgradeName)
	if !slices.Contains(updated.Status.PrePulledNodes, tupprv1alpha1.PrePulledNode{NodeName: fakeNodeA, Image: prePullTestImage}) {
		t.Fatalf("expected node-a record updated to the new image, got: %v", updated.Status.PrePulledNodes)
	}
	if len(updated.Status.PrePulledNodes) != 1 {
		t.Fatalf("expected the stale record replaced, not duplicated, got: %v", updated.Status.PrePulledNodes)
	}
}

func TestTalosReconcile_PrePull_RePullsWhenNodeRecreated(t *testing.T) {
	scheme := newTestScheme()
	// node-a was pre-pulled, then reinstalled under the same name (new UID):
	// its image store is empty even though the resolved ref is unchanged.
	tu := newTalosUpgrade(testUpgradeName, withFinalizer, withPhase(tupprv1alpha1.JobPhasePending),
		withPrePulledNodes(tupprv1alpha1.PrePulledNode{NodeName: fakeNodeA, NodeUID: "uid-old", Image: prePullTestImage}))
	nodeA := newNode(fakeNodeA, testNodeIP1)
	nodeA.UID = "uid-new"

	tc := newPrePullMockClient()
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, testUpgradeName)

	if !slices.Equal(tc.pullCalls, []string{testNodeIP1}) {
		t.Fatalf("expected a re-pull for the recreated node, got: %v", tc.pullCalls)
	}
	updated := getTalosUpgrade(t, cl, testUpgradeName)
	if !slices.Contains(updated.Status.PrePulledNodes, tupprv1alpha1.PrePulledNode{NodeName: fakeNodeA, NodeUID: "uid-new", Image: prePullTestImage}) {
		t.Fatalf("expected node-a record updated to the new UID, got: %v", updated.Status.PrePulledNodes)
	}
	if len(updated.Status.PrePulledNodes) != 1 {
		t.Fatalf("expected the stale record replaced, not duplicated, got: %v", updated.Status.PrePulledNodes)
	}
}

func TestTalosReconcile_PrePull_LateNodePulledMidRun(t *testing.T) {
	scheme := newTestScheme()
	// Mid-run state: node-a already upgraded, node-a and node-b already
	// pre-pulled. node-c joined after the run started.
	tu := newTalosUpgrade(testUpgradeName, withFinalizer, withPhase(tupprv1alpha1.JobPhasePending),
		withCompletedNodes(fakeNodeA),
		withPrePulledNodes(
			tupprv1alpha1.PrePulledNode{NodeName: fakeNodeA, Image: prePullTestImage},
			tupprv1alpha1.PrePulledNode{NodeName: fakeNodeB, Image: prePullTestImage},
		))
	nodeA := newNode(fakeNodeA, testNodeIP1)
	nodeB := newNode(fakeNodeB, testNodeIP2)
	nodeC := newNode(fakeNodeC, testNodeIP3)

	tc := newPrePullMockClient()
	tc.nodeVersions[testNodeIP1] = fakeTalosVersion // node-a is at target
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA, nodeB, nodeC).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, testUpgradeName)

	// Only the late joiner is pulled; the batch still proceeds on node-b.
	if !slices.Equal(tc.pullCalls, []string{testNodeIP3}) {
		t.Fatalf("expected exactly one pre-pull for the late node, got: %v", tc.pullCalls)
	}
	if jobs := listJobs(t, cl); len(jobs) != 1 || jobs[0].Labels[targetNodeLabelKey] != fakeNodeB {
		t.Fatalf("expected one job for node-b, got: %+v", jobs)
	}
	updated := getTalosUpgrade(t, cl, testUpgradeName)
	if !hasPrePulledNode(updated.Status.PrePulledNodes, fakeNodeC) {
		t.Fatalf("expected node-c recorded as pre-pulled, got: %v", updated.Status.PrePulledNodes)
	}
}

func TestTalosReconcile_PrePull_FailureStreakGrowsWithBackoff(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade(testUpgradeName, withFinalizer, withPhase(tupprv1alpha1.JobPhasePending))
	nodeA := newNode(fakeNodeA, testNodeIP1)

	tc := newPrePullMockClient()
	tc.pullErrs = map[string]error{testNodeIP1: errors.New("no pull progress after 1m30s")}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	result := reconcileTalos(t, r, testUpgradeName)

	if result.RequeueAfter != time.Minute {
		t.Fatalf("expected 1m backoff after the first failure, got: %s", result.RequeueAfter)
	}
	updated := getTalosUpgrade(t, cl, testUpgradeName)
	f := updated.Status.PrePullFailure
	if f == nil || f.Attempts != 1 || !strings.Contains(f.LastError, "no pull progress") {
		t.Fatalf("expected failure streak {1, no pull progress...} in status, got: %+v", f)
	}

	result = reconcileTalos(t, r, testUpgradeName)

	if result.RequeueAfter != 2*time.Minute {
		t.Fatalf("expected 2m backoff after the second failure, got: %s", result.RequeueAfter)
	}
	updated = getTalosUpgrade(t, cl, testUpgradeName)
	if f := updated.Status.PrePullFailure; f == nil || f.Attempts != 2 {
		t.Fatalf("expected failure streak to persist across cycles, got: %+v", f)
	}
	// kubectl get is self-diagnosing: the attempt count rides the message.
	if !strings.Contains(updated.Status.Message, "(attempt 2)") {
		t.Fatalf("expected attempt count in status message, got: %q", updated.Status.Message)
	}
}

func TestTalosReconcile_PrePull_FailureStreakClearedOnSuccess(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade(testUpgradeName, withFinalizer, withPhase(tupprv1alpha1.JobPhasePending))
	tu.Status.PrePullFailure = &tupprv1alpha1.PrePullFailure{Attempts: 3, LastError: "registry down"}
	nodeA := newNode(fakeNodeA, testNodeIP1)

	tc := newPrePullMockClient()
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, testUpgradeName)

	updated := getTalosUpgrade(t, cl, testUpgradeName)
	if updated.Status.PrePullFailure != nil {
		t.Fatalf("expected failure streak cleared after a successful pass, got: %+v", updated.Status.PrePullFailure)
	}
	if jobs := listJobs(t, cl); len(jobs) != 1 {
		t.Fatalf("expected the upgrade to proceed after recovery, got %d jobs", len(jobs))
	}
}

func TestPrePullFailureBackoff(t *testing.T) {
	for attempts, want := range map[int]time.Duration{
		1: time.Minute,
		2: 2 * time.Minute,
		3: 4 * time.Minute,
		4: 5 * time.Minute,
		9: 5 * time.Minute,
	} {
		if got := prePullFailureBackoff(attempts); got != want {
			t.Errorf("prePullFailureBackoff(%d) = %s, want %s", attempts, got, want)
		}
	}
}

func TestTalosReconcile_PrePull_ResetOnGenerationChange(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade(testUpgradeName, withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePending),
		withGeneration(2, 1),
		withPrePulledNodes(tupprv1alpha1.PrePulledNode{NodeName: fakeNodeA, Image: prePullTestImage}),
	)
	nodeA := newNode(fakeNodeA, testNodeIP1)

	tc := newPrePullMockClient()
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, testUpgradeName)

	updated := getTalosUpgrade(t, cl, testUpgradeName)
	if len(updated.Status.PrePulledNodes) != 0 {
		t.Fatalf("expected status.prePulledNodes reset when the spec changes, got: %v", updated.Status.PrePulledNodes)
	}
}
