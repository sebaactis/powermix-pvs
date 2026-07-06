// Package handler contiene los handlers HTTP y middlewares.
package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// --- Colectores de métricas ---
// Registrados globalmente para simplificar el wiring.
// En producción avanzada se recomienda un Registry separado.

var (
	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total de requests HTTP procesados.",
		},
		[]string{"method", "path", "status"},
	)

	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duracion de requests HTTP en segundos.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	// PVS calls (outbound)
	pvsCallsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pvs_calls_total",
			Help: "Total de llamadas salientes a PVS.",
		},
		[]string{"endpoint", "status"},
	)

	pvsCallDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pvs_call_duration_seconds",
			Help:    "Duracion de llamadas a PVS.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint"},
	)

	// Reconciler
	reconcilerRuns = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "reconciler_runs_total",
			Help: "Total de ejecuciones del reconciler.",
		},
	)

	reconcilerFixed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "reconciler_fixed_total",
			Help: "Total de ordenes corregidas por el reconciler.",
		},
	)
)

func init() {
	prometheus.MustRegister(httpRequests, httpDuration,
		pvsCallsTotal, pvsCallDuration,
		reconcilerRuns, reconcilerFixed)
}

// metricsMiddleware registra contadores y duracion de cada request HTTP.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		status := strconv.Itoa(sw.status)
		httpRequests.WithLabelValues(r.Method, r.URL.Path, status).Inc()
		httpDuration.WithLabelValues(r.Method, r.URL.Path, status).
			Observe(time.Since(start).Seconds())
	})
}

// MetricsHandler devuelve un http.Handler que expone las métricas en
// formato Prometheus text/plain. Tipicamente se monta en GET /metrics.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// statusWriter captura el codigo de status HTTP para las metricas.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
