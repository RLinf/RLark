package gateway

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"

	rlarkmetrics "github.com/rlinf/rlark/apps/rlark/pkg/metrics"
)

const (
	subsystem = "gateway"
)

var (
	metrics = newGatewayMetrics()
)

type gatewayMetrics struct {
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	inFlight        *prometheus.GaugeVec
}

func newGatewayMetrics() *gatewayMetrics {
	requestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "requests_total",
			Help:      "Total number of HTTP requests handled by the gateway.",
		},
		[]string{"method", "path", "status"},
	)
	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "request_duration_seconds",
			Help:      "HTTP request latency in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
	inFlight := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: rlarkmetrics.Namespace,
			Subsystem: subsystem,
			Name:      "in_flight_requests",
			Help:      "Current in-flight HTTP requests being handled by the gateway.",
		},
		[]string{"method", "path"},
	)

	prometheus.MustRegister(requestsTotal, requestDuration, inFlight)
	return &gatewayMetrics{
		requestsTotal:   requestsTotal,
		requestDuration: requestDuration,
		inFlight:        inFlight,
	}
}

// IncRequest is an exported method.
func (m *gatewayMetrics) IncRequest(method, path, status string) {
	m.requestsTotal.WithLabelValues(method, path, status).Inc()
}

// ObserveDuration is an exported method.
func (m *gatewayMetrics) ObserveDuration(method, path string, seconds float64) {
	m.requestDuration.WithLabelValues(method, path).Observe(seconds)
}

// IncInFlight is an exported method.
func (m *gatewayMetrics) IncInFlight(method, path string) {
	m.inFlight.WithLabelValues(method, path).Inc()
}

// DecInFlight is an exported method.
func (m *gatewayMetrics) DecInFlight(method, path string) {
	m.inFlight.WithLabelValues(method, path).Dec()
}

// MetricsMiddleware instruments gin routes with request count, latency, and in-flight gauges.
// The "path" label is the matched route template (e.g. /api/v1/rlinf.io/v1alpha1/jobs/:name).
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		method := c.Request.Method
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}

		metrics.IncInFlight(method, path)
		c.Next()
		metrics.DecInFlight(method, path)

		status := c.Writer.Status()
		seconds := time.Since(start).Seconds()
		metrics.IncRequest(method, path, strconv.Itoa(status))
		metrics.ObserveDuration(method, path, seconds)
	}
}
