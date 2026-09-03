package sidecar

import (
	"github.com/prometheus/client_golang/prometheus"

	rlarkmetrics "github.com/rlinf/rlark/apps/rlark/pkg/metrics"
	"github.com/rlinf/rlark/apps/rlark/pkg/network/tun"
)

const subsystem = "sidecar"

var metrics = newSidecarMetrics()

type sidecarMetrics struct {
	tunPacketsTotal        *prometheus.CounterVec
	proxyConnectionsTotal  *prometheus.CounterVec
	proxyConnectionsActive prometheus.Gauge
	hostsSyncTotal         *prometheus.CounterVec
}

func newSidecarMetrics() *sidecarMetrics {
	tunPacketsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "tun_packets_total",
			Help:      "Total number of packets forwarded through the TUN device.",
		},
		[]string{"direction"}, // tx (pod→remote) / rx (remote→pod)
	)
	proxyConnectionsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "proxy_connections_total",
			Help:      "Total number of inbound connections accepted by the proxy listener.",
		},
		[]string{"protocol"}, // tcp / udp / icmp
	)
	proxyConnectionsActive := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "proxy_connections_active",
			Help:      "Current number of active inbound proxy connections.",
		},
	)
	hostsSyncTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "hosts_sync_total",
			Help:      "Total number of hosts file sync attempts.",
		},
		[]string{"status"}, // success / error
	)

	prometheus.MustRegister(tunPacketsTotal, proxyConnectionsTotal, proxyConnectionsActive, hostsSyncTotal)
	m := &sidecarMetrics{
		tunPacketsTotal:        tunPacketsTotal,
		proxyConnectionsTotal:  proxyConnectionsTotal,
		proxyConnectionsActive: proxyConnectionsActive,
		hostsSyncTotal:         hostsSyncTotal,
	}
	// 注入到 tun 包,避免 import cycle(sidecar → tun → sidecar)
	tun.RegisterMetrics(tunPacketsTotal, proxyConnectionsTotal, proxyConnectionsActive)
	return m
}

// Metrics returns the package-level metrics singleton, for use by other
// packages (e.g. network/tun) that need to record sidecar-related events.
func Metrics() *sidecarMetrics {
	return metrics
}

// IncTunPackets records a forwarded packet on the TUN device.
// direction must be "tx" (pod → remote) or "rx" (remote → pod).
func (m *sidecarMetrics) IncTunPackets(direction string) {
	m.tunPacketsTotal.WithLabelValues(direction).Inc()
}

// IncProxyConnections records a new inbound proxy connection.
// protocol must be "tcp", "udp", or "icmp".
func (m *sidecarMetrics) IncProxyConnections(protocol string) {
	m.proxyConnectionsTotal.WithLabelValues(protocol).Inc()
}

// IncProxyActive increments the active proxy connection gauge.
func (m *sidecarMetrics) IncProxyActive() {
	m.proxyConnectionsActive.Inc()
}

// DecProxyActive decrements the active proxy connection gauge.
func (m *sidecarMetrics) DecProxyActive() {
	m.proxyConnectionsActive.Dec()
}

// IncHostsSync records a hosts sync attempt. status must be "success" or "error".
func (m *sidecarMetrics) IncHostsSync(status string) {
	m.hostsSyncTotal.WithLabelValues(status).Inc()
}
