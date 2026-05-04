package talosupgrade

import (
	"context"
	"testing"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testCephImage = "ceph/ceph:v17"
	testCephCmd   = "ceph"
	testCephx     = "cephx"
	testV111      = "v1.11.0"
)

func withHooks(pre, post []tupprv1alpha1.HookSpec) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Spec.Hooks = &tupprv1alpha1.HooksSpec{Pre: pre, Post: post}
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
	var list batchv1.JobList
	if err := cl.List(context.Background(), &list,
		client.InNamespace("default"),
		client.MatchingLabels{
			"app.kubernetes.io/name":     hookJobAppName,
			"app.kubernetes.io/instance": "tu",
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

func markJobFailed(t *testing.T, cl client.Client, job batchv1.Job) {
	t.Helper()
	if job.Spec.BackoffLimit != nil {
		job.Status.Failed = *job.Spec.BackoffLimit + 1
	} else {
		job.Status.Failed = 1
	}
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
