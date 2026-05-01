package talosupgrade

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/controller/jobs"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
)

const (
	hookPhasePre  = "pre"
	hookPhasePost = "post"

	hookJobAppName            = "talos-upgrade-hook"
	hookPhaseLabel            = "tuppr.home-operations.com/hook-phase"
	hookNameLabel             = "tuppr.home-operations.com/hook-name"
	hookIndexLabel            = "tuppr.home-operations.com/hook-index"
	hookJobActiveDeadline     = int64(600)
	hookJobBackoffLimit       = int32(0)
	hookJobTTLAfterFinished   = int32(300)
	hookJobGracePeriodSeconds = int64(30)
)

func hookList(tu *tupprv1alpha1.TalosUpgrade, phase string) []tupprv1alpha1.HookSpec {
	if phase == hookPhasePre {
		return tu.Spec.Hooks.Pre
	}
	return tu.Spec.Hooks.Post
}

func hookIndex(tu *tupprv1alpha1.TalosUpgrade, phase string) int {
	if phase == hookPhasePre {
		return tu.Status.PreHookIndex
	}
	return tu.Status.PostHookIndex
}

// processHookPhase advances the pre or post hook sequence by one step.
// done=true means the parent phase should transition out (whether all hooks
// succeeded, or a pre-hook failed and the run should be marked failed).
func (r *Reconciler) processHookPhase(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade, phase string) (ctrl.Result, bool, error) {
	logger := log.FromContext(ctx)

	hooks := hookList(tu, phase)
	idx := hookIndex(tu, phase)

	if idx >= len(hooks) {
		return ctrl.Result{}, true, nil
	}

	job, err := r.findActiveHookJob(ctx, tu, phase)
	if err != nil {
		logger.Error(err, "Failed to look up active hook job", "phase", phase)
		return ctrl.Result{RequeueAfter: time.Minute}, false, nil
	}

	if job != nil {
		return r.handleHookJobStatus(ctx, tu, job, phase, len(hooks))
	}

	return r.startHookJob(ctx, tu, hooks[idx], phase, idx, len(hooks))
}

// findActiveHookJob returns the in-flight hook Job (at most one — they run
// sequentially) for this CR + phase + current index.
func (r *Reconciler) findActiveHookJob(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade, phase string) (*batchv1.Job, error) {
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(r.ControllerNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     hookJobAppName,
			"app.kubernetes.io/instance": tu.Name,
			hookPhaseLabel:               phase,
		},
	); err != nil {
		return nil, fmt.Errorf("list hook jobs for %s/%s: %w", tu.Name, phase, err)
	}

	idx := hookIndex(tu, phase)
	for i := range jobList.Items {
		job := &jobList.Items[i]
		owner := metav1.GetControllerOf(job)
		if owner == nil || owner.UID != tu.UID {
			continue
		}
		if job.Labels[hookIndexLabel] != fmt.Sprintf("%d", idx) {
			continue
		}
		return job, nil
	}
	return nil, nil
}

