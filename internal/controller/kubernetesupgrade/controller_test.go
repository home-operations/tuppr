package kubernetesupgrade

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/upgradeaudit"
	"github.com/home-operations/tuppr/internal/metrics"
)

const (
	fakeCrtl       = "ctrl-1"
	testNamespace  = "default"
	testNodeIP     = "10.0.0.1"
	testJobNameStr = "test-upgrade-ctrl-1-abcd1234"
	testNameStr    = "test"
	testV1330      = "v1.33.0"
)

type mockTalosClient struct {
	nodeVersions  map[string]string
	getVersionErr error
}

func (m *mockTalosClient) GetNodeVersion(ctx context.Context, nodeIP string) (string, error) {
	if m.getVersionErr != nil {
		return "", m.getVersionErr
	}
	if v, ok := m.nodeVersions[nodeIP]; ok {
		return v, nil
	}
	return "", fmt.Errorf("node %s not found", nodeIP)
}

type mockHealthChecker struct {
	err error
}

func (m *mockHealthChecker) CheckHealth(ctx context.Context, healthChecks []tupprv1alpha1.HealthCheckSpec) error {
	return m.err
}

type mockVersionGetter struct {
	version string
	err     error
}

func (m *mockVersionGetter) GetCurrentKubernetesVersion(ctx context.Context) (string, error) {
	return m.version, m.err
}

type fixedClock struct {
	t time.Time
}

func (f *fixedClock) Now() time.Time {
	return f.t
}

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = tupprv1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = batchv1.AddToScheme(s)
	return s
}

func newKubernetesUpgrade(name string, opts ...func(*tupprv1alpha1.KubernetesUpgrade)) *tupprv1alpha1.KubernetesUpgrade {
	ku := &tupprv1alpha1.KubernetesUpgrade{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Generation: 1,
		},
		Spec: tupprv1alpha1.KubernetesUpgradeSpec{
			Kubernetes: tupprv1alpha1.KubernetesSpec{
				Version: testK8sVersion,
			},
		},
	}
	for _, opt := range opts {
		opt(ku)
	}
	return ku
}

func withK8sFinalizer(ku *tupprv1alpha1.KubernetesUpgrade) {
	controllerutil.AddFinalizer(ku, KubernetesUpgradeFinalizer)
}

func withK8sPhase(phase tupprv1alpha1.JobPhase) func(*tupprv1alpha1.KubernetesUpgrade) {
	return func(ku *tupprv1alpha1.KubernetesUpgrade) {
		ku.Status.Phase = phase
		ku.Status.ObservedGeneration = ku.Generation
	}
}

func withK8sAnnotation(key, value string) func(*tupprv1alpha1.KubernetesUpgrade) {
	return func(ku *tupprv1alpha1.KubernetesUpgrade) {
		if ku.Annotations == nil {
			ku.Annotations = map[string]string{}
		}
		ku.Annotations[key] = value
	}
}

func withK8sGeneration(gen, observed int64) func(*tupprv1alpha1.KubernetesUpgrade) {
	return func(ku *tupprv1alpha1.KubernetesUpgrade) {
		ku.Generation = gen
		ku.Status.ObservedGeneration = observed
	}
}

func newControllerNode(name, ip string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "",
			},
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: ip},
			},
		},
	}
}

func newControllerNodeWithVersion(name, ip, version string) *corev1.Node {
	n := newControllerNode(name, ip)
	n.Status.NodeInfo.KubeletVersion = version
	return n
}

func newK8sReconciler(cl client.Client, vg VersionGetter, tc TalosClient, hc HealthCheckRunner) *Reconciler {
	return &Reconciler{
		Client:              cl,
		Scheme:              newTestScheme(),
		TalosConfigSecret:   "test-talosconfig",
		ControllerNamespace: testNamespace,
		TalosClient:         tc,
		HealthChecker:       hc,
		MetricsReporter:     metrics.NewReporter(),
		VersionGetter:       vg,
		Now:                 &fixedClock{time.Now()},
	}
}

func getK8sUpgrade(t *testing.T, cl client.Client, name string) *tupprv1alpha1.KubernetesUpgrade { //nolint:unparam
	t.Helper()
	var ku tupprv1alpha1.KubernetesUpgrade
	if err := cl.Get(context.Background(), types.NamespacedName{Name: name}, &ku); err != nil {
		t.Fatalf("failed to get KubernetesUpgrade %q: %v", name, err)
	}
	return &ku
}

func reconcileK8s(t *testing.T, r *Reconciler, name string) ctrl.Result { //nolint:unparam
	t.Helper()
	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: name},
	})
	if err != nil {
		t.Fatalf("Reconcile() returned unexpected error: %v", err)
	}
	return result
}

