package observability

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus counters and histograms for this service.
type Metrics struct {
	RequestTotal    *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	ErrorTotal      *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		RequestTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		}, []string{"method", "path", "status"}),

		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),

		ErrorTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_errors_total",
			Help: "Total number of HTTP errors (status >= 500).",
		}, []string{"method", "path"}),
	}
	reg.MustRegister(m.RequestTotal, m.RequestDuration, m.ErrorTotal)
	return m
}

// statusRecorder wraps http.ResponseWriter to capture the response status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// InstrumentHandler returns middleware that records Prometheus metrics and
// structured slog log lines for every request.
func InstrumentHandler(logger *slog.Logger, m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			dur := time.Since(start)
			status := strconv.Itoa(rec.status)
			path := r.URL.Path

			m.RequestTotal.WithLabelValues(r.Method, path, status).Inc()
			m.RequestDuration.WithLabelValues(r.Method, path).Observe(dur.Seconds())

			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
				m.ErrorTotal.WithLabelValues(r.Method, path).Inc()
			} else if rec.status >= 400 {
				level = slog.LevelWarn
			}

			logger.LogAttrs(context.Background(), level, "request",
				slog.String("method", r.Method),
				slog.String("path", path),
				slog.Int("status", rec.status),
				slog.Duration("duration", dur),
			)
		})
	}
}

// NewJSONLogger returns a slog.Logger that writes JSON to stdout.
// In production, structured JSON logs are ingested by log aggregators
// (Datadog, Loki, CloudWatch) for searching and alerting.
func NewJSONLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// BuildRouter wires up the application routes plus /metrics.
func BuildRouter(logger *slog.Logger, m *Metrics, reg *prometheus.Registry) http.Handler {
	r := chi.NewRouter()
	r.Use(InstrumentHandler(logger, m))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Get("/todos", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []any{})
	})

	r.Get("/error", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
	})

	// /metrics is served by Prometheus and excluded from instrumentation to
	// avoid inflating request counts with scrape traffic.
	r.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	return r
}
