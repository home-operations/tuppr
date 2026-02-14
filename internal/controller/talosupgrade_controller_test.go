package controller

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	fakeNodeA = "node-a"
	fakeNodeB = "node-b"
	fakeNodeC = "node-c"

	fakeTalosVersion = "v1.12.0"
)

type mockTalosClient struct {
	nodeVersions  map[string]string
	installImages map[string]string
	waitReadyErr  error
	getVersionErr error
	getInstallErr error
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

func (m *mockTalosClient) WaitForNodeReady(ctx context.Context, nodeIP, nodeName string) error {
	return m.waitReadyErr
}

func (m *mockTalosClient) GetNodeInstallImage(ctx context.Context, nodeIP string) (string, error) {
	if m.getInstallErr != nil {
		return "", m.getInstallErr
	}
	if img, ok := m.installImages[nodeIP]; ok {
		return img, nil
	}
	return "", fmt.Errorf("install image not found for %s", nodeIP)
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

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = tupprv1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = batchv1.AddToScheme(s)
	return s
}

func newTalosUpgrade(name string, opts ...func(*tupprv1alpha1.TalosUpgrade)) *tupprv1alpha1.TalosUpgrade {
	tu := &tupprv1alpha1.TalosUpgrade{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Generation: 1,
		},
		Spec: tupprv1alpha1.TalosUpgradeSpec{
			Talos: tupprv1alpha1.TalosSpec{
				Version: fakeTalosVersion,
			},
		},
	}
	for _, opt := range opts {
		opt(tu)
	}
	return tu
}

func withFinalizer(tu *tupprv1alpha1.TalosUpgrade) {
	controllerutil.AddFinalizer(tu, TalosUpgradeFinalizer)
}

func withPhase(phase string) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Status.Phase = phase
		tu.Status.ObservedGeneration = tu.Generation
	}
}

func withAnnotation(key, value string) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		if tu.Annotations == nil {
			tu.Annotations = map[string]string{}
		}
		tu.Annotations[key] = value
	}
}

func withGeneration(gen, observed int64) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Generation = gen
		tu.Status.ObservedGeneration = observed
	}
}

func withFailedNodes(nodes ...string) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		for _, n := range nodes {
			tu.Status.FailedNodes = append(tu.Status.FailedNodes, tupprv1alpha1.NodeUpgradeStatus{
				NodeName:  n,
				LastError: "test failure",
			})
		}
	}
}

func withCompletedNodes(nodes ...string) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		tu.Status.CompletedNodes = nodes
	}
}

func newNode(name, ip string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: ip},
			},
		},
	}
}

func newTalosReconciler(cl client.Client, scheme *runtime.Scheme, talosClient TalosClientInterface, healthChecker HealthCheckRunner) *TalosUpgradeReconciler {
	return &TalosUpgradeReconciler{
		Client:              cl,
		Scheme:              scheme,
		TalosConfigSecret:   "test-talosconfig",
		ControllerNamespace: "default",
		TalosClient:         talosClient,
		HealthChecker:       healthChecker,
		MetricsReporter:     NewMetricsReporter(),
		now:                 &realClock{},
		ImageChecker:        &mockImageChecker{availableImages: nil},
	}
}

func getTalosUpgrade(t *testing.T, cl client.Client, name string) *tupprv1alpha1.TalosUpgrade { //nolint:unparam
	t.Helper()
	var tu tupprv1alpha1.TalosUpgrade
	if err := cl.Get(context.Background(), types.NamespacedName{Name: name}, &tu); err != nil {
		t.Fatalf("failed to get TalosUpgrade %q: %v", name, err)
	}
	return &tu
}

func reconcileTalos(t *testing.T, r *TalosUpgradeReconciler, name string) ctrl.Result { //nolint:unparam
	t.Helper()
	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: name},
	})
	if err != nil {
		t.Fatalf("Reconcile() returned unexpected error: %v", err)
	}
	return result
}

func TestTalosReconcile_AddsFinalizer(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade")
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	reconcileTalos(t, r, "test-upgrade")

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if !controllerutil.ContainsFinalizer(updated, TalosUpgradeFinalizer) {
		t.Fatal("expected finalizer to be added")
	}
}

func TestTalosReconcile_SuspendAnnotation(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withAnnotation(constants.SuspendAnnotation, "maintenance window"),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Minute {
		t.Fatalf("expected 30m requeue, got: %v", result.RequeueAfter)
	}

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhasePending {
		t.Fatalf("expected phase Pending, got: %s", updated.Status.Phase)
	}
	if updated.Status.Message == "" {
		t.Fatal("expected non-empty status message explaining suspension")
	}
}

func TestTalosReconcile_ResetAnnotation(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhaseFailed),
		withAnnotation(constants.ResetAnnotation, "true"),
		withFailedNodes(fakeNodeA),
		withCompletedNodes(fakeNodeB),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue, got: %v", result.RequeueAfter)
	}

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if _, exists := updated.Annotations[constants.ResetAnnotation]; exists {
		t.Fatal("expected reset annotation to be removed")
	}
	if updated.Status.Phase != constants.PhasePending {
		t.Fatalf("expected phase reset to Pending, got: %s", updated.Status.Phase)
	}
	if updated.Status.Message != "Reset requested via annotation" {
		t.Fatalf("expected reset message, got: %s", updated.Status.Message)
	}
	if len(updated.Status.CompletedNodes) != 0 {
		t.Fatalf("expected completedNodes to be cleared, got: %v", updated.Status.CompletedNodes)
	}
	if len(updated.Status.FailedNodes) != 0 {
		t.Fatalf("expected failedNodes to be cleared, got: %v", updated.Status.FailedNodes)
	}
}

