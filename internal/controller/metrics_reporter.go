package controller

import (
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Constants for upgrade types and context keys
const (
	UpgradeTypeTalos      = "talos"
	UpgradeTypeKubernetes = "kubernetes"
)

// Define custom types for context keys to avoid collisions
type contextKey string

const (
	ContextKeyUpgradeType contextKey = "upgradeType"
	ContextKeyUpgradeName contextKey = "upgradeName"
)

var (
	// TalosUpgrade metrics
	talosUpgradePhaseGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_talos_upgrade_phase",
			Help: "Current phase of Talos upgrades (0=Pending, 1=InProgress, 2=Completed, 3=Failed)",
		},
		[]string{"name", "phase"},
	)

	talosUpgradeNodesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_talos_upgrade_nodes_total",
			Help: "Total number of nodes in Talos upgrade",
		},
		[]string{"name"},
	)

	talosUpgradeNodesCompleted = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_talos_upgrade_nodes_completed",
			Help: "Number of nodes completed in Talos upgrade",
		},
		[]string{"name"},
	)

	talosUpgradeNodesFailed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_talos_upgrade_nodes_failed",
			Help: "Number of nodes failed in Talos upgrade",
		},
		[]string{"name"},
	)

	talosUpgradeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tuppr_talos_upgrade_duration_seconds",
			Help:    "Time taken for Talos upgrade phases",
			Buckets: []float64{30, 60, 300, 600, 1200, 1800, 3600, 7200}, // 30s to 2h
		},
		[]string{"name", "phase"},
	)

	// KubernetesUpgrade metrics
	kubernetesUpgradePhaseGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_kubernetes_upgrade_phase",
			Help: "Current phase of Kubernetes upgrades (0=Pending, 1=InProgress, 2=Completed, 3=Failed)",
		},
		[]string{"name", "phase"},
	)

	kubernetesUpgradeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tuppr_kubernetes_upgrade_duration_seconds",
			Help:    "Time taken for Kubernetes upgrade phases",
			Buckets: []float64{30, 60, 300, 600, 1200, 1800, 3600}, // 30s to 1h
		},
		[]string{"name", "phase"},
	)

	// Health check metrics
	healthCheckDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tuppr_health_check_duration_seconds",
			Help:    "Time taken for health checks to pass",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600}, // 1s to 10m
		},
		[]string{"upgrade_type", "upgrade_name"},
	)

	healthCheckFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tuppr_health_check_failures_total",
			Help: "Total number of health check failures",
		},
		[]string{"upgrade_type", "upgrade_name", "check_index"},
	)

	// Job metrics
	upgradeJobsActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_upgrade_jobs_active",
			Help: "Number of active upgrade jobs",
		},
		[]string{"upgrade_type"},
	)

	upgradeJobDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tuppr_upgrade_job_duration_seconds",
			Help:    "Time taken for upgrade jobs to complete",
			Buckets: []float64{60, 300, 600, 1200, 1800, 3600, 7200}, // 1m to 2h
		},
		[]string{"upgrade_type", "node_name", "result"},
	)
)

func init() {
	// Register metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		talosUpgradePhaseGauge,
		talosUpgradeNodesTotal,
		talosUpgradeNodesCompleted,
		talosUpgradeNodesFailed,
		talosUpgradeDuration,
		kubernetesUpgradePhaseGauge,
		kubernetesUpgradeDuration,
		healthCheckDuration,
		healthCheckFailuresTotal,
		upgradeJobsActive,
		upgradeJobDuration,
	)
}

// Helper functions to convert phase strings to numbers for Prometheus
func phaseToFloat64(phase string) float64 {
	switch phase {
	case "Pending":
		return 0
	case "InProgress":
		return 1
	case "Completed":
		return 2
	case "Failed":
		return 3
	default:
		return -1
	}
}

// MetricsReporter handles metric reporting for upgrades
type MetricsReporter struct {
	mu         sync.RWMutex // Add mutex for thread safety
	startTimes map[string]*prometheus.Timer
}

func NewMetricsReporter() *MetricsReporter {
	return &MetricsReporter{
		startTimes: make(map[string]*prometheus.Timer),
	}
}

func (m *MetricsReporter) RecordTalosUpgradePhase(name, phase string) {
	// Clear previous phase metrics for this upgrade
	talosUpgradePhaseGauge.DeletePartialMatch(prometheus.Labels{"name": name})

	// Set current phase
	talosUpgradePhaseGauge.WithLabelValues(name, phase).Set(phaseToFloat64(phase))
}

func (m *MetricsReporter) RecordTalosUpgradeNodes(name string, total, completed, failed int) {
	talosUpgradeNodesTotal.WithLabelValues(name).Set(float64(total))
	talosUpgradeNodesCompleted.WithLabelValues(name).Set(float64(completed))
	talosUpgradeNodesFailed.WithLabelValues(name).Set(float64(failed))
}

func (m *MetricsReporter) RecordKubernetesUpgradePhase(name, phase string) {
	// Clear previous phase metrics for this upgrade
	kubernetesUpgradePhaseGauge.DeletePartialMatch(prometheus.Labels{"name": name})

	// Set current phase
	kubernetesUpgradePhaseGauge.WithLabelValues(name, phase).Set(phaseToFloat64(phase))
}

func (m *MetricsReporter) StartPhaseTiming(upgradeType, name, phase string) {
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

func (m *MetricsReporter) EndPhaseTiming(upgradeType, name, phase string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := upgradeType + ":" + name + ":" + phase
	if timer, exists := m.startTimes[key]; exists {
		timer.ObserveDuration()
		delete(m.startTimes, key)
	}
}

func (m *MetricsReporter) RecordHealthCheckDuration(upgradeType, upgradeName string, duration float64) {
	healthCheckDuration.WithLabelValues(upgradeType, upgradeName).Observe(duration)
}

func (m *MetricsReporter) RecordHealthCheckFailure(upgradeType, upgradeName string, checkIndex int) {
	healthCheckFailuresTotal.WithLabelValues(upgradeType, upgradeName, strconv.Itoa(checkIndex)).Inc()
}

func (m *MetricsReporter) RecordActiveJobs(upgradeType string, count int) {
	upgradeJobsActive.WithLabelValues(upgradeType).Set(float64(count))
}

func (m *MetricsReporter) RecordJobDuration(upgradeType, nodeName, result string, duration float64) {
	upgradeJobDuration.WithLabelValues(upgradeType, nodeName, result).Observe(duration)
}

// Cleanup removes all metrics for a specific upgrade (useful when upgrade is deleted)
func (m *MetricsReporter) CleanupUpgradeMetrics(upgradeType, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clean up any pending timers
	for key := range m.startTimes {
		if strings.HasPrefix(key, upgradeType+":"+name+":") {
			delete(m.startTimes, key)
		}
	}

	// Clean up gauge metrics
	switch upgradeType {
	case UpgradeTypeTalos:
		talosUpgradePhaseGauge.DeletePartialMatch(prometheus.Labels{"name": name})
		talosUpgradeNodesTotal.DeleteLabelValues(name)
		talosUpgradeNodesCompleted.DeleteLabelValues(name)
		talosUpgradeNodesFailed.DeleteLabelValues(name)
	case UpgradeTypeKubernetes:
		kubernetesUpgradePhaseGauge.DeletePartialMatch(prometheus.Labels{"name": name})
	}
}
