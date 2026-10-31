package talosupgrade

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
)

const testHookDRBD = "drbd-uptodate"

func withPreBatchHooks(retry *tupprv1alpha1.HookRetrySpec, hooks ...tupprv1alpha1.HookSpec) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		if tu.Spec.Hooks == nil {
			tu.Spec.Hooks = &tupprv1alpha1.HooksSpec{}
		}
		tu.Spec.Hooks.PreBatch = hooks
		tu.Spec.Hooks.PreBatchRetry = retry
	}
}

func withPreBatchState(state *tupprv1alpha1.PreBatchHookStatus) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Status.PreBatchHooks = state
	}
}

func preBatchHook(name string) tupprv1alpha1.HookSpec {
	return tupprv1alpha1.HookSpec{Name: name, Image: testCephImage}
}

func listUpgradeJobs(t *testing.T, cl client.Client) []batchv1.Job {
	t.Helper()
	var out []batchv1.Job
	for _, job := range listJobs(t, cl) {
		if job.Labels[appLabelKey] == talosUpgradeAppName {
			out = append(out, job)
		}
	}
	return out
}

// onePreBatchJob returns the only pre-batch hook Job, failing otherwise.
func onePreBatchJob(t *testing.T, cl client.Client) batchv1.Job {
	t.Helper()
	jobs := listHookJobs(t, cl, hookPhasePreBatch)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 pre-batch hook job, got %d", len(jobs))
	}
	return jobs[0]
}

func hookEnv(job batchv1.Job, name string) string {
	for _, e := range job.Spec.Template.Spec.Containers[0].Env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

type preBatchFixture struct {
	r     *Reconciler
	cl    client.Client
	hc    *mockHealthChecker
	tc    *mockTalosClient
	clock *fixedClock
}

func (f *preBatchFixture) reconcile(t *testing.T) {
	t.Helper()
	reconcileTalos(t, f.r, "tu")
}

func (f *preBatchFixture) status(t *testing.T) tupprv1alpha1.TalosUpgradeStatus {
	t.Helper()
	return getTalosUpgrade(t, f.cl, "tu").Status
}

// newPreBatchFixture builds a run on node-a and node-b, both needing the
// upgrade, at parallelism 1.
func newPreBatchFixture(t *testing.T, opts ...func(*tupprv1alpha1.TalosUpgrade)) *preBatchFixture {
	t.Helper()
	scheme := newTestScheme()
	tu := newTalosUpgrade("tu", append([]func(*tupprv1alpha1.TalosUpgrade){withFinalizer, withPhase(tupprv1alpha1.JobPhasePending)}, opts...)...)
	tc := &mockTalosClient{
		nodeVersions:  map[string]string{testNodeIP1: testV110Talos, testNodeIP2: testV110Talos},
		installImages: map[string]string{testNodeIP1: testFactoryInstaller, testNodeIP2: testFactoryInstaller},
	}
	hc := &mockHealthChecker{}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, newNode(fakeNodeA, testNodeIP1), newNode(fakeNodeB, testNodeIP2)).
		WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, hc)
	clock := &fixedClock{t: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	r.Now = clock
	return &preBatchFixture{r: r, cl: cl, hc: hc, tc: tc, clock: clock}
}