func TestTalosReconcile_NodeVersionOverride(t *testing.T) {
	scheme := newScheme()
	// Global target is fakeTalosVersion (v1.12.0)
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
	)

	// Node is already at v1.12.0 (matches global), so normally wouldn't upgrade.
	// But we add an annotation requesting v1.12.1
	node := newNode(fakeNodeA, "10.0.0.1")
	node.Annotations = map[string]string{
		constants.VersionAnnotation: "v1.12.1",
	}

	tc := &mockTalosClient{
		nodeVersions: map[string]string{"10.0.0.1": fakeTalosVersion}, // Node is at v1.12.0
		// The controller will fetch the current image to get the base
		installImages: map[string]string{"10.0.0.1": "factory.talos.dev/installer/b55fbf4fdc6aec0c43e108cc8bde16d5533fbdeec3cb114ff3913ed9e8d019fe:v1.12.0"},
	}

	// We must mock that the specific overridden image is available
	ic := &mockImageChecker{
		availableImages: map[string]bool{
			"factory.talos.dev/installer/b55fbf4fdc6aec0c43e108cc8bde16d5533fbdeec3cb114ff3913ed9e8d019fe:v1.12.1": true,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()

	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})
	r.ImageChecker = ic

	// Run Reconcile
	result := reconcileTalos(t, r, "test-upgrade")

	// Expect job creation (30s requeue)
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue (job creation), got: %v", result.RequeueAfter)
	}

	// Verify the job uses the OVERRIDDEN version
	var jobList batchv1.JobList
	if err := cl.List(context.Background(), &jobList); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Fatal("expected 1 job created")
	}

	container := jobList.Items[0].Spec.Template.Spec.Containers[0]
	expectedArg := "--image=factory.talos.dev/installer/b55fbf4fdc6aec0c43e108cc8bde16d5533fbdeec3cb114ff3913ed9e8d019fe:v1.12.1"

	found := false
	for _, arg := range container.Args {
		if arg == expectedArg {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("job args %v did not contain expected override image %s", container.Args, expectedArg)
	}
}

func TestTalosReconcile_NodeSchematicOverride(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
	)

	node := newNode(fakeNodeA, "10.0.0.1")
	// Add schematic override
	node.Annotations = map[string]string{
		constants.SchematicAnnotation: "custom-schematic-id",
	}

	tc := &mockTalosClient{
		nodeVersions: map[string]string{"10.0.0.1": "v1.11.0"}, // Needs upgrade to v1.12.0
		// The current image is vanilla, but we expect the upgrade to use the schematic
		installImages: map[string]string{"10.0.0.1": "factory.talos.dev/installer/b55fbf4fdc6aec0c43e108cc8bde16d5533fbdeec3cb114ff3913ed9e8d019fe:v1.11.0"},
	}

	// The expected image is DefaultFactoryURL + schematic + global version
	// Note: buildTalosUpgradeImage uses "%s/%s" for factory/schematic, then adds ":%s" for version
	expectedImage := fmt.Sprintf("%s/custom-schematic-id:%s", constants.DefaultFactoryURL, fakeTalosVersion)

	ic := &mockImageChecker{
		availableImages: map[string]bool{
			expectedImage: true,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()

	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})
	r.ImageChecker = ic

	result := reconcileTalos(t, r, "test-upgrade")

	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue, got: %v", result.RequeueAfter)
	}

	var jobList batchv1.JobList
	err := cl.List(context.Background(), &jobList)
	if err != nil {
		t.Fatalf("Error not expected %s", err)
	}

	container := jobList.Items[0].Spec.Template.Spec.Containers[0]
	expectedArg := "--image=" + expectedImage

	found := false
	for _, arg := range container.Args {
		if arg == expectedArg {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("job args %v did not contain expected schematic image %s", container.Args, expectedArg)
	}
}

func TestTalosReconcile_GenerationChange(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withGeneration(2, 1),
		withCompletedNodes("node-old"),
	)
	tu.Status.Phase = constants.PhaseInProgress
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue, got: %v", result.RequeueAfter)
	}

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhasePending {
		t.Fatalf("expected phase reset to Pending, got: %s", updated.Status.Phase)
	}
	if updated.Status.Message != "Spec updated, restarting upgrade process" {
		t.Fatalf("expected generation change message, got: %s", updated.Status.Message)
	}
	if len(updated.Status.CompletedNodes) != 0 {
		t.Fatalf("expected completedNodes to be cleared on generation change, got: %v", updated.Status.CompletedNodes)
	}
	if len(updated.Status.FailedNodes) != 0 {
		t.Fatalf("expected failedNodes to be cleared on generation change, got: %v", updated.Status.FailedNodes)
	}
}

func TestTalosReconcile_BlockedByKubernetesUpgrade(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
	)
	ku := &tupprv1alpha1.KubernetesUpgrade{
		ObjectMeta: metav1.ObjectMeta{Name: "k8s-upgrade", Generation: 1},
		Status: tupprv1alpha1.KubernetesUpgradeStatus{
			Phase:              constants.PhaseInProgress,
			ObservedGeneration: 1,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, ku).WithStatusSubresource(tu, ku).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")
	if result.RequeueAfter != 2*time.Minute {
		t.Fatalf("expected 2m requeue, got: %v", result.RequeueAfter)
	}

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhasePending {
		t.Fatalf("expected phase Pending while blocked, got: %s", updated.Status.Phase)
	}
	if updated.Status.Message == "" {
		t.Fatal("expected blocking message in status")
	}
}

