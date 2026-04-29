package jobs

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/home-operations/tuppr/internal/constants"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = batchv1.AddToScheme(s)
	return s
}

func newJob(name, namespace, appName string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app.kubernetes.io/name": appName},
		},
	}
}

func TestBuildTalosctlPodSpec_SecurityPolicy(t *testing.T) {
	spec := BuildTalosctlPodSpec(PodSpecOptions{
		ContainerName: "upgrade", Image: "img:v1", TalosConfigSecret: "talosconfig",
	})

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"RestartPolicy", spec.RestartPolicy, corev1.RestartPolicyNever},
		{"PriorityClassName", spec.PriorityClassName, "system-node-critical"},
		{"RunAsNonRoot", *spec.SecurityContext.RunAsNonRoot, true},
		{"RunAsUser", *spec.SecurityContext.RunAsUser, int64(65532)},
		{"RunAsGroup", *spec.SecurityContext.RunAsGroup, int64(65532)},
		{"FSGroup", *spec.SecurityContext.FSGroup, int64(65532)},
		{"SeccompProfile", spec.SecurityContext.SeccompProfile.Type, corev1.SeccompProfileTypeRuntimeDefault},
		{"AllowPrivilegeEscalation", *spec.Containers[0].SecurityContext.AllowPrivilegeEscalation, false},
		{"ReadOnlyRootFilesystem", *spec.Containers[0].SecurityContext.ReadOnlyRootFilesystem, true},
		{"CapDrop", spec.Containers[0].SecurityContext.Capabilities.Drop[0], corev1.Capability("ALL")},
		{"TolerationOp", spec.Tolerations[0].Operator, corev1.TolerationOpExists},
		{"CPURequest", spec.Containers[0].Resources.Requests.Cpu().Cmp(resource.MustParse("10m")), 0},
		{"MemoryRequest", spec.Containers[0].Resources.Requests.Memory().Cmp(resource.MustParse("64Mi")), 0},
		{"MemoryLimit", spec.Containers[0].Resources.Limits.Memory().Cmp(resource.MustParse("512Mi")), 0},
		{"VolumeMount", spec.Containers[0].VolumeMounts[0].Name, constants.TalosSecretName},
		{"VolumeMountPath", spec.Containers[0].VolumeMounts[0].MountPath, "/var/run/secrets/talos.dev"},
		{"VolumeMountReadOnly", spec.Containers[0].VolumeMounts[0].ReadOnly, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestBuildTalosctlPodSpec_OptsApplied(t *testing.T) {
	affinity := &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}}

	tests := []struct {
		name  string
		opts  PodSpecOptions
		check func(t *testing.T, spec corev1.PodSpec)
	}{
		{
			name: "container fields from opts",
			opts: PodSpecOptions{
				ContainerName: "upgrade-k8s", Image: "my-reg/talosctl:v1.9.0",
				PullPolicy: corev1.PullAlways, Args: []string{"upgrade-k8s", "--nodes=10.0.0.1"},
			},
			check: func(t *testing.T, spec corev1.PodSpec) {
				c := spec.Containers[0]
				if c.Name != "upgrade-k8s" {
					t.Errorf("Name: got %q", c.Name)
				}
				if c.Image != "my-reg/talosctl:v1.9.0" {
					t.Errorf("Image: got %q", c.Image)
				}
				if c.ImagePullPolicy != corev1.PullAlways {
					t.Errorf("PullPolicy: got %q", c.ImagePullPolicy)
				}
				if len(c.Args) != 2 || c.Args[0] != "upgrade-k8s" {
					t.Errorf("Args: got %v", c.Args)
				}
			},
		},
		{
			name: "grace period from opts",
			opts: PodSpecOptions{GracePeriod: 600},
			check: func(t *testing.T, spec corev1.PodSpec) {
				if *spec.TerminationGracePeriodSeconds != 600 {
					t.Errorf("GracePeriod: got %d", *spec.TerminationGracePeriodSeconds)
				}
			},
		},
		{
			name: "talos config secret from opts",
			opts: PodSpecOptions{TalosConfigSecret: "my-secret"},
			check: func(t *testing.T, spec corev1.PodSpec) {
				if spec.Volumes[0].Secret.SecretName != "my-secret" {
					t.Errorf("SecretName: got %q", spec.Volumes[0].Secret.SecretName)
				}
			},
		},
		{
			name: "affinity nil when not provided",
			opts: PodSpecOptions{Affinity: nil},
			check: func(t *testing.T, spec corev1.PodSpec) {
				if spec.Affinity != nil {
					t.Error("want nil Affinity")
				}
			},
		},
		{
			name: "affinity set when provided",
			opts: PodSpecOptions{Affinity: affinity},
			check: func(t *testing.T, spec corev1.PodSpec) {
				if spec.Affinity != affinity {
					t.Error("want provided Affinity")
				}
			},
		},
		{
			name: "host aliases empty when not provided",
			opts: PodSpecOptions{},
			check: func(t *testing.T, spec corev1.PodSpec) {
				if len(spec.HostAliases) != 0 {
					t.Errorf("want no HostAliases, got %v", spec.HostAliases)
				}
			},
		},
		{
			name: "host aliases set when provided",
			opts: PodSpecOptions{
				HostAliases: []corev1.HostAlias{{IP: "10.0.1.100", Hostnames: []string{"kube.cluster.local"}}},
			},
			check: func(t *testing.T, spec corev1.PodSpec) {
				if len(spec.HostAliases) != 1 ||
					spec.HostAliases[0].IP != "10.0.1.100" ||
					len(spec.HostAliases[0].Hostnames) != 1 ||
					spec.HostAliases[0].Hostnames[0] != "kube.cluster.local" {
					t.Errorf("HostAliases: got %v", spec.HostAliases)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, BuildTalosctlPodSpec(tt.opts))
		})
	}
}