func TestPreBatchHooks_GateEveryBatch(t *testing.T) {
	f := newPreBatchFixture(t,
		withPreBatchHooks(nil, preBatchHook(testHookDRBD), preBatchHook("db-primary")),
		withHealthChecks(nodeCheck()))

	// Batch 1: the health checks run, then the first hook starts.
	f.reconcile(t)
	st := f.status(t)
	if st.Phase != tupprv1alpha1.JobPhasePreHook || st.PreBatchHooks == nil {
		t.Fatalf("expected PreHook with pre-batch state, got phase %s, state %+v", st.Phase, st.PreBatchHooks)
	}
	if want := `Running pre-batch hook "drbd-uptodate" (1/2) for node(s) node-a`; st.Message != want {
		t.Fatalf("expected message %q, got %q", want, st.Message)
	}
	if f.hc.calls != 1 {
		t.Fatalf("expected the health checks before the first hook, got %d calls", f.hc.calls)
	}
	job := onePreBatchJob(t, f.cl)
	if job.Name != st.PreBatchHooks.Job {
		t.Fatalf("expected status to name the hook job %s, got %q", job.Name, st.PreBatchHooks.Job)
	}
	if got := hookEnv(job, constants.PreBatchNodesEnv); got != fakeNodeA {
		t.Fatalf("expected %s=%s, got %q", constants.PreBatchNodesEnv, fakeNodeA, got)
	}

	markJobSucceeded(t, f.cl, job)
	f.reconcile(t)
	if st := f.status(t); st.PreBatchHooks.Index != 1 || st.PreBatchHooks.Job != "" {
		t.Fatalf("expected the first hook recorded as done, got %+v", st.PreBatchHooks)
	}
	if jobs := listHookJobs(t, f.cl, hookPhasePreBatch); len(jobs) != 0 {
		t.Fatalf("expected the finished hook job deleted, got %d", len(jobs))
	}

	// The health checks gate the second hook too.
	f.reconcile(t)
	if f.hc.calls != 2 {
		t.Fatalf("expected the health checks before the second hook, got %d calls", f.hc.calls)
	}
	markJobSucceeded(t, f.cl, onePreBatchJob(t, f.cl))
	f.reconcile(t)
	st = f.status(t)
	if st.Phase != tupprv1alpha1.JobPhasePreHook || st.PreBatchHooks.CompletedAt == nil {
		t.Fatalf("expected the hooks complete while still in PreHook, got phase %s, state %+v", st.Phase, st.PreBatchHooks)
	}

	// The batch starts without re-running the checks.
	f.reconcile(t)
	st = f.status(t)
	if st.Phase != tupprv1alpha1.JobPhaseUpgrading || st.PreBatchHooks != nil {
		t.Fatalf("expected Upgrading with the pre-batch state cleared, got phase %s (%s), state %+v", st.Phase, st.Message, st.PreBatchHooks)
	}
	if f.hc.calls != 2 {
		t.Fatalf("expected no health check between the hooks and the batch, got %d calls", f.hc.calls)
	}
	upgradeJobs := listUpgradeJobs(t, f.cl)
	if len(upgradeJobs) != 1 || upgradeJobs[0].Labels[targetNodeLabelKey] != fakeNodeA {
		t.Fatalf("expected one upgrade job for %s, got %d", fakeNodeA, len(upgradeJobs))
	}

	// Batch 1 completes; batch 2 runs the checks and hooks again.
	upgradeJobs[0].Status.Succeeded = 1
	if err := f.cl.Status().Update(context.Background(), &upgradeJobs[0]); err != nil {
		t.Fatalf("mark upgrade job succeeded: %v", err)
	}
	f.tc.nodeVersions[testNodeIP1] = fakeTalosVersion
	f.reconcile(t)
	f.reconcile(t)

	st = f.status(t)
	if st.Phase != tupprv1alpha1.JobPhasePreHook || st.PreBatchHooks == nil || st.PreBatchHooks.Nodes[0] != fakeNodeB {
		t.Fatalf("expected the pre-batch hooks for %s, got phase %s (%s), state %+v", fakeNodeB, st.Phase, st.Message, st.PreBatchHooks)
	}
	if f.hc.calls != 3 {
		t.Fatalf("expected the health checks before batch 2's hooks, got %d calls", f.hc.calls)
	}
	if got := hookEnv(onePreBatchJob(t, f.cl), constants.PreBatchNodesEnv); got != fakeNodeB {
		t.Fatalf("expected %s=%s for batch 2, got %q", constants.PreBatchNodesEnv, fakeNodeB, got)
	}
}

func TestPreBatchHooks_HealthChecksGateEveryRetry(t *testing.T) {
	f := newPreBatchFixture(t, withPreBatchHooks(nil, preBatchHook(testHookDRBD)), withHealthChecks(nodeCheck()))

	f.reconcile(t)
	markJobFailed(t, f.cl, onePreBatchJob(t, f.cl))
	f.reconcile(t)

	st := f.status(t)
	state := st.PreBatchHooks
	if st.Phase != tupprv1alpha1.JobPhasePreHook || state.Failed || state.FailingSince == nil ||
		state.RetryAt == nil || !state.RetryAt.Time.Equal(f.clock.t.Add(defaultPreBatchRetryInterval)) {
		t.Fatalf("expected a retry one interval out, got phase %s, state %+v", st.Phase, state)
	}
	if !strings.Contains(st.Message, "retrying in 30s") {
		t.Fatalf("expected a retry message, got %q", st.Message)
	}

	// Before retryAt: no checks, no new Job.
	f.reconcile(t)
	if f.hc.calls != 1 || len(listHookJobs(t, f.cl, hookPhasePreBatch)) != 0 {
		t.Fatalf("expected nothing to run before retryAt, got %d check calls", f.hc.calls)
	}

	// A failing check holds the retry back.
	f.clock.t = f.clock.t.Add(defaultPreBatchRetryInterval)
	f.hc.err = errors.New("node not ready")
	f.reconcile(t)
	st = f.status(t)
	if st.Phase != tupprv1alpha1.JobPhaseHealthChecking || len(listHookJobs(t, f.cl, hookPhasePreBatch)) != 0 {
		t.Fatalf("expected the retry held by the health checks, got phase %s", st.Phase)
	}
	if st.PreBatchHooks == nil || st.PreBatchHooks.FailingSince == nil {
		t.Fatalf("expected the failure streak kept while the checks fail, got %+v", st.PreBatchHooks)
	}

	f.hc.err = nil
	f.reconcile(t)
	markJobSucceeded(t, f.cl, onePreBatchJob(t, f.cl))
	f.reconcile(t)
	state = f.status(t).PreBatchHooks
	if state.Index != 1 || state.FailingSince != nil || state.RetryAt != nil {
		t.Fatalf("expected the hook done with the failure streak cleared, got %+v", state)
	}
}