func TestTalosReconcile_FailedNodesSetPhaseFailed(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
		withFailedNodes(fakeNodeA),
	)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")
	if result.RequeueAfter != 5*time.Minute {
		t.Fatalf("expected 5m requeue, got: %v", result.RequeueAfter)
	}

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseFailed {
		t.Fatalf("expected phase Failed when nodes have failed, got: %s", updated.Status.Phase)
	}
	if updated.Status.Message == "" {
		t.Fatal("expected failure message mentioning failed nodes")
	}
}

func TestTalosReconcile_HealthCheckFailure(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
	)
	node := newNode(fakeNodeA, "10.0.0.1")
	tc := &mockTalosClient{
		nodeVersions:  map[string]string{"10.0.0.1": "v1.10.0"},
		installImages: map[string]string{"10.0.0.1": "factory.talos.dev/installer:v1.10.0"},
	}
	hc := &mockHealthChecker{err: fmt.Errorf("nodes not ready")}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, hc)

	result := reconcileTalos(t, r, "test-upgrade")
	if result.RequeueAfter != time.Minute {
		t.Fatalf("expected 1m requeue, got: %v", result.RequeueAfter)
	}

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhasePending {
		t.Fatalf("expected phase Pending during health check failure, got: %s", updated.Status.Phase)
	}
	if !strings.Contains(updated.Status.Message, "health") {
		t.Fatalf("expected message about health checks, got: %s", updated.Status.Message)
	}
}

func TestTalosReconcile_AllNodesUpToDate(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
	)
	node := newNode(fakeNodeA, "10.0.0.1")
	tc := &mockTalosClient{
		nodeVersions: map[string]string{"10.0.0.1": fakeTalosVersion},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, "test-upgrade")

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseCompleted {
		t.Fatalf("expected phase Completed when all nodes at target, got: %s", updated.Status.Phase)
	}
	if !strings.Contains(updated.Status.Message, "Successfully upgraded") {
		t.Fatalf("expected completion message, got: %s", updated.Status.Message)
	}
}

func TestTalosReconcile_SingleNodeVersionCheckFailure(t *testing.T) {
	// Regression test for https://github.com/home-operations/tuppr/issues/65
	// On single-node clusters, if GetNodeVersion fails (e.g. TLS expired cert),
	// the controller should retry instead of silently completing with 0 nodes.
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
	)
	node := newNode(fakeNodeA, "10.0.0.1")
	tc := &mockTalosClient{
		getVersionErr: fmt.Errorf("rpc error: code = Unavailable desc = connection error: desc = \"error reading server preface: remote error: tls: expired certificate\""),
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")

	// Should requeue for retry, not complete
	if result.RequeueAfter != time.Minute {
		t.Fatalf("expected 1m requeue for transient error, got: %v", result.RequeueAfter)
	}

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	// Must NOT be Completed — the node was never checked successfully
	if updated.Status.Phase == constants.PhaseCompleted {
		t.Fatal("expected phase to NOT be Completed when version check fails on single-node cluster")
	}
}

func TestTalosReconcile_MultiNodePartialVersionCheckFailure(t *testing.T) {
	// When one node's version check fails, the entire findNextNode should error
	// and the controller should retry, not skip that node.
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
	)
	nodeA := newNode(fakeNodeA, "10.0.0.1")
	nodeB := newNode(fakeNodeB, "10.0.0.2")
	// nodeA is already at target, nodeB fails version check
	tc := &mockTalosClient{
		nodeVersions: map[string]string{"10.0.0.1": fakeTalosVersion},
		// nodeB (10.0.0.2) is not in the map, so GetNodeVersion returns an error
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA, nodeB).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")

	// Should requeue for retry since nodeB version check failed
	if result.RequeueAfter != time.Minute {
		t.Fatalf("expected 1m requeue for node version check failure, got: %v", result.RequeueAfter)
	}

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase == constants.PhaseCompleted {
		t.Fatal("expected phase to NOT be Completed when a node version check fails")
	}
}

func TestTalosReconcile_CreatesJobForNextNode(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
	)
	node := newNode(fakeNodeA, "10.0.0.1")
	tc := &mockTalosClient{
		nodeVersions:  map[string]string{"10.0.0.1": "v1.10.0"},
		installImages: map[string]string{"10.0.0.1": "factory.talos.dev/installer:v1.10.0"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue, got: %v", result.RequeueAfter)
	}

	// Verify job was created
	var jobList batchv1.JobList
	if err := cl.List(context.Background(), &jobList, client.InNamespace("default")); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Fatalf("expected 1 job, got: %d", len(jobList.Items))
	}
	if jobList.Items[0].Labels["tuppr.home-operations.com/target-node"] != fakeNodeA {
		t.Fatalf("expected job for node-1, got: %s", jobList.Items[0].Labels["tuppr.home-operations.com/target-node"])
	}

	// Verify status was updated to InProgress
	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseInProgress {
		t.Fatalf("expected phase InProgress after job creation, got: %s", updated.Status.Phase)
	}
	if updated.Status.CurrentNode != fakeNodeA {
		t.Fatalf("expected currentNode=node-1, got: %s", updated.Status.CurrentNode)
	}
}

