package talosupgrade

import (
	"context"
	"fmt"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// silenceSyncTimeout bounds ALL Alertmanager calls of one sync pass, so a
	// hung (not refusing) Alertmanager can't stall the reconcile loop for
	// N entries x the per-request timeout. Status patches use the parent
	// context and are unaffected.
	silenceSyncTimeout = 15 * time.Second
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
			r.silenceWarnOnce(talosUpgrade, "SilencesNotConfigured",
				"spec.silences is set but the operator has no Alertmanager connection (Helm silences.*); no silences are created")
		}
		// IDs left over from a previous configuration are stale: the silences
		// lapsed at their TTL long ago and nothing can expire them here.
		if len(talosUpgrade.Status.AlertSilenceIDs) > 0 {
			r.patchSilenceIDs(ctx, talosUpgrade, nil)
		}
		return
	}

	amCtx, cancel := context.WithTimeout(ctx, silenceSyncTimeout)
	defer cancel()

	held := len(specs) > 0 && talosUpgrade.Status.Phase.IsActive() && !suspended
	if !held {
		r.expireSilences(ctx, amCtx, talosUpgrade)
		return
	}

	ids := talosUpgrade.Status.AlertSilenceIDs
	newIDs := make([]string, len(specs))
	copy(newIDs, ids)

	// Leftover leases beyond the spec (silences removed by a spec edit):
	// release them rather than waiting out the TTL.
	for _, id := range ids[min(len(specs), len(ids)):] {
		r.expireSilence(amCtx, talosUpgrade, id)
	}

	since := talosUpgrade.Status.AlertSilencesSince
	logger := log.FromContext(ctx)

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
		if since != nil && r.Now.Now().After(since.Add(maxDuration)) {
			// Hard cap on a continuous hold: stop extending and let the
			// silence lapse, so a run stuck beyond maxDuration alerts again.
			// The ID stays in status; the expire path clears it (and re-arms
			// the budget) once the run leaves its active phases.
			r.silenceWarnOnce(talosUpgrade, "SilenceMaxDurationReached",
				"Silences held past silences[%d].maxDuration (%s); no longer extended", i, maxDuration)
			continue
		}

		matchers := make([]alerting.Matcher, 0, len(spec.Matchers))
		for _, m := range spec.Matchers {
			matchers = append(matchers, alerting.NewMatcher(m.Name, m.Value, m.MatchType))
		}

		newID, err := r.Silencer.Ensure(amCtx, id, matchers, silenceTTL,
			"tuppr/"+talosUpgrade.Name,
			fmt.Sprintf("tuppr: alerts silenced while TalosUpgrade %s rolls out %s", talosUpgrade.Name, talosUpgrade.Spec.Talos.Version),
		)
		switch {
		case err != nil:
			logger.V(1).Info("Failed to ensure Alertmanager silence", "index", i, "silenceID", id, "error", err)
			r.silenceEvent(talosUpgrade, corev1.EventTypeWarning, "SilenceEnsureFailed",
				"Failed to create or extend Alertmanager silence for silences[%d]: %s", i, err)
		case newID == "":
			// Defensive: a v2-compatible endpoint answered 200 without a
			// silenceID. Keep the old ID so the expire path can still reach it.
			logger.V(1).Info("Alertmanager returned no silence ID; keeping the previous one", "index", i, "silenceID", id)
		case id == "":
			r.silenceEvent(talosUpgrade, corev1.EventTypeNormal, "SilenceCreated",
				"Alertmanager silence %s created for silences[%d]", newID, i)
			newIDs[i] = newID
		case newID != id:
			// Alertmanager replaced the silence (expired, deleted, or matchers
			// changed); surface the ID move so status stays auditable.
			r.silenceEvent(talosUpgrade, corev1.EventTypeNormal, "SilenceRecreated",
				"Alertmanager silence %s replaced %s for silences[%d]", newID, id, i)
			newIDs[i] = newID
		}
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
func (r *Reconciler) expireSilences(ctx, amCtx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) {
	if len(talosUpgrade.Status.AlertSilenceIDs) == 0 {
		return
	}
	for _, id := range talosUpgrade.Status.AlertSilenceIDs {
		r.expireSilence(amCtx, talosUpgrade, id)
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
// mirrors them onto the in-memory status, tracking alertSilencesSince as the
// hold's start so maxDuration budgets a continuous hold rather than run
// wall-clock. The patch pins observedGeneration to its current value so this
// side write can never satisfy handleGenerationChange's spec-edit detection
// before the reset itself lands. A failed patch is only logged: the next
// reconcile re-runs the sync and converges.
func (r *Reconciler) patchSilenceIDs(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, ids []string) {
	updates := map[string]any{
		"observedGeneration": talosUpgrade.Status.ObservedGeneration,
	}
	since := talosUpgrade.Status.AlertSilencesSince
	if len(ids) > 0 {
		updates[statusAlertSilenceIDs] = ids
		if since == nil {
			now := metav1.NewTime(r.Now.Now())
			since = &now
			updates[statusAlertSilencesSince] = now
		}
	} else {
		updates[statusAlertSilenceIDs] = nil
		updates[statusAlertSilencesSince] = nil
		since = nil
		r.silenceWarnings.Delete(silenceWarningKey(talosUpgrade, "SilenceMaxDurationReached"))
	}
	if err := r.updateStatus(ctx, talosUpgrade, updates); err != nil {
		log.FromContext(ctx).V(1).Info("Failed to persist Alertmanager silence IDs", "silenceIDs", ids, "error", err)
		return
	}
	talosUpgrade.Status.AlertSilenceIDs = ids
	talosUpgrade.Status.AlertSilencesSince = since
}

func (r *Reconciler) silenceEvent(talosUpgrade *tupprv1alpha1.TalosUpgrade, eventType, reason, format string, args ...any) {
	if r.Recorder != nil {
		r.Recorder.Eventf(talosUpgrade, eventType, reason, format, args...)
	}
}

// silenceWarnOnce emits a Warning event once per CR per condition instead of
// on every reconcile, so steady-state warnings can't drain the apiserver's
// per-object event budget and starve later, genuinely new events.
func (r *Reconciler) silenceWarnOnce(talosUpgrade *tupprv1alpha1.TalosUpgrade, reason, format string, args ...any) {
	if _, alreadyWarned := r.silenceWarnings.LoadOrStore(silenceWarningKey(talosUpgrade, reason), struct{}{}); !alreadyWarned {
		r.silenceEvent(talosUpgrade, corev1.EventTypeWarning, reason, format, args...)
	}
}

func silenceWarningKey(talosUpgrade *tupprv1alpha1.TalosUpgrade, reason string) string {
	return string(talosUpgrade.UID) + "/" + reason
}
