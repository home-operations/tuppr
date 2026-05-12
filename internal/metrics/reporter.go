package metrics

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

const (
	UpgradeTypeTalos      = "talos"
	UpgradeTypeKubernetes = "kubernetes"
)

const (
	ResultSuccess = "success"
	ResultFailure = "failure"
)

const (
	NodeRoleControlPlane = "control-plane"
	NodeRoleWorker       = "worker"
)

const (
	labelName        = "name"
	labelUpgradeName = "upgrade_name"
	labelPhase       = "phase"
	labelUpgradeType = "upgrade_type"
	labelResult      = "result"
)

var (
	talosPhases = []string{
		string(tupprv1alpha1.JobPhasePending),
		string(tupprv1alpha1.JobPhaseHealthChecking),
		string(tupprv1alpha1.JobPhasePreHook),
		string(tupprv1alpha1.JobPhaseDraining),
		string(tupprv1alpha1.JobPhaseUpgrading),
		string(tupprv1alpha1.JobPhaseRebooting),
		string(tupprv1alpha1.JobPhasePostHook),
		string(tupprv1alpha1.JobPhaseMaintenanceWindow),
		string(tupprv1alpha1.JobPhaseCompleted),
		string(tupprv1alpha1.JobPhaseFailed),
	}
	kubernetesPhases = []string{
		string(tupprv1alpha1.JobPhasePending),
		string(tupprv1alpha1.JobPhaseHealthChecking),
		string(tupprv1alpha1.JobPhaseUpgrading),
		string(tupprv1alpha1.JobPhaseMaintenanceWindow),
		string(tupprv1alpha1.JobPhaseCompleted),
		string(tupprv1alpha1.JobPhaseFailed),
	}
)

type ContextKey string

const (
	ContextKeyUpgradeType ContextKey = "upgradeType"
	ContextKeyUpgradeName ContextKey = "upgradeName"
)

var (
	talosUpgradePhaseGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_talos_upgrade_phase",
			Help: "Current phase of a Talos upgrade. One phase label is 1 at a time; node_name is populated for node-scoped phases (Draining, Upgrading, Rebooting) and empty otherwise.",
		},
		[]string{labelName, labelPhase, "node_name"},
	)

	talosUpgradeNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_talos_upgrade_nodes",
			Help: "Total number of nodes in Talos upgrade",
		},
		[]string{labelName},
	)

	talosUpgradeNodesCompleted = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_talos_upgrade_nodes_completed",
			Help: "Number of nodes completed in Talos upgrade",
		},
		[]string{labelName},
	)

	talosUpgradeNodesFailed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_talos_upgrade_nodes_failed",
			Help: "Number of nodes failed in Talos upgrade",
		},
		[]string{labelName},
	)

	talosUpgradeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tuppr_talos_upgrade_duration_seconds",
			Help:    "Time taken for Talos upgrade phases",
			Buckets: []float64{30, 60, 300, 600, 1200, 1800, 3600, 7200},
		},
		[]string{labelName, labelPhase},
	)

	kubernetesUpgradePhaseGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_kubernetes_upgrade_phase",
			Help: "Current phase of a Kubernetes upgrade, labelled by phase name. Only one phase label is active (value=1) at a time. Possible phases: Pending, HealthChecking, Upgrading, MaintenanceWindow, Completed, Failed.",
		},
		[]string{labelName, labelPhase},
	)

	kubernetesUpgradeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tuppr_kubernetes_upgrade_duration_seconds",
			Help:    "Time taken for Kubernetes upgrade phases",
			Buckets: []float64{30, 60, 300, 600, 1200, 1800, 3600},
		},
		[]string{labelName, labelPhase},
	)

	healthCheckDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tuppr_health_check_duration_seconds",
			Help:    "Time taken for health checks to pass",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{labelUpgradeType, labelUpgradeName},
	)

	healthCheckFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tuppr_health_check_failures_total",
			Help: "Total number of health check failures",
		},
		[]string{labelUpgradeType, labelUpgradeName, "check_index"},
	)

	upgradeJobsActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_upgrade_jobs_active",
			Help: "Number of active upgrade jobs",
		},
		[]string{labelUpgradeType},
	)

	upgradeJobDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tuppr_upgrade_job_duration_seconds",
			Help:    "Time taken for upgrade jobs to complete",
			Buckets: []float64{60, 300, 600, 1200, 1800, 3600, 7200},
		},
		[]string{labelUpgradeType, labelUpgradeName, "node_name", labelResult},
	)

	hookExecutionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tuppr_hook_executions_total",
			Help: "Total hook executions",
		},
		[]string{labelUpgradeName, "hook_phase", "hook_name", labelResult},
	)

	maintenanceWindowActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_maintenance_window_active",
			Help: "Whether upgrade is currently inside a maintenance window (1=inside, 0=outside)",
		},
		[]string{labelUpgradeType, labelName},
	)

	maintenanceWindowNextOpenTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_maintenance_window_next_open_timestamp",
			Help: "Unix timestamp of the next maintenance window start",
		},
		[]string{labelUpgradeType, labelName},
	)

	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_build_info",
			Help: "Build info for the tuppr operator. Always 1; useful as a stat panel and a label-join source.",
		},
		[]string{"version", "commit", "go_version"},
	)

	upgradesByPhase = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_upgrades",
			Help: "Number of upgrade CRs currently in each phase, refreshed periodically by an inventory loop.",
		},
		[]string{labelUpgradeType, labelPhase},
	)

	managedNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_managed_nodes",
			Help: "Number of cluster nodes the operator can see, by role.",
		},
		[]string{"role"},
	)

	nodeInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_node_info",
			Help: "Per-node version inventory. Always 1; talos_version is parsed from Node.status.nodeInfo.osImage and may be empty for non-Talos nodes.",
		},
		[]string{"node", "role", "talos_version", "kubernetes_version"},
	)

	upgradesCompletedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tuppr_upgrades_completed_total",
			Help: "Total number of upgrades that reached a terminal phase, by type and result.",
		},
		[]string{labelUpgradeType, labelResult},
	)

	upgradeLastCompletionTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_upgrade_last_completion_timestamp_seconds",
			Help: "Unix timestamp of the last completion for an upgrade, by type, name, and result.",
		},
		[]string{labelUpgradeType, labelName, labelResult},
	)

	upgradeProgressing = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_upgrade_progressing",
			Help: "1 when the Progressing condition is True, 0 otherwise. The reason label exposes the sub-state (e.g. WaitingForImage).",
		},
		[]string{labelUpgradeType, labelName, "reason"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		talosUpgradePhaseGauge,
		talosUpgradeNodes,
		talosUpgradeNodesCompleted,
		talosUpgradeNodesFailed,
		talosUpgradeDuration,
		kubernetesUpgradePhaseGauge,
		kubernetesUpgradeDuration,
		healthCheckDuration,
		healthCheckFailuresTotal,
		upgradeJobsActive,
		upgradeJobDuration,
		hookExecutionsTotal,
		maintenanceWindowActive,
		maintenanceWindowNextOpenTimestamp,
		buildInfo,
		upgradesByPhase,
		managedNodes,
		nodeInfo,
		upgradesCompletedTotal,
		upgradeLastCompletionTimestamp,
		upgradeProgressing,
	)
}