func TestK8sReconcile_AddsFinalizer(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade")
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{version: testV1330}, &mockTalosClient{}, &mockHealthChecker{})

	reconcileK8s(t, r, "test-upgrade")

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if !controllerutil.ContainsFinalizer(updated, KubernetesUpgradeFinalizer) {
		t.Fatal("expected finalizer to be added")
	}
}

func TestK8sReconcile_SuspendAnnotation(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sAnnotation(constants.SuspendAnnotation, "maintenance"),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Minute {
		t.Fatalf("expected 30m requeue, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhasePending {
		t.Fatalf("expected phase Pending, got: %s", updated.Status.Phase)
	}
	if updated.Status.Message == "" {
		t.Fatal("expected non-empty suspension message")
	}
}

func TestK8sReconcile_PartialUpgrade_PreventsCompletion(t *testing.T) {
	scheme := newTestScheme()

	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseUpgrading),
	)

	// Node A is upgraded
	nodeA := newControllerNodeWithVersion("ctrl-1", testNodeIP, testK8sVersion)
	// Node B is still on old version
	nodeB := newControllerNodeWithVersion("ctrl-2", "10.0.0.2", testV1330)

	vg := &mockVersionGetter{version: testK8sVersion}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, nodeA, nodeB).WithStatusSubresource(ku).Build()

	tc := &mockTalosClient{nodeVersions: map[string]string{"10.0.0.2": testV1330}}
	r := newK8sReconciler(cl, vg, tc, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")

	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue (job creation), got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")

	if updated.Status.Phase == tupprv1alpha1.JobPhaseCompleted {
		t.Fatal("Regression! Controller marked upgrade as Completed despite Node B being old version")
	}

	if updated.Status.ControllerNode != "ctrl-2" {
		t.Fatalf("expected controller to target ctrl-2, got %s", updated.Status.ControllerNode)
	}
}

func TestK8sReconcile_ResetAnnotation(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseFailed),
		withK8sAnnotation(constants.ResetAnnotation, "true"),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if _, exists := updated.Annotations[constants.ResetAnnotation]; exists {
		t.Fatal("expected reset annotation to be removed")
	}
	if updated.Status.Phase != tupprv1alpha1.JobPhasePending {
		t.Fatalf("expected phase reset to Pending, got: %s", updated.Status.Phase)
	}
}

func TestK8sReconcile_GenerationChange(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sGeneration(2, 1),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhasePending {
		t.Fatalf("expected phase reset to Pending, got: %s", updated.Status.Phase)
	}
	if updated.Status.ObservedGeneration != 2 {
		t.Fatalf("expected observedGeneration=2, got: %d", updated.Status.ObservedGeneration)
	}
	if !strings.Contains(updated.Status.Message, "Spec updated") {
		t.Fatalf("expected generation change message, got: %s", updated.Status.Message)
	}
}

func TestK8sReconcile_BlockedByTalosUpgrade(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhasePending),
	)
	tu := newTalosUpgrade("talos-upgrade",
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhaseUpgrading),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, tu).WithStatusSubresource(ku, tu).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 2*time.Minute {
		t.Fatalf("expected 2m requeue, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhasePending {
		t.Fatalf("expected phase Pending while blocked, got: %s", updated.Status.Phase)
	}
	if updated.Status.Message == "" {
		t.Fatal("expected blocking message in status")
	}
}

func TestK8sReconcile_DoesNotFlickerWhileBlockedByCoordination(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhasePending),
	)
	tu := newTalosUpgrade("talos-upgrade",
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhaseUpgrading),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, tu).WithStatusSubresource(ku, tu).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	for i := 0; i < 3; i++ {
		reconcileK8s(t, r, "test-upgrade")
		updated := getK8sUpgrade(t, cl, "test-upgrade")
		if updated.Status.Phase != tupprv1alpha1.JobPhasePending {
			t.Fatalf("reconcile %d: expected Pending throughout, got: %s", i, updated.Status.Phase)
		}
		cond := findK8sCondition(updated.Status.Conditions, tupprv1alpha1.ConditionTypeProgressing)
		if cond == nil {
			t.Fatalf("reconcile %d: missing Progressing condition", i)
			return
		}
		if cond.Status != metav1.ConditionFalse {
			t.Fatalf("reconcile %d: expected Progressing=False, got %s", i, cond.Status)
		}
		if cond.Reason != upgradeaudit.ReasonWaitingForOtherUpgrade {
			t.Fatalf("reconcile %d: expected Reason=%s, got %s", i, upgradeaudit.ReasonWaitingForOtherUpgrade, cond.Reason)
		}
	}
}

