package server

import (
	"github.com/prometheus/client_golang/prometheus"

	rlarkmetrics "github.com/rlinf/rlark/apps/rlark/pkg/metrics"
)

const (
	subsystem = "server"
)

var (
	metrics = newServerMetrics()
)

type serverMetrics struct {
	proxyRequestsTotal *prometheus.CounterVec
	peerConnections    prometheus.Gauge
	sshConnections     *prometheus.CounterVec
}

func newServerMetrics() *serverMetrics {
	proxyRequestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "proxy_requests_total",
			Help:      "Total number of HTTP requests proxied by the server.",
		},
		[]string{"target", "status"},
	)
	peerConnections := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "peer_connections",
			Help:      "Current number of peer server connections.",
		},
	)
	sshConnections := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "ssh_connections_total",
			Help:      "Total number of SSH connections handled by the server.",
		},
		[]string{"user", "status"},
	)

	prometheus.MustRegister(proxyRequestsTotal, peerConnections, sshConnections)
	return &serverMetrics{
		proxyRequestsTotal: proxyRequestsTotal,
		peerConnections:    peerConnections,
		sshConnections:     sshConnections,
	}
}

// IncProxyRequest is an exported method.
func (m *serverMetrics) IncProxyRequest(target, status string) {
	m.proxyRequestsTotal.WithLabelValues(target, status).Inc()
}

// SetPeerConnections sets the peerConnections.
func (m *serverMetrics) SetPeerConnections(cnt int) {
	m.peerConnections.Set(float64(cnt))
}

// IncSSHConnection is an exported method.
func (m *serverMetrics) IncSSHConnection(user, status string) {
	m.sshConnections.WithLabelValues(user, status).Inc()
}
