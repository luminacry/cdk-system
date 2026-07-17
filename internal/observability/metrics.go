package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the application Prometheus metrics.
type Metrics struct {
	RequestDuration *prometheus.HistogramVec
	RequestTotal    *prometheus.CounterVec
	DBWaitDuration  prometheus.Histogram
	RateLimitDenied prometheus.Counter
	OutboxBacklog   prometheus.Gauge
}

// NewMetrics creates and registers metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request latency in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),
		RequestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_request_total",
				Help: "Total HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		DBWaitDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "db_wait_duration_seconds",
				Help:    "Time spent waiting for a database connection",
				Buckets: prometheus.DefBuckets,
			},
		),
		RateLimitDenied: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "rate_limit_denied_total",
				Help: "Total rate-limited requests",
			},
		),
		OutboxBacklog: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "webhook_outbox_backlog",
				Help: "Number of pending/failed webhook outbox events",
			},
		),
	}

	if reg != nil {
		reg.MustRegister(
			m.RequestDuration,
			m.RequestTotal,
			m.DBWaitDuration,
			m.RateLimitDenied,
			m.OutboxBacklog,
		)
	}
	return m
}

// MetricsMiddleware records request duration and counts.
func (m *Metrics) MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		status := strconv.Itoa(ww.Status())
		labels := prometheus.Labels{
			"method": r.Method,
			"path":   r.URL.Path,
			"status": status,
		}
		duration := time.Since(start).Seconds()
		m.RequestDuration.With(labels).Observe(duration)
		m.RequestTotal.With(labels).Inc()
	})
}

// Handler returns the Prometheus metrics HTTP handler.
func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}
