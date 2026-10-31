package talosupgrade

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
)

// defaultClusterHealthTimeout bounds one whole gate pass. The Talos calls run
// on the reconcile context, which has no deadline, and etcd's client waits for
// a connection rather than failing, so a hung member would otherwise stall the
// single reconcile worker every TalosUpgrade shares.
const defaultClusterHealthTimeout = time.Minute

// unhealthyError is a failed gate condition. reason is stable across passes and
// becomes the status message; err carries the volatile detail (gRPC text, retry
// counts, Talos's unmet conditions) for the log, since a changing status
// message would trigger another reconcile on every write.
type unhealthyError struct {
	reason string
	err    error
}

func (e *unhealthyError) Error() string {
	if e.err == nil {
		return e.reason
	}
	return e.reason + ": " + e.err.Error()
}

func (e *unhealthyError) Unwrap() error { return e.err }

// checkClusterHealth is the built-in gate before every batch and before the
// run finishes, run whatever spec.healthChecks holds. The Talos API answering
// at the target version does not mean the node has rejoined the cluster, so
// every selected node and every control-plane node must be Ready in Kubernetes
// and in its Talos MachineStatus, and etcd must have a healthy voting member
// on each control-plane node. Nodes outside both sets are left alone so a
// stale or foreign Node cannot block the run. MachineStatus only sees the
// local node, hence the separate etcd membership check. Talos 1.13+ upgrades
// through LifecycleService, which checks neither etcd health nor upgrades in
// flight on other nodes (only the legacy MachineService.Upgrade did), so
// nothing else stops a control-plane reboot while etcd is degraded.
func (r *Reconciler) checkClusterHealth(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) error {
	timeout := r.clusterHealthTimeout
	if timeout <= 0 {
		timeout = defaultClusterHealthTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	nodes, err := r.clusterHealthNodes(ctx, talosUpgrade)
	if err != nil {
		return err
	}

	var notReady []string
	for i := range nodes {
		if !isNodeReady(&nodes[i]) {
			notReady = append(notReady, nodes[i].Name)
		}
	}
	if len(notReady) > 0 {
		return &unhealthyError{reason: "node(s) not Ready: " + strings.Join(notReady, ", ")}
	}

	var controlPlaneIPs []string
	for i := range nodes {
		node := &nodes[i]
		ip, err := nodeutil.GetNodeIP(node)
		if err != nil {
			return &unhealthyError{reason: fmt.Sprintf("node %s has no usable IP", node.Name), err: err}
		}
		if err := r.TalosClient.CheckMachineReady(ctx, ip); err != nil {
			return &unhealthyError{reason: fmt.Sprintf("node %s: Talos machine not ready", node.Name), err: err}
		}
		if _, isCP := node.Labels[controlPlaneLabel]; isCP {
			controlPlaneIPs = append(controlPlaneIPs, ip)
		}
	}

	if err := r.TalosClient.CheckEtcdHealthy(ctx, controlPlaneIPs); err != nil {
		return &unhealthyError{reason: "etcd not healthy", err: err}
	}
	return nil
}

// waitForClusterHealth runs checkClusterHealth and, while it fails, parks the
// run in HealthChecking. Returns done=true when the caller should return the
// result.
func (r *Reconciler) waitForClusterHealth(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, bool) {
	logger := log.FromContext(ctx)

	err := r.checkClusterHealth(ctx, talosUpgrade)
	if err == nil {
		return ctrl.Result{}, false
	}

	reason := err.Error()
	if unhealthy, ok := errors.AsType[*unhealthyError](err); ok {
		reason = unhealthy.reason
	}
	logger.Info("Waiting for cluster health", "reason", reason, "error", err.Error())
	if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseHealthChecking, "Waiting for cluster health: "+reason); err != nil {
		logger.Error(err, "Failed to update phase for cluster health")
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, true
}

// clusterHealthNodes returns the nodes selected by the TalosUpgrade plus every
// control-plane node, deduplicated and sorted by name.
func (r *Reconciler) clusterHealthNodes(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) ([]corev1.Node, error) {
	selected, err := r.getSortedNodes(ctx, talosUpgrade.Spec.NodeSelector)
	if err != nil {
		return nil, err
	}
	controlPlane := &corev1.NodeList{}
	if err := r.List(ctx, controlPlane, client.HasLabels{controlPlaneLabel}); err != nil {
		return nil, fmt.Errorf("failed to list control-plane nodes: %w", err)
	}

	nodes := slices.Concat(selected, controlPlane.Items)
	slices.SortFunc(nodes, func(a, b corev1.Node) int {
		return strings.Compare(a.Name, b.Name)
	})
	return slices.CompactFunc(nodes, func(a, b corev1.Node) bool {
		return a.Name == b.Name
	}), nil
}
