package talosupgrade

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

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

func withPrePullCompleted(tu *tupprv1alpha1.TalosUpgrade) {
	tu.Status.PrePullCompleted = true
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
	expectedImage := "factory.talos.dev/installer:" + fakeTalosVersion
	if got := tc.pullImageRefs[testNodeIP2]; got != expectedImage {
		t.Fatalf("expected pre-pull of %s, got: %s", expectedImage, got)
	}

	if jobs := listJobs(t, cl); len(jobs) != 1 {
		t.Fatalf("expected 1 upgrade job after pre-pull, got: %d", len(jobs))
	}

	updated := getTalosUpgrade(t, cl, testUpgradeName)
	if !updated.Status.PrePullCompleted {
		t.Fatal("expected prePullCompleted to be latched in status")
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
	if updated.Status.PrePullCompleted {
		t.Fatal("expected prePullCompleted to stay false when disabled")
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
	if updated.Status.PrePullCompleted {
		t.Fatal("expected prePullCompleted to stay false after failure")
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
	if !updated.Status.PrePullCompleted {
		t.Fatal("expected prePullCompleted to be latched despite the skipped node")
	}
}

func TestTalosReconcile_PrePull_LatchedNotRepeated(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade(testUpgradeName, withFinalizer, withPhase(tupprv1alpha1.JobPhasePending), withPrePullCompleted)
	nodeA := newNode(fakeNodeA, testNodeIP1)

	tc := newPrePullMockClient()
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, testUpgradeName)

	if len(tc.pullCalls) != 0 {
		t.Fatalf("expected no pre-pull calls once latched, got: %v", tc.pullCalls)
	}
	if jobs := listJobs(t, cl); len(jobs) != 1 {
		t.Fatalf("expected the upgrade to proceed, got %d jobs", len(jobs))
	}
}

func TestTalosReconcile_PrePull_ResetOnGenerationChange(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade(testUpgradeName, withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePending),
		withGeneration(2, 1),
		withPrePullCompleted,
	)
	nodeA := newNode(fakeNodeA, testNodeIP1)

	tc := newPrePullMockClient()
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, testUpgradeName)

	updated := getTalosUpgrade(t, cl, testUpgradeName)
	if updated.Status.PrePullCompleted {
		t.Fatal("expected prePullCompleted to be reset when the spec changes")
	}
}
