---
title: Monitoring
weight: 40
---

# Monitoring

## The four golden signals

Monitor these four metrics for any service:

| Signal | What it measures | Example metric |
|---|---|---|
| **Latency** | Time to serve a request | `http_request_duration_seconds` |
| **Traffic** | Demand on the system | `http_requests_total` |
| **Errors** | Rate of failed requests | `http_requests_total{status=~"5.."}` |
| **Saturation** | How "full" the service is | `go_goroutines`, CPU/memory % |

## Exposing Prometheus metrics in Go

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "http_requests_total",
        Help: "Total number of HTTP requests",
    }, []string{"method", "path", "status"})

    requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "http_request_duration_seconds",
        Help:    "HTTP request latency",
        Buckets: prometheus.DefBuckets,
    }, []string{"method", "path"})
)

// Register the /metrics endpoint
mux.Handle("/metrics", promhttp.Handler())
```

## Alerting rules

```yaml
# prometheus/alerts.yaml
groups:
  - name: myservice
    rules:
      - alert: HighErrorRate
        expr: |
          sum(rate(http_requests_total{status=~"5.."}[5m])) /
          sum(rate(http_requests_total[5m])) > 0.05
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Error rate above 5%"

      - alert: HighLatency
        expr: |
          histogram_quantile(0.99,
            rate(http_request_duration_seconds_bucket[5m])
          ) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "p99 latency above 1s"
```

## Grafana dashboard tips

- Always include a **time range** variable (`$__timeRange`)
- Use **rate()** for counters, never raw counter values
- Show **p50, p95, p99** latency side by side, not averages
- Add annotations for deployments so you can correlate incidents

## Structured logging with slog

```go
slog.Info("request handled",
    "method", r.Method,
    "path",   r.URL.Path,
    "status", status,
    "duration_ms", elapsed.Milliseconds(),
    "request_id", requestID,
)
```

> [!TIP]
> In production use `slog.NewJSONHandler` — structured JSON logs are easy to ship to Loki, OpenSearch or Datadog.
