package daemon

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/RomanAgaltsev/gantry/internal/engine"
)

// PrometheusObserver records reconcile outcomes as Prometheus metrics on a private registry.
// It satisfies Observer; NewPrometheusObserver returns it together with the /metrics handler
// bound to that registry. Recording is a pure side effect — it never errors out of a method.
type PrometheusObserver struct {
	reconcile   *prometheus.CounterVec   // {env,result}
	duration    *prometheus.HistogramVec // {env}
	deploys     *prometheus.CounterVec   // {env}
	rollbacks   *prometheus.CounterVec   // {env,kind}
	verifyFails *prometheus.CounterVec   // {env}
	driftAge    *prometheus.GaugeVec     // {env}
}

// NewPrometheusObserver builds the observer on a private registry and returns it plus the HTTP
// handler that scrapes that registry (mounted at /metrics by the serve command). The version
// string populates gantry_build_info; pass the CLI version (set by ldflags).
func NewPrometheusObserver(version string) (*PrometheusObserver, http.Handler) {
	o := &PrometheusObserver{
		reconcile: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gantry_reconcile_total",
			Help: "Reconcile passes by result.",
		}, []string{"env", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gantry_reconcile_duration_seconds",
			Help:    "Reconcile duration.",
			Buckets: prometheus.DefBuckets,
		}, []string{"env"}),
		deploys: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gantry_deploys_total",
			Help: "Deploys performed.",
		}, []string{"env"}),
		rollbacks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gantry_rollbacks_total",
			Help: "Rollbacks performed.",
		}, []string{"env", "kind"}),
		verifyFails: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gantry_verify_failures_total",
			Help: "Post-deploy verify failures.",
		}, []string{"env"}),
		driftAge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gantry_drift_age_seconds",
			Help: "Age of the oldest drifted component.",
		}, []string{"env"}),
	}

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gantry_build_info",
		Help: "Build info.",
	}, []string{"version"})
	buildInfo.WithLabelValues(version).Set(1)

	reg := prometheus.NewRegistry()
	reg.MustRegister(o.reconcile, o.duration, o.deploys, o.rollbacks, o.verifyFails, o.driftAge, buildInfo)
	return o, promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}

// ReconcileDone records one reconcile's outcome. The duration is always observed; the result
// label is one of deployed/failed/nochange; deploys, verify failures, and auto rollbacks
// increment their own counters when they occur.
func (o *PrometheusObserver) ReconcileDone(env string, res engine.SyncResult, err error, dur time.Duration) {
	o.duration.WithLabelValues(env).Observe(dur.Seconds())
	switch {
	case err != nil:
		o.reconcile.WithLabelValues(env, "failed").Inc()
	case res.Deployed:
		o.reconcile.WithLabelValues(env, "deployed").Inc()
		o.deploys.WithLabelValues(env).Inc()
	default:
		o.reconcile.WithLabelValues(env, "nochange").Inc()
	}
	if res.VerifyFailed {
		o.verifyFails.WithLabelValues(env).Inc()
	}
	if res.AutoRolledBack {
		o.rollbacks.WithLabelValues(env, "auto").Inc()
	}
}

// DriftObserved records the age of the oldest drifted component (seconds) for env; writing 0
// clears the gauge when drift resolves.
func (o *PrometheusObserver) DriftObserved(env string, ageSeconds float64) {
	o.driftAge.WithLabelValues(env).Set(ageSeconds)
}