func findK8sCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

func TestK8sReconcile_AlreadyAtTargetVersion(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhasePending),
	)
	node := newControllerNodeWithVersion("ctrl-1", testNodeIP, testK8sVersion)

	vg := &mockVersionGetter{version: testK8sVersion}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, node).WithStatusSubresource(ku).Build()

	r := newK8sReconciler(cl, vg, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")

	if result.RequeueAfter != time.Hour {
		t.Fatalf("expected 1h requeue when at target, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhaseCompleted {
		t.Fatalf("expected phase Completed, got: %s", updated.Status.Phase)
	}
	if updated.Status.CurrentVersion != testK8sVersion {
		t.Fatalf("expected currentVersion to be advanced to target, got: %s", updated.Status.CurrentVersion)
	}
}

func TestK8sReconcile_HealthCheckFailure(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhasePending),
	)
	node := newControllerNodeWithVersion(fakeCrtl, testNodeIP, testV1330)
	vg := &mockVersionGetter{version: testV1330} // needs upgrade
	hc := &mockHealthChecker{err: fmt.Errorf("cluster not healthy")}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, node).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, &mockTalosClient{}, hc)

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != time.Minute {
		t.Fatalf("expected 1m requeue on health check failure, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhaseHealthChecking {
		t.Fatalf("expected phase HealthChecking while health checks fail, got: %s", updated.Status.Phase)
	}
	if !strings.Contains(updated.Status.Message, "health") {
		t.Fatalf("expected message about health checks, got: %s", updated.Status.Message)
	}
}

func TestK8sReconcile_StartsUpgrade(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhasePending),
	)
	ctrlNode := newControllerNodeWithVersion(fakeCrtl, testNodeIP, testV1330)
	vg := &mockVersionGetter{version: testV1330}
	tc := &mockTalosClient{nodeVersions: map[string]string{testNodeIP: testV1330}}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, ctrlNode).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, tc, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue after job creation, got: %v", result.RequeueAfter)
	}

	// Verify job was created
	var jobList batchv1.JobList
	if err := cl.List(context.Background(), &jobList, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Fatalf("expected 1 job, got: %d", len(jobList.Items))
	}
	if jobList.Items[0].Labels[targetNodeLabelKey] != fakeCrtl {
		t.Fatalf("expected job for ctrl-1, got: %s", jobList.Items[0].Labels[targetNodeLabelKey])
	}

	// Verify status updated to InProgress
	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhaseUpgrading {
		t.Fatalf("expected phase Upgrading, got: %s", updated.Status.Phase)
	}
	if updated.Status.ControllerNode != fakeCrtl {
		t.Fatalf("expected controllerNode=ctrl-1, got: %s", updated.Status.ControllerNode)
	}
}

func TestK8sReconcile_NoControllerNodeFailsWhenWorkerLags(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhasePending),
	)
	workerLagging := newLaggingWorkerNode("10.0.0.2")
	vg := &mockVersionGetter{version: testV1330}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, workerLagging).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")

	if result.RequeueAfter != 5*time.Minute {
		t.Fatalf("expected 5m requeue when no control plane nodes found, got: %v", result.RequeueAfter)
	}
	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhaseFailed {
		t.Fatalf("expected phase Failed when no control plane node exists, got: %s", updated.Status.Phase)
	}
}

func TestK8sReconcile_VersionDetectionFailure(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhasePending),
	)
	vg := &mockVersionGetter{err: fmt.Errorf("connection refused")}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != time.Minute {
		t.Fatalf("expected 1m requeue, got: %v", result.RequeueAfter)
	}
}

func TestK8sReconcile_HandlesActiveJobRunning(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseUpgrading),
	)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testJobNameStr,
			Namespace: testNamespace,
			Labels: map[string]string{
				appLabelKey:        kubernetesUpgradeAppName,
				targetNodeLabelKey: fakeCrtl,
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr.To(int32(2)), Template: corev1.PodTemplateSpec{}},
		Status: batchv1.JobStatus{Active: 1},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, job).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhaseUpgrading {
		t.Fatalf("expected phase Upgrading while job running, got: %s", updated.Status.Phase)
	}
	if updated.Status.JobName != testJobNameStr {
		t.Fatalf("expected jobName to be set, got: %s", updated.Status.JobName)
	}
}

