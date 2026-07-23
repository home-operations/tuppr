package talosupgrade

import (
	"context"
	"fmt"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/alerting"
)

const (
	// silenceTTL is the lease length. Each reconcile while the run is active
	// re-extends every silence to now+TTL, so the leases ride the existing
	// 5s-1m requeues; if the controller stops driving the run, they lapse on
	// their own within the TTL.
	silenceTTL = 25 * time.Minute

	// silenceTail is how long a silence outlives the run, so the last
	// reboot's alert tail doesn't fire right at completion.
	silenceTail = 5 * time.Minute

	// silenceDefaultMaxDuration mirrors the CRD default for specs created
	// without the API server's defaulting (tests, old objects).
	silenceDefaultMaxDuration = 4 * time.Hour
)

// syncAlertSilences maintains one Alertmanager silence lease per spec.silences
// entry. It is best-effort by design: an unreachable Alertmanager is logged
// and evented but never gates the upgrade, and every failure mode converges
// to the silences lapsing within their TTL.
//
// Lifecycle: created once the run leaves its initial health check (alerts
// stay live while the cluster is still being verified), re-extended on every
// reconcile while the phase is active - including inter-batch health checks -
// and expired down to a short tail when the run parks (Pending,
// MaintenanceWindow), is suspended, or reaches a terminal phase.
func (r *Reconciler) syncAlertSilences(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, suspended bool) {
	specs := talosUpgrade.Spec.Silences

	if r.Silencer == nil {
		// Configured silences with no operator-level Alertmanager connection
		// would otherwise be silently inert (the webhook also warns at apply).
		if len(specs) > 0 && talosUpgrade.Status.Phase.IsInFlight() {
			r.silenceEvent(talosUpgrade, corev1.EventTypeWarning, "SilencesNotConfigured",
				"spec.silences is set but the operator has no Alertmanager connection (Helm silences.*); no silences are created")
		}
		return
	}

	held := len(specs) > 0 && talosUpgrade.Status.Phase.IsActive() && !suspended
	if !held {
		r.expireSilences(ctx, talosUpgrade)
		return
	}

	ids := talosUpgrade.Status.AlertSilenceIDs
	newIDs := make([]string, len(specs))
	copy(newIDs, ids)

	// Leftover leases beyond the spec (silences removed by a spec edit):
	// release them rather than waiting out the TTL.
	for _, id := range ids[min(len(specs), len(ids)):] {
		r.expireSilence(ctx, talosUpgrade, id)
	}

	for i, spec := range specs {
		id := newIDs[i]

		// Create only once the run has left its initial health check; alerts
		// should stay live while the cluster is still being verified.
		if id == "" && !talosUpgrade.Status.Phase.IsInFlight() {
			continue
		}

		maxDuration := silenceDefaultMaxDuration
		if spec.MaxDuration != nil {
			maxDuration = spec.MaxDuration.Duration
		}
		if started := talosUpgrade.Status.StartedAt; started != nil && r.Now.Now().After(started.Add(maxDuration)) {
			// Hard cap: stop extending and let the silence lapse, so a run
			// stuck beyond maxDuration alerts again. The ID stays in status;
			// the expire path clears it once the run leaves its active phases.
			r.silenceEvent(talosUpgrade, corev1.EventTypeWarning, "SilenceMaxDurationReached",
				"Upgrade still active past silences[%d].maxDuration (%s); silence no longer extended", i, maxDuration)
			continue
		}

		matchers := make([]alerting.Matcher, 0, len(spec.Matchers))
		for _, m := range spec.Matchers {
			matchers = append(matchers, alerting.NewMatcher(m.Name, m.Value, m.MatchType))
		}

		newID, err := r.Silencer.Ensure(ctx, id, matchers, silenceTTL,
			"tuppr/"+talosUpgrade.Name,
			fmt.Sprintf("tuppr: alerts silenced while TalosUpgrade %s rolls out %s", talosUpgrade.Name, talosUpgrade.Spec.Talos.Version),
		)
		if err != nil {
			log.FromContext(ctx).V(1).Info("Failed to ensure Alertmanager silence", "index", i, "silenceID", id, "error", err)
			r.silenceEvent(talosUpgrade, corev1.EventTypeWarning, "SilenceEnsureFailed",
				"Failed to create or extend Alertmanager silence for silences[%d]: %s", i, err)
			continue
		}
		if newID != id && id == "" {
			r.silenceEvent(talosUpgrade, corev1.EventTypeNormal, "SilenceCreated",
				"Alertmanager silence %s created for silences[%d]", newID, i)
		}
		newIDs[i] = newID
	}

	// A list of only placeholders (nothing created yet) stays unpersisted.
	if !slices.ContainsFunc(newIDs, func(id string) bool { return id != "" }) {
		newIDs = nil
	}
	if !slices.Equal(ids, newIDs) {
		r.patchSilenceIDs(ctx, talosUpgrade, newIDs)
	}
}

// expireSilences releases every held silence and clears them from status. The
// IDs are cleared even when an expire call fails: the silences lapse at their
// TTL regardless, and keeping an ID would only re-trigger expiry attempts for
// a silence Alertmanager may no longer know.
func (r *Reconciler) expireSilences(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) {
	if len(talosUpgrade.Status.AlertSilenceIDs) == 0 {
		return
	}
	for _, id := range talosUpgrade.Status.AlertSilenceIDs {
		r.expireSilence(ctx, talosUpgrade, id)
	}
	r.patchSilenceIDs(ctx, talosUpgrade, nil)
}

func (r *Reconciler) expireSilence(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, id string) {
	if id == "" {
		return
	}
	if err := r.Silencer.Expire(ctx, id, silenceTail); err != nil {
		log.FromContext(ctx).V(1).Info("Failed to expire Alertmanager silence; it lapses at its TTL", "silenceID", id, "error", err)
		r.silenceEvent(talosUpgrade, corev1.EventTypeWarning, "SilenceExpireFailed",
			"Failed to expire Alertmanager silence %s (it lapses at its TTL): %s", id, err)
	} else {
		r.silenceEvent(talosUpgrade, corev1.EventTypeNormal, "SilenceExpired",
			"Alertmanager silence %s expiring in %s", id, silenceTail)
	}
}

// patchSilenceIDs persists the silence IDs (or removes them when nil) and
// mirrors them onto the in-memory status. A failed patch is only logged: the
// next reconcile re-runs the sync and converges.
func (r *Reconciler) patchSilenceIDs(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, ids []string) {
	var value any
	if len(ids) > 0 {
		value = ids
	}
	if err := r.updateStatus(ctx, talosUpgrade, map[string]any{statusAlertSilenceIDs: value}); err != nil {
		log.FromContext(ctx).V(1).Info("Failed to persist Alertmanager silence IDs", "silenceIDs", ids, "error", err)
		return
	}
	talosUpgrade.Status.AlertSilenceIDs = ids
}

func (r *Reconciler) silenceEvent(talosUpgrade *tupprv1alpha1.TalosUpgrade, eventType, reason, format string, args ...any) {
	if r.Recorder != nil {
		r.Recorder.Eventf(talosUpgrade, eventType, reason, format, args...)
	}
}
