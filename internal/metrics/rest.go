// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// RESTMetrics records request counts and latency for the REST gateway.
// Requests are labelled by method and status code only: the gateway's mux
// paths are a fixed, small set of registered routes, but raw path values are
// deliberately never used as a label to avoid unbounded cardinality.
type RESTMetrics struct {
	requestsTotal *prometheus.CounterVec
	duration      *prometheus.HistogramVec
}

// NewRESTMetrics registers the REST gateway collectors on reg.
func NewRESTMetrics(reg *prometheus.Registry) *RESTMetrics {
	m := &RESTMetrics{
		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "authz_rest_requests_total",
			Help: "Total number of REST gateway requests handled, by method and status code.",
		}, []string{"method", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "authz_rest_request_duration_seconds",
			Help:    "REST gateway request handling duration in seconds, by method.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method"}),
	}
	reg.MustRegister(m.requestsTotal, m.duration)
	return m
}

// Middleware wraps next, recording request counts and latency.
func (m *RESTMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rec, r)

		m.requestsTotal.WithLabelValues(r.Method, strconv.Itoa(rec.statusCode)).Inc()
		m.duration.WithLabelValues(r.Method).Observe(time.Since(start).Seconds())
	})
}

// statusRecorder captures the status code written by the wrapped handler.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}
