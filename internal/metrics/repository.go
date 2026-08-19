// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// RepositoryRecorder implements repository.Metrics. A single instance is
// shared by every repository implementation; operation already distinguishes
// callers (e.g. "insert", "claim_batch", "find_candidates").
type RepositoryRecorder struct {
	queriesTotal *prometheus.CounterVec
	duration     *prometheus.HistogramVec
}

// NewRepositoryRecorder registers the repository-layer collectors on reg.
func NewRepositoryRecorder(reg *prometheus.Registry) *RepositoryRecorder {
	m := &RepositoryRecorder{
		queriesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "authz_repository_queries_total",
			Help: "Total number of repository queries, by operation and result.",
		}, []string{"operation", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "authz_repository_query_duration_seconds",
			Help:    "Duration of a repository query, by operation.",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation"}),
	}
	reg.MustRegister(m.queriesTotal, m.duration)
	return m
}

// ObserveQuery implements repository.Metrics.
func (m *RepositoryRecorder) ObserveQuery(operation string, err error, duration time.Duration) {
	result := "ok"
	if err != nil {
		result = "error"
	}
	m.queriesTotal.WithLabelValues(operation, result).Inc()
	m.duration.WithLabelValues(operation).Observe(duration.Seconds())
}
