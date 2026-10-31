package talosupgrade

import (
	"context"
	"errors"
	"slices"
	"testing"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testCephImage = "ceph/ceph:v17"
	testCephCmd   = "ceph"
	testCephx     = "cephx"
	testKindNode  = "Node"
	testKindCeph  = "CephCluster"
	testCephAPI   = "ceph.rook.io/v1"
	testCheckExpr = "status.ok"
	testV111      = "v1.11.0"
)

func withHooks(pre, post []tupprv1alpha1.HookSpec) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.Hooks = &tupprv1alpha1.HooksSpec{Pre: pre, Post: post}
	}
}

func withHealthChecks(checks ...tupprv1alpha1.HealthCheckSpec) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.HealthChecks = checks
	}
}

func withPreHooksDone(tu *tupprv1alpha1.TalosUpgrade) {
	tu.Status.PreHookIndex = len(tu.Spec.Hooks.Pre)
}

func nodeCheck() tupprv1alpha1.HealthCheckSpec {
	return tupprv1alpha1.HealthCheckSpec{APIVersion: "v1", Kind: testKindNode, Expr: testCheckExpr}
}

func cephCheckSkipped() tupprv1alpha1.HealthCheckSpec {
	return tupprv1alpha1.HealthCheckSpec{APIVersion: testCephAPI, Kind: testKindCeph, Expr: testCheckExpr, SkipAfterPreHooks: true}
}

// singleNodeTalosClient reports one node at testV111 so a reconcile reaches the
// batch path.
func singleNodeTalosClient() *mockTalosClient {
	return &mockTalosClient{
		nodeVersions:  map[string]string{testNodeIP1: testV111},
		installImages: map[string]string{testNodeIP1: "factory.talos.dev/installer:" + testV111},
	}
}

func cephNoutHook(name, action string) tupprv1alpha1.HookSpec {
	return tupprv1alpha1.HookSpec{
		Name:    name,
		Image:   testCephImage,
		Command: []string{testCephCmd},
		Args:    []string{"osd", action, "noout"},
	}
}

func listHookJobs(t *testing.T, cl client.Client, phase string) []batchv1.Job {
	t.Helper()
	return listHookJobsFor(t, cl, "tu", phase)
}

func listHookJobsFor(t *testing.T, cl client.Client, upgradeName, phase string) []batchv1.Job {
	t.Helper()
	var list batchv1.JobList
	if err := cl.List(context.Background(), &list,
		client.InNamespace("default"),
		client.MatchingLabels{
			"app.kubernetes.io/name":     hookJobAppName,
			"app.kubernetes.io/instance": upgradeName,
			hookPhaseLabel:               phase,
		},
	); err != nil {
		t.Fatalf("list hook jobs: %v", err)
	}
	return list.Items
}

// markJobSucceeded mutates the Job's status via the fake client.
func markJobSucceeded(t *testing.T, cl client.Client, job batchv1.Job) {
	t.Helper()
	job.Status.Succeeded = 1
	if err := cl.Status().Update(context.Background(), &job); err != nil {
		t.Fatalf("mark job succeeded: %v", err)
	}
}

// markJobFailed mimics the job controller declaring the Job Failed.
func markJobFailed(t *testing.T, cl client.Client, job batchv1.Job) {
	t.Helper()
	if job.Spec.BackoffLimit != nil {
		job.Status.Failed = *job.Spec.BackoffLimit + 1
	} else {
		job.Status.Failed = 1
	}
	job.Status.Conditions = append(job.Status.Conditions, batchv1.JobCondition{
		Type:   batchv1.JobFailed,
		Status: corev1.ConditionTrue,
	})
	if err := cl.Status().Update(context.Background(), &job); err != nil {
		t.Fatalf("mark job failed: %v", err)
	}
}

