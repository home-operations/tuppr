package metrics

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabel "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

// Talos sets Node.status.nodeInfo.osImage to e.g. "Talos (v1.7.5)".
var talosVersionRegexp = regexp.MustCompile(`v\d+\.\d+\.\d+[^\s)]*`)

const defaultTimezone = "UTC"

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
	nodes, nodesOK := r.refreshNodes(ctx, logger)
	talosList, k8sList, upgradesOK := r.refreshUpgrades(ctx, logger)
	if upgradesOK {
		r.refreshMaintenanceWindows(talosList, k8sList)
	}
	if nodesOK && upgradesOK {
		r.refreshNodeTargets(logger, nodes, talosList, k8sList)
	}
}

func (r *InventoryRefresher) refreshNodes(ctx context.Context, logger logr.Logger) ([]corev1.Node, bool) {
	var nodes corev1.NodeList
	if err := r.Client.List(ctx, &nodes); err != nil {
		logger.Error(err, "failed to list nodes for inventory metrics")
		return nil, false
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
	return nodes.Items, true
}

func (r *InventoryRefresher) refreshUpgrades(ctx context.Context, logger logr.Logger) (*tupprv1alpha1.TalosUpgradeList, *tupprv1alpha1.KubernetesUpgradeList, bool) {
	ok := true

	var talos tupprv1alpha1.TalosUpgradeList
	if err := r.Client.List(ctx, &talos); err != nil {
		logger.Error(err, "failed to list TalosUpgrades for inventory metrics")
		ok = false
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
		ok = false
	} else {
		byPhase := make(map[string]int, len(k8s.Items))
		for i := range k8s.Items {
			byPhase[string(k8s.Items[i].Status.Phase)]++
		}
		r.Reporter.RecordUpgradesByPhase(UpgradeTypeKubernetes, byPhase)
	}

	return &talos, &k8s, ok
}

func (r *InventoryRefresher) refreshMaintenanceWindows(talos *tupprv1alpha1.TalosUpgradeList, k8s *tupprv1alpha1.KubernetesUpgradeList) {
	var windows []MaintenanceWindowSnapshot

	for i := range talos.Items {
		tu := &talos.Items[i]
		if tu.Status.Phase.IsTerminal() || tu.Spec.Maintenance == nil {
			continue
		}
		windows = appendWindows(windows, UpgradeTypeTalos, tu.Name, tu.Spec.Maintenance.Windows)
	}

	for i := range k8s.Items {
		ku := &k8s.Items[i]
		if ku.Status.Phase.IsTerminal() || ku.Spec.Maintenance == nil {
			continue
		}
		windows = appendWindows(windows, UpgradeTypeKubernetes, ku.Name, ku.Spec.Maintenance.Windows)
	}

	r.Reporter.RecordMaintenanceWindows(windows)
}

func appendWindows(out []MaintenanceWindowSnapshot, upgradeType, name string, windows []tupprv1alpha1.WindowSpec) []MaintenanceWindowSnapshot {
	for i, w := range windows {
		tz := w.Timezone
		if tz == "" {
			tz = defaultTimezone
		}
		out = append(out, MaintenanceWindowSnapshot{
			UpgradeType: upgradeType,
			Name:        name,
			Index:       strconv.Itoa(i),
			Start:       w.Start,
			Duration:    w.Duration.Duration.String(),
			Timezone:    tz,
		})
	}
	return out
}

func (r *InventoryRefresher) refreshNodeTargets(logger logr.Logger, nodes []corev1.Node, talos *tupprv1alpha1.TalosUpgradeList, k8s *tupprv1alpha1.KubernetesUpgradeList) {
	var targets []NodeTargetSnapshot

	for i := range talos.Items {
		tu := &talos.Items[i]
		if tu.Status.Phase.IsTerminal() {
			continue
		}
		version := tu.Spec.Talos.Version
		if version == "" {
			continue
		}
		selector, err := selectorFor(tu.Spec.NodeSelector)
		if err != nil {
			logger.Error(err, "skipping TalosUpgrade for target metric", "name", tu.Name)
			continue
		}
		for j := range nodes {
			n := &nodes[j]
			if !selector.Matches(k8slabel.Set(n.Labels)) {
				continue
			}
			targets = append(targets, NodeTargetSnapshot{
				Node:        n.Name,
				Role:        nodeRole(n),
				Kind:        UpgradeTypeTalos,
				Version:     version,
				UpgradeName: tu.Name,
			})
		}
	}

	for i := range k8s.Items {
		ku := &k8s.Items[i]
		if ku.Status.Phase.IsTerminal() {
			continue
		}
		version := ku.Spec.Kubernetes.Version
		if version == "" {
			continue
		}
		for j := range nodes {
			n := &nodes[j]
			targets = append(targets, NodeTargetSnapshot{
				Node:        n.Name,
				Role:        nodeRole(n),
				Kind:        UpgradeTypeKubernetes,
				Version:     version,
				UpgradeName: ku.Name,
			})
		}
	}

	r.Reporter.RecordNodeTargets(targets)
}

func selectorFor(ls *metav1.LabelSelector) (k8slabel.Selector, error) {
	if ls == nil {
		return k8slabel.Everything(), nil
	}
	return metav1.LabelSelectorAsSelector(ls)
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