func TestPreBatchHooks_FailurePastTimeoutRunsPostHooksThenFails(t *testing.T) {
	retry := &tupprv1alpha1.HookRetrySpec{
		Timeout:  &metav1.Duration{Duration: time.Minute},
		Interval: &metav1.Duration{Duration: 10 * time.Second},
	}
	f := newPreBatchFixture(t,
		withHooks(nil, []tupprv1alpha1.HookSpec{cephNoutHook("unset-noout", "unset")}),
		withPreBatchHooks(retry, preBatchHook(testHookDRBD)))

	f.reconcile(t)
	markJobFailed(t, f.cl, onePreBatchJob(t, f.cl))
	f.reconcile(t)
	f.clock.t = f.clock.t.Add(time.Minute)
	f.reconcile(t)
	markJobFailed(t, f.cl, onePreBatchJob(t, f.cl))
	f.reconcile(t)

	st := f.status(t)
	if st.Phase != tupprv1alpha1.JobPhasePostHook || st.PreBatchHooks == nil || !st.PreBatchHooks.Failed {
		t.Fatalf("expected PostHook with the pre-batch failure recorded, got phase %s, state %+v", st.Phase, st.PreBatchHooks)
	}

	f.reconcile(t)
	postJobs := listHookJobs(t, f.cl, hookPhasePost)
	if len(postJobs) != 1 {
		t.Fatalf("expected the post-hook to run as cleanup, got %d jobs", len(postJobs))
	}
	markJobSucceeded(t, f.cl, postJobs[0])
	f.reconcile(t)
	f.reconcile(t)

	st = f.status(t)
	if st.Phase != tupprv1alpha1.JobPhaseFailed {
		t.Fatalf("expected Failed after the post-hooks, got %s (%s)", st.Phase, st.Message)
	}
	if !strings.Contains(st.Message, "Pre-batch hook failed for node(s) "+fakeNodeA) {
		t.Fatalf("expected the failure message to name the batch, got %q", st.Message)
	}
	if jobs := listUpgradeJobs(t, f.cl); len(jobs) != 0 {
		t.Fatalf("expected no upgrade job after the pre-batch hook failed, got %d", len(jobs))
	}
}

func TestPreBatchHooks_RecordedFailureFinalizesFromAnyPhase(t *testing.T) {
	// A suspend landed between recording the failure and finalizing.
	f := newPreBatchFixture(t,
		withPreBatchHooks(nil, preBatchHook(testHookDRBD)),
		withPreBatchState(&tupprv1alpha1.PreBatchHookStatus{Nodes: []string{fakeNodeA}, Failed: true}))

	f.reconcile(t)

	if st := f.status(t); st.Phase != tupprv1alpha1.JobPhaseFailed {
		t.Fatalf("expected Failed, got %s (%s)", st.Phase, st.Message)
	}
	if jobs := listJobs(t, f.cl); len(jobs) != 0 {
		t.Fatalf("expected no job, got %d", len(jobs))
	}
}

