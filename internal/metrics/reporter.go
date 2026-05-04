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
	labelName        = "name"
	labelUpgradeName = "upgrade_name"
	labelPhase       = "phase"
	labelUpgradeType = "upgrade_type"
)

// Complete phase lists for the state-set gauge: all labels are always present (0 or 1).
var (
	talosPhases = []string{
		string(tupprv1alpha1.JobPhasePending),
		string(tupprv1alpha1.JobPhaseHealthChecking),
		string(tupprv1alpha1.JobPhaseDraining),
		string(tupprv1alpha1.JobPhaseUpgrading),
		string(tupprv1alpha1.JobPhaseRebooting),
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
		[]string{labelUpgradeType, labelUpgradeName, "node_name", "result"},
	)

	hookExecutionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tuppr_hook_executions_total",
			Help: "Total hook executions",
		},
		[]string{labelUpgradeName, "hook_phase", "hook_name", "result"},
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
	)
}

type talosPhaseNode struct {
	phase string
	node  string
}

type Reporter struct {
	mu               sync.RWMutex
	startTimes       map[string]*prometheus.Timer
	jobStartTimes    map[string]time.Time
	talosUpgradePrev map[string]talosPhaseNode
}

func NewReporter() *Reporter {
	return &Reporter{
		startTimes:       make(map[string]*prometheus.Timer),
		jobStartTimes:    make(map[string]time.Time),
		talosUpgradePrev: make(map[string]talosPhaseNode),
	}
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
}