func TestTalosReconcile_HandlesActiveJobRunning(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhaseInProgress),
	)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-upgrade-node-1-abcd1234",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name":                "talos-upgrade",
				"tuppr.home-operations.com/target-node": fakeNodeA,
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr.To(int32(2)), Template: corev1.PodTemplateSpec{}},
		Status: batchv1.JobStatus{Active: 1},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, job).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue for active job, got: %v", result.RequeueAfter)
	}

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseInProgress {
		t.Fatalf("expected phase InProgress while job running, got: %s", updated.Status.Phase)
	}
	if updated.Status.CurrentNode != fakeNodeA {
		t.Fatalf("expected currentNode=node-1, got: %s", updated.Status.CurrentNode)
	}
}

func TestTalosReconcile_HandlesJobSuccess(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhaseInProgress),
	)
	node := newNode(fakeNodeA, "10.0.0.1")
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-upgrade-node-1-abcd1234",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name":                "talos-upgrade",
				"tuppr.home-operations.com/target-node": fakeNodeA,
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr.To(int32(2)), Template: corev1.PodTemplateSpec{}},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	tc := &mockTalosClient{
		nodeVersions: map[string]string{"10.0.0.1": fakeTalosVersion}, // matches target
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node, job).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")
	if result.RequeueAfter != 5*time.Second {
		t.Fatalf("expected 5s requeue, got: %v", result.RequeueAfter)
	}

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if !slices.Contains(updated.Status.CompletedNodes, fakeNodeA) {
		t.Fatalf("expected node-1 in CompletedNodes, got: %v", updated.Status.CompletedNodes)
	}

	// Verify job was cleaned up
	var jobList batchv1.JobList
	if err := cl.List(context.Background(), &jobList, client.InNamespace("default")); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	if len(jobList.Items) != 0 {
		t.Fatalf("expected job to be cleaned up after success, got %d jobs", len(jobList.Items))
	}
}

func TestTalosReconcile_HandlesJobFailure(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhaseInProgress),
	)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-upgrade-node-1-abcd1234",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name":                "talos-upgrade",
				"tuppr.home-operations.com/target-node": fakeNodeA,
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr.To(int32(2)), Template: corev1.PodTemplateSpec{}},
		Status: batchv1.JobStatus{Failed: 2},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, job).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")
	if result.RequeueAfter != 10*time.Minute {
		t.Fatalf("expected 10m requeue, got: %v", result.RequeueAfter)
	}

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseFailed {
		t.Fatalf("expected phase Failed, got: %s", updated.Status.Phase)
	}
	if len(updated.Status.FailedNodes) == 0 {
		t.Fatal("expected node-1 in FailedNodes")
	}
	if updated.Status.FailedNodes[0].NodeName != fakeNodeA {
		t.Fatalf("expected failed node name node-1, got: %s", updated.Status.FailedNodes[0].NodeName)
	}
}

func TestTalosReconcile_JobVerificationFailure(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhaseInProgress),
	)
	node := newNode(fakeNodeA, "10.0.0.1")
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-upgrade-node-1-abcd1234",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name":                "talos-upgrade",
				"tuppr.home-operations.com/target-node": fakeNodeA,
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr.To(int32(2)), Template: corev1.PodTemplateSpec{}},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	// Job "succeeded" but version still doesn't match
	tc := &mockTalosClient{
		nodeVersions: map[string]string{"10.0.0.1": "v1.10.0"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node, job).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")
	if result.RequeueAfter != 10*time.Minute {
		t.Fatalf("expected 10m requeue, got: %v", result.RequeueAfter)
	}

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseFailed {
		t.Fatalf("expected phase Failed after verification failure, got: %s", updated.Status.Phase)
	}
}

func TestTalosReconcile_MultiNodeUpgradeOrdering(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
	)
	node1 := newNode(fakeNodeA, "10.0.0.1")
	node2 := newNode(fakeNodeB, "10.0.0.2")
	node3 := newNode(fakeNodeC, "10.0.0.3")
	tc := &mockTalosClient{
		nodeVersions: map[string]string{
			"10.0.0.1": "v1.10.0",
			"10.0.0.2": "v1.10.0",
			"10.0.0.3": "v1.10.0",
		},
		installImages: map[string]string{
			"10.0.0.1": "factory.talos.dev/installer:v1.10.0",
			"10.0.0.2": "factory.talos.dev/installer:v1.10.0",
			"10.0.0.3": "factory.talos.dev/installer:v1.10.0",
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node1, node2, node3).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, "test-upgrade")

	var jobList batchv1.JobList
	if err := cl.List(context.Background(), &jobList, client.InNamespace("default")); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Fatalf("expected 1 job, got: %d", len(jobList.Items))
	}
	if jobList.Items[0].Labels["tuppr.home-operations.com/target-node"] != fakeNodeA {
		t.Fatalf("expected first job for node-a (alphabetical), got: %s",
			jobList.Items[0].Labels["tuppr.home-operations.com/target-node"])
	}
}

func TestTalosReconcile_SkipsAlreadyUpgradedNodes(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
	)
	node1 := newNode(fakeNodeA, "10.0.0.1")
	node2 := newNode(fakeNodeB, "10.0.0.2")
	tc := &mockTalosClient{
		nodeVersions: map[string]string{
			"10.0.0.1": fakeTalosVersion, // already at target
			"10.0.0.2": "v1.10.0",        // needs upgrade
		},
		installImages: map[string]string{
			"10.0.0.2": "factory.talos.dev/installer:v1.10.0",
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node1, node2).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	reconcileTalos(t, r, "test-upgrade")

	var jobList batchv1.JobList
	if err := cl.List(context.Background(), &jobList, client.InNamespace("default")); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Fatalf("expected 1 job, got: %d", len(jobList.Items))
	}
	if jobList.Items[0].Labels["tuppr.home-operations.com/target-node"] != fakeNodeB {
		t.Fatalf("expected job for node-b (node-a already upgraded), got: %s",
			jobList.Items[0].Labels["tuppr.home-operations.com/target-node"])
	}
}