func TestPreBatchHooks_RerunWhenResultDoesNotCoverBatch(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		state *tupprv1alpha1.PreBatchHookStatus
	}{
		{
			name:  "hooks ran for a different batch",
			state: &tupprv1alpha1.PreBatchHookStatus{Nodes: []string{fakeNodeB}, Index: 1, CompletedAt: &metav1.Time{Time: now}},
		},
		{
			name:  "hook result went stale",
			state: &tupprv1alpha1.PreBatchHookStatus{Nodes: []string{fakeNodeA}, Index: 1, CompletedAt: &metav1.Time{Time: now.Add(-preBatchHookValidity)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPreBatchFixture(t, withPreBatchHooks(nil, preBatchHook(testHookDRBD)), withHealthChecks(nodeCheck()), withPreBatchState(tt.state))

			tu := getTalosUpgrade(t, f.cl, "tu")
			stale := f.r.buildHookJob(tu, preBatchHook(testHookDRBD), hookPhasePreBatch, 0)
			stale.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: tupprv1alpha1.GroupVersion.String(), Kind: "TalosUpgrade", Name: "tu", UID: tu.UID, Controller: ptr.To(true),
			}}
			stale.Status.Succeeded = 1
			if err := f.cl.Create(context.Background(), stale); err != nil {
				t.Fatalf("create stale hook job: %v", err)
			}

			f.reconcile(t)

			state := f.status(t).PreBatchHooks
			if state == nil || state.Index != 0 || state.CompletedAt != nil || len(state.Nodes) != 1 || state.Nodes[0] != fakeNodeA {
				t.Fatalf("expected the hooks restarted for [%s], got %+v", fakeNodeA, state)
			}
			if f.hc.calls != 1 {
				t.Fatalf("expected the health checks before the re-run, got %d calls", f.hc.calls)
			}
			if job := onePreBatchJob(t, f.cl); job.Name == stale.Name || job.Name != state.Job {
				t.Fatalf("expected the stale job replaced by the recorded one, got %s (stale %s, recorded %s)", job.Name, stale.Name, state.Job)
			}
			if jobs := listUpgradeJobs(t, f.cl); len(jobs) != 0 {
				t.Fatalf("expected no upgrade job, got %d", len(jobs))
			}
		})
	}
}

func TestPreBatchHooks_StartedBatchIsNotGatedAgain(t *testing.T) {
	// The batch was drained, then creating its upgrade Job kept failing
	// past the result's validity.
	stale := metav1.Time{Time: time.Date(2026, 10, 31, 11, 0, 0, 0, time.UTC)}
	f := newPreBatchFixture(t,
		withPreBatchHooks(nil, preBatchHook(testHookDRBD)),
		withHealthChecks(nodeCheck()),
		withPhase(tupprv1alpha1.JobPhaseDraining),
		withPreBatchState(&tupprv1alpha1.PreBatchHookStatus{Nodes: []string{fakeNodeA}, Index: 1, CompletedAt: &stale, BatchStarted: true}))
	f.hc.err = errors.New("replicas degraded by the drain")

	f.reconcile(t)

	if st := f.status(t); st.Phase != tupprv1alpha1.JobPhaseUpgrading {
		t.Fatalf("expected the started batch to proceed to Upgrading, got %s (%s)", st.Phase, st.Message)
	}
	if f.hc.calls != 0 || len(listHookJobs(t, f.cl, hookPhasePreBatch)) != 0 {
		t.Fatalf("expected neither checks nor hooks for a started batch, got %d check calls", f.hc.calls)
	}
}

func TestPreBatchHooks_OnlyRecordedJobCounts(t *testing.T) {
	const recorded = "tu-pre-batch-hook-recorded"
	f := newPreBatchFixture(t,
		withPreBatchHooks(nil, preBatchHook(testHookDRBD)),
		withPhase(tupprv1alpha1.JobPhasePreHook),
		withPreBatchState(&tupprv1alpha1.PreBatchHookStatus{Nodes: []string{fakeNodeA}, Job: recorded}))

	// A succeeded Job for the same hook index that isn't the recorded one.
	tu := getTalosUpgrade(t, f.cl, "tu")
	other := f.r.buildHookJob(tu, preBatchHook(testHookDRBD), hookPhasePreBatch, 0)
	other.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: tupprv1alpha1.GroupVersion.String(), Kind: "TalosUpgrade", Name: "tu", UID: tu.UID, Controller: ptr.To(true),
	}}
	other.Status.Succeeded = 1
	if err := f.cl.Create(context.Background(), other); err != nil {
		t.Fatalf("create other hook job: %v", err)
	}

	// The recorded Job is missing (its create failed, or it was deleted
	// before its outcome was recorded), so the hook runs under that name.
	f.reconcile(t)

	if state := f.status(t).PreBatchHooks; state.Index != 0 || state.Job != recorded {
		t.Fatalf("expected the hook still pending on %s, got %+v", recorded, state)
	}
	var job batchv1.Job
	if err := f.cl.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: recorded}, &job); err != nil {
		t.Fatalf("expected the recorded hook job re-created: %v", err)
	}
	if got := hookEnv(job, constants.PreBatchNodesEnv); got != fakeNodeA {
		t.Fatalf("expected %s=%s on the re-created job, got %q", constants.PreBatchNodesEnv, fakeNodeA, got)
	}
}

