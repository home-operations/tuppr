package talosupgrade

import (
	"context"
	"fmt"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
	"github.com/home-operations/tuppr/internal/controller/upgradeaudit"
	"github.com/home-operations/tuppr/internal/talos"
)

// prePullTimeout bounds a single node's installer pull so a stalled registry
// can't hang the reconcile; a timed-out pull parks the run and containerd
// resumes from the already-fetched layers on the next attempt.
const prePullTimeout = 10 * time.Minute

// prePullInstallerImages pulls every pending node's resolved installer image
// into the node's system containerd store before the node is cordoned, so an
// unreachable registry or a bad schematic/tag parks the run before any
// disruption (and the upgrade's own pull becomes a skip). The image is
// resolved per node every pass: schematic matching means two nodes can need
// different installer refs, and records are keyed by the resolved ref so a
// node that becomes eligible mid-run, or whose resolution inputs change
// (annotations, machine config), is pulled before its next batch. Nodes on
// Talos < v1.13 predate the ImageService API and are skipped with an event.
// Returns done=true when the caller should return the result.
func (r *Reconciler) prePullInstallerImages(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, pendingNodes []string) (ctrl.Result, bool) {
	if !talosUpgrade.Spec.PrePullEnabled() {
		return ctrl.Result{}, false
	}

	var toPull []tupprv1alpha1.PrePulledNode
	for _, nodeName := range pendingNodes {
		targetImage, err := r.buildTalosUpgradeImage(ctx, talosUpgrade, nodeName)
		if err != nil {
			return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonBuildTargetImage, fmt.Sprintf("build target image for node %s", nodeName), time.Minute, err), true
		}
		if !slices.Contains(talosUpgrade.Status.PrePulledNodes, tupprv1alpha1.PrePulledNode{NodeName: nodeName, Image: targetImage}) {
			toPull = append(toPull, tupprv1alpha1.PrePulledNode{NodeName: nodeName, Image: targetImage})
		}
	}
	if len(toPull) == 0 {
		return ctrl.Result{}, false
	}

	logger := log.FromContext(ctx)

	message := fmt.Sprintf("Pre-pulling installer image on %d node(s)", len(toPull))
	logger.Info("Starting installer image pre-pull", "count", len(toPull))
	if err := r.setPendingWithReason(ctx, talosUpgrade, upgradeaudit.ReasonPrePulling, message); err != nil {
		logger.Error(err, "Failed to update status for pre-pull")
	}

	records := slices.Clone(talosUpgrade.Status.PrePulledNodes)
	for _, entry := range toPull {
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: entry.NodeName}, node); err != nil {
			r.recordPrePulledNodes(ctx, talosUpgrade, records)
			return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonPrePullFailed, fmt.Sprintf("get node %s for pre-pull", entry.NodeName), time.Minute, err), true
		}
		nodeIP, err := nodeutil.GetNodeIP(node)
		if err != nil {
			r.recordPrePulledNodes(ctx, talosUpgrade, records)
			return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonPrePullFailed, fmt.Sprintf("get node IP for %s", entry.NodeName), time.Minute, err), true
		}

		logger.V(1).Info("Pre-pulling installer image", "node", entry.NodeName, "image", entry.Image)
		pullCtx, cancel := context.WithTimeout(ctx, prePullTimeout)
		err = r.TalosClient.PullImage(pullCtx, nodeIP, entry.Image)
		cancel()

		switch {
		case err == nil:
			logger.V(1).Info("Pre-pulled installer image", "node", entry.NodeName, "image", entry.Image)
			records = upsertPrePulledNode(records, entry)
		case talos.IsUnimplementedError(err):
			// Talos < v1.13 has no ImageService; the node upgrades as before,
			// just without the pre-pull protection. Recorded as handled so the
			// skip isn't re-attempted (and re-evented) every reconcile.
			logger.Info("Node does not support image pre-pull (Talos < v1.13), skipping", "node", entry.NodeName)
			if r.Recorder != nil {
				r.Recorder.Eventf(talosUpgrade, corev1.EventTypeNormal, "PrePullSkipped",
					"Node %s does not support the image pre-pull API (Talos < v1.13)", entry.NodeName)
			}
			records = upsertPrePulledNode(records, entry)
		default:
			logger.Error(err, "Failed to pre-pull installer image", "node", entry.NodeName, "image", entry.Image)
			if r.Recorder != nil {
				r.Recorder.Eventf(talosUpgrade, corev1.EventTypeWarning, "PrePullFailed",
					"Failed to pre-pull image %s on node %s: %v", entry.Image, entry.NodeName, err)
			}
			// Progress up to the failing node is persisted so the retry
			// resumes there instead of re-walking the fleet.
			r.recordPrePulledNodes(ctx, talosUpgrade, records)
			message := fmt.Sprintf("Pre-pull failed on node %s for image %s: %s", entry.NodeName, entry.Image, err.Error())
			if err := r.setPendingWithReason(ctx, talosUpgrade, upgradeaudit.ReasonPrePullFailed, message); err != nil {
				logger.Error(err, "Failed to update phase after pre-pull failure")
			}
			return ctrl.Result{RequeueAfter: time.Minute}, true
		}
	}

	r.recordPrePulledNodes(ctx, talosUpgrade, records)
	logger.Info("Installer image pre-pull complete", "count", len(toPull))
	return ctrl.Result{}, false
}

// upsertPrePulledNode replaces the node's record (its resolved ref changed)
// or appends a new one.
func upsertPrePulledNode(records []tupprv1alpha1.PrePulledNode, entry tupprv1alpha1.PrePulledNode) []tupprv1alpha1.PrePulledNode {
	for i := range records {
		if records[i].NodeName == entry.NodeName {
			records[i].Image = entry.Image
			return records
		}
	}
	return append(records, entry)
}

// recordPrePulledNodes persists the pre-pulled node records. Best-effort: on
// a failed write the next pass redoes the delta and the pulls are skips.
func (r *Reconciler) recordPrePulledNodes(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, records []tupprv1alpha1.PrePulledNode) {
	if slices.Equal(talosUpgrade.Status.PrePulledNodes, records) {
		return
	}
	talosUpgrade.Status.PrePulledNodes = records
	if err := r.updateStatus(ctx, talosUpgrade, map[string]any{statusPrePulledNodes: records}); err != nil {
		log.FromContext(ctx).Error(err, "Failed to record pre-pulled nodes")
	}
}
