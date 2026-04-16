package talosupgrade

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabel "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller/coordination"
	"github.com/home-operations/tuppr/internal/controller/drain"
	"github.com/home-operations/tuppr/internal/controller/maintenance"
	"github.com/home-operations/tuppr/internal/controller/nodeutil"
	"github.com/home-operations/tuppr/internal/metrics"
)

func (r *Reconciler) processUpgrade(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"talosupgrade", talosUpgrade.Name,
		"generation", talosUpgrade.Generation,
	)

	logger.V(1).Info("Starting upgrade processing")

	if suspended, err := r.handleSuspendAnnotation(ctx, talosUpgrade); err != nil || suspended {
		return ctrl.Result{RequeueAfter: time.Minute * 30}, err
	}

	if resetRequested, err := r.handleResetAnnotation(ctx, talosUpgrade); err != nil || resetRequested {
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	if reset, err := r.handleGenerationChange(ctx, talosUpgrade); err != nil || reset {
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	if talosUpgrade.Status.Phase.IsTerminal() {
		logger.V(1).Info("Talos upgrade in terminal state, skipping", "phase", talosUpgrade.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	if !talosUpgrade.Status.Phase.IsActive() {
		if result, done, err := r.checkMaintenanceWindow(ctx, talosUpgrade); done {
			return result, err
		}
		if result, done := r.checkCoordination(ctx, talosUpgrade); done {
			return result, nil
		}
	}

	if activeJobs, activeNodes, err := r.findActiveJobs(ctx, talosUpgrade); err != nil {
		logger.Error(err, "Failed to find active jobs")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if len(activeJobs) > 0 {
		logger.V(1).Info("Found active jobs, handling batch status", "count", len(activeJobs), "nodes", activeNodes)
		return r.handleBatchJobStatus(ctx, talosUpgrade, activeJobs, activeNodes)
	}

	if len(talosUpgrade.Status.FailedNodes) > 0 {
		logger.Info("Upgrade stopped due to failed nodes",
			"failedNodes", len(talosUpgrade.Status.FailedNodes))
		message := fmt.Sprintf("Upgrade stopped due to %d failed nodes", len(talosUpgrade.Status.FailedNodes))
		if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseFailed, "", message); err != nil {
			logger.Error(err, "Failed to update phase for failed nodes")
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	return r.processNextBatch(ctx, talosUpgrade)
}

func (r *Reconciler) checkMaintenanceWindow(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, bool, error) {
	logger := log.FromContext(ctx)

	maintenanceRes, err := maintenance.CheckWindow(talosUpgrade.Spec.Maintenance, r.Now.Now())
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Second * 30}, true, err
	}
	if !maintenanceRes.Allowed {
		requeueAfter := time.Until(*maintenanceRes.NextWindowStart)
		if requeueAfter > 5*time.Minute {
			requeueAfter = 5 * time.Minute
		}
		nextTimestamp := maintenanceRes.NextWindowStart.Unix()
		r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeTalos, talosUpgrade.Name, false, &nextTimestamp)
		prevPhase := talosUpgrade.Status.Phase
		totalNodes, err := r.getTotalNodeCount(ctx)
		if err != nil {
			logger.Error(err, "Failed to get total node count for metrics")
		}
		if err := r.updateStatus(ctx, talosUpgrade, map[string]any{
			"phase":                 tupprv1alpha1.JobPhaseMaintenanceWindow,
			"currentNode":           "",
			"message":               fmt.Sprintf("Waiting for maintenance window (next: %s)", maintenanceRes.NextWindowStart.Format(time.RFC3339)),
			"nextMaintenanceWindow": metav1.NewTime(*maintenanceRes.NextWindowStart),
		}); err != nil {
			logger.Error(err, "Failed to update status for maintenance window")
		} else {
			talosUpgrade.Status.Phase = tupprv1alpha1.JobPhaseMaintenanceWindow
			r.recordPhaseTransition(talosUpgrade, prevPhase, tupprv1alpha1.JobPhaseMaintenanceWindow)
			r.MetricsReporter.RecordTalosUpgradeNodes(talosUpgrade.Name, totalNodes, len(talosUpgrade.Status.CompletedNodes), len(talosUpgrade.Status.FailedNodes))
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, true, nil
	}
	r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeTalos, talosUpgrade.Name, true, nil)
	return ctrl.Result{}, false, nil
}

func (r *Reconciler) checkCoordination(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, bool) {
	logger := log.FromContext(ctx)

	blocked, message, err := coordination.IsAnotherUpgradeActive(ctx, r.Client, talosUpgrade.Name, coordination.UpgradeTypeTalos)
	if err != nil {
		logger.Error(err, "Failed to check for other active upgrades")
		return ctrl.Result{RequeueAfter: time.Minute}, true
	}
	if blocked {
		logger.Info("Waiting for another upgrade to complete", "reason", message)
		if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhasePending, "", message); err != nil {
			logger.Error(err, "Failed to update phase for coordination wait")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 2}, true
	}
	return ctrl.Result{}, false
}

func (r *Reconciler) completeUpgrade(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	completedCount := len(talosUpgrade.Status.CompletedNodes)
	failedCount := len(talosUpgrade.Status.FailedNodes)

	var phase tupprv1alpha1.JobPhase
	var message string
	if failedCount > 0 {
		phase = tupprv1alpha1.JobPhaseFailed
		message = fmt.Sprintf("Completed with failures: %d successful, %d failed", completedCount, failedCount)
		logger.Info("Upgrade completed with failures", "completed", completedCount, "failed", failedCount)
	} else {
		phase = tupprv1alpha1.JobPhaseCompleted
		message = fmt.Sprintf("Successfully upgraded %d nodes", completedCount)
		logger.Info("Upgrade completed successfully", "nodes", completedCount)
	}

	if err := r.setPhase(ctx, talosUpgrade, phase, "", message); err != nil {
		logger.Error(err, "Failed to update completion phase")
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}
	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

func (r *Reconciler) processNextBatch(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	maintenanceRes, err := maintenance.CheckWindow(talosUpgrade.Spec.Maintenance, r.Now.Now())
	if err != nil {
		logger.Error(err, "Failed to check maintenance window")
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}
	if !maintenanceRes.Allowed {
		requeueAfter := time.Until(*maintenanceRes.NextWindowStart)
		if requeueAfter > 5*time.Minute {
			requeueAfter = 5 * time.Minute
		}
		nextTimestamp := maintenanceRes.NextWindowStart.Unix()
		r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeTalos, talosUpgrade.Name, false, &nextTimestamp)
		prevPhase := talosUpgrade.Status.Phase
		totalNodes, err := r.getTotalNodeCount(ctx)
		if err != nil {
			logger.Error(err, "Failed to get total node count for metrics")
		}
		if err := r.updateStatus(ctx, talosUpgrade, map[string]any{
			"phase":                 tupprv1alpha1.JobPhaseMaintenanceWindow,
			"currentNode":           "",
			"currentNodes":          []string{},
			"message":               fmt.Sprintf("Maintenance window closed between nodes, waiting (next: %s)", maintenanceRes.NextWindowStart.Format(time.RFC3339)),
			"nextMaintenanceWindow": metav1.NewTime(*maintenanceRes.NextWindowStart),
		}); err != nil {
			logger.Error(err, "Failed to update status for maintenance window")
		} else {
			talosUpgrade.Status.Phase = tupprv1alpha1.JobPhaseMaintenanceWindow
			r.recordPhaseTransition(talosUpgrade, prevPhase, tupprv1alpha1.JobPhaseMaintenanceWindow)
			r.MetricsReporter.RecordTalosUpgradeNodes(talosUpgrade.Name, totalNodes, len(talosUpgrade.Status.CompletedNodes), len(talosUpgrade.Status.FailedNodes))
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	r.MetricsReporter.RecordMaintenanceWindow(metrics.UpgradeTypeTalos, talosUpgrade.Name, true, nil)

	ctx = context.WithValue(ctx, metrics.ContextKeyUpgradeType, metrics.UpgradeTypeTalos)
	ctx = context.WithValue(ctx, metrics.ContextKeyUpgradeName, talosUpgrade.Name)

	if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseHealthChecking, "", "Running health checks"); err != nil {
		logger.Error(err, "Failed to update phase for health check")
	}
	if err := r.HealthChecker.CheckHealth(ctx, talosUpgrade.Spec.HealthChecks); err != nil {
		logger.Info("Waiting for health checks to pass", "error", err.Error())
		if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhaseHealthChecking, "", fmt.Sprintf("Waiting for health checks: %s", err.Error())); err != nil {
			logger.Error(err, "Failed to update phase for health check")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	parallelism := getParallelism(talosUpgrade.Spec)
	nextNodes, err := r.findNextNodes(ctx, talosUpgrade, parallelism)
	if err != nil {
		logger.Error(err, "Failed to find next nodes to upgrade")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if len(nextNodes) == 0 {
		return r.completeUpgrade(ctx, talosUpgrade)
	}

	// Build and verify images for all nodes in this batch
	type nodeImage struct {
		nodeName string
		image    string
	}
	var batch []nodeImage

	for _, nodeName := range nextNodes {
		targetImage, err := r.buildTalosUpgradeImage(ctx, talosUpgrade, nodeName)
		if err != nil {
			logger.Error(err, "Failed to determine target image", "node", nodeName)
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		logger.V(1).Info("Verifying target image availability", "node", nodeName, "image", targetImage)
		if err := r.ImageChecker.Check(ctx, targetImage); err != nil {
			logger.Info("Waiting for target image to become available", "node", nodeName, "image", targetImage, "error", err.Error())
			message := fmt.Sprintf("Waiting for image availability for node %s: %s", nodeName, err.Error())
			if err := r.setPhase(ctx, talosUpgrade, tupprv1alpha1.JobPhasePending, nodeName, message); err != nil {
				logger.Error(err, "Failed to update phase while waiting for image")
			}
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}

		batch = append(batch, nodeImage{nodeName: nodeName, image: targetImage})
	}

	logger.Info("Starting batch upgrade", "nodes", nextNodes, "batchSize", len(batch))

	// Drain all nodes in batch before creating any jobs
	if talosUpgrade.Spec.Drain != nil {
		if err := r.setPhaseWithNodes(ctx, talosUpgrade, tupprv1alpha1.JobPhaseDraining, nextNodes, fmt.Sprintf("Draining %d nodes", len(nextNodes))); err != nil {
			logger.Error(err, "Failed to update phase for draining")
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		}
		var drainedNodes []string
		for _, ni := range batch {
			logger.Info("Draining node before upgrade", "node", ni.nodeName)
			if err := r.drainNode(ctx, ni.nodeName, talosUpgrade.Spec.Drain); err != nil {
				logger.Error(err, "Failed to drain node, rolling back already-drained nodes in batch", "node", ni.nodeName)
				drainer := drain.NewDrainer(r.Client)
				for _, drained := range drainedNodes {
					if uncordonErr := drainer.UncordonNode(ctx, drained); uncordonErr != nil {
						logger.Error(uncordonErr, "Failed to uncordon node during rollback", "node", drained)
					} else {
						logger.Info("Rolled back drain for node", "node", drained)
					}
				}
				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
			drainedNodes = append(drainedNodes, ni.nodeName)
			logger.V(1).Info("Node drained successfully", "node", ni.nodeName)
		}
	}

	// Create jobs for all nodes in batch
	for _, ni := range batch {
		if _, err := r.createJob(ctx, talosUpgrade, ni.nodeName, ni.image); err != nil {
			logger.Error(err, "Failed to create upgrade job", "node", ni.nodeName)
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		if err := r.addNodeUpgradingLabel(ctx, ni.nodeName); err != nil {
			logger.Error(err, "Failed to add upgrading label to node", "node", ni.nodeName)
		}
	}

	if err := r.setPhaseWithNodes(ctx, talosUpgrade, tupprv1alpha1.JobPhaseUpgrading, nextNodes, fmt.Sprintf("Upgrading %d nodes", len(nextNodes))); err != nil {
		logger.Error(err, "Failed to update phase for batch upgrade")
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// findNextNodes returns up to `count` node names that need upgrading, sorted alphabetically.
func (r *Reconciler) findNextNodes(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, count int) ([]string, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Finding next nodes to upgrade", "talosupgrade", talosUpgrade.Name, "count", count)

	nodes, err := r.getSortedNodes(ctx, talosUpgrade.Spec.NodeSelector)
	if err != nil {
		logger.Error(err, "Failed to get nodes")
		return nil, err
	}

	crdTargetVersion := talosUpgrade.Spec.Talos.Version
	var result []string

	for i := range nodes {
		if len(result) >= count {
			break
		}

		node := &nodes[i]

		if slices.Contains(talosUpgrade.Status.CompletedNodes, node.Name) {
			continue
		}

		if slices.ContainsFunc(talosUpgrade.Status.FailedNodes, func(fn tupprv1alpha1.NodeUpgradeStatus) bool {
			return fn.NodeName == node.Name
		}) {
			continue
		}

		needsUpgrade, err := r.nodeNeedsUpgrade(ctx, node, crdTargetVersion)
		if err != nil {
			logger.Error(err, "Failed to check if node needs upgrade", "node", node.Name)
			return nil, fmt.Errorf("failed to check node %s: %w", node.Name, err)
		}

		if needsUpgrade {
			logger.V(1).Info("Node needs upgrade", "node", node.Name)
			result = append(result, node.Name)
		}
	}

	if len(result) == 0 {
		logger.V(1).Info("All nodes are up to date")
	}
	return result, nil
}

func (r *Reconciler) getSortedNodes(ctx context.Context, nodeSelector *metav1.LabelSelector) ([]corev1.Node, error) {
	var selector k8slabel.Selector
	var err error
	nodeList := &corev1.NodeList{}
	if nodeSelector != nil {
		selector, err = metav1.LabelSelectorAsSelector(nodeSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to parse nodeSelector: %w", err)
		}
	} else {
		selector = k8slabel.Everything()
	}

	listOpts := &client.ListOptions{LabelSelector: selector}
	if err := r.List(ctx, nodeList, listOpts); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	nodes := nodeList.Items
	controllerNode := r.ControllerNodeName
	slices.SortFunc(nodes, func(a, b corev1.Node) int {
		// if present, always keep controller node as last item
		if controllerNode != "" {
			if a.Name == controllerNode {
				return 1
			}
			if b.Name == controllerNode {
				return -1
			}
		}
		return strings.Compare(a.Name, b.Name)
	})

	return nodes, nil
}

func (r *Reconciler) nodeNeedsUpgrade(ctx context.Context, node *corev1.Node, crdTargetVersion string) (bool, error) {
	logger := log.FromContext(ctx)

	nodeIP, err := nodeutil.GetNodeIP(node)
	if err != nil {
		return false, fmt.Errorf("failed to get node IP for %s: %w", node.Name, err)
	}

	currentVersion, err := r.TalosClient.GetNodeVersion(ctx, nodeIP)
	if err != nil {
		return false, fmt.Errorf("failed to get current version for node %s (%s): %w", node.Name, nodeIP, err)
	}

	targetVersion := r.getTargetVersion(node, crdTargetVersion)

	if currentVersion != targetVersion {
		logger.V(1).Info("Node version mismatch detected",
			"node", node.Name,
			"current", currentVersion,
			"target", targetVersion)
		return true, nil
	}
	if targetSchematic, ok := node.Annotations[constants.SchematicAnnotation]; ok && targetSchematic != "" {
		currentImage, err := r.TalosClient.GetNodeInstallImage(ctx, nodeIP)
		if err != nil {
			logger.Error(err, "Failed to get install image to verify schematic", "node", node.Name)
			return false, err
		}

		if !strings.Contains(currentImage, targetSchematic) {
			logger.V(1).Info("Node schematic mismatch detected",
				"node", node.Name,
				"currentImage", currentImage,
				"targetSchematic", targetSchematic)
			return true, nil
		}
	}

	return false, nil
}

func (r *Reconciler) drainNode(ctx context.Context, nodeName string, drainSpec *tupprv1alpha1.DrainSpec) error {
	drainer := drain.NewDrainer(r.Client)

	// Cordon the node first
	if err := drainer.CordonNode(ctx, nodeName); err != nil {
		return fmt.Errorf("failed to cordon node %s: %w", nodeName, err)
	}

	opts := drain.DrainOptions{
		RespectPDBs: drainSpec.DisableEviction == nil || !*drainSpec.DisableEviction,
		Timeout:     10 * time.Minute,
		GracePeriod: nil,
	}

	// Drain the node
	if err := drainer.DrainNode(ctx, nodeName, opts); err != nil {
		return fmt.Errorf("failed to drain node %s: %w", nodeName, err)
	}

	return nil
}

func (r *Reconciler) buildTalosUpgradeImage(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (string, error) {
	logger := log.FromContext(ctx)

	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	targetVersion := r.getTargetVersion(node, talosUpgrade.Spec.Talos.Version)

	var imageBase string

	if schematic, ok := node.Annotations[constants.SchematicAnnotation]; ok && schematic != "" {
		imageBase = fmt.Sprintf("%s/%s", constants.DefaultFactoryURL, schematic)
		logger.V(1).Info("Using schematic override from annotation", "node", nodeName, "schematic", schematic)
	} else {
		nodeIP, err := nodeutil.GetNodeIP(node)
		if err != nil {
			return "", fmt.Errorf("failed to get node IP for %s: %w", nodeName, err)
		}

		currentImage, err := r.TalosClient.GetNodeInstallImage(ctx, nodeIP)
		if err != nil {
			return "", fmt.Errorf("failed to get install image from Talos client for %s: %w", nodeName, err)
		}

		parts := strings.Split(currentImage, ":")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid current image format for node %s: %s", nodeName, currentImage)
		}
		imageBase = parts[0]
	}

	targetImage := fmt.Sprintf("%s:%s", imageBase, targetVersion)

	logger.V(1).Info("Built target image",
		"node", nodeName,
		"targetImage", targetImage,
		"version", targetVersion)

	return targetImage, nil
}

func (r *Reconciler) getTargetVersion(node *corev1.Node, crdTargetVersion string) string {
	if v, ok := node.Annotations[constants.VersionAnnotation]; ok && v != "" {
		return v
	}
	return crdTargetVersion
}

func (r *Reconciler) verifyNodeUpgrade(ctx context.Context, talosUpgrade *tupprv1alpha1.TalosUpgrade, nodeName string) (bool, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Verifying node upgrade using Talos client", "node", nodeName)

	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return false, fmt.Errorf("failed to get node: %w", err)
	}

	nodeIP, err := nodeutil.GetNodeIP(node)
	if err != nil {
		return false, fmt.Errorf("failed to get node IP for %s: %w", nodeName, err)
	}

	targetVersion := talosUpgrade.Spec.Talos.Version

	if err := r.TalosClient.CheckNodeReady(ctx, nodeIP, nodeName); err != nil {
		if isTransientError(err) {
			logger.V(1).Info("Node not ready yet, will retry", "node", nodeName, "error", err)
			return false, nil
		}
		return false, err
	}

	currentVersion, err := r.TalosClient.GetNodeVersion(ctx, nodeIP)
	if err != nil {
		if isTransientError(err) {
			logger.V(1).Info("Node not ready yet, will retry", "node", nodeName, "error", err)
			return false, nil
		}
		return false, fmt.Errorf("failed to get current version from Talos for %s: %w", nodeName, err)
	}

	if currentVersion != targetVersion {
		return false, fmt.Errorf("node %s version mismatch: current=%s, target=%s",
			nodeName, currentVersion, targetVersion)
	}

	logger.V(1).Info("Node upgrade verification successful",
		"node", nodeName,
		"version", currentVersion)
	return true, nil
}

// isNodeReady returns true if the node has a Ready condition set to True.
func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	// Check Standard Go Context Timeouts
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check gRPC Status
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded:
			return true
		default:
			// If it's a valid gRPC error but NOT one of the above, it's likely permanent (e.g. Unauthenticated)
			// We return false here to stop falling through to string matching.
			return false
		}
	}

	// Check Network Errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
	}

	//  Check Syscall Errors (Connection issues)
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ETIMEDOUT, syscall.EPIPE:
			return true
		}
	}

	//  Fallback String Matching
	errStr := strings.ToLower(err.Error())
	transientIndicators := []string{
		"connection refused",
		"connection reset",
		"i/o timeout",
		"eof", // Often happens if server restarts mid-stream
	}

	for _, indicator := range transientIndicators {
		if strings.Contains(errStr, indicator) {
			return true
		}
	}

	return false
}
