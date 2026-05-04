package kubernetesupgrade

import (
	"context"
	"slices"
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

const (
	testK8sVersion        = "v1.34.0"
	testKubeClusterHost   = "kube.cluster.local"
	testHostAliasOverride = "192.168.99.99"
	testV110              = "v1.10.0"
)

func TestK8sBuildJob_Properties(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	ku.Spec.Kubernetes.Version = testK8sVersion
	tc := &mockTalosClient{
		nodeVersions: map[string]string{testNodeIP: testV110},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, testNodeIP)).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, testNodeIP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if job.Labels[appLabelKey] != kubernetesUpgradeAppName {
		t.Fatalf("expected kubernetes-upgrade label, got: %s", job.Labels[appLabelKey])
	}

	podSpec := job.Spec.Template.Spec
	if !*podSpec.SecurityContext.RunAsNonRoot {
		t.Fatal("expected RunAsNonRoot")
	}

	container := podSpec.Containers[0]
	if container.Name != upgradeK8sCommand {
		t.Fatalf("expected container name 'upgrade-k8s', got: %s", container.Name)
	}

	foundUpgradeCmd, foundVersion := false, false
	for _, arg := range container.Args {
		if arg == upgradeK8sCommand {
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
		WithObjects(ku, newControllerNode(fakeCrtl, testNodeIP)).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, &mockTalosClient{}, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, testNodeIP)
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
		nodeVersions: map[string]string{testNodeIP: testV110},
		machineConfigs: map[string]*talosconfigresource.MachineConfig{
			testNodeIP: mustMachineConfigFromYAML(t, hcloudHostnameMachineConfig),
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, testNodeIP)).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, testNodeIP)
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
	if len(aliases[0].Hostnames) != 1 || aliases[0].Hostnames[0] != testKubeClusterHost {
		t.Fatalf("expected single hostname kube.cluster.local, got: %v", aliases[0].Hostnames)
	}
}

func TestK8sBuildJob_ExplicitHostAliasOverridesAutoDiscovery(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	ku.Spec.Kubernetes.Version = testK8sVersion
	ku.Spec.Kubernetes.HostAliases = []corev1.HostAlias{
		{IP: testHostAliasOverride, Hostnames: []string{testKubeClusterHost}},
	}

	tc := &mockTalosClient{
		nodeVersions: map[string]string{testNodeIP: testV110},
		machineConfigs: map[string]*talosconfigresource.MachineConfig{
			testNodeIP: mustMachineConfigFromYAML(t, hcloudHostnameMachineConfig),
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, testNodeIP)).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, testNodeIP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	aliases := job.Spec.Template.Spec.HostAliases
	if len(aliases) != 1 {
		t.Fatalf("expected only the explicit hostAlias, got %d: %+v", len(aliases), aliases)
	}
	if aliases[0].IP != testHostAliasOverride {
		t.Fatalf("expected explicit override IP 192.168.99.99, got: %s", aliases[0].IP)
	}
}

func TestK8sBuildJob_ExplicitHostAliasOverrideMatchesAnyHostnameInEntry(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	ku.Spec.Kubernetes.Version = testK8sVersion
	ku.Spec.Kubernetes.HostAliases = []corev1.HostAlias{
		{IP: testHostAliasOverride, Hostnames: []string{"other.host", testKubeClusterHost}},
	}

	tc := &mockTalosClient{
		nodeVersions: map[string]string{testNodeIP: testV110},
		machineConfigs: map[string]*talosconfigresource.MachineConfig{
			testNodeIP: mustMachineConfigFromYAML(t, hcloudHostnameMachineConfig),
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, testNodeIP)).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, testNodeIP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	aliases := job.Spec.Template.Spec.HostAliases
	if len(aliases) != 1 {
		t.Fatalf("expected only the explicit hostAlias when endpoint hostname is covered as a non-first entry, got %d: %+v", len(aliases), aliases)
	}
	if aliases[0].IP != testHostAliasOverride {
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
		nodeVersions: map[string]string{testNodeIP: testV110},
		machineConfigs: map[string]*talosconfigresource.MachineConfig{
			testNodeIP: mustMachineConfigFromYAML(t, hcloudHostnameMachineConfig),
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, testNodeIP)).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, testNodeIP)
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

func TestComponentImageArgs(t *testing.T) {
	cases := []struct {
		name       string
		repository string
		want       []string
	}{
		{name: "empty", repository: "", want: nil},
		{
			name:       "trailing slash trimmed",
			repository: "registry.example.com/k8s/",
			want: []string{
				"--apiserver-image=registry.example.com/k8s/kube-apiserver:v1.34.0",
				"--controller-manager-image=registry.example.com/k8s/kube-controller-manager:v1.34.0",
				"--scheduler-image=registry.example.com/k8s/kube-scheduler:v1.34.0",
				"--proxy-image=registry.example.com/k8s/kube-proxy:v1.34.0",
				"--kubelet-image=registry.example.com/k8s/kubelet:v1.34.0",
			},
		},
		{
			name:       "no trailing slash",
			repository: "registry.example.com/k8s",
			want: []string{
				"--apiserver-image=registry.example.com/k8s/kube-apiserver:v1.34.0",
				"--controller-manager-image=registry.example.com/k8s/kube-controller-manager:v1.34.0",
				"--scheduler-image=registry.example.com/k8s/kube-scheduler:v1.34.0",
				"--proxy-image=registry.example.com/k8s/kube-proxy:v1.34.0",
				"--kubelet-image=registry.example.com/k8s/kubelet:v1.34.0",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := componentImageArgs(c.repository, "v1.34.0")
			if !slices.Equal(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
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
		nodeVersions: map[string]string{testNodeIP: testV110},
		machineConfigs: map[string]*talosconfigresource.MachineConfig{
			testNodeIP: mustMachineConfigFromYAML(t, ipLiteralConfig),
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ku, newControllerNode(fakeCrtl, testNodeIP)).WithStatusSubresource(ku).Build()
	r := newK8sReconciler(cl, &mockVersionGetter{}, tc, &mockHealthChecker{})

	job, err := r.buildJob(context.Background(), ku, fakeCrtl, testNodeIP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(job.Spec.Template.Spec.HostAliases) != 0 {
		t.Fatalf("expected no hostAliases for IP-literal endpoint, got: %+v", job.Spec.Template.Spec.HostAliases)
	}
}
