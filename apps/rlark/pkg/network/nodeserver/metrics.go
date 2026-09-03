package nodeserver

import (
	"github.com/prometheus/client_golang/prometheus"

	rlarkmetrics "github.com/rlinf/rlark/apps/rlark/pkg/metrics"
)

const subsystem = "nodeserver"

var metrics = newNodeServerMetrics()

type nodeServerMetrics struct {
	connectionsTotal  prometheus.Counter
	connectionsActive prometheus.Gauge
	dialTotal         *prometheus.CounterVec
	sshReconnectTotal *prometheus.CounterVec
}

func newNodeServerMetrics() *nodeServerMetrics {
	connectionsTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "connections_total",
			Help:      "Total number of connections accepted on the nodeserver unix socket.",
		},
	)
	connectionsActive := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "connections_active",
			Help:      "Current number of active connections on the nodeserver unix socket.",
		},
	)
	dialTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "dial_total",
			Help:      "Total number of upstream dials, by type (direct/ssh) and status (success/error).",
		},
		[]string{"type", "status"},
	)
	sshReconnectTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "ssh_reconnect_total",
			Help:      "Total number of SSH tunnel reconnections, by domain.",
		},
		[]string{"domain"},
	)

	prometheus.MustRegister(connectionsTotal, connectionsActive, dialTotal, sshReconnectTotal)
	return &nodeServerMetrics{
		connectionsTotal:  connectionsTotal,
		connectionsActive: connectionsActive,
		dialTotal:         dialTotal,
		sshReconnectTotal: sshReconnectTotal,
	}
}

// Metrics returns the package-level metrics singleton, for use by other
// packages (e.g. agent/container) that need to record nodeserver-related events.
func Metrics() *nodeServerMetrics {
	return metrics
}

// IncConnections records a newly accepted connection.
func (m *nodeServerMetrics) IncConnections() {
	m.connectionsTotal.Inc()
}

// IncActive increments the active connection gauge.
func (m *nodeServerMetrics) IncActive() {
	m.connectionsActive.Inc()
}

// DecActive decrements the active connection gauge.
func (m *nodeServerMetrics) DecActive() {
	m.connectionsActive.Dec()
}

// IncDial records an upstream dial attempt.
// dialType must be "direct" or "ssh"; status must be "success" or "error".
func (m *nodeServerMetrics) IncDial(dialType, status string) {
	m.dialTotal.WithLabelValues(dialType, status).Inc()
}

// IncSSHReconnect records an SSH tunnel reconnection for the given domain.
func (m *nodeServerMetrics) IncSSHReconnect(domain string) {
	m.sshReconnectTotal.WithLabelValues(domain).Inc()
}

// OnReconnect returns a callback suitable for SSHDialerConfig.OnReconnect.
func OnReconnect() func(domainID string) {
	return func(domainID string) {
		metrics.IncSSHReconnect(domainID)
	}
}
