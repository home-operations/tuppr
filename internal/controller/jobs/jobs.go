package jobs

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/home-operations/tuppr/internal/constants"
)

var (
	resourceCPU10m      = resource.MustParse("10m")
	resourceMemory64Mi  = resource.MustParse("64Mi")
	resourceMemory512Mi = resource.MustParse("512Mi")
)

// PodSpecOptions configures the shared talosctl pod spec.
type PodSpecOptions struct {
	ContainerName     string
	Image             string
	PullPolicy        corev1.PullPolicy
	Args              []string
	TalosConfigSecret string
	GracePeriod       int64
	Affinity          *corev1.Affinity
}

// BuildTalosctlPodSpec returns the shared pod spec used by both upgrade controllers.
func BuildTalosctlPodSpec(opts PodSpecOptions) corev1.PodSpec {
	spec := corev1.PodSpec{
		RestartPolicy:                 corev1.RestartPolicyNever,
		TerminationGracePeriodSeconds: ptr.To(opts.GracePeriod),
		PriorityClassName:             "system-node-critical",
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: ptr.To(true),
			RunAsUser:    ptr.To(int64(65532)),
			RunAsGroup:   ptr.To(int64(65532)),
			FSGroup:      ptr.To(int64(65532)),
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		Tolerations: []corev1.Toleration{
			{
				Operator: corev1.TolerationOpExists,
			},
		},
		Containers: []corev1.Container{{
			Name:            opts.ContainerName,
			Image:           opts.Image,
			ImagePullPolicy: opts.PullPolicy,
			Args:            opts.Args,
			Env: []corev1.EnvVar{{
				Name:  "TALOSCONFIG",
				Value: "/var/run/secrets/talos.dev/talosconfig",
			}},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(false),
				ReadOnlyRootFilesystem:   ptr.To(true),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resourceCPU10m,
					corev1.ResourceMemory: resourceMemory64Mi,
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resourceMemory512Mi,
				},
			},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      constants.TalosSecretName,
				MountPath: "/var/run/secrets/talos.dev",
				ReadOnly:  true,
			}},
		}},
		Volumes: []corev1.Volume{{
			Name: constants.TalosSecretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  opts.TalosConfigSecret,
					DefaultMode: ptr.To(int32(0o420)),
				},
			},
		}},
	}

	if opts.Affinity != nil {
		spec.Affinity = opts.Affinity
	}

	return spec
}

// ListJobsByLabel lists all Jobs in namespace matching the given app.kubernetes.io/name label.
func ListJobsByLabel(ctx context.Context, c client.Client, namespace, appName string) ([]batchv1.Job, error) {
	jobList := &batchv1.JobList{}
	if err := c.List(ctx, jobList,
		client.InNamespace(namespace),
		client.MatchingLabels{
			"app.kubernetes.io/name": appName,
		}); err != nil {
		return nil, fmt.Errorf("list jobs with app.kubernetes.io/name=%s in %s: %w", appName, namespace, err)
	}
	return jobList.Items, nil
}

// FindActiveJobByLabel returns the first Job matching appName in namespace, or nil if none.
func FindActiveJobByLabel(ctx context.Context, c client.Client, namespace, appName string) (*batchv1.Job, error) {
	jobs, err := ListJobsByLabel(ctx, c, namespace, appName)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, nil
	}
	return &jobs[0], nil
}

// DeleteJob deletes a Job with foreground propagation.
func DeleteJob(ctx context.Context, c client.Client, job *batchv1.Job) error {
	return c.Delete(ctx, job, &client.DeleteOptions{
		PropagationPolicy: ptr.To(metav1.DeletePropagationForeground),
	})
}

func IsTerminal(job *batchv1.Job) bool {
	if job.Status.Succeeded > 0 {
		return true
	}
	if job.Spec.BackoffLimit != nil && job.Status.Failed >= *job.Spec.BackoffLimit {
		return true
	}
	return false
}
