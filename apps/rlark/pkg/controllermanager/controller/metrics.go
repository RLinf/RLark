package controller

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
)

// Metrics publishes one reconcile pass per controller as three series.
//
// Every label value is drawn from a closed set that is declared in Go source:
// the controller's resource kind, the Outcome enum, and the shell's reason
// constants. Object names, namespaces, UIDs and raw error strings are
// deliberately never used as labels, because a label whose domain grows with
// the number of objects (or with the number of distinct error messages a
// dependency can produce) turns a metrics endpoint into an unbounded memory
// leak and makes the series unusable for alerting.
type Metrics struct {
	reconcileTotal *prometheus.CounterVec
	failureTotal   *prometheus.CounterVec
	duration       *prometheus.HistogramVec
}

// NewMetrics builds the collectors and registers them with the supplied
// registerer.
//
// The registerer is an explicit constructor argument rather than the package
// default, so that a test can use a throwaway registry and so that there is
// exactly one place in the process that decides where control-plane metrics are
// published.
func NewMetrics(registerer prometheus.Registerer) (*Metrics, error) {
	m := &Metrics{
		reconcileTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "rlark_controller_reconcile_total",
			Help: "Number of completed reconcile passes, partitioned by the classified outcome.",
		}, []string{"controller", "outcome"}),
		failureTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "rlark_controller_reconcile_failures_total",
			Help: "Number of reconcile passes that returned an error, partitioned by the shell's reason code.",
		}, []string{"controller", "reason"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "rlark_controller_reconcile_duration_seconds",
			Help:    "Wall-clock duration of a reconcile pass.",
			Buckets: prometheus.DefBuckets,
		}, []string{"controller"}),
	}

	for _, collector := range []prometheus.Collector{m.reconcileTotal, m.failureTotal, m.duration} {
		if err := registerer.Register(collector); err != nil {
			return nil, fmt.Errorf("register controller reconcile metrics: %w", err)
		}
	}

	return m, nil
}

// observe records one pass. A nil *Metrics is a no-op so that a controller
// constructed without metrics (unit tests, one-off tooling) behaves exactly
// like one with them.
func (m *Metrics) observe(controller string, result Result, err error, elapsed time.Duration) {
	if m == nil {
		return
	}

	outcome := result.Outcome
	reason := result.Reason
	if err != nil {
		// An error means the pass did not reach a decision it can stand behind,
		// so it is always counted as retrying even if the caller had already
		// filled in a more specific outcome before failing.
		outcome = sharedreconcile.OutcomeRetrying
		if reason == "" {
			reason = ReasonReconcileFailed
		}
		m.failureTotal.WithLabelValues(controller, reason).Inc()
	}
	if outcome == "" {
		outcome = sharedreconcile.OutcomeSucceeded
	}

	m.reconcileTotal.WithLabelValues(controller, string(outcome)).Inc()
	m.duration.WithLabelValues(controller).Observe(elapsed.Seconds())
}