type talosPhaseNode struct {
	phase string
	node  string
}

type progressingKey struct {
	upgradeType string
	name        string
}

type Reporter struct {
	mu               sync.RWMutex
	startTimes       map[string]*prometheus.Timer
	jobStartTimes    map[string]time.Time
	talosUpgradePrev map[string]talosPhaseNode
	progressingPrev  map[progressingKey]string
}

func NewReporter() *Reporter {
	return &Reporter{
		startTimes:       make(map[string]*prometheus.Timer),
		jobStartTimes:    make(map[string]time.Time),
		talosUpgradePrev: make(map[string]talosPhaseNode),
		progressingPrev:  make(map[progressingKey]string),
	}
}

func (m *Reporter) Initialize(upgradeName, upgradeType string) {
	switch upgradeType {
	case UpgradeTypeTalos:
		for _, phase := range talosPhases {
			talosUpgradePhaseGauge.WithLabelValues(upgradeName, phase, "").Set(0)
		}
		talosUpgradeNodes.WithLabelValues(upgradeName).Set(0)
		talosUpgradeNodesCompleted.WithLabelValues(upgradeName).Set(0)
		talosUpgradeNodesFailed.WithLabelValues(upgradeName).Set(0)
	case UpgradeTypeKubernetes:
		for _, phase := range kubernetesPhases {
			kubernetesUpgradePhaseGauge.WithLabelValues(upgradeName, phase).Set(0)
		}
	}
	upgradeJobsActive.WithLabelValues(upgradeType).Set(0)
	maintenanceWindowActive.WithLabelValues(upgradeType, upgradeName).Set(0)
}

func (m *Reporter) RecordTalosUpgradePhase(name, phase, nodeName string) {
	m.mu.Lock()
	prev := m.talosUpgradePrev[name]
	m.talosUpgradePrev[name] = talosPhaseNode{phase: phase, node: nodeName}
	m.mu.Unlock()

	if prev.phase != "" && (prev.phase != phase || prev.node != nodeName) {
		talosUpgradePhaseGauge.DeleteLabelValues(name, prev.phase, prev.node)
	}

	for _, p := range talosPhases {
		if p == phase {
			talosUpgradePhaseGauge.WithLabelValues(name, p, nodeName).Set(1.0)
		} else {
			talosUpgradePhaseGauge.WithLabelValues(name, p, "").Set(0.0)
		}
	}
}