func TestBuildHookJob_PodSpecShape(t *testing.T) {
	scheme := newTestScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	tu := newTalosUpgrade("tu", withFinalizer)
	hook := tupprv1alpha1.HookSpec{
		Name:    "set-noout",
		Image:   testCephImage,
		Command: []string{testCephCmd},
		Args:    []string{"osd", "set", "noout"},
		Env: []corev1.EnvVar{
			{Name: "CEPH_USER", Value: "admin"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: testCephx, MountPath: "/etc/ceph", ReadOnly: true},
		},
		Volumes: []corev1.Volume{{
			Name: testCephx,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: "ceph-client-admin"},
			},
		}},
		ServiceAccountName: "ceph-hook",
	}

	job := r.buildHookJob(tu, hook, hookPhasePre, 0)

	if job.Labels[hookPhaseLabel] != hookPhasePre {
		t.Fatalf("expected hook-phase=pre label, got %v", job.Labels)
	}
	if job.Labels[hookNameLabel] != "set-noout" {
		t.Fatalf("expected hook-name label, got %v", job.Labels)
	}
	if job.Labels[hookIndexLabel] != "0" {
		t.Fatalf("expected hook-index=0, got %v", job.Labels[hookIndexLabel])
	}
	c := job.Spec.Template.Spec.Containers[0]
	if c.Image != "ceph/ceph:v17" {
		t.Fatalf("expected image, got %s", c.Image)
	}
	if len(c.Command) != 1 || c.Command[0] != "ceph" {
		t.Fatalf("expected command [ceph], got %v", c.Command)
	}
	if c.SecurityContext == nil || c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
		t.Fatal("expected ReadOnlyRootFilesystem=true")
	}
	if job.Spec.Template.Spec.ServiceAccountName != "ceph-hook" {
		t.Fatalf("expected serviceAccountName, got %q", job.Spec.Template.Spec.ServiceAccountName)
	}
	if len(job.Spec.Template.Spec.Volumes) != 1 || job.Spec.Template.Spec.Volumes[0].Name != "cephx" {
		t.Fatalf("expected cephx volume, got %v", job.Spec.Template.Spec.Volumes)
	}
}

func TestProcessHookPhase_CreatesJobThenAdvancesOnSuccess(t *testing.T) {
	scheme := newTestScheme()

	tu := newTalosUpgrade("tu",
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePreHook),
		withHooks([]tupprv1alpha1.HookSpec{cephNoutHook("set-noout", "set")}, nil),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	// First reconcile: creates the hook Job, doesn't advance the index yet.
	reconcileTalos(t, r, "tu")

	jobs := listHookJobs(t, cl, hookPhasePre)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 pre-hook job, got %d", len(jobs))
	}
	if got := getTalosUpgrade(t, cl, "tu").Status.PreHookIndex; got != 0 {
		t.Fatalf("expected PreHookIndex=0 after job creation, got %d", got)
	}

	// Mark the Job succeeded and reconcile again — index should advance.
	markJobSucceeded(t, cl, jobs[0])
	reconcileTalos(t, r, "tu")

	updated := getTalosUpgrade(t, cl, "tu")
	if updated.Status.PreHookIndex != 1 {
		t.Fatalf("expected PreHookIndex=1 after success, got %d", updated.Status.PreHookIndex)
	}
	if updated.Status.PreHookFailed {
		t.Fatal("expected PreHookFailed=false on success path")
	}
}

// Regression for #522: a hook with the default backoffLimit=0 must not be
// judged by Failed >= limit (true at creation); it stays pending until the
// job controller reports an outcome.
func TestProcessHookPhase_FreshJobWithZeroBackoffIsNotFailed(t *testing.T) {
	scheme := newTestScheme()

	tu := newTalosUpgrade("tu",
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePreHook),
		withHooks([]tupprv1alpha1.HookSpec{cephNoutHook("set-noout", "set")}, nil),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	// First reconcile creates the hook Job with backoffLimit=0 and an empty status.
	reconcileTalos(t, r, "tu")
	jobs := listHookJobs(t, cl, hookPhasePre)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 pre-hook job, got %d", len(jobs))
	}
	if jobs[0].Spec.BackoffLimit == nil || *jobs[0].Spec.BackoffLimit != 0 {
		t.Fatalf("expected default backoffLimit=0, got %v", jobs[0].Spec.BackoffLimit)
	}

	// Poll again before the pod has run: the hook must still be in flight.
	reconcileTalos(t, r, "tu")

	updated := getTalosUpgrade(t, cl, "tu")
	if updated.Status.PreHookFailed {
		t.Fatal("expected PreHookFailed=false while the job has no outcome")
	}
	if updated.Status.PreHookIndex != 0 {
		t.Fatalf("expected PreHookIndex=0 while the job has no outcome, got %d", updated.Status.PreHookIndex)
	}
	if jobs := listHookJobs(t, cl, hookPhasePre); len(jobs) != 1 {
		t.Fatalf("expected the pending hook job to survive the poll, got %d jobs", len(jobs))
	}

	// A real pod failure (job controller sets JobFailed) still fails the hook.
	markJobFailed(t, cl, jobs[0])
	reconcileTalos(t, r, "tu")
	if updated := getTalosUpgrade(t, cl, "tu"); !updated.Status.PreHookFailed {
		t.Fatal("expected PreHookFailed=true after the job controller marks it Failed")
	}
}