func TestK8sReconcile_HandlesJobFailure(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseUpgrading),
	)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testJobNameStr,
			Namespace: testNamespace,
			Labels: map[string]string{
				appLabelKey:        kubernetesUpgradeAppName,
				targetNodeLabelKey: fakeCrtl,
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr.To(int32(2)), Template: corev1.PodTemplateSpec{}},
		Status: batchv1.JobStatus{Failed: 2},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, job).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 10*time.Minute {
		t.Fatalf("expected 10m requeue, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhaseFailed {
		t.Fatalf("expected phase Failed, got: %s", updated.Status.Phase)
	}
	if updated.Status.LastError == "" {
		t.Fatal("expected lastError to be set on failure")
	}
	if updated.Status.JobName == "" {
		t.Fatal("expected jobName to be preserved on failure")
	}
	if updated.Status.ObservedGeneration != ku.Generation {
		t.Fatalf("expected observedGeneration=%d after failure, got %d",
			ku.Generation, updated.Status.ObservedGeneration)
	}

	var jobList batchv1.JobList
	if err := cl.List(context.Background(), &jobList, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	if len(jobList.Items) != 0 {
		t.Fatalf("expected failed job to be cleaned up, got %d jobs", len(jobList.Items))
	}

	if len(updated.Status.History) != 1 {
		t.Fatalf("expected exactly 1 history entry after first failure, got %d", len(updated.Status.History))
	}
}

func TestK8sReconcile_FailedState_ResetsOnRetry(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		func(ku *tupprv1alpha1.KubernetesUpgrade) {
			ku.Status.Phase = tupprv1alpha1.JobPhaseFailed
			ku.Status.ObservedGeneration = ku.Generation - 1
			ku.Status.LastError = "Job failed permanently"
		},
	)
	// No job present (TTL cleaned it up before the 10-minute requeue fired)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue after generation-change reset, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhasePending {
		t.Fatalf("expected phase reset to Pending so upgrade is retried, got: %s", updated.Status.Phase)
	}
	if updated.Status.ObservedGeneration != ku.Generation {
		t.Fatalf("expected observedGeneration=%d after reset, got: %d", ku.Generation, updated.Status.ObservedGeneration)
	}
}

func TestK8sReconcile_HandlesJobSuccess(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseUpgrading),
		func(ku *tupprv1alpha1.KubernetesUpgrade) {
			ku.Status.CurrentVersion = testV1330
			ku.Status.TargetVersion = testK8sVersion
		},
	)
	node := newControllerNodeWithVersion(fakeCrtl, testNodeIP, testK8sVersion)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testJobNameStr,
			Namespace: testNamespace,
			Labels: map[string]string{
				appLabelKey:        kubernetesUpgradeAppName,
				targetNodeLabelKey: fakeCrtl,
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr.To(int32(2)), Template: corev1.PodTemplateSpec{}},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	// Version now matches target after successful upgrade
	vg := &mockVersionGetter{version: testK8sVersion}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, node, job).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != time.Hour {
		t.Fatalf("expected 1h requeue after success, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhaseCompleted {
		t.Fatalf("expected phase Completed, got: %s", updated.Status.Phase)
	}
	if updated.Status.CurrentVersion != testK8sVersion {
		t.Fatalf("expected currentVersion to be advanced to target, got: %s", updated.Status.CurrentVersion)
	}
}

func TestK8sReconcile_JobSuccess_PartialUpgrade_ContinuesToNextNode(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseUpgrading),
	)
	nodeA := newControllerNodeWithVersion("ctrl-1", testNodeIP, testK8sVersion)
	nodeB := newControllerNodeWithVersion("ctrl-2", "10.0.0.2", testV1330)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testJobNameStr,
			Namespace: testNamespace,
			Labels: map[string]string{
				appLabelKey:        kubernetesUpgradeAppName,
				targetNodeLabelKey: "ctrl-1",
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr.To(int32(2)), Template: corev1.PodTemplateSpec{}},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	vg := &mockVersionGetter{version: testK8sVersion}
	tc := &mockTalosClient{nodeVersions: map[string]string{"10.0.0.2": testV1330}}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, nodeA, nodeB, job).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, tc, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 10*time.Second {
		t.Fatalf("expected 10s requeue to continue to next node, got: %v", result.RequeueAfter)
	}
	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase == tupprv1alpha1.JobPhaseCompleted {
		t.Fatal("controller marked upgrade Completed despite ctrl-2 still on old version")
	}
	if updated.Status.Phase != tupprv1alpha1.JobPhaseUpgrading {
		t.Fatalf("expected phase Upgrading, got: %s", updated.Status.Phase)
	}
	if updated.Status.JobName != "" {
		t.Fatalf("expected jobName cleared, got: %s", updated.Status.JobName)
	}

	result = reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue after creating job for ctrl-2, got: %v", result.RequeueAfter)
	}
	updated = getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.ControllerNode != "ctrl-2" {
		t.Fatalf("expected next job targeting ctrl-2, got: %s", updated.Status.ControllerNode)
	}
}