func TestListJobsByLabel(t *testing.T) {
	tests := []struct {
		name      string
		objects   []*batchv1.Job
		namespace string
		appName   string
		wantCount int
	}{
		{
			name:      "empty",
			wantCount: 0,
		},
		{
			name:      "filters by label",
			objects:   []*batchv1.Job{newJob("a", "default", "talos-upgrade"), newJob("b", "default", "kubernetes-upgrade")},
			namespace: "default", appName: "talos-upgrade",
			wantCount: 1,
		},
		{
			name:      "filters by namespace",
			objects:   []*batchv1.Job{newJob("a", "default", "talos-upgrade"), newJob("b", "other", "talos-upgrade")},
			namespace: "default", appName: "talos-upgrade",
			wantCount: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(newTestScheme())
			for _, o := range tt.objects {
				builder = builder.WithObjects(o)
			}
			result, err := ListJobsByLabel(context.Background(), builder.Build(), tt.namespace, tt.appName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != tt.wantCount {
				t.Fatalf("got %d jobs, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestFindActiveJobByLabel(t *testing.T) {
	tests := []struct {
		name    string
		objects []*batchv1.Job
		wantNil bool
		wantJob string
	}{
		{name: "no jobs returns nil", wantNil: true},
		{
			name:    "returns job when present",
			objects: []*batchv1.Job{newJob("job-1", "default", "talos-upgrade")},
			wantJob: "job-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(newTestScheme())
			for _, o := range tt.objects {
				builder = builder.WithObjects(o)
			}
			got, err := FindActiveJobByLabel(context.Background(), builder.Build(), "default", "talos-upgrade")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil && got != nil {
				t.Errorf("got %s, want nil", got.Name)
			}
			if !tt.wantNil && (got == nil || got.Name != tt.wantJob) {
				t.Errorf("got %v, want %s", got, tt.wantJob)
			}
		})
	}
}

func TestDeleteJob(t *testing.T) {
	tests := []struct {
		name      string
		setup     []*batchv1.Job
		toDelete  string
		wantCount int
	}{
		{
			name:      "removes the job",
			setup:     []*batchv1.Job{newJob("job-1", "default", "talos-upgrade")},
			toDelete:  "job-1",
			wantCount: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(newTestScheme())
			for _, o := range tt.setup {
				builder = builder.WithObjects(o)
			}
			cl := builder.Build()

			var target *batchv1.Job
			for _, o := range tt.setup {
				if o.Name == tt.toDelete {
					target = o
				}
			}
			if err := DeleteJob(context.Background(), cl, target); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			remaining, err := ListJobsByLabel(context.Background(), cl, "default", "talos-upgrade")
			if err != nil || len(remaining) != tt.wantCount {
				t.Fatalf("got (%v, %v), want %d remaining", remaining, err, tt.wantCount)
			}
		})
	}
}