func TestTalosReconcile_InProgressBypassesCoordination(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhaseInProgress),
	)
	ku := &tupprv1alpha1.KubernetesUpgrade{
		ObjectMeta: metav1.ObjectMeta{Name: "k8s-upgrade", Generation: 1},
		Status: tupprv1alpha1.KubernetesUpgradeStatus{
			Phase:              constants.PhaseInProgress,
			ObservedGeneration: 1,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, ku).WithStatusSubresource(tu, ku).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")
	// Should NOT be blocked at 2m - should proceed past coordination to findActiveJob
	if result.RequeueAfter == 2*time.Minute {
		t.Fatal("InProgress upgrade should bypass coordination check, but got 2m requeue (blocked)")
	}
	// With no active job and no nodes, it should complete quickly
	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase == constants.PhasePending {
		t.Fatal("expected phase to not be Pending (should have bypassed coordination)")
	}
}

func TestTalosReconcile_Cleanup(t *testing.T) {
	scheme := newScheme()
	now := metav1.Now()
	tu := &tupprv1alpha1.TalosUpgrade{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-upgrade",
			Generation:        1,
			DeletionTimestamp: &now,
			Finalizers:        []string{TalosUpgradeFinalizer},
		},
		Spec: tupprv1alpha1.TalosUpgradeSpec{
			Talos: tupprv1alpha1.TalosSpec{Version: fakeTalosVersion},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})

	result := reconcileTalos(t, r, "test-upgrade")
	if result != (ctrl.Result{}) {
		t.Fatalf("expected empty result after cleanup, got: %v", result)
	}

	// Object should be gone (fake client deletes when finalizer removed + DeletionTimestamp set)
	var updated tupprv1alpha1.TalosUpgrade
	err := cl.Get(context.Background(), types.NamespacedName{Name: "test-upgrade"}, &updated)
	if err == nil {
		t.Fatal("expected object to be deleted after cleanup")
	}
}

func TestTalosReconcile_MultiNodeFullLifecycle(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
	)
	nodeA := newNode(fakeNodeA, "10.0.0.1")
	nodeB := newNode(fakeNodeB, "10.0.0.2")

	tc := &mockTalosClient{
		nodeVersions: map[string]string{
			"10.0.0.1": "v1.10.0",
			"10.0.0.2": "v1.10.0",
		},
		installImages: map[string]string{
			"10.0.0.1": "factory.talos.dev/installer:v1.10.0",
			"10.0.0.2": "factory.talos.dev/installer:v1.10.0",
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, nodeA, nodeB).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	// --- Step 1: First reconcile creates job for node-a (alphabetical) ---
	reconcileTalos(t, r, "test-upgrade")

	var jobList batchv1.JobList
	if err := cl.List(context.Background(), &jobList, client.InNamespace("default")); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Fatalf("step 1: expected 1 job, got %d", len(jobList.Items))
	}
	if jobList.Items[0].Labels["tuppr.home-operations.com/target-node"] != fakeNodeA {
		t.Fatalf("step 1: expected job for node-a, got: %s",
			jobList.Items[0].Labels["tuppr.home-operations.com/target-node"])
	}

	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseInProgress {
		t.Fatalf("step 1: expected phase InProgress, got: %s", updated.Status.Phase)
	}
	if updated.Status.CurrentNode != fakeNodeA {
		t.Fatalf("step 1: expected currentNode=node-a, got: %s", updated.Status.CurrentNode)
	}

	// --- Step 2: Mark job as succeeded, update mock to show node-a upgraded ---
	jobList.Items[0].Status.Succeeded = 1
	if err := cl.Status().Update(context.Background(), &jobList.Items[0]); err != nil {
		t.Fatalf("failed to update job status: %v", err)
	}
	tc.nodeVersions["10.0.0.1"] = fakeTalosVersion // node-a now at target

	reconcileTalos(t, r, "test-upgrade")

	updated = getTalosUpgrade(t, cl, "test-upgrade")
	if !slices.Contains(updated.Status.CompletedNodes, fakeNodeA) {
		t.Fatalf("step 2: expected node-a in CompletedNodes, got: %v", updated.Status.CompletedNodes)
	}

	// --- Step 3: Next reconcile should create job for node-b ---
	reconcileTalos(t, r, "test-upgrade")

	if err := cl.List(context.Background(), &jobList, client.InNamespace("default")); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	// Find the job targeting node-b
	foundNodeB := false
	for _, job := range jobList.Items {
		if job.Labels["tuppr.home-operations.com/target-node"] == fakeNodeB {
			foundNodeB = true
			break
		}
	}
	if !foundNodeB {
		t.Fatal("step 3: expected job for node-b to be created")
	}

	updated = getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.CurrentNode != fakeNodeB {
		t.Fatalf("step 3: expected currentNode=node-b, got: %s", updated.Status.CurrentNode)
	}

	// --- Step 4: Mark node-b job as succeeded ---
	if err := cl.List(context.Background(), &jobList, client.InNamespace("default")); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	for i := range jobList.Items {
		if jobList.Items[i].Labels["tuppr.home-operations.com/target-node"] == fakeNodeB {
			jobList.Items[i].Status.Succeeded = 1
			if err := cl.Status().Update(context.Background(), &jobList.Items[i]); err != nil {
				t.Fatalf("failed to update job status: %v", err)
			}
		}
	}
	tc.nodeVersions["10.0.0.2"] = fakeTalosVersion // node-b now at target

	reconcileTalos(t, r, "test-upgrade")

	updated = getTalosUpgrade(t, cl, "test-upgrade")
	if !slices.Contains(updated.Status.CompletedNodes, fakeNodeB) {
		t.Fatalf("step 4: expected node-b in CompletedNodes, got: %v", updated.Status.CompletedNodes)
	}

	// --- Step 5: Final reconcile should complete the upgrade ---
	reconcileTalos(t, r, "test-upgrade")

	updated = getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhaseCompleted {
		t.Fatalf("step 5: expected phase Completed, got: %s", updated.Status.Phase)
	}
	if !strings.Contains(updated.Status.Message, "Successfully upgraded") {
		t.Fatalf("step 5: expected completion message, got: %s", updated.Status.Message)
	}
	if len(updated.Status.CompletedNodes) != 2 {
		t.Fatalf("step 5: expected 2 completed nodes, got: %d", len(updated.Status.CompletedNodes))
	}
}