func TestK8sReconcile_JobSuccess_VerifyErrorRequeues(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseUpgrading),
	)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testJobNameStr,
			Namespace: testNamespace,
			Labels: map[string]string{
				appLabelKey:        kubernetesUpgradeAppName,
				targetNodeLabelKey: fakeCrtl,
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr.To(int32(2)), Template: corev1.PodTemplateSpec{}},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	vg := &mockVersionGetter{version: testV1330}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, job).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue after verification error, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase == tupprv1alpha1.JobPhaseFailed {
		t.Fatalf("verification error must not flip phase to Failed, got: %s", updated.Status.Phase)
	}
}

func TestK8sReconcile_Cleanup(t *testing.T) {
	scheme := newTestScheme()
	now := metav1.Now()
	ku := &tupprv1alpha1.KubernetesUpgrade{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-upgrade",
			Generation:        1,
			DeletionTimestamp: &now,
			Finalizers:        []string{KubernetesUpgradeFinalizer},
		},
		Spec: tupprv1alpha1.KubernetesUpgradeSpec{
			Kubernetes: tupprv1alpha1.KubernetesSpec{Version: testK8sVersion},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result != (ctrl.Result{}) {
		t.Fatalf("expected empty result, got: %v", result)
	}

	// Object should be gone
	var updated tupprv1alpha1.KubernetesUpgrade
	err := cl.Get(context.Background(), types.NamespacedName{Name: "test-upgrade"}, &updated)
	if err == nil {
		t.Fatal("expected object to be deleted after cleanup")
	}
}

func TestK8sReconcile_InProgressBypassesCoordination(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseUpgrading),
	)
	tu := newTalosUpgrade("talos-upgrade",
		withFinalizer,
		withPhase(tupprv1alpha1.JobPhaseUpgrading),
	)
	node := newControllerNodeWithVersion(fakeCrtl, testNodeIP, testK8sVersion)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, tu, node).WithStatusSubresource(ku, tu).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter == 2*time.Minute {
		t.Fatal("InProgress upgrade should bypass coordination check, but got 2m requeue (blocked)")
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase == tupprv1alpha1.JobPhasePending {
		t.Fatal("expected phase to not be Pending (should have bypassed coordination)")
	}
}

func TestK8sFindControllerNode(t *testing.T) {
	scheme := newTestScheme()
	ctrlNode := newControllerNodeWithVersion(fakeCrtl, testNodeIP, testV1330)
	upgradedNode := newControllerNodeWithVersion("ctrl-2", "10.0.0.3", testK8sVersion)

	workerNode := newWorkerNode("10.0.0.2")

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ctrlNode, upgradedNode, workerNode).Build()

	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	name, ip, err := r.findControllerNode(context.Background(), testK8sVersion)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if name != fakeCrtl || ip != testNodeIP {
		t.Fatalf("expected to pick node needing upgrade (ctrl-1), got: %s/%s", name, ip)
	}
}

func TestK8sFindControllerNode_NoControlPlane(t *testing.T) {
	scheme := newTestScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(newWorkerNode("10.0.0.2")).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	_, _, err := r.findControllerNode(context.Background(), testK8sVersion)
	if err == nil {
		t.Fatal("expected error when no control-plane node")
	}
}

func TestK8sBuildJob_VersionDetectionFailure(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	tc := &mockTalosClient{getVersionErr: fmt.Errorf("connection refused")}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, testNodeIP)).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	_, err := r.buildJob(context.Background(), ku, fakeCrtl, testNodeIP)
	if err == nil {
		t.Fatal("expected error when version detection fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to detect talosctl version") {
		t.Fatalf("expected version detection error message, got: %v", err)
	}
}

