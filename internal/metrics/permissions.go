// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// PermissionsRecorder implements permissions.Metrics.
type PermissionsRecorder struct {
	operationsTotal *prometheus.CounterVec
	duration        *prometheus.HistogramVec
}

// NewPermissionsRecorder registers the permissions-service collectors on reg.
func NewPermissionsRecorder(reg *prometheus.Registry) *PermissionsRecorder {
	m := &PermissionsRecorder{
		operationsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "authz_permissions_operations_total",
			Help: "Total number of permission-registration operations, by operation and result.",
		}, []string{"operation", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "authz_permissions_operation_duration_seconds",
			Help:    "Duration of a permission-registration operation, by operation.",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation"}),
	}
	reg.MustRegister(m.operationsTotal, m.duration)
	return m
}

// ObserveOperation implements permissions.Metrics.
func (m *PermissionsRecorder) ObserveOperation(operation string, err error, duration time.Duration) {
	result := "ok"
	if err != nil {
		result = "error"
	}
	m.operationsTotal.WithLabelValues(operation, result).Inc()
	m.duration.WithLabelValues(operation).Observe(duration.Seconds())
}