func TestTalosBuildJob_Properties(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade", withFinalizer)
	tu.Spec.Talos.Version = fakeTalosVersion
	tu.Spec.Policy.Placement = "hard"
	tu.Spec.Policy.Debug = true
	tu.Spec.Policy.Force = true
	tu.Spec.Policy.RebootMode = "powercycle"
	tu.Spec.Policy.Stage = true

	tc := &mockTalosClient{
		nodeVersions:  map[string]string{"10.0.0.1": "v1.10.0"},
		installImages: map[string]string{"10.0.0.1": "factory.talos.dev/installer:v1.10.0"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, newNode(fakeNodeA, "10.0.0.1")).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})
	targetImage := "factory.talos.dev/installer:" + fakeTalosVersion

	job := r.buildJob(context.Background(), tu, fakeNodeA, "10.0.0.1", targetImage)

	if job.Labels["app.kubernetes.io/name"] != "talos-upgrade" {
		t.Fatalf("expected talos-upgrade label, got: %s", job.Labels["app.kubernetes.io/name"])
	}
	if job.Labels["tuppr.home-operations.com/target-node"] != fakeNodeA {
		t.Fatalf("expected target-node label, got: %s", job.Labels["tuppr.home-operations.com/target-node"])
	}

	podSpec := job.Spec.Template.Spec
	if !*podSpec.SecurityContext.RunAsNonRoot {
		t.Fatal("expected RunAsNonRoot")
	}
	if *podSpec.SecurityContext.RunAsUser != 65532 {
		t.Fatalf("expected RunAsUser 65532, got: %d", *podSpec.SecurityContext.RunAsUser)
	}

	container := podSpec.Containers[0]
	if *container.SecurityContext.AllowPrivilegeEscalation {
		t.Fatal("expected AllowPrivilegeEscalation=false")
	}
	if !*container.SecurityContext.ReadOnlyRootFilesystem {
		t.Fatal("expected ReadOnlyRootFilesystem=true")
	}

	if podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		t.Fatal("expected required node affinity for hard placement")
	}

	wantArgs := map[string]bool{
		"--debug=true":             false,
		"--force=true":             false,
		"--reboot-mode=powercycle": false,
		"--stage":                  false,
		"--image=" + targetImage:   false,
	}
	for _, arg := range container.Args {
		if _, ok := wantArgs[arg]; ok {
			wantArgs[arg] = true
		}
	}
	for arg, found := range wantArgs {
		if !found {
			t.Fatalf("expected %s in args, got: %v", arg, container.Args)
		}
	}

	if len(podSpec.Tolerations) != 1 || podSpec.Tolerations[0].Operator != corev1.TolerationOpExists {
		t.Fatal("expected universal toleration")
	}
	if podSpec.PriorityClassName != "system-node-critical" {
		t.Fatalf("expected system-node-critical priority, got: %s", podSpec.PriorityClassName)
	}
}