func TestKubernetesUpgradeReconciler_MaintenanceWindowBlocks(t *testing.T) {
	scheme := newTestScheme()
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Window: every day at 02:00 UTC for 4 hours (outside current time)
	ku := newKubernetesUpgrade("test", func(ku *tupprv1alpha1.KubernetesUpgrade) {
		controllerutil.AddFinalizer(ku, KubernetesUpgradeFinalizer)
		ku.Spec.Maintenance = &tupprv1alpha1.MaintenanceSpec{
			Windows: []tupprv1alpha1.WindowSpec{
				{
					Start:    "0 2 * * *",
					Duration: metav1.Duration{Duration: 4 * time.Hour},
					Timezone: "UTC",
				},
			},
		}
		ku.Status.ObservedGeneration = ku.Generation
	})

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ku).WithStatusSubresource(ku).Build()
	r := &Reconciler{
		Client:              cl,
		Scheme:              scheme,
		ControllerNamespace: testNamespace,
		TalosConfigSecret:   "talosconfig",
		HealthChecker:       &mockHealthChecker{},
		TalosClient:         &mockTalosClient{},
		VersionGetter:       &mockVersionGetter{version: testK8sVersion},
		MetricsReporter:     metrics.NewReporter(),
		Now:                 &fixedClock{t: now},
	}

	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testNameStr},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatal("expected requeue when outside maintenance window")
	}

	// Verify status updated
	var updated tupprv1alpha1.KubernetesUpgrade
	if err := cl.Get(context.Background(), types.NamespacedName{Name: testNameStr}, &updated); err != nil {
		t.Fatalf("failed to get updated upgrade: %v", err)
	}
	if updated.Status.Phase != tupprv1alpha1.JobPhaseMaintenanceWindow {
		t.Fatalf("expected phase MaintenanceWindow, got %s", updated.Status.Phase)
	}
	if !strings.Contains(updated.Status.Message, "Waiting for maintenance window") {
		t.Fatalf("expected message about waiting for window, got: %s", updated.Status.Message)
	}
	if updated.Status.NextMaintenanceWindow == nil {
		t.Fatal("expected nextMaintenanceWindow to be set")
	}
}

func TestKubernetesUpgradeReconciler_MaintenanceWindowAllows(t *testing.T) {
	scheme := newTestScheme()
	now := time.Date(2025, 6, 15, 3, 0, 0, 0, time.UTC) // Inside window

	ku := newKubernetesUpgrade("test", func(ku *tupprv1alpha1.KubernetesUpgrade) {
		controllerutil.AddFinalizer(ku, KubernetesUpgradeFinalizer)
		ku.Spec.Maintenance = &tupprv1alpha1.MaintenanceSpec{
			Windows: []tupprv1alpha1.WindowSpec{
				{
					Start:    "0 2 * * *",
					Duration: metav1.Duration{Duration: 4 * time.Hour},
					Timezone: "UTC",
				},
			},
		}
		ku.Status.ObservedGeneration = ku.Generation
	})

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, testNodeIP)).
		WithStatusSubresource(ku).Build()
	r := &Reconciler{
		Client:              cl,
		Scheme:              scheme,
		ControllerNamespace: testNamespace,
		TalosConfigSecret:   "talosconfig",
		HealthChecker:       &mockHealthChecker{},
		TalosClient: &mockTalosClient{
			nodeVersions: map[string]string{testNodeIP: "v1.11.0"},
		},
		VersionGetter:   &mockVersionGetter{version: testK8sVersion},
		MetricsReporter: metrics.NewReporter(),
		Now:             &fixedClock{t: now},
	}

	// Inside window — should proceed with upgrade logic
	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testNameStr},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatal("expected requeue for normal processing")
	}

	// Should NOT be blocked by maintenance window
	var updated tupprv1alpha1.KubernetesUpgrade
	if err := cl.Get(context.Background(), types.NamespacedName{Name: testNameStr}, &updated); err != nil {
		t.Fatalf("failed to get updated upgrade: %v", err)
	}
	if strings.Contains(updated.Status.Message, "Waiting for maintenance window") {
		t.Fatalf("should not be blocked by maintenance window inside window, message: %s", updated.Status.Message)
	}
}

func newTalosUpgrade(name string, opts ...func(*tupprv1alpha1.TalosUpgrade)) *tupprv1alpha1.TalosUpgrade {
	tu := &tupprv1alpha1.TalosUpgrade{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Generation: 1,
		},
		Spec: tupprv1alpha1.TalosUpgradeSpec{
			Talos: tupprv1alpha1.TalosSpec{
				Version: "v1.12.0",
			},
		},
	}
	for _, opt := range opts {
		opt(tu)
	}
	return tu
}

func withFinalizer(tu *tupprv1alpha1.TalosUpgrade) {
	controllerutil.AddFinalizer(tu, "tuppr.home-operations.com/talos-finalizer")
}

func withPhase(phase tupprv1alpha1.JobPhase) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Status.Phase = phase
		tu.Status.ObservedGeneration = tu.Generation
	}
}

func newWorkerNode(ip string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: ip},
			},
		},
	}
}

