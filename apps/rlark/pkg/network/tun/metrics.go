package tun

import (
	"github.com/prometheus/client_golang/prometheus"
)

// tunMetrics holds the metrics for the tun package. It is populated via
// RegisterMetrics to avoid an import cycle with the sidecar package
// (sidecar → tun → sidecar).
var tunMetrics = &tunMetricsType{}

type tunMetricsType struct {
	packetsTotal           *prometheus.CounterVec
	proxyConnectionsTotal  *prometheus.CounterVec
	proxyConnectionsActive prometheus.Gauge
}

// RegisterMetrics injects the sidecar-owned metric vectors so the tun package
// can record events without importing the sidecar package.
func RegisterMetrics(
	packetsTotal *prometheus.CounterVec,
	proxyConnectionsTotal *prometheus.CounterVec,
	proxyConnectionsActive prometheus.Gauge,
) {
	tunMetrics.packetsTotal = packetsTotal
	tunMetrics.proxyConnectionsTotal = proxyConnectionsTotal
	tunMetrics.proxyConnectionsActive = proxyConnectionsActive
}

// IncPackets records a forwarded packet on the TUN device.
// direction must be "tx" (pod → remote) or "rx" (remote → pod).
func (m *tunMetricsType) IncPackets(direction string) {
	if m.packetsTotal != nil {
		m.packetsTotal.WithLabelValues(direction).Inc()
	}
}

// IncProxyConnections records a new inbound proxy connection.
// protocol must be "tcp", "udp", or "icmp".
func (m *tunMetricsType) IncProxyConnections(protocol string) {
	if m.proxyConnectionsTotal != nil {
		m.proxyConnectionsTotal.WithLabelValues(protocol).Inc()
	}
}

// IncProxyActive increments the active proxy connection gauge.
func (m *tunMetricsType) IncProxyActive() {
	if m.proxyConnectionsActive != nil {
		m.proxyConnectionsActive.Inc()
	}
}

// DecProxyActive decrements the active proxy connection gauge.
func (m *tunMetricsType) DecProxyActive() {
	if m.proxyConnectionsActive != nil {
		m.proxyConnectionsActive.Dec()
	}
}