func (m *Reporter) RecordTalosUpgradeNodes(name string, total, completed, failed int) {
	talosUpgradeNodes.WithLabelValues(name).Set(float64(total))
	talosUpgradeNodesCompleted.WithLabelValues(name).Set(float64(completed))
	talosUpgradeNodesFailed.WithLabelValues(name).Set(float64(failed))
}

func (m *Reporter) RecordKubernetesUpgradePhase(name, phase string) {
	for _, p := range kubernetesPhases {
		val := 0.0
		if p == phase {
			val = 1.0
		}
		kubernetesUpgradePhaseGauge.WithLabelValues(name, p).Set(val)
	}
}

// RecordProgressing mirrors the Progressing condition. Deletes the previous
// reason's series so only one reason is exposed per (type,name) at a time.
func (m *Reporter) RecordProgressing(upgradeType, name, reason string, progressing bool) {
	if reason == "" {
		reason = "Unknown"
	}
	key := progressingKey{upgradeType: upgradeType, name: name}

	m.mu.Lock()
	prev, hadPrev := m.progressingPrev[key]
	m.progressingPrev[key] = reason
	m.mu.Unlock()

	if hadPrev && prev != reason {
		upgradeProgressing.DeleteLabelValues(upgradeType, name, prev)
	}
	val := 0.0
	if progressing {
		val = 1.0
	}
	upgradeProgressing.WithLabelValues(upgradeType, name, reason).Set(val)
}

func (m *Reporter) StartPhaseTiming(upgradeType, name, phase string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := upgradeType + ":" + name + ":" + phase

	switch upgradeType {
	case UpgradeTypeTalos:
		m.startTimes[key] = prometheus.NewTimer(talosUpgradeDuration.WithLabelValues(name, phase))
	case UpgradeTypeKubernetes:
		m.startTimes[key] = prometheus.NewTimer(kubernetesUpgradeDuration.WithLabelValues(name, phase))
	}
}

func (m *Reporter) EndPhaseTiming(upgradeType, name, phase string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := upgradeType + ":" + name + ":" + phase
	if timer, exists := m.startTimes[key]; exists {
		timer.ObserveDuration()
		delete(m.startTimes, key)
	}
}

func (m *Reporter) RecordHealthCheckDuration(upgradeType, upgradeName string, duration float64) {
	healthCheckDuration.WithLabelValues(upgradeType, upgradeName).Observe(duration)
}

func (m *Reporter) RecordHealthCheckFailure(upgradeType, upgradeName string, checkIndex int) {
	healthCheckFailuresTotal.WithLabelValues(upgradeType, upgradeName, strconv.Itoa(checkIndex)).Inc()
}

func (m *Reporter) RecordActiveJobs(upgradeType string, count int) {
	upgradeJobsActive.WithLabelValues(upgradeType).Set(float64(count))
}

func (m *Reporter) RecordJobDuration(upgradeType, upgradeName, nodeName, result string, duration float64) {
	upgradeJobDuration.WithLabelValues(upgradeType, upgradeName, nodeName, result).Observe(duration)
}

func (m *Reporter) StartJobTiming(upgradeType, upgradeName, nodeName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobStartTimes[upgradeType+":"+upgradeName+":"+nodeName] = time.Now()
}

func (m *Reporter) EndJobTiming(upgradeType, upgradeName, nodeName, result string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := upgradeType + ":" + upgradeName + ":" + nodeName
	if start, ok := m.jobStartTimes[key]; ok {
		upgradeJobDuration.WithLabelValues(upgradeType, upgradeName, nodeName, result).Observe(time.Since(start).Seconds())
		delete(m.jobStartTimes, key)
	}
}

func (m *Reporter) RecordHookExecution(upgradeName, hookPhase, hookName, result string) {
	hookExecutionsTotal.WithLabelValues(upgradeName, hookPhase, hookName, result).Inc()
}

func (m *Reporter) RecordMaintenanceWindow(upgradeType, name string, active bool, nextOpenTimestamp *int64) {
	if active {
		maintenanceWindowActive.WithLabelValues(upgradeType, name).Set(1)
		maintenanceWindowNextOpenTimestamp.DeleteLabelValues(upgradeType, name)
	} else {
		maintenanceWindowActive.WithLabelValues(upgradeType, name).Set(0)
		if nextOpenTimestamp != nil {
			maintenanceWindowNextOpenTimestamp.WithLabelValues(upgradeType, name).Set(float64(*nextOpenTimestamp))
		}
	}
}

