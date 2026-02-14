package controller

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	fakeCrtl = "ctrl-1"
)

func newKubernetesUpgrade(name string, opts ...func(*tupprv1alpha1.KubernetesUpgrade)) *tupprv1alpha1.KubernetesUpgrade { //nolint:unparam
	ku := &tupprv1alpha1.KubernetesUpgrade{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Generation: 1,
		},
		Spec: tupprv1alpha1.KubernetesUpgradeSpec{
			Kubernetes: tupprv1alpha1.KubernetesSpec{
				Version: "v1.34.0",
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

func withK8sPhase(phase string) func(*tupprv1alpha1.KubernetesUpgrade) {
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

func newControllerNode(name, ip string) *corev1.Node { //nolint:unparam
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

func newK8sReconciler(cl client.Client, vg VersionGetter, tc TalosClientInterface, hc HealthCheckRunner) *KubernetesUpgradeReconciler {
	return &KubernetesUpgradeReconciler{
		Client:              cl,
		Scheme:              newScheme(),
		TalosConfigSecret:   "test-talosconfig",
		ControllerNamespace: "default",
		TalosClient:         tc,
		HealthChecker:       hc,
		MetricsReporter:     NewMetricsReporter(),
		VersionGetter:       vg,
		now:                 &fixedClock{time.Now()},
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

func reconcileK8s(t *testing.T, r *KubernetesUpgradeReconciler, name string) ctrl.Result { //nolint:unparam
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
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade")
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{version: "v1.33.0"}, &mockTalosClient{}, &mockHealthChecker{})

	reconcileK8s(t, r, "test-upgrade")

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if !controllerutil.ContainsFinalizer(updated, KubernetesUpgradeFinalizer) {
		t.Fatal("expected finalizer to be added")
	}
}

func TestK8sReconcile_SuspendAnnotation(t *testing.T) {
	scheme := newScheme()
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
	if updated.Status.Phase != constants.PhasePending {
		t.Fatalf("expected phase Pending, got: %s", updated.Status.Phase)
	}
	if updated.Status.Message == "" {
		t.Fatal("expected non-empty suspension message")
	}
}

func TestK8sReconcile_ResetAnnotation(t *testing.T) {
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(constants.PhaseFailed),
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
	if updated.Status.Phase != constants.PhasePending {
		t.Fatalf("expected phase reset to Pending, got: %s", updated.Status.Phase)
	}
}

func TestK8sReconcile_GenerationChange(t *testing.T) {
	scheme := newScheme()
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
	if updated.Status.Phase != constants.PhasePending {
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
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(constants.PhasePending),
	)
	tu := newTalosUpgrade("talos-upgrade",
		withFinalizer,
		withPhase(constants.PhaseInProgress),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, tu).WithStatusSubresource(ku, tu).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 2*time.Minute {
		t.Fatalf("expected 2m requeue, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhasePending {
		t.Fatalf("expected phase Pending while blocked, got: %s", updated.Status.Phase)
	}
	if updated.Status.Message == "" {
		t.Fatal("expected blocking message in status")
	}
}

func TestK8sReconcile_AlreadyAtTargetVersion(t *testing.T) {
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(constants.PhasePending),
	)
	vg := &mockVersionGetter{version: "v1.34.0"} // matches target
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != time.Hour {
		t.Fatalf("expected 1h requeue when at target, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseCompleted {
		t.Fatalf("expected phase Completed, got: %s", updated.Status.Phase)
	}
}

func TestK8sReconcile_HealthCheckFailure(t *testing.T) {
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(constants.PhasePending),
	)
	vg := &mockVersionGetter{version: "v1.33.0"} // needs upgrade
	hc := &mockHealthChecker{err: fmt.Errorf("cluster not healthy")}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, &mockTalosClient{}, hc)

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != time.Minute {
		t.Fatalf("expected 1m requeue on health check failure, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhasePending {
		t.Fatalf("expected phase Pending while health checks fail, got: %s", updated.Status.Phase)
	}
	if !strings.Contains(updated.Status.Message, "health") {
		t.Fatalf("expected message about health checks, got: %s", updated.Status.Message)
	}
}

func TestK8sReconcile_StartsUpgrade(t *testing.T) {
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(constants.PhasePending),
	)
	ctrlNode := newControllerNode(fakeCrtl, "10.0.0.1")
	vg := &mockVersionGetter{version: "v1.33.0"}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, ctrlNode).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue after job creation, got: %v", result.RequeueAfter)
	}

	// Verify job was created
	var jobList batchv1.JobList
	if err := cl.List(context.Background(), &jobList, client.InNamespace("default")); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Fatalf("expected 1 job, got: %d", len(jobList.Items))
	}
	if jobList.Items[0].Labels["tuppr.home-operations.com/target-node"] != fakeCrtl {
		t.Fatalf("expected job for ctrl-1, got: %s", jobList.Items[0].Labels["tuppr.home-operations.com/target-node"])
	}

	// Verify status updated to InProgress
	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseInProgress {
		t.Fatalf("expected phase InProgress, got: %s", updated.Status.Phase)
	}
	if updated.Status.ControllerNode != fakeCrtl {
		t.Fatalf("expected controllerNode=ctrl-1, got: %s", updated.Status.ControllerNode)
	}
}

func TestK8sReconcile_NoControllerNode(t *testing.T) {
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(constants.PhasePending),
	)
	workerNode := newNode("worker-1", "10.0.0.2")
	vg := &mockVersionGetter{version: "v1.33.0"}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, workerNode).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 5*time.Minute {
		t.Fatalf("expected 5m requeue, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseFailed {
		t.Fatalf("expected phase Failed when no controller node, got: %s", updated.Status.Phase)
	}
}

func TestK8sReconcile_VersionDetectionFailure(t *testing.T) {
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(constants.PhasePending),
	)
	vg := &mockVersionGetter{err: fmt.Errorf("connection refused")}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 5*time.Minute {
		t.Fatalf("expected 5m requeue, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseFailed {
		t.Fatalf("expected phase Failed on version detection failure, got: %s", updated.Status.Phase)
	}
	if !strings.Contains(updated.Status.Message, "version") {
		t.Fatalf("expected message about version detection, got: %s", updated.Status.Message)
	}
}

func TestK8sReconcile_HandlesActiveJobRunning(t *testing.T) {
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(constants.PhaseInProgress),
	)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-upgrade-ctrl-1-abcd1234",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name":                "kubernetes-upgrade",
				"tuppr.home-operations.com/target-node": fakeCrtl,
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
	if updated.Status.Phase != constants.PhaseInProgress {
		t.Fatalf("expected phase InProgress while job running, got: %s", updated.Status.Phase)
	}
	if updated.Status.JobName != "test-upgrade-ctrl-1-abcd1234" {
		t.Fatalf("expected jobName to be set, got: %s", updated.Status.JobName)
	}
}