func TestProcessHookPhase_PreHookFailureSetsFlag(t *testing.T) {
	scheme := newTestScheme()

	tu := newTalosUpgrade("tu",
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePreHook),
		withHooks([]tupprv1alpha1.HookSpec{cephNoutHook("set-noout", "set")}, nil),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	reconcileTalos(t, r, "tu")
	jobs := listHookJobs(t, cl, hookPhasePre)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 pre-hook job, got %d", len(jobs))
	}
	markJobFailed(t, cl, jobs[0])
	reconcileTalos(t, r, "tu")

	updated := getTalosUpgrade(t, cl, "tu")
	if !updated.Status.PreHookFailed {
		t.Fatal("expected PreHookFailed=true after pre-hook failure")
	}
	// PreHookIndex should be advanced past the end so processHookPhase reports done.
	if updated.Status.PreHookIndex < 1 {
		t.Fatalf("expected PreHookIndex >= len(pre)=1, got %d", updated.Status.PreHookIndex)
	}
}

func TestPostHookFailure_DoesNotChangeOutcome(t *testing.T) {
	scheme := newTestScheme()

	tu := newTalosUpgrade("tu",
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePostHook),
		withHooks(nil, []tupprv1alpha1.HookSpec{cephNoutHook("unset-noout", "unset")}),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	reconcileTalos(t, r, "tu")
	jobs := listHookJobs(t, cl, hookPhasePost)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 post-hook job, got %d", len(jobs))
	}
	markJobFailed(t, cl, jobs[0])
	reconcileTalos(t, r, "tu")

	updated := getTalosUpgrade(t, cl, "tu")
	if updated.Status.PostHookIndex != 1 {
		t.Fatalf("expected PostHookIndex advanced past failed post-hook, got %d", updated.Status.PostHookIndex)
	}
	// Drive one more reconcile so completeUpgrade fires.
	reconcileTalos(t, r, "tu")
	updated = getTalosUpgrade(t, cl, "tu")
	if updated.Status.Phase != tupprv1alpha1.JobPhaseCompleted {
		t.Fatalf("expected Completed despite post-hook failure (no upgrade failure recorded), got %s", updated.Status.Phase)
	}
}

func TestNoHooks_NoHookJobs(t *testing.T) {
	scheme := newTestScheme()

	tu := newTalosUpgrade("tu",
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePending),
	)
	node := newNode(fakeNodeA, testNodeIP1)
	tc := &mockTalosClient{
		nodeVersions:  map[string]string{testNodeIP1: testV111},
		installImages: map[string]string{testNodeIP1: "factory.talos.dev/installer:v1.11.0"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})
	r.ImageChecker = &mockImageChecker{availableImages: map[string]bool{
		"factory.talos.dev/installer:" + fakeTalosVersion: true,
	}}

	reconcileTalos(t, r, "tu")

	if jobs := listHookJobs(t, cl, hookPhasePre); len(jobs) != 0 {
		t.Fatalf("expected no pre-hook jobs without spec.hooks, got %d", len(jobs))
	}
	if jobs := listHookJobs(t, cl, hookPhasePost); len(jobs) != 0 {
		t.Fatalf("expected no post-hook jobs without spec.hooks, got %d", len(jobs))
	}
	got := getTalosUpgrade(t, cl, "tu").Status.Phase
	if got == tupprv1alpha1.JobPhasePreHook || got == tupprv1alpha1.JobPhasePostHook {
		t.Fatalf("expected lifecycle to skip hook phases without spec.hooks, got %s", got)
	}
}

func TestPostHookCompletion_TransitionsToCompleted(t *testing.T) {
	scheme := newTestScheme()

	tu := newTalosUpgrade("tu",
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePostHook),
		withHooks(nil, []tupprv1alpha1.HookSpec{cephNoutHook("unset-noout", "unset")}),
	)
	// PostHookIndex already at end → processHookPhase reports done immediately.
	tu.Status.PostHookIndex = 1
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	reconcileTalos(t, r, "tu")
	got := getTalosUpgrade(t, cl, "tu").Status.Phase
	if got != tupprv1alpha1.JobPhaseCompleted {
		t.Fatalf("expected Completed after post-hooks done with no failure, got %s", got)
	}
}

