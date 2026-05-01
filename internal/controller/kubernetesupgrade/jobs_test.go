package kubernetesupgrade

import (
	"context"
	"testing"

	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	talosconfigresource "github.com/siderolabs/talos/pkg/machinery/resources/config"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func mustMachineConfigFromYAML(t *testing.T, yaml string) *talosconfigresource.MachineConfig {
	t.Helper()
	provider, err := configloader.NewFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("failed to load machine config yaml: %v", err)
	}
	return talosconfigresource.NewMachineConfig(provider)
}

const hcloudHostnameMachineConfig = `version: v1alpha1
machine:
  type: controlplane
  network:
    extraHostEntries:
      - ip: 10.0.1.100
        aliases:
          - kube.cluster.local
cluster:
  controlPlane:
    endpoint: https://kube.cluster.local:6443
`

const testK8sVersion = "v1.34.0"

func TestK8sBuildJob_Properties(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	ku.Spec.Kubernetes.Version = testK8sVersion
	tc := &mockTalosClient{
		nodeVersions: map[string]string{"10.0.0.1": "v1.10.0"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, "10.0.0.1")).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, "10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
		if arg == "--to="+testK8sVersion {
			foundVersion = true
		}
	}
	if !foundUpgradeCmd {
		t.Fatal("expected upgrade-k8s command in args")
	}
	if !foundVersion {
		t.Fatalf("expected --to=%s in args", testK8sVersion)
	}
}

func TestK8sBuildJob_CustomImage(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	ku.Spec.Talosctl.Image.Repository = "my-registry.io/talosctl"
	ku.Spec.Talosctl.Image.Tag = "v1.9.0"
	ku.Spec.Talosctl.Image.PullPolicy = corev1.PullAlways

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, "10.0.0.1")).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, "10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	container := job.Spec.Template.Spec.Containers[0]
	if container.Image != "my-registry.io/talosctl:v1.9.0" {
		t.Fatalf("expected custom image, got: %s", container.Image)
	}
	if container.ImagePullPolicy != corev1.PullAlways {
		t.Fatalf("expected PullAlways, got: %s", container.ImagePullPolicy)
	}
}

func TestK8sBuildJob_AutoDiscoversHostAlias(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	ku.Spec.Kubernetes.Version = testK8sVersion

	tc := &mockTalosClient{
		nodeVersions: map[string]string{"10.0.0.1": "v1.10.0"},
		machineConfigs: map[string]*talosconfigresource.MachineConfig{
			"10.0.0.1": mustMachineConfigFromYAML(t, hcloudHostnameMachineConfig),
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, "10.0.0.1")).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, "10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	aliases := job.Spec.Template.Spec.HostAliases
	if len(aliases) != 1 {
		t.Fatalf("expected 1 hostAlias, got %d: %+v", len(aliases), aliases)
	}
	if aliases[0].IP != "10.0.1.100" {
		t.Fatalf("expected VIP 10.0.1.100, got: %s", aliases[0].IP)
	}
	if len(aliases[0].Hostnames) != 1 || aliases[0].Hostnames[0] != "kube.cluster.local" {
		t.Fatalf("expected single hostname kube.cluster.local, got: %v", aliases[0].Hostnames)
	}
}

func TestK8sBuildJob_ExplicitHostAliasOverridesAutoDiscovery(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	ku.Spec.Kubernetes.Version = testK8sVersion
	ku.Spec.Kubernetes.HostAliases = []corev1.HostAlias{
		{IP: "192.168.99.99", Hostnames: []string{"kube.cluster.local"}},
	}

	tc := &mockTalosClient{
		nodeVersions: map[string]string{"10.0.0.1": "v1.10.0"},
		machineConfigs: map[string]*talosconfigresource.MachineConfig{
			"10.0.0.1": mustMachineConfigFromYAML(t, hcloudHostnameMachineConfig),
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, "10.0.0.1")).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, "10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	aliases := job.Spec.Template.Spec.HostAliases
	if len(aliases) != 1 {
		t.Fatalf("expected only the explicit hostAlias, got %d: %+v", len(aliases), aliases)
	}
	if aliases[0].IP != "192.168.99.99" {
		t.Fatalf("expected explicit override IP 192.168.99.99, got: %s", aliases[0].IP)
	}
}

func TestK8sBuildJob_ExplicitHostAliasOverrideMatchesAnyHostnameInEntry(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	ku.Spec.Kubernetes.Version = testK8sVersion
	ku.Spec.Kubernetes.HostAliases = []corev1.HostAlias{
		{IP: "192.168.99.99", Hostnames: []string{"other.host", "kube.cluster.local"}},
	}

	tc := &mockTalosClient{
		nodeVersions: map[string]string{"10.0.0.1": "v1.10.0"},
		machineConfigs: map[string]*talosconfigresource.MachineConfig{
			"10.0.0.1": mustMachineConfigFromYAML(t, hcloudHostnameMachineConfig),
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, "10.0.0.1")).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, "10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	aliases := job.Spec.Template.Spec.HostAliases
	if len(aliases) != 1 {
		t.Fatalf("expected only the explicit hostAlias when endpoint hostname is covered as a non-first entry, got %d: %+v", len(aliases), aliases)
	}
	if aliases[0].IP != "192.168.99.99" {
		t.Fatalf("expected explicit override IP, got: %s", aliases[0].IP)
	}
}

func TestK8sBuildJob_ExplicitHostAliasMergedWhenNoOverlap(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	ku.Spec.Kubernetes.Version = testK8sVersion
	ku.Spec.Kubernetes.HostAliases = []corev1.HostAlias{
		{IP: "10.99.99.99", Hostnames: []string{"unrelated.example"}},
	}

	tc := &mockTalosClient{
		nodeVersions: map[string]string{"10.0.0.1": "v1.10.0"},
		machineConfigs: map[string]*talosconfigresource.MachineConfig{
			"10.0.0.1": mustMachineConfigFromYAML(t, hcloudHostnameMachineConfig),
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, "10.0.0.1")).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, "10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	aliases := job.Spec.Template.Spec.HostAliases
	if len(aliases) != 2 {
		t.Fatalf("expected explicit + auto entries (2), got %d: %+v", len(aliases), aliases)
	}
	if aliases[0].IP != "10.99.99.99" {
		t.Fatalf("expected explicit entry first, got: %s", aliases[0].IP)
	}
	if aliases[1].IP != "10.0.1.100" {
		t.Fatalf("expected auto-discovered entry second, got: %s", aliases[1].IP)
	}
}

func TestK8sBuildJob_NoHostAliasForIPLiteralEndpoint(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	ku.Spec.Kubernetes.Version = testK8sVersion

	const ipLiteralConfig = `version: v1alpha1
machine:
  type: controlplane
cluster:
  controlPlane:
    endpoint: https://10.0.1.100:6443
`
	tc := &mockTalosClient{
		nodeVersions: map[string]string{"10.0.0.1": "v1.10.0"},
		machineConfigs: map[string]*talosconfigresource.MachineConfig{
			"10.0.0.1": mustMachineConfigFromYAML(t, ipLiteralConfig),
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, "10.0.0.1")).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, "10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(job.Spec.Template.Spec.HostAliases) != 0 {
		t.Fatalf("expected no hostAliases for IP-literal endpoint, got: %+v", job.Spec.Template.Spec.HostAliases)
	}
}