func (m *Reporter) CleanupUpgradeMetrics(upgradeType, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	prefix := upgradeType + ":" + name + ":"
	for key := range m.startTimes {
		if strings.HasPrefix(key, prefix) {
			delete(m.startTimes, key)
		}
	}
	for key := range m.jobStartTimes {
		if strings.HasPrefix(key, prefix) {
			delete(m.jobStartTimes, key)
		}
	}

	switch upgradeType {
	case UpgradeTypeTalos:
		prev := m.talosUpgradePrev[name]
		delete(m.talosUpgradePrev, name)

		for _, phase := range talosPhases {
			talosUpgradePhaseGauge.DeleteLabelValues(name, phase, "")
		}
		if prev.phase != "" {
			talosUpgradePhaseGauge.DeleteLabelValues(name, prev.phase, prev.node)
		}
		talosUpgradeNodes.DeleteLabelValues(name)
		talosUpgradeNodesCompleted.DeleteLabelValues(name)
		talosUpgradeNodesFailed.DeleteLabelValues(name)
	case UpgradeTypeKubernetes:
		for _, phase := range kubernetesPhases {
			kubernetesUpgradePhaseGauge.DeleteLabelValues(name, phase)
		}
	}

	maintenanceWindowActive.DeleteLabelValues(upgradeType, name)
	maintenanceWindowNextOpenTimestamp.DeleteLabelValues(upgradeType, name)
	upgradeLastCompletionTimestamp.DeleteLabelValues(upgradeType, name, ResultSuccess)
	upgradeLastCompletionTimestamp.DeleteLabelValues(upgradeType, name, ResultFailure)

	progKey := progressingKey{upgradeType: upgradeType, name: name}
	if prevReason, ok := m.progressingPrev[progKey]; ok {
		upgradeProgressing.DeleteLabelValues(upgradeType, name, prevReason)
		delete(m.progressingPrev, progKey)
	}
}

func (m *Reporter) RecordBuildInfo(version, commit, goVersion string) {
	buildInfo.Reset()
	buildInfo.WithLabelValues(version, commit, goVersion).Set(1)
}

// InitializeAtBoot seeds always-on series at zero so dashboards render before any reconcile.
func (m *Reporter) InitializeAtBoot() {
	upgradeJobsActive.WithLabelValues(UpgradeTypeTalos).Set(0)
	upgradeJobsActive.WithLabelValues(UpgradeTypeKubernetes).Set(0)
	for _, p := range talosPhases {
		upgradesByPhase.WithLabelValues(UpgradeTypeTalos, p).Set(0)
	}
	for _, p := range kubernetesPhases {
		upgradesByPhase.WithLabelValues(UpgradeTypeKubernetes, p).Set(0)
	}
	for _, role := range []string{NodeRoleControlPlane, NodeRoleWorker} {
		managedNodes.WithLabelValues(role).Set(0)
	}
}

func (m *Reporter) RecordUpgradesByPhase(upgradeType string, byPhase map[string]int) {
	var phases []string
	switch upgradeType {
	case UpgradeTypeTalos:
		phases = talosPhases
	case UpgradeTypeKubernetes:
		phases = kubernetesPhases
	}
	for _, p := range phases {
		upgradesByPhase.WithLabelValues(upgradeType, p).Set(float64(byPhase[p]))
	}
}

func (m *Reporter) RecordManagedNodes(byRole map[string]int) {
	for _, role := range []string{NodeRoleControlPlane, NodeRoleWorker} {
		managedNodes.WithLabelValues(role).Set(float64(byRole[role]))
	}
}

type NodeInfoSnapshot struct {
	Node              string
	Role              string
	TalosVersion      string
	KubernetesVersion string
}

func (m *Reporter) RecordNodeInfo(snapshot []NodeInfoSnapshot) {
	// Resetting first prevents stale series when a node is removed or upgraded.
	nodeInfo.Reset()
	for _, n := range snapshot {
		nodeInfo.WithLabelValues(n.Node, n.Role, n.TalosVersion, n.KubernetesVersion).Set(1)
	}
}

func (m *Reporter) RecordUpgradeCompleted(upgradeType, name, result string) {
	upgradesCompletedTotal.WithLabelValues(upgradeType, result).Inc()
	upgradeLastCompletionTimestamp.WithLabelValues(upgradeType, name, result).Set(float64(time.Now().Unix()))
}

func TerminalResult(phase tupprv1alpha1.JobPhase) string {
	if phase == tupprv1alpha1.JobPhaseCompleted {
		return ResultSuccess
	}
	return ResultFailure
}
