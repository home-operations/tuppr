package kubernetesupgrade

import (
	"context"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testK8sVersion = "v1.34.0"
	testV110       = "v1.10.0"
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

	foundUpgradeCmd, foundVersion, foundEndpoints := false, false, false
	for _, arg := range container.Args {
		if arg == upgradeK8sCommand {
			foundUpgradeCmd = true
		}
		if arg == "--to="+testK8sVersion {
			foundVersion = true
		}
		if arg == "--endpoints="+testNodeIP {
			foundEndpoints = true
		}
	}
	if !foundUpgradeCmd {
		t.Fatal("expected upgrade-k8s command in args")
	}
	if !foundVersion {
		t.Fatalf("expected --to=%s in args", testK8sVersion)
	}
	if !foundEndpoints {
		t.Fatalf("expected --endpoints=%s in args", testNodeIP)
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

func TestK8sBuildJob_DefaultEndpointFlag(t *testing.T) {
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

	want := "--endpoint=" + defaultKubernetesAPIEndpoint
	if !slices.Contains(job.Spec.Template.Spec.Containers[0].Args, want) {
		t.Fatalf("expected %q in args, got: %v", want, job.Spec.Template.Spec.Containers[0].Args)
	}
}

func TestK8sBuildJob_CustomEndpoint(t *testing.T) {
	scheme := newTestScheme()
	ku := newKubernetesUpgrade("test-upgrade", withK8sFinalizer)
	ku.Spec.Kubernetes.Version = testK8sVersion
	ku.Spec.Kubernetes.Endpoint = "https://api.example.com:6443"

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

	args := job.Spec.Template.Spec.Containers[0].Args
	want := "--endpoint=https://api.example.com:6443"
	if !slices.Contains(args, want) {
		t.Fatalf("expected %q in args, got: %v", want, args)
	}
	for _, a := range args {
		if strings.HasPrefix(a, "--endpoint=") && a != want {
			t.Fatalf("unexpected extra --endpoint flag: %q", a)
		}
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
				"--apiserver-image=registry.example.com/k8s/kube-apiserver",
				"--controller-manager-image=registry.example.com/k8s/kube-controller-manager",
				"--scheduler-image=registry.example.com/k8s/kube-scheduler",
				"--proxy-image=registry.example.com/k8s/kube-proxy",
				"--kubelet-image=registry.example.com/k8s/kubelet",
			},
		},
		{
			name:       "no trailing slash",
			repository: "registry.example.com/k8s",
			want: []string{
				"--apiserver-image=registry.example.com/k8s/kube-apiserver",
				"--controller-manager-image=registry.example.com/k8s/kube-controller-manager",
				"--scheduler-image=registry.example.com/k8s/kube-scheduler",
				"--proxy-image=registry.example.com/k8s/kube-proxy",
				"--kubelet-image=registry.example.com/k8s/kubelet",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := componentImageArgs(c.repository)
			if !slices.Equal(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}