func TestK8sReconcile_CompletedReentersOnLaggingNode(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseCompleted),
	)
	nodeAtTarget := newControllerNodeWithVersion("ctrl-1", testNodeIP, testK8sVersion)
	nodeLagging := newControllerNodeWithVersion("ctrl-2", "10.0.0.2", testV1330)

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, nodeAtTarget, nodeLagging).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{version: testK8sVersion}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 5*time.Second {
		t.Fatalf("expected 5s requeue after re-entering Pending, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhasePending {
		t.Fatalf("expected phase Pending after detecting lagging node, got: %s", updated.Status.Phase)
	}
}

func TestK8sReconcile_CompletedStaysWhenNoDrift(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseCompleted),
	)
	node := newControllerNodeWithVersion("ctrl-1", testNodeIP, testK8sVersion)

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, node).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{version: testK8sVersion}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != time.Hour {
		t.Fatalf("expected 1h requeue when no drift, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhaseCompleted {
		t.Fatalf("expected phase to remain Completed when no drift, got: %s", updated.Status.Phase)
	}
}

func TestK8sReconcile_FailedRemainsSticky(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseFailed),
	)
	node := newControllerNodeWithVersion("ctrl-1", testNodeIP, testV1330)

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, node).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{version: testK8sVersion}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 5*time.Minute {
		t.Fatalf("expected 5m requeue for sticky Failed state, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhaseFailed {
		t.Fatalf("expected phase to remain Failed (sticky), got: %s", updated.Status.Phase)
	}
}

func newLaggingWorkerNode(ip string) *corev1.Node {
	n := newWorkerNode(ip)
	n.Status.NodeInfo.KubeletVersion = testV1330
	return n
}

func TestK8sReconcile_CompletedReentersOnLaggingWorker(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseCompleted),
	)
	cpAtTarget := newControllerNodeWithVersion("ctrl-1", testNodeIP, testK8sVersion)
	workerLagging := newLaggingWorkerNode("10.0.0.5")

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, cpAtTarget, workerLagging).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{version: testK8sVersion}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 5*time.Second {
		t.Fatalf("expected 5s requeue after re-entering Pending, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhasePending {
		t.Fatalf("expected phase Pending after detecting lagging worker, got: %s", updated.Status.Phase)
	}
}

func TestK8sReconcile_PendingFromLaggingWorkerStartsUpgrade(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhasePending),
	)
	cpAtTarget := newControllerNodeWithVersion(fakeCrtl, testNodeIP, testK8sVersion)
	workerLagging := newLaggingWorkerNode("10.0.0.5")
	tc := &mockTalosClient{nodeVersions: map[string]string{testNodeIP: testK8sVersion}}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, cpAtTarget, workerLagging).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{version: testK8sVersion}, tc, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue after job creation, got: %v", result.RequeueAfter)
	}

	var jobList batchv1.JobList
	if err := cl.List(context.Background(), &jobList, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Fatalf("expected upgrade job to be created when worker lags, got: %d jobs", len(jobList.Items))
	}
	if jobList.Items[0].Labels[targetNodeLabelKey] != fakeCrtl {
		t.Fatalf("expected job for %s (CP node), got: %s", fakeCrtl, jobList.Items[0].Labels[targetNodeLabelKey])
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhaseUpgrading {
		t.Fatalf("expected phase Upgrading (not flicker back to Completed) when worker lags, got: %s", updated.Status.Phase)
	}
}

func TestK8sReconcile_CompletedCyclesExhaustedTransitionsToFailed(t *testing.T) {
	scheme := newTestScheme()
	now := metav1.Now()
	history := make([]tupprv1alpha1.UpgradeHistoryEntry, upgradeaudit.MaxCompletionCycles)
	for i := range history {
		history[i] = tupprv1alpha1.UpgradeHistoryEntry{
			ToVersion:   testK8sVersion,
			Phase:       tupprv1alpha1.JobPhaseCompleted,
			StartedAt:   now,
			CompletedAt: now,
		}
	}
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseCompleted),
		func(ku *tupprv1alpha1.KubernetesUpgrade) {
			ku.Status.History = history
		},
	)
	cpAtTarget := newControllerNodeWithVersion("ctrl-1", testNodeIP, testK8sVersion)
	workerLagging := newLaggingWorkerNode("10.0.0.5")

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, cpAtTarget, workerLagging).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{version: testK8sVersion}, &mockTalosClient{}, &mockHealthChecker{})

	reconcileK8s(t, r, "test-upgrade")

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhaseFailed {
		t.Fatalf("expected phase Failed after exhausting completion cycles, got: %s", updated.Status.Phase)
	}
	if !strings.Contains(updated.Status.Message, "never converged") {
		t.Fatalf("expected Failed message to mention non-convergence, got: %q", updated.Status.Message)
	}
}

