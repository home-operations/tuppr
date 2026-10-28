package talosupgrade

import (
	"context"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/home-operations/tuppr/internal/controller/nodeutil"
)

// checkClusterHealth is the built-in gate before every batch, run whatever
// spec.healthChecks holds. The Talos API answering at the target version does
// not mean the previous batch has rejoined the cluster, so every node must be
// Ready in Kubernetes and in its Talos MachineStatus, and etcd must have a
// healthy voting member on each control-plane node. MachineStatus only sees
// the local node, hence the separate etcd membership check. Talos re-checks
// etcd itself when it receives an upgrade, but its refusal fails the Job and
// stops the run, so wait here instead.
func (r *Reconciler) checkClusterHealth(ctx context.Context) error {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}
	nodes := nodeList.Items
	slices.SortFunc(nodes, func(a, b corev1.Node) int {
		return strings.Compare(a.Name, b.Name)
	})

	var notReady []string
	for i := range nodes {
		if !isNodeReady(&nodes[i]) {
			notReady = append(notReady, nodes[i].Name)
		}
	}
	if len(notReady) > 0 {
		return fmt.Errorf("node(s) not Ready: %s", strings.Join(notReady, ", "))
	}

	var controlPlaneIPs []string
	for i := range nodes {
		node := &nodes[i]
		ip, err := nodeutil.GetNodeIP(node)
		if err != nil {
			return fmt.Errorf("failed to get node IP for %s: %w", node.Name, err)
		}
		if err := r.TalosClient.CheckMachineReady(ctx, ip); err != nil {
			return fmt.Errorf("node %s: %w", node.Name, err)
		}
		if _, isCP := node.Labels[controlPlaneLabel]; isCP {
			controlPlaneIPs = append(controlPlaneIPs, ip)
		}
	}

	if err := r.TalosClient.CheckEtcdHealthy(ctx, controlPlaneIPs); err != nil {
		return fmt.Errorf("etcd not healthy: %w", err)
	}
	return nil
}