func TestNodeNeedsUpgrade(t *testing.T) {
	scheme := newScheme()
	r := newTalosReconciler(fake.NewClientBuilder().WithScheme(scheme).Build(), scheme, nil, nil)

	tests := []struct {
		name          string
		nodeVersion   string
		nodeImage     string
		globalVersion string
		annotations   map[string]string
		wantUpgrade   bool
		wantError     bool
	}{
		{
			name:          "Standard: Versions match, no annotations -> No Upgrade",
			nodeVersion:   "v1.12.0",
			globalVersion: "v1.12.0",
			wantUpgrade:   false,
		},
		{
			name:          "Standard: Versions mismatch -> Upgrade",
			nodeVersion:   "v1.11.0",
			globalVersion: "v1.12.0",
			wantUpgrade:   true,
		},
		{
			name:          "Override: Version annotation differs from current -> Upgrade",
			nodeVersion:   "v1.12.0",
			globalVersion: "v1.12.0", // Global matches
			annotations: map[string]string{
				constants.VersionAnnotation: "v1.12.1", // Override requests update
			},
			wantUpgrade: true,
		},
		{
			name:          "Override: Version annotation matches current (Global differs) -> No Upgrade",
			nodeVersion:   "v1.12.0",
			globalVersion: "v1.13.0", // Global wants update
			annotations: map[string]string{
				constants.VersionAnnotation: "v1.12.0", // Override pins to current
			},
			wantUpgrade: false,
		},
		{
			name:          "Schematic: Versions match, Schematic annotation differs -> Upgrade",
			nodeVersion:   "v1.12.0",
			globalVersion: "v1.12.0",
			nodeImage:     "factory.talos.dev/installer/12345:v1.12.0",
			annotations: map[string]string{
				constants.SchematicAnnotation: "custom-schematic-id", // Request custom
			},
			wantUpgrade: true,
		},
		{
			name:          "Schematic: Versions match, Schematic annotation matches -> No Upgrade",
			nodeVersion:   "v1.12.0",
			globalVersion: "v1.12.0",
			nodeImage:     "factory.talos.dev/installer/custom-schematic-id:v1.12.0", // Already has schematic
			annotations: map[string]string{
				constants.SchematicAnnotation: "custom-schematic-id",
			},
			wantUpgrade: false,
		},
		{
			name:          "Schematic: Versions match, Image fetch fails -> Error",
			nodeVersion:   "v1.12.0",
			globalVersion: "v1.12.0",
			nodeImage:     "error", // Simulates failure
			annotations: map[string]string{
				constants.SchematicAnnotation: "custom-schematic-id",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := newNode("test-node", "10.0.0.1")
			if tt.annotations != nil {
				node.Annotations = tt.annotations
			}

			// Mock Client Setup
			tc := &mockTalosClient{
				nodeVersions: map[string]string{
					"10.0.0.1": tt.nodeVersion,
				},
			}

			if tt.nodeImage == "error" {
				tc.getInstallErr = fmt.Errorf("failed to fetch image")
			} else if tt.nodeImage != "" {
				tc.installImages = map[string]string{
					"10.0.0.1": tt.nodeImage,
				}
			}

			r.TalosClient = tc

			gotUpgrade, err := r.nodeNeedsUpgrade(context.Background(), node, tt.globalVersion)

			if (err != nil) != tt.wantError {
				t.Errorf("nodeNeedsUpgrade() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if gotUpgrade != tt.wantUpgrade {
				t.Errorf("nodeNeedsUpgrade() = %v, want %v", gotUpgrade, tt.wantUpgrade)
			}
		})
	}
}

func TestTalosBuildJob_SoftPlacement(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade", withFinalizer)
	tu.Spec.Policy.Placement = PlacementSoft
	tc := &mockTalosClient{
		nodeVersions:  map[string]string{"10.0.0.1": "v1.10.0"},
		installImages: map[string]string{"10.0.0.1": "factory.talos.dev/installer:v1.10.0"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, newNode(fakeNodeA, "10.0.0.1")).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})
	targetImage := "factory.talos.dev/installer:" + fakeTalosVersion
	job := r.buildJob(context.Background(), tu, fakeNodeA, "10.0.0.1", targetImage)
	podSpec := job.Spec.Template.Spec
	if podSpec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution == nil {
		t.Fatal("expected preferred node affinity for soft placement")
	}
	if podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		t.Fatal("soft placement should not have required affinity")
	}
}

func TestTalosBuildTalosUpgradeImage(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade", withFinalizer)
	tu.Spec.Talos.Version = fakeTalosVersion
	node := newNode(fakeNodeA, "10.0.0.1")
	tc := &mockTalosClient{
		installImages: map[string]string{"10.0.0.1": "factory.talos.dev/metal-installer/abc123:v1.10.0"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	image, err := r.buildTalosUpgradeImage(context.Background(), tu, fakeNodeA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if image != "factory.talos.dev/metal-installer/abc123:"+fakeTalosVersion {
		t.Fatalf("expected version-swapped image, got: %s", image)
	}
}

func TestTalosBuildTalosUpgradeImage_InvalidFormat(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade", withFinalizer)
	node := newNode(fakeNodeA, "10.0.0.1")
	tc := &mockTalosClient{
		installImages: map[string]string{"10.0.0.1": "no-colon-image"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	_, err := r.buildTalosUpgradeImage(context.Background(), tu, fakeNodeA)
	if err == nil {
		t.Fatal("expected error for invalid image format")
	}
}

func TestGetActiveDeadlineSeconds(t *testing.T) {
	timeout := 30 * time.Minute
	result := getActiveDeadlineSeconds(timeout)
	expected := int64(3*1800 + 600)
	if result != expected {
		t.Fatalf("getActiveDeadlineSeconds(%v) = %d, want %d", timeout, result, expected)
	}
}

func TestTalosUpgradeReconciler_MaintenanceWindowBlocks(t *testing.T) {
	scheme := newScheme()
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Window: every day at 02:00 UTC for 4 hours (outside current time)
	tu := newTalosUpgrade("test", func(tu *tupprv1alpha1.TalosUpgrade) {
		controllerutil.AddFinalizer(tu, TalosUpgradeFinalizer)
		tu.Spec.Maintenance = &tupprv1alpha1.MaintenanceSpec{
			Windows: []tupprv1alpha1.WindowSpec{
				{
					Start:    "0 2 * * *",
					Duration: metav1.Duration{Duration: 4 * time.Hour},
					Timezone: "UTC",
				},
			},
		}
		tu.Status.ObservedGeneration = tu.Generation
	})

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})
	r.now = &fixedClock{now}

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
	var updated tupprv1alpha1.TalosUpgrade
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

func TestTalosUpgradeReconciler_MaintenanceWindowAllows(t *testing.T) {
	scheme := newScheme()
	now := time.Date(2025, 6, 15, 3, 0, 0, 0, time.UTC) // Inside window

	tu := newTalosUpgrade("test", func(tu *tupprv1alpha1.TalosUpgrade) {
		controllerutil.AddFinalizer(tu, TalosUpgradeFinalizer)
		tu.Spec.Maintenance = &tupprv1alpha1.MaintenanceSpec{
			Windows: []tupprv1alpha1.WindowSpec{
				{
					Start:    "0 2 * * *",
					Duration: metav1.Duration{Duration: 4 * time.Hour},
					Timezone: "UTC",
				},
			},
		}
		tu.Status.ObservedGeneration = tu.Generation
	})

	nodes := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{Name: fakeNodeA},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.1"}},
				},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tu).WithLists(nodes).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})
	r.now = &fixedClock{now}
	// Inside window — should proceed with upgrade logic (find next node)
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
	var updated tupprv1alpha1.TalosUpgrade
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "test"}, &updated); err != nil {
		t.Fatalf("failed to get updated upgrade: %v", err)
	}
	if strings.Contains(updated.Status.Message, "Waiting for maintenance window") {
		t.Fatalf("should not be blocked by maintenance window inside window, message: %s", updated.Status.Message)
	}
}