func TestK8sReconcile_CompletedIgnoresEmptyKubeletVersion(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(tupprv1alpha1.JobPhaseCompleted),
	)
	cpAtTarget := newControllerNodeWithVersion("ctrl-1", testNodeIP, testK8sVersion)
	freshWorker := newWorkerNode("10.0.0.5") // KubeletVersion not yet reported

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, cpAtTarget, freshWorker).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{version: testK8sVersion}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != time.Hour {
		t.Fatalf("expected 1h requeue (no false drift from empty KubeletVersion), got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != tupprv1alpha1.JobPhaseCompleted {
		t.Fatalf("expected phase to remain Completed, got: %s", updated.Status.Phase)
	}
}

func TestK8sFindControllerNode_FallbackWhenAllControlPlanesAtTarget(t *testing.T) {
	scheme := newTestScheme()
	cpAtTarget := newControllerNodeWithVersion(fakeCrtl, testNodeIP, testK8sVersion)
	workerLagging := newLaggingWorkerNode("10.0.0.5")

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(cpAtTarget, workerLagging).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	name, ip, err := r.findControllerNode(context.Background(), testK8sVersion)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != fakeCrtl || ip != testNodeIP {
		t.Fatalf("expected fallback to ctrl-1 when only worker lags, got: %s/%s", name, ip)
	}
}

func TestNodeToKubernetesUpgrades_EnqueuesOnlyCompleted(t *testing.T) {
	scheme := newTestScheme()
	completed := newKubernetesUpgrade("completed-upgrade", withK8sPhase(tupprv1alpha1.JobPhaseCompleted))
	pending := newKubernetesUpgrade("pending-upgrade", withK8sPhase(tupprv1alpha1.JobPhasePending))
	failed := newKubernetesUpgrade("failed-upgrade", withK8sPhase(tupprv1alpha1.JobPhaseFailed))
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(completed, pending, failed).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	requests := r.nodeToKubernetesUpgrades(context.Background(), &corev1.Node{})
	if len(requests) != 1 {
		t.Fatalf("expected 1 reconcile request (Completed only), got: %d", len(requests))
	}
	if requests[0].Name != "completed-upgrade" {
		t.Fatalf("expected request for completed-upgrade, got: %s", requests[0].Name)
	}
}

func TestNodeToKubernetesUpgrades_EmptyWhenNoneCompleted(t *testing.T) {
	scheme := newTestScheme()
	pending := newKubernetesUpgrade("pending-upgrade", withK8sPhase(tupprv1alpha1.JobPhasePending))
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pending).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	requests := r.nodeToKubernetesUpgrades(context.Background(), &corev1.Node{})
	if len(requests) != 0 {
		t.Fatalf("expected 0 requests when no Completed upgrades, got: %d", len(requests))
	}
}

func TestK8sReconcile_ReportsReconcileErrorInStatus(t *testing.T) {
	tests := []struct {
		name       string
		failList   client.ObjectList
		errMsg     string
		wantReason string
	}{
		{name: "FindActiveJob", failList: &batchv1.JobList{}, errMsg: "simulated job list failure", wantReason: upgradeaudit.ReasonFindActiveJobs},
		{name: "CheckNodeVersions", failList: &corev1.NodeList{}, errMsg: "simulated node list failure", wantReason: upgradeaudit.ReasonCheckNodeVersions},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer, withK8sPhase(tupprv1alpha1.JobPhasePending))
			target := fmt.Sprintf("%T", tt.failList)
			cl := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(ku).WithStatusSubresource(ku).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						if fmt.Sprintf("%T", list) == target {
							return fmt.Errorf("%s", tt.errMsg)
						}
						return c.List(ctx, list, opts...)
					},
				}).Build()
			r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

			result := reconcileK8s(t, r, "test-upgrade")
			if result.RequeueAfter != 1*time.Minute {
				t.Fatalf("expected 1m requeue, got: %v", result.RequeueAfter)
			}

			updated := getK8sUpgrade(t, cl, "test-upgrade")
			if updated.Status.Phase != tupprv1alpha1.JobPhasePending {
				t.Fatalf("expected phase Pending, got: %s", updated.Status.Phase)
			}
			cond := findK8sCondition(updated.Status.Conditions, tupprv1alpha1.ConditionTypeProgressing)
			if cond == nil {
				t.Fatal("missing Progressing condition")
			}
			if cond.Reason != tt.wantReason {
				t.Fatalf("expected Reason=%s, got %s", tt.wantReason, cond.Reason)
			}
			if !strings.Contains(updated.Status.Message, tt.errMsg) {
				t.Fatalf("expected message to include the underlying error, got: %s", updated.Status.Message)
			}
		})
	}
}
