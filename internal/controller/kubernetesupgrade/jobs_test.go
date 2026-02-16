package kubernetesupgrade

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestK8sBuildJob_Properties(t *testing.T) {
	scheme := newTestScheme()
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
	scheme := newTestScheme()
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