func TestPreBatchHooks_SuspendKeepsRunningHook(t *testing.T) {
	f := newPreBatchFixture(t, withPreBatchHooks(nil, preBatchHook(testHookDRBD)))
	f.reconcile(t)
	job := onePreBatchJob(t, f.cl)

	setSuspend := func(on bool) {
		t.Helper()
		tu := getTalosUpgrade(t, f.cl, "tu")
		if on {
			tu.Annotations = map[string]string{constants.SuspendAnnotation: "true"}
		} else {
			delete(tu.Annotations, constants.SuspendAnnotation)
		}
		if err := f.cl.Update(context.Background(), tu); err != nil {
			t.Fatalf("update suspend annotation: %v", err)
		}
	}

	setSuspend(true)
	f.reconcile(t)
	if st := f.status(t); st.Phase != tupprv1alpha1.JobPhasePending || st.PreBatchHooks == nil || st.PreBatchHooks.Job != job.Name {
		t.Fatalf("expected Pending with the hook state kept, got phase %s, state %+v", st.Phase, st.PreBatchHooks)
	}

	setSuspend(false)
	f.reconcile(t)
	st := f.status(t)
	if st.Phase != tupprv1alpha1.JobPhasePreHook || st.PreBatchHooks.Job != job.Name {
		t.Fatalf("expected the same hook job resumed in PreHook, got phase %s, state %+v", st.Phase, st.PreBatchHooks)
	}
	if jobs := listHookJobs(t, f.cl, hookPhasePreBatch); len(jobs) != 1 || jobs[0].Name != job.Name {
		t.Fatalf("expected the running hook job kept, got %d jobs", len(jobs))
	}
}

func TestPreBatchHooks_RunResetsClearState(t *testing.T) {
	state := &tupprv1alpha1.PreBatchHookStatus{Nodes: []string{fakeNodeA}, Index: 1, Failed: true}
	tests := []struct {
		name string
		opt  func(*tupprv1alpha1.TalosUpgrade)
	}{
		{
			name: "reset annotation",
			opt: func(tu *tupprv1alpha1.TalosUpgrade) {
				tu.Annotations = map[string]string{constants.ResetAnnotation: "now"}
			},
		},
		{
			name: "spec change",
			opt: func(tu *tupprv1alpha1.TalosUpgrade) {
				tu.Generation = 2
				tu.Status.ObservedGeneration = 1
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPreBatchFixture(t, withPreBatchHooks(nil, preBatchHook(testHookDRBD)), withPreBatchState(state.DeepCopy()), tt.opt)

			f.reconcile(t)

			if st := f.status(t); st.PreBatchHooks != nil {
				t.Fatalf("expected the pre-batch state cleared, got %+v", st.PreBatchHooks)
			}
		})
	}
}

func TestBuildHookJob_PreBatchNodesEnv(t *testing.T) {
	scheme := newTestScheme()
	r := newTalosReconciler(fake.NewClientBuilder().WithScheme(scheme).Build(), scheme, &mockTalosClient{}, &mockHealthChecker{})
	tu := newTalosUpgrade("tu", withPreBatchState(&tupprv1alpha1.PreBatchHookStatus{Nodes: []string{fakeNodeA, fakeNodeB}}))
	hook := preBatchHook(testHookDRBD)
	hook.Env = []corev1.EnvVar{{Name: "LINSTOR_URL", Value: "http://linstor"}}

	job := r.buildHookJob(tu, hook, hookPhasePreBatch, 0)

	if got := hookEnv(*job, constants.PreBatchNodesEnv); got != fakeNodeA+","+fakeNodeB {
		t.Fatalf("expected %s=%s,%s, got %q", constants.PreBatchNodesEnv, fakeNodeA, fakeNodeB, got)
	}
	if got := hookEnv(*job, "LINSTOR_URL"); got != "http://linstor" {
		t.Fatalf("expected the hook's own env kept, got %q", got)
	}
	if len(hook.Env) != 1 {
		t.Fatalf("expected the spec's env left untouched, got %v", hook.Env)
	}
	if got := hookEnv(*r.buildHookJob(tu, hook, hookPhasePre, 0), constants.PreBatchNodesEnv); got != "" {
		t.Fatalf("expected no %s on a pre-hook, got %q", constants.PreBatchNodesEnv, got)
	}
}
