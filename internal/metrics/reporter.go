package metrics

import (
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	UpgradeTypeTalos      = "talos"
	UpgradeTypeKubernetes = "kubernetes"
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
			Help: "Current phase of Talos upgrades (0=Pending, 1=Draining, 2=Upgrading, 3=Rebooting, 4=Completed, 5=Failed)",
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
			Buckets: []float64{30, 60, 300, 600, 1200, 1800, 3600, 7200},
		},
		[]string{"name", "phase"},
	)

	kubernetesUpgradePhaseGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_kubernetes_upgrade_phase",
			Help: "Current phase of Kubernetes upgrades (0=Pending, 1=Draining, 2=Upgrading, 3=Rebooting, 4=Completed, 5=Failed)",
		},
		[]string{"name", "phase"},
	)

	kubernetesUpgradeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tuppr_kubernetes_upgrade_duration_seconds",
			Help:    "Time taken for Kubernetes upgrade phases",
			Buckets: []float64{30, 60, 300, 600, 1200, 1800, 3600},
		},
		[]string{"name", "phase"},
	)

	healthCheckDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tuppr_health_check_duration_seconds",
			Help:    "Time taken for health checks to pass",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
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
			Buckets: []float64{60, 300, 600, 1200, 1800, 3600, 7200},
		},
		[]string{"upgrade_type", "node_name", "result"},
	)

	maintenanceWindowActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_maintenance_window_active",
			Help: "Whether upgrade is currently inside a maintenance window (1=inside, 0=outside)",
		},
		[]string{"upgrade_type", "name"},
	)

	maintenanceWindowNextOpenTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tuppr_maintenance_window_next_open_timestamp",
			Help: "Unix timestamp of the next maintenance window start",
		},
		[]string{"upgrade_type", "name"},
	)
)

func init() {
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
		maintenanceWindowActive,
		maintenanceWindowNextOpenTimestamp,
	)
}

func phaseToFloat64(phase string) float64 {
	switch phase {
	case "Pending":
		return 0
	case "HealthChecking":
		return 1
	case "Draining":
		return 2
	case "Upgrading":
		return 3
	case "Rebooting":
		return 4
	case "Completed":
		return 5
	case "Failed":
		return 6
	default:
		return -1
	}
}

type Reporter struct {
	mu         sync.RWMutex
	startTimes map[string]*prometheus.Timer
}

func NewReporter() *Reporter {
	return &Reporter{
		startTimes: make(map[string]*prometheus.Timer),
	}
}

func (m *Reporter) RecordTalosUpgradePhase(name, phase string) {
	talosUpgradePhaseGauge.DeletePartialMatch(prometheus.Labels{"name": name})
	talosUpgradePhaseGauge.WithLabelValues(name, phase).Set(phaseToFloat64(phase))
}

func (m *Reporter) RecordTalosUpgradeNodes(name string, total, completed, failed int) {
	talosUpgradeNodesTotal.WithLabelValues(name).Set(float64(total))
	talosUpgradeNodesCompleted.WithLabelValues(name).Set(float64(completed))
	talosUpgradeNodesFailed.WithLabelValues(name).Set(float64(failed))
}

func (m *Reporter) RecordKubernetesUpgradePhase(name, phase string) {
	kubernetesUpgradePhaseGauge.DeletePartialMatch(prometheus.Labels{"name": name})
	kubernetesUpgradePhaseGauge.WithLabelValues(name, phase).Set(phaseToFloat64(phase))
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

func (m *Reporter) RecordJobDuration(upgradeType, nodeName, result string, duration float64) {
	upgradeJobDuration.WithLabelValues(upgradeType, nodeName, result).Observe(duration)
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

	for key := range m.startTimes {
		if strings.HasPrefix(key, upgradeType+":"+name+":") {
			delete(m.startTimes, key)
		}
	}

	switch upgradeType {
	case UpgradeTypeTalos:
		talosUpgradePhaseGauge.DeletePartialMatch(prometheus.Labels{"name": name})
		talosUpgradeNodesTotal.DeleteLabelValues(name)
		talosUpgradeNodesCompleted.DeleteLabelValues(name)
		talosUpgradeNodesFailed.DeleteLabelValues(name)
	case UpgradeTypeKubernetes:
		kubernetesUpgradePhaseGauge.DeletePartialMatch(prometheus.Labels{"name": name})
	}

	maintenanceWindowActive.DeleteLabelValues(upgradeType, name)
	maintenanceWindowNextOpenTimestamp.DeleteLabelValues(upgradeType, name)
}