func TestK8sReconcile_HandlesJobFailure(t *testing.T) {
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(constants.PhaseInProgress),
	)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-upgrade-ctrl-1-abcd1234",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name":                "kubernetes-upgrade",
				"tuppr.home-operations.com/target-node": fakeCrtl,
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
	if updated.Status.Phase != constants.PhaseFailed {
		t.Fatalf("expected phase Failed, got: %s", updated.Status.Phase)
	}
	if updated.Status.LastError == "" {
		t.Fatal("expected lastError to be set on failure")
	}
	if updated.Status.JobName == "" {
		t.Fatal("expected jobName to be preserved on failure")
	}
}

func TestK8sReconcile_HandlesJobSuccess(t *testing.T) {
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(constants.PhaseInProgress),
	)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-upgrade-ctrl-1-abcd1234",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name":                "kubernetes-upgrade",
				"tuppr.home-operations.com/target-node": fakeCrtl,
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr.To(int32(2)), Template: corev1.PodTemplateSpec{}},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	// Version now matches target after successful upgrade
	vg := &mockVersionGetter{version: "v1.34.0"}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, job).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != time.Hour {
		t.Fatalf("expected 1h requeue after success, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseCompleted {
		t.Fatalf("expected phase Completed, got: %s", updated.Status.Phase)
	}
}

