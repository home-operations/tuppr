package metrics

import (
	"context"
	"regexp"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

// Talos sets Node.status.nodeInfo.osImage to e.g. "Talos (v1.7.5)".
var talosVersionRegexp = regexp.MustCompile(`v\d+\.\d+\.\d+[^\s)]*`)

type InventoryRefresher struct {
	Client   client.Client
	Cache    cache.Cache
	Reporter *Reporter
	Interval time.Duration
}

func (r *InventoryRefresher) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("inventory")

	interval := r.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	// Block until the informer cache is ready; otherwise the first List races
	// the sync and silently zeroes the inventory metrics.
	if r.Cache != nil && !r.Cache.WaitForCacheSync(ctx) {
		return nil
	}

	r.refresh(ctx, logger)

	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			r.refresh(ctx, logger)
		}
	}
}

// Leader-only: running on every replica double-counts sum-based aggregations.
func (r *InventoryRefresher) NeedLeaderElection() bool {
	return true
}

func (r *InventoryRefresher) refresh(ctx context.Context, logger logr.Logger) {
	r.refreshNodes(ctx, logger)
	r.refreshUpgrades(ctx, logger)
}

func (r *InventoryRefresher) refreshNodes(ctx context.Context, logger logr.Logger) {
	var nodes corev1.NodeList
	if err := r.Client.List(ctx, &nodes); err != nil {
		logger.Error(err, "failed to list nodes for inventory metrics")
		return
	}

	byRole := map[string]int{NodeRoleControlPlane: 0, NodeRoleWorker: 0}
	snapshot := make([]NodeInfoSnapshot, 0, len(nodes.Items))

	for i := range nodes.Items {
		n := &nodes.Items[i]
		role := nodeRole(n)
		byRole[role]++
		snapshot = append(snapshot, NodeInfoSnapshot{
			Node:              n.Name,
			Role:              role,
			TalosVersion:      parseTalosVersion(n.Status.NodeInfo.OSImage),
			KubernetesVersion: n.Status.NodeInfo.KubeletVersion,
		})
	}

	r.Reporter.RecordManagedNodes(byRole)
	r.Reporter.RecordNodeInfo(snapshot)
}

func (r *InventoryRefresher) refreshUpgrades(ctx context.Context, logger logr.Logger) {
	var talos tupprv1alpha1.TalosUpgradeList
	if err := r.Client.List(ctx, &talos); err != nil {
		logger.Error(err, "failed to list TalosUpgrades for inventory metrics")
	} else {
		byPhase := make(map[string]int, len(talos.Items))
		for i := range talos.Items {
			byPhase[string(talos.Items[i].Status.Phase)]++
		}
		r.Reporter.RecordUpgradesByPhase(UpgradeTypeTalos, byPhase)
	}

	var k8s tupprv1alpha1.KubernetesUpgradeList
	if err := r.Client.List(ctx, &k8s); err != nil {
		logger.Error(err, "failed to list KubernetesUpgrades for inventory metrics")
	} else {
		byPhase := make(map[string]int, len(k8s.Items))
		for i := range k8s.Items {
			byPhase[string(k8s.Items[i].Status.Phase)]++
		}
		r.Reporter.RecordUpgradesByPhase(UpgradeTypeKubernetes, byPhase)
	}
}

func nodeRole(n *corev1.Node) string {
	if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; ok {
		return NodeRoleControlPlane
	}
	if _, ok := n.Labels["node-role.kubernetes.io/master"]; ok {
		return NodeRoleControlPlane
	}
	return NodeRoleWorker
}

func parseTalosVersion(osImage string) string {
	return talosVersionRegexp.FindString(osImage)
}
