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
// disruption (and the upgrade's own pull becomes a skip). Pulled nodes are
// recorded in status so pulls aren't repeated every reconcile; a node that
// becomes eligible mid-run is pre-pulled before its first batch. The image is
// resolved per node: schematic matching means two nodes can need different
// installer refs. Nodes on Talos < v1.13 predate the ImageService API and are
// skipped with an event. Returns done=true when the caller should return the
// result.
func (r *Reconciler) prePullInstallerImages(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, pendingNodes []string) (ctrl.Result, bool) {
	if !talosUpgrade.Spec.PrePullEnabled() {
		return ctrl.Result{}, false
	}

	var toPull []string
	for _, nodeName := range pendingNodes {
		if !slices.Contains(talosUpgrade.Status.PrePulledNodes, nodeName) {
			toPull = append(toPull, nodeName)
		}
	}
	if len(toPull) == 0 {
		return ctrl.Result{}, false
	}

	logger := log.FromContext(ctx)

	message := fmt.Sprintf("Pre-pulling installer image on %d node(s)", len(toPull))
	logger.Info("Starting installer image pre-pull", "nodes", toPull)
	if err := r.setPendingWithReason(ctx, talosUpgrade, upgradeaudit.ReasonPrePulling, message); err != nil {
		logger.Error(err, "Failed to update status for pre-pull")
	}

	pulled := slices.Clone(talosUpgrade.Status.PrePulledNodes)
	for _, nodeName := range toPull {
		targetImage, err := r.buildTalosUpgradeImage(ctx, talosUpgrade, nodeName)
		if err != nil {
			r.recordPrePulledNodes(ctx, talosUpgrade, pulled)
			return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonBuildTargetImage, fmt.Sprintf("build target image for node %s", nodeName), time.Minute, err), true
		}

		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			r.recordPrePulledNodes(ctx, talosUpgrade, pulled)
			return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonPrePullFailed, fmt.Sprintf("get node %s for pre-pull", nodeName), time.Minute, err), true
		}
		nodeIP, err := nodeutil.GetNodeIP(node)
		if err != nil {
			r.recordPrePulledNodes(ctx, talosUpgrade, pulled)
			return r.reportReconcileError(ctx, talosUpgrade, upgradeaudit.ReasonPrePullFailed, fmt.Sprintf("get node IP for %s", nodeName), time.Minute, err), true
		}

		logger.V(1).Info("Pre-pulling installer image", "node", nodeName, "image", targetImage)
		pullCtx, cancel := context.WithTimeout(ctx, prePullTimeout)
		err = r.TalosClient.PullImage(pullCtx, nodeIP, targetImage)
		cancel()

		switch {
		case err == nil:
			logger.V(1).Info("Pre-pulled installer image", "node", nodeName, "image", targetImage)
			pulled = append(pulled, nodeName)
		case talos.IsUnimplementedError(err):
			// Talos < v1.13 has no ImageService; the node upgrades as before,
			// just without the pre-pull protection. Recorded as handled so the
			// skip isn't re-attempted (and re-evented) every reconcile.
			logger.Info("Node does not support image pre-pull (Talos < v1.13), skipping", "node", nodeName)
			if r.Recorder != nil {
				r.Recorder.Eventf(talosUpgrade, corev1.EventTypeNormal, "PrePullSkipped",
					"Node %s does not support the image pre-pull API (Talos < v1.13)", nodeName)
			}
			pulled = append(pulled, nodeName)
		default:
			logger.Error(err, "Failed to pre-pull installer image", "node", nodeName, "image", targetImage)
			if r.Recorder != nil {
				r.Recorder.Eventf(talosUpgrade, corev1.EventTypeWarning, "PrePullFailed",
					"Failed to pre-pull image %s on node %s: %v", targetImage, nodeName, err)
			}
			// Progress up to the failing node is persisted so the retry
			// resumes there instead of re-walking the fleet.
			r.recordPrePulledNodes(ctx, talosUpgrade, pulled)
			message := fmt.Sprintf("Pre-pull failed on node %s for image %s: %s", nodeName, targetImage, err.Error())
			if err := r.setPendingWithReason(ctx, talosUpgrade, upgradeaudit.ReasonPrePullFailed, message); err != nil {
				logger.Error(err, "Failed to update phase after pre-pull failure")
			}
			return ctrl.Result{RequeueAfter: time.Minute}, true
		}
	}

	r.recordPrePulledNodes(ctx, talosUpgrade, pulled)
	logger.Info("Installer image pre-pull complete", "nodes", len(toPull))
	return ctrl.Result{}, false
}

// recordPrePulledNodes persists the pre-pulled node list. Best-effort: on a
// failed write the next pass redoes the delta and the pulls are skips.
func (r *Reconciler) recordPrePulledNodes(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodes []string) {
	if slices.Equal(talosUpgrade.Status.PrePulledNodes, nodes) {
		return
	}
	talosUpgrade.Status.PrePulledNodes = nodes
	if err := r.updateStatus(ctx, talosUpgrade, map[string]any{statusPrePulledNodes: nodes}); err != nil {
		log.FromContext(ctx).Error(err, "Failed to record pre-pulled nodes")
	}
}