func TestK8sReconcile_JobSuccessButVersionMismatch(t *testing.T) {
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(constants.PhaseInProgress),
	)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-upgrade-ctrl-1-abcd1234",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name":                "kubernetes-upgrade",
				"tuppr.home-operations.com/target-node": fakeCrtl,
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr.To(int32(2)), Template: corev1.PodTemplateSpec{}},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	// Version doesn't match target - upgrade didn't actually work
	vg := &mockVersionGetter{version: "v1.33.0"}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, job).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, vg, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter != 10*time.Minute {
		t.Fatalf("expected 10m requeue after verification failure, got: %v", result.RequeueAfter)
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseFailed {
		t.Fatalf("expected phase Failed after version mismatch, got: %s", updated.Status.Phase)
	}
}

func TestK8sReconcile_Cleanup(t *testing.T) {
	scheme := newScheme()
	now := metav1.Now()
	ku := &tupprv1alpha1.KubernetesUpgrade{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-upgrade",
			Generation:        1,
			DeletionTimestamp: &now,
			Finalizers:        []string{KubernetesUpgradeFinalizer},
		},
		Spec: tupprv1alpha1.KubernetesUpgradeSpec{
			Kubernetes: tupprv1alpha1.KubernetesSpec{Version: "v1.34.0"},
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
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade",
		withK8sFinalizer,
		withK8sPhase(constants.PhaseInProgress),
	)
	tu := newTalosUpgrade("talos-upgrade",
		withFinalizer,
		withPhase(constants.PhaseInProgress),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, tu).WithStatusSubresource(ku, tu).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileK8s(t, r, "test-upgrade")
	if result.RequeueAfter == 2*time.Minute {
		t.Fatal("InProgress upgrade should bypass coordination check, but got 2m requeue (blocked)")
	}

	updated := getK8sUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase == constants.PhasePending {
		t.Fatal("expected phase to not be Pending (should have bypassed coordination)")
	}
}

func TestK8sBuildJob_Properties(t *testing.T) {
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	ku.Spec.Kubernetes.Version = "v1.34.0"
	tc := &mockTalosClient{
		nodeVersions: map[string]string{"10.0.0.1": "v1.10.0"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, "10.0.0.1")).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job := r.buildJob(context.Background(), ku, fakeCrtl, "10.0.0.1")

	if job.Labels["app.kubernetes.io/name"] != "kubernetes-upgrade" {
		t.Fatalf("expected kubernetes-upgrade label, got: %s", job.Labels["app.kubernetes.io/name"])
	}

	podSpec := job.Spec.Template.Spec
	if !*podSpec.SecurityContext.RunAsNonRoot {
		t.Fatal("expected RunAsNonRoot")
	}

	container := podSpec.Containers[0]
	if container.Name != "upgrade-k8s" {
		t.Fatalf("expected container name 'upgrade-k8s', got: %s", container.Name)
	}

	foundUpgradeCmd, foundVersion := false, false
	for _, arg := range container.Args {
		if arg == "upgrade-k8s" {
			foundUpgradeCmd = true
		}
		if arg == "--to=v1.34.0" {
			foundVersion = true
		}
	}
	if !foundUpgradeCmd {
		t.Fatal("expected upgrade-k8s command in args")
	}
	if !foundVersion {
		t.Fatal("expected --to=v1.34.0 in args")
	}
}

func TestK8sBuildJob_CustomImage(t *testing.T) {
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	ku.Spec.Talosctl.Image.Repository = "my-registry.io/talosctl"
	ku.Spec.Talosctl.Image.Tag = "v1.9.0"
	ku.Spec.Talosctl.Image.PullPolicy = corev1.PullAlways

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, "10.0.0.1")).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	job := r.buildJob(context.Background(), ku, fakeCrtl, "10.0.0.1")
	container := job.Spec.Template.Spec.Containers[0]
	if container.Image != "my-registry.io/talosctl:v1.9.0" {
		t.Fatalf("expected custom image, got: %s", container.Image)
	}
	if container.ImagePullPolicy != corev1.PullAlways {
		t.Fatalf("expected PullAlways, got: %s", container.ImagePullPolicy)
	}
}

