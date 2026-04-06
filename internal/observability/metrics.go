package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	OrdersReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "orders_received_total",
		Help: "Total orders accepted by the API",
	})
	TradesExecuted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "trades_executed_total",
		Help: "Total trades produced by matching",
	})
	MatchLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "match_latency_ms",
		Help:    "End-to-end match path latency in milliseconds",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 16),
	}, []string{"result"})
	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status"})
	DBQueryLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "db_query_latency_seconds",
		Help:    "Database operation latency",
		Buckets: prometheus.DefBuckets,
	}, []string{"op"})
)

// MetricsHandler serves /metrics for Prometheus.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// HTTPMiddleware records request duration and status.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		d := time.Since(start).Seconds()
		path := r.URL.Path
		if len(path) > 64 {
			path = path[:64]
		}
		HTTPDuration.WithLabelValues(r.Method, path, strconv.Itoa(sw.status)).Observe(d)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
