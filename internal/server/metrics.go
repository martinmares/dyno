package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metrics struct {
	registry         *prometheus.Registry
	requestsTotal    *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	pageRenderCache  *prometheus.CounterVec
	searchRequests   prometheus.Counter
	searchDuration   prometheus.Histogram
	apiProxyRequests *prometheus.CounterVec
	apiProxyDuration *prometheus.HistogramVec
	watchReloads     prometheus.Counter
}

func newMetrics() *metrics {
	registry := prometheus.NewRegistry()
	m := &metrics{
		registry: registry,
		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "dyno",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests by route, method, and status.",
		}, []string{"route", "method", "status"}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "dyno",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration by route and method.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"route", "method"}),
		pageRenderCache: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "dyno",
			Name:      "page_render_cache_total",
			Help:      "Page render cache hits and misses.",
		}, []string{"result"}),
		searchRequests: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "dyno",
			Name:      "search_requests_total",
			Help:      "Total search requests.",
		}),
		searchDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "dyno",
			Name:      "search_duration_seconds",
			Help:      "Search request duration.",
			Buckets:   prometheus.DefBuckets,
		}),
		apiProxyRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "dyno",
			Name:      "api_proxy_requests_total",
			Help:      "Total API proxy requests by status class.",
		}, []string{"status"}),
		apiProxyDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "dyno",
			Name:      "api_proxy_duration_seconds",
			Help:      "API proxy upstream request duration by status class.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"status"}),
		watchReloads: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "dyno",
			Name:      "watch_reloads_total",
			Help:      "Total watcher-triggered reloads.",
		}),
	}
	registry.MustRegister(
		m.requestsTotal,
		m.requestDuration,
		m.pageRenderCache,
		m.searchRequests,
		m.searchDuration,
		m.apiProxyRequests,
		m.apiProxyDuration,
		m.watchReloads,
	)
	return m
}

func (m *metrics) handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func metricsMiddleware(m *metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		route := classifyRoute(r.URL.Path)
		status := strconv.Itoa(rw.status)
		m.requestsTotal.WithLabelValues(route, r.Method, status).Inc()
		m.requestDuration.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
	})
}

func classifyRoute(path string) string {
	switch {
	case strings.HasPrefix(path, "/assets/"):
		return "assets"
	case path == "/search":
		return "search"
	case path == "/api-proxy":
		return "api_proxy"
	case path == "/metrics":
		return "metrics"
	case path == "/health" || path == "/healthz" || path == "/livez" || path == "/readyz":
		return "health"
	default:
		return "page"
	}
}