func TestK8sFindControllerNode(t *testing.T) {
	scheme := newScheme()
	ctrlNode := newControllerNode(fakeCrtl, "10.0.0.1")
	workerNode := newNode("worker-1", "10.0.0.2")
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ctrlNode, workerNode).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	name, ip, err := r.findControllerNode(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != fakeCrtl || ip != "10.0.0.1" {
		t.Fatalf("expected ctrl-1/10.0.0.1, got: %s/%s", name, ip)
	}
}

func TestK8sFindControllerNode_NoControlPlane(t *testing.T) {
	scheme := newScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(newNode("worker-1", "10.0.0.2")).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	_, _, err := r.findControllerNode(context.Background())
	if err == nil {
		t.Fatal("expected error when no control-plane node")
	}
}

func TestK8sBuildJob_FallbackTag(t *testing.T) {
	scheme := newScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	tc := &mockTalosClient{getVersionErr: fmt.Errorf("connection refused")}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, "10.0.0.1")).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job := r.buildJob(context.Background(), ku, fakeCrtl, "10.0.0.1")
	container := job.Spec.Template.Spec.Containers[0]
	expectedImage := "ghcr.io/siderolabs/talosctl:latest"
	if container.Image != expectedImage {
		t.Fatalf("expected fallback image %s, got: %s", expectedImage, container.Image)
	}
}

func TestKubernetesUpgradeReconciler_MaintenanceWindowBlocks(t *testing.T) {
	scheme := newScheme()
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
	r := &KubernetesUpgradeReconciler{
		Client:              cl,
		Scheme:              scheme,
		ControllerNamespace: "default",
		TalosConfigSecret:   "talosconfig",
		HealthChecker:       &mockHealthChecker{},
		TalosClient:         &mockTalosClient{},
		VersionGetter:       &mockVersionGetter{version: "v1.34.0"},
		MetricsReporter:     NewMetricsReporter(),
		now:                 &fixedClock{t: now},
	}

	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatal("expected requeue when outside maintenance window")
	}

	// Verify status updated
	var updated tupprv1alpha1.KubernetesUpgrade
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "test"}, &updated); err != nil {
		t.Fatalf("failed to get updated upgrade: %v", err)
	}
	if updated.Status.Phase != constants.PhasePending {
		t.Fatalf("expected phase Pending, got %s", updated.Status.Phase)
	}
	if !strings.Contains(updated.Status.Message, "Waiting for maintenance window") {
		t.Fatalf("expected message about waiting for window, got: %s", updated.Status.Message)
	}
	if updated.Status.NextMaintenanceWindow == nil {
		t.Fatal("expected nextMaintenanceWindow to be set")
	}
}

func TestKubernetesUpgradeReconciler_MaintenanceWindowAllows(t *testing.T) {
	scheme := newScheme()
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
		WithObjects(ku, newControllerNode(fakeCrtl, "10.0.0.1")).
		WithStatusSubresource(ku).Build()
	r := &KubernetesUpgradeReconciler{
		Client:              cl,
		Scheme:              scheme,
		ControllerNamespace: "default",
		TalosConfigSecret:   "talosconfig",
		HealthChecker:       &mockHealthChecker{},
		TalosClient: &mockTalosClient{
			nodeVersions: map[string]string{"10.0.0.1": "v1.11.0"},
		},
		VersionGetter:   &mockVersionGetter{version: "v1.34.0"},
		MetricsReporter: NewMetricsReporter(),
		now:             &fixedClock{t: now},
	}

	// Inside window — should proceed with upgrade logic
	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatal("expected requeue for normal processing")
	}

	// Should NOT be blocked by maintenance window
	var updated tupprv1alpha1.KubernetesUpgrade
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "test"}, &updated); err != nil {
		t.Fatalf("failed to get updated upgrade: %v", err)
	}
	if strings.Contains(updated.Status.Message, "Waiting for maintenance window") {
		t.Fatalf("should not be blocked by maintenance window inside window, message: %s", updated.Status.Message)
	}
}