func TestPreHookFailed_TerminalIsFailedAfterPostHooks(t *testing.T) {
	scheme := newTestScheme()

	tu := newTalosUpgrade("tu",
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePostHook),
		withHooks(nil, []tupprv1alpha1.HookSpec{cephNoutHook("unset-noout", "unset")}),
	)
	tu.Status.PreHookFailed = true
	tu.Status.PostHookIndex = 1 // pretend post-hooks already done
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	reconcileTalos(t, r, "tu")
	got := getTalosUpgrade(t, cl, "tu").Status.Phase
	if got != tupprv1alpha1.JobPhaseFailed {
		t.Fatalf("expected Failed when PreHookFailed (even after post-hooks succeed), got %s", got)
	}
}

func TestInterBatchHealthChecks(t *testing.T) {
	keep := nodeCheck()
	hooked := cephCheckSkipped()
	pre := []tupprv1alpha1.HookSpec{cephNoutHook("set-noout", "set")}

	tests := []struct {
		name      string
		opts      []func(*tupprv1alpha1.TalosUpgrade)
		wantKinds []string
		wantSkip  bool
	}{
		{
			name: "no hooks runs every check between batches",
			opts: []func(*tupprv1alpha1.TalosUpgrade){
				withHealthChecks(keep, hooked),
				withCompletedNodes(fakeNodeA),
			},
			wantKinds: []string{testKindNode, testKindCeph},
		},
		{
			name: "hooks not yet run keeps every check for the initial check",
			opts: []func(*tupprv1alpha1.TalosUpgrade){
				withHooks(pre, nil),
				withHealthChecks(keep, hooked),
			},
			wantKinds: []string{testKindNode, testKindCeph},
		},
		{
			name: "after pre-hooks drops marked checks before the first batch",
			opts: []func(*tupprv1alpha1.TalosUpgrade){
				withHooks(pre, nil),
				withPreHooksDone,
				withHealthChecks(keep, hooked),
			},
			wantKinds: []string{testKindNode},
		},
		{
			name: "after pre-hooks drops marked checks between batches",
			opts: []func(*tupprv1alpha1.TalosUpgrade){
				withHooks(pre, nil),
				withPreHooksDone,
				withCompletedNodes(fakeNodeA),
				withHealthChecks(keep, hooked),
			},
			wantKinds: []string{testKindNode},
		},
		{
			name: "after pre-hooks with every check marked skips the phase",
			opts: []func(*tupprv1alpha1.TalosUpgrade){
				withHooks(pre, nil),
				withPreHooksDone,
				withHealthChecks(hooked),
			},
			wantSkip: true,
		},
		{
			name: "after pre-hooks with no checks skips the phase",
			opts: []func(*tupprv1alpha1.TalosUpgrade){
				withHooks(pre, nil),
				withPreHooksDone,
			},
			wantSkip: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tu := newTalosUpgrade("tu", tt.opts...)
			checks, skip := interBatchHealthChecks(tu)
			if skip != tt.wantSkip {
				t.Fatalf("skip = %v, want %v", skip, tt.wantSkip)
			}
			var kinds []string
			for _, hc := range checks {
				kinds = append(kinds, hc.Kind)
			}
			if !slices.Equal(kinds, tt.wantKinds) {
				t.Fatalf("checks = %v, want %v", kinds, tt.wantKinds)
			}
		})
	}
}

func TestAfterPreHooks_RetainedCheckGatesBatch(t *testing.T) {
	scheme := newTestScheme()
	keep := nodeCheck()
	hooked := cephCheckSkipped()

	tu := newTalosUpgrade("tu",
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePending),
		withHooks([]tupprv1alpha1.HookSpec{cephNoutHook("set-noout", "set")}, nil),
		withPreHooksDone,
		withHealthChecks(hooked, keep),
	)
	node := newNode(fakeNodeA, testNodeIP1)
	tc := singleNodeTalosClient()
	hc := &mockHealthChecker{err: errors.New("deployment not ready")}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, hc)

	reconcileTalos(t, r, "tu")

	if got := getTalosUpgrade(t, cl, "tu").Status.Phase; got != tupprv1alpha1.JobPhaseHealthChecking {
		t.Fatalf("expected HealthChecking while the retained check fails, got %s", got)
	}
	if len(hc.got) != 1 || hc.got[0].Kind != testKindNode {
		t.Fatalf("expected only the retained check to run, got %v", hc.got)
	}
}