func (r *Reconciler) startHookJob(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade, hook tupprv1alpha1.HookSpec, phase string, idx, total int) (ctrl.Result, bool, error) {
	logger := log.FromContext(ctx)

	job := r.buildHookJob(tu, hook, phase, idx)
	if err := controllerutil.SetControllerReference(tu, job, r.Scheme); err != nil {
		return ctrl.Result{}, false, fmt.Errorf("set controller ref on hook job: %w", err)
	}
	if err := r.Create(ctx, job); err != nil {
		return ctrl.Result{}, false, fmt.Errorf("create hook job %s: %w", job.Name, err)
	}
	logger.Info("Started hook job", "phase", phase, "hook", hook.Name, "index", idx, "job", job.Name)

	parentPhase := tupprv1alpha1.JobPhasePreHook
	if phase == hookPhasePost {
		parentPhase = tupprv1alpha1.JobPhasePostHook
	}
	if err := r.setPhase(ctx, tu, parentPhase, "", fmt.Sprintf("Running %s-hook %q (%d/%d)", phase, hook.Name, idx+1, total)); err != nil {
		logger.Error(err, "Failed to update phase for hook start")
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, false, nil
}

func (r *Reconciler) handleHookJobStatus(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade, job *batchv1.Job, phase string, total int) (ctrl.Result, bool, error) {
	logger := log.FromContext(ctx)

	if !jobs.IsTerminal(job) {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, false, nil
	}

	idx := hookIndex(tu, phase)
	hookName := job.Labels[hookNameLabel]

	if job.Status.Succeeded > 0 {
		logger.Info("Hook job succeeded", "phase", phase, "hook", hookName, "index", idx, "job", job.Name)
		r.MetricsReporter.RecordHookExecution(tu.Name, phase, hookName, "success")
		if err := r.advanceHookIndex(ctx, tu, phase, idx+1); err != nil {
			return ctrl.Result{}, false, err
		}
	} else {
		logger.Info("Hook job failed", "phase", phase, "hook", hookName, "index", idx, "job", job.Name)
		r.MetricsReporter.RecordHookExecution(tu.Name, phase, hookName, "failure")
		if phase == hookPhasePre {
			// Skip remaining pre-hooks; processHookPhase will report done so the
			// caller can run post-hooks (cleanup) and reach Failed.
			if err := r.failPreHook(ctx, tu, total); err != nil {
				return ctrl.Result{}, false, err
			}
		} else {
			// Post-hook failures don't change the run's outcome.
			if err := r.advanceHookIndex(ctx, tu, phase, idx+1); err != nil {
				return ctrl.Result{}, false, err
			}
		}
	}

	if err := jobs.DeleteJob(ctx, r.Client, job); err != nil {
		logger.Error(err, "Failed to clean up hook job (continuing)", "job", job.Name)
	}
	return ctrl.Result{RequeueAfter: time.Second}, false, nil
}

func (r *Reconciler) advanceHookIndex(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade, phase string, next int) error {
	updates := map[string]any{}
	if phase == hookPhasePre {
		updates["preHookIndex"] = next
		tu.Status.PreHookIndex = next
	} else {
		updates["postHookIndex"] = next
		tu.Status.PostHookIndex = next
	}
	return r.updateStatus(ctx, tu, updates)
}

func (r *Reconciler) failPreHook(ctx context.Context, tu *tupprv1alpha1.TalosUpgrade, total int) error {
	updates := map[string]any{
		"preHookFailed": true,
		"preHookIndex":  total,
	}
	tu.Status.PreHookFailed = true
	tu.Status.PreHookIndex = total
	return r.updateStatus(ctx, tu, updates)
}

func (r *Reconciler) buildHookJob(tu *tupprv1alpha1.TalosUpgrade, hook tupprv1alpha1.HookSpec, phase string, idx int) *batchv1.Job {
	jobName := nodeutil.GenerateSafeJobName(fmt.Sprintf("%s-%s-hook", tu.Name, phase), hook.Name)

	labels := map[string]string{
		"app.kubernetes.io/name":     hookJobAppName,
		"app.kubernetes.io/instance": tu.Name,
		"app.kubernetes.io/part-of":  "tuppr",
		hookPhaseLabel:               phase,
		hookNameLabel:                hook.Name,
		hookIndexLabel:               fmt.Sprintf("%d", idx),
	}

	activeDeadline := hookJobActiveDeadline
	if hook.ActiveDeadlineSeconds != nil {
		activeDeadline = *hook.ActiveDeadlineSeconds
	}
	backoff := hookJobBackoffLimit
	if hook.BackoffLimit != nil {
		backoff = *hook.BackoffLimit
	}

	pod := jobs.BuildHookPodSpec(jobs.HookPodSpecOptions{
		ContainerName:      "hook",
		Image:              hook.Image,
		PullPolicy:         hook.ImagePullPolicy,
		Command:            hook.Command,
		Args:               hook.Args,
		Env:                hook.Env,
		EnvFrom:            hook.EnvFrom,
		VolumeMounts:       hook.VolumeMounts,
		Volumes:            hook.Volumes,
		ServiceAccountName: hook.ServiceAccountName,
	})
	pod.TerminationGracePeriodSeconds = ptr.To(hookJobGracePeriodSeconds)

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: r.ControllerNamespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To(backoff),
			Completions:             ptr.To(int32(1)),
			Parallelism:             ptr.To(int32(1)),
			ActiveDeadlineSeconds:   ptr.To(activeDeadline),
			TTLSecondsAfterFinished: ptr.To(hookJobTTLAfterFinished),
			PodReplacementPolicy:    ptr.To(batchv1.Failed),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       pod,
			},
		},
	}
}

func resetHookProgress(status *tupprv1alpha1.TalosUpgradeStatus) {
	status.PreHookIndex = 0
	status.PostHookIndex = 0
	status.PreHookFailed = false
}