func TestTalosReconcile_WaitsForImageAvailability(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
	)
	node := newNode(fakeNodeA, "10.0.0.1")

	tc := &mockTalosClient{
		nodeVersions:  map[string]string{"10.0.0.1": "v1.10.0"},
		installImages: map[string]string{"10.0.0.1": "factory.talos.dev/installer/abc:v1.10.0"},
	}

	// Setup ImageChecker to fail (simulate 500 error)
	ic := &mockImageChecker{
		availableImages: map[string]bool{}, // Empty map = image not found
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()

	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})
	r.ImageChecker = ic

	result := reconcileTalos(t, r, "test-upgrade")

	if result.RequeueAfter != 1*time.Minute {
		t.Fatalf("expected 1m requeue when image unavailable, got: %v", result.RequeueAfter)
	}

	// Verify Job was NOT created
	var jobList batchv1.JobList
	err := cl.List(context.Background(), &jobList)
	if err != nil {
		t.Fatalf("error not expected %s", err)
	}
	if len(jobList.Items) > 0 {
		t.Fatal("expected no job to be created when image is unavailable")
	}

	// Verify Status message
	updated := getTalosUpgrade(t, cl, "test-upgrade")
	if updated.Status.Phase != constants.PhasePending {
		t.Fatalf("expected phase Pending, got: %s", updated.Status.Phase)
	}
	if !strings.Contains(updated.Status.Message, "Waiting for image availability") {
		t.Fatalf("expected waiting message, got: %s", updated.Status.Message)
	}
}

func TestTalosReconcile_ProceedsWhenImageAvailable(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade",
		withFinalizer,
		withPhase(constants.PhasePending),
	)
	node := newNode(fakeNodeA, "10.0.0.1")

	targetImage := "factory.talos.dev/installer/abc:" + fakeTalosVersion

	tc := &mockTalosClient{
		nodeVersions:  map[string]string{"10.0.0.1": "v1.10.0"},
		installImages: map[string]string{"10.0.0.1": "factory.talos.dev/installer/abc:v1.10.0"},
	}

	ic := &mockImageChecker{
		availableImages: map[string]bool{
			targetImage: true,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()

	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})
	r.ImageChecker = ic
	// Run Reconcile
	result := reconcileTalos(t, r, "test-upgrade")

	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("expected 30s requeue (job created), got: %v", result.RequeueAfter)
	}

	var jobList batchv1.JobList
	err := cl.List(context.Background(), &jobList)
	if err != nil {
		t.Fatalf("error not expected %s", err)
	}
	if len(jobList.Items) != 1 {
		t.Fatal("expected upgrade job to be created")
	}

	container := jobList.Items[0].Spec.Template.Spec.Containers[0]
	expectedArg := "--image=" + targetImage
	foundImageArg := false
	for _, arg := range container.Args {
		if arg == expectedArg {
			foundImageArg = true
			break
		}
	}
	if !foundImageArg {
		t.Fatalf("job does not contain expected image arg: %s", expectedArg)
	}
}

type mockImageChecker struct {
	availableImages map[string]bool
	err             error
}

func (m *mockImageChecker) Check(ctx context.Context, imageRef string) error {
	if m.err != nil {
		return m.err
	}
	if m.availableImages == nil {
		return nil
	}
	if available, ok := m.availableImages[imageRef]; ok && available {
		return nil
	}
	// Simulate 500 or 404 error
	return fmt.Errorf("fetch failed after status: 500 Internal Server Error")
}

func TestTalosBuildTalosUpgradeImage_WithSchematicAnnotation(t *testing.T) {
	scheme := newScheme()
	tu := newTalosUpgrade("test-upgrade", withFinalizer)
	tu.Spec.Talos.Version = fakeTalosVersion // v1.12.0

	node := newNode(fakeNodeA, "10.0.0.1")
	node.Annotations = map[string]string{
		constants.SchematicAnnotation: "abc123schematic",
	}

	// TalosClient shouldn't even be called for image info if annotation exists,
	// but we provide it just in case
	tc := &mockTalosClient{
		installImages: map[string]string{"10.0.0.1": "factory.talos.dev/installer/b55fbf4fdc6aec0c43e108cc8bde16d5533fbdeec3cb114ff3913ed9e8d019fe:v1.10.0"},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).WithStatusSubresource(tu).Build()

	r := newTalosReconciler(cl, scheme, tc, &mockHealthChecker{})

	image, err := r.buildTalosUpgradeImage(context.Background(), tu, fakeNodeA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use factory URL + schematic + CR version
	expected := "factory.talos.dev/installer/abc123schematic:" + fakeTalosVersion
	if image != expected {
		t.Fatalf("expected schematic image %s, got: %s", expected, image)
	}
}