func TestAfterPreHooks_AllChecksSkipped_StartsBatch(t *testing.T) {
	scheme := newTestScheme()
	hooked := cephCheckSkipped()

	tu := newTalosUpgrade("tu",
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhasePending),
		withHooks([]tupprv1alpha1.HookSpec{cephNoutHook("set-noout", "set")}, nil),
		withPreHooksDone,
		withHealthChecks(hooked),
	)
	node := newNode(fakeNodeA, testNodeIP1)
	tc := singleNodeTalosClient()
	hc := &mockHealthChecker{err: errors.New("ceph HEALTH_WARN: noout set")}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, hc)

	reconcileTalos(t, r, "tu")

	if hc.calls != 0 {
		t.Fatalf("expected no health check run when every check is skipped, got %d", hc.calls)
	}
	if got := getTalosUpgrade(t, cl, "tu").Status.Phase; got != tupprv1alpha1.JobPhaseUpgrading {
		t.Fatalf("expected Upgrading once every check is skipped, got %s", got)
	}
	jobs := listJobs(t, cl)
	if len(jobs) != 1 || jobs[0].Labels[targetNodeLabelKey] != fakeNodeA {
		t.Fatalf("expected one upgrade job for %s, got %d jobs", fakeNodeA, len(jobs))
	}
}

// A failed batch must still run the post-hooks, so a pre-hook's `noout` is
// undone after a node fails. The run ends Failed whatever the post-hook does.
func TestBatchFailure_RunsPostHooksThenFails(t *testing.T) {
	for _, postHookFails := range []bool{false, true} {
		name := "post-hook succeeds"
		if postHookFails {
			name = "post-hook fails"
		}
		t.Run(name, func(t *testing.T) {
			scheme := newTestScheme()
			tu := newTalosUpgrade(testUpgradeName,
				withFinalizer,
				withPhase(tupprv1alpha1.JobPhaseUpgrading),
				withHooks(
					[]tupprv1alpha1.HookSpec{cephNoutHook("set-noout", "set")},
					[]tupprv1alpha1.HookSpec{cephNoutHook("unset-noout", "unset")},
				),
				withPreHooksDone,
			)
			job, pod := offTargetFailedJob()
			tc := &mockTalosClient{nodeVersions: map[string]string{testNodeIP1: testV110Talos}}
			cl := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(tu, newNode(fakeNodeA, testNodeIP1), job, pod).WithStatusSubresource(tu).Build()
			r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

			reconcileTalos(t, r, testUpgradeName)
			updated := getTalosUpgrade(t, cl, testUpgradeName)
			if updated.Status.Phase != tupprv1alpha1.JobPhasePostHook {
				t.Fatalf("expected PostHook after the batch failed, got %s (%s)", updated.Status.Phase, updated.Status.Message)
			}
			if len(updated.Status.FailedNodes) != 1 || updated.Status.FailedNodes[0].NodeName != fakeNodeA {
				t.Fatalf("expected %s recorded as failed, got %v", fakeNodeA, updated.Status.FailedNodes)
			}

			reconcileTalos(t, r, testUpgradeName)
			postJobs := listHookJobsFor(t, cl, testUpgradeName, hookPhasePost)
			if len(postJobs) != 1 {
				t.Fatalf("expected the post-hook to run, got %d jobs", len(postJobs))
			}
			if postHookFails {
				markJobFailed(t, cl, postJobs[0])
			} else {
				markJobSucceeded(t, cl, postJobs[0])
			}
			reconcileTalos(t, r, testUpgradeName)
			reconcileTalos(t, r, testUpgradeName)

			updated = getTalosUpgrade(t, cl, testUpgradeName)
			if updated.Status.Phase != tupprv1alpha1.JobPhaseFailed {
				t.Fatalf("expected Failed after the post-hooks, got %s (%s)", updated.Status.Phase, updated.Status.Message)
			}
			if want := "Upgrade stopped after failures: 0 successful, 1 failed"; updated.Status.Message != want {
				t.Fatalf("expected message %q, got %q", want, updated.Status.Message)
			}
		})
	}
}

