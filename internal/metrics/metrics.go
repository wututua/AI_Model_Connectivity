// Package metrics exposes the runtime as Prometheus metrics so operators
// can plug the connectivity checker into existing Grafana / Alertmanager
// pipelines without polling /api/status from a sidecar.
package metrics

import (
	"net/http"

	"cg/internal/probe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns its own prometheus.Registry rather than using the default
// global registry — this keeps testability sane (no leaked metrics across
// tests) and avoids accidental collisions with libraries that auto-register
// go/process collectors.
type Metrics struct {
	Registry *prometheus.Registry

	ProbeLatency  *prometheus.HistogramVec
	ProbeTotal    *prometheus.CounterVec
	TokensTotal   *prometheus.CounterVec
	CheckRuns     *prometheus.CounterVec
	CheckDuration *prometheus.HistogramVec
}

// New constructs and registers all metrics on a fresh registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Registry: reg,
		ProbeLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cg_probe_latency_ms",
			Help:    "Per-probe latency in milliseconds, labeled by provider/model/status.",
			Buckets: []float64{50, 100, 250, 500, 1000, 2000, 5000, 10000, 30000},
		}, []string{"provider", "model", "status"}),
		ProbeTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cg_probe_total",
			Help: "Total probes executed, labeled by provider/model/status.",
		}, []string{"provider", "model", "status"}),
		TokensTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cg_probe_tokens_total",
			Help: "Tokens consumed during probes, labeled by provider/model/kind (prompt|completion).",
		}, []string{"provider", "model", "kind"}),
		CheckRuns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cg_check_runs_total",
			Help: "Total check runs executed, labeled by kind/status.",
		}, []string{"kind", "status"}),
		CheckDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cg_check_duration_seconds",
			Help:    "Wall-clock duration of a full check run, labeled by kind/status.",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
		}, []string{"kind", "status"}),
	}
	// Standard Go and process collectors give Grafana dashboards the
	// usual goroutine / GC / RSS panels for free.
	reg.MustRegister(prometheus.NewGoCollector())
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	reg.MustRegister(m.ProbeLatency, m.ProbeTotal, m.TokensTotal, m.CheckRuns, m.CheckDuration)
	return m
}

// RecordProbe ingests one probe result.  Token counters only tick when
// the upstream actually reported usage (some providers omit the field).
func (m *Metrics) RecordProbe(r probe.Result) {
	if m == nil {
		return
	}
	m.ProbeLatency.WithLabelValues(r.ProviderID, r.Model, r.Status).Observe(float64(r.LatencyMS))
	m.ProbeTotal.WithLabelValues(r.ProviderID, r.Model, r.Status).Inc()
	if r.PromptTokens > 0 {
		m.TokensTotal.WithLabelValues(r.ProviderID, r.Model, "prompt").Add(float64(r.PromptTokens))
	}
	if r.CompletionTokens > 0 {
		m.TokensTotal.WithLabelValues(r.ProviderID, r.Model, "completion").Add(float64(r.CompletionTokens))
	}
}

// RecordCheck logs the outcome of a full check run.  Status is one of
// success/error/canceled to match storage.CheckTask.Status conventions.
func (m *Metrics) RecordCheck(kind, status string, durationSeconds float64) {
	if m == nil {
		return
	}
	m.CheckRuns.WithLabelValues(kind, status).Inc()
	m.CheckDuration.WithLabelValues(kind, status).Observe(durationSeconds)
}

// Handler returns the HTTP handler that serves the metrics in
// Prometheus text exposition format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}