// Resetting a run mid post-hooks must not leave the previous run's hook Job
// racing the new run's hooks, nor have it counted as the new run's.
func TestRunReset_DeletesHookJobs(t *testing.T) {
	resetAnnotation := func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Annotations = map[string]string{constants.ResetAnnotation: "now"}
	}
	specChange := func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Generation = 2
		tu.Status.ObservedGeneration = 1
	}
	tests := []struct {
		name          string
		opt           func(*tupprv1alpha1.TalosUpgrade)
		isSpecChange  bool
		podNeverStops bool
	}{
		{name: "reset annotation", opt: resetAnnotation},
		{name: "spec change", opt: specChange, isSpecChange: true},
		{name: "reset annotation with a hook pod that never stops", opt: resetAnnotation, podNeverStops: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			tu := newTalosUpgrade("tu",
				withFinalizer,
				withPhase(tupprv1alpha1.JobPhasePostHook),
				withHooks(nil, []tupprv1alpha1.HookSpec{cephNoutHook("unset-noout", "unset")}),
			)
			cl := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(tu).WithStatusSubresource(tu).Build()
			r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

			// The post-hook of the run being reset is still running. The
			// finalizer stands in for its pod still shutting down: a
			// foreground delete leaves the Job until then.
			reconcileTalos(t, r, "tu")
			jobs := listHookJobs(t, cl, hookPhasePost)
			if len(jobs) != 1 {
				t.Fatalf("expected 1 running post-hook job, got %d", len(jobs))
			}
			old := jobs[0]
			old.Finalizers = append(old.Finalizers, "test.tuppr/pod-stopping")
			if err := cl.Update(context.Background(), &old); err != nil {
				t.Fatalf("hold the hook job: %v", err)
			}

			current := getTalosUpgrade(t, cl, "tu")
			tt.opt(current)
			if err := cl.Update(context.Background(), current); err != nil {
				t.Fatalf("update TalosUpgrade: %v", err)
			}
			if current.Status.ObservedGeneration != 0 {
				if err := cl.Status().Update(context.Background(), current); err != nil {
					t.Fatalf("update TalosUpgrade status: %v", err)
				}
			}

			// Held while the old Job is still stopping: nothing is reset and
			// no hook of a new run starts.
			for range 2 {
				reconcileTalos(t, r, "tu")
			}
			held := getTalosUpgrade(t, cl, "tu")
			if held.Status.Phase != tupprv1alpha1.JobPhasePostHook {
				t.Fatalf("expected the reset held in PostHook, got %s", held.Status.Phase)
			}
			if _, stillAnnotated := held.Annotations[constants.ResetAnnotation]; !tt.isSpecChange && !stillAnnotated {
				t.Fatal("expected the reset annotation kept until the old hook stopped")
			}
			if tt.isSpecChange && held.Status.ObservedGeneration >= held.Generation {
				t.Fatal("expected the spec change left unobserved until the old hook stopped")
			}
			jobs = listHookJobs(t, cl, hookPhasePost)
			if len(jobs) != 1 || jobs[0].Name != old.Name || jobs[0].DeletionTimestamp == nil {
				t.Fatalf("expected only the old hook job, marked for deletion, got %d jobs", len(jobs))
			}

			if tt.podNeverStops {
				// Its pod is stuck Terminating on a node that went down: the
				// reset stops waiting after hookJobStopTimeout.
				r.Now = &fixedClock{t: jobs[0].DeletionTimestamp.Add(hookJobStopTimeout)}
			} else {
				// Its pod stopped: the Job goes.
				jobs[0].Finalizers = nil
				if err := cl.Update(context.Background(), &jobs[0]); err != nil {
					t.Fatalf("release the hook job: %v", err)
				}
			}
			reconcileTalos(t, r, "tu")

			if got := getTalosUpgrade(t, cl, "tu").Status.Phase; got != tupprv1alpha1.JobPhasePending {
				t.Fatalf("expected Pending after the reset, got %s", got)
			}
			if !tt.podNeverStops {
				if jobs := listHookJobs(t, cl, hookPhasePost); len(jobs) != 0 {
					t.Fatalf("expected the previous run's hook job gone, got %d", len(jobs))
				}
			}
		})
	}
}
