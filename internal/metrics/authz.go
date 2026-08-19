// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// CheckRecorder implements authz.Metrics. Cardinality is bounded: result and
// reason are both small fixed enums, never raw request data.
type CheckRecorder struct {
	checksTotal      *prometheus.CounterVec
	checkDuration    *prometheus.HistogramVec
	stsExchangeTime  prometheus.Histogram
	resourceMapTime  prometheus.Histogram
	openFGACheckTime prometheus.Histogram
}

// NewCheckRecorder registers the external-authz Check collectors on reg.
func NewCheckRecorder(reg *prometheus.Registry) *CheckRecorder {
	m := &CheckRecorder{
		checksTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "authz_check_total",
			Help: "Total number of external authorization Check calls, by result and reason.",
		}, []string{"result", "reason"}),
		checkDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "authz_check_duration_seconds",
			Help:    "Total duration of an external authorization Check call, by result.",
			Buckets: prometheus.DefBuckets,
		}, []string{"result"}),
		stsExchangeTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "authz_check_sts_exchange_duration_seconds",
			Help:    "Duration of the STS session-exchange call within Check.",
			Buckets: prometheus.DefBuckets,
		}),
		resourceMapTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "authz_check_resource_map_duration_seconds",
			Help:    "Duration of the rule/tuple resolution step within Check.",
			Buckets: prometheus.DefBuckets,
		}),
		openFGACheckTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "authz_check_openfga_duration_seconds",
			Help:    "Duration of the OpenFGA BatchCheck call within Check.",
			Buckets: prometheus.DefBuckets,
		}),
	}
	reg.MustRegister(m.checksTotal, m.checkDuration, m.stsExchangeTime, m.resourceMapTime, m.openFGACheckTime)
	return m
}

// RecordCheck implements authz.Metrics.
func (m *CheckRecorder) RecordCheck(result, reason string, duration time.Duration) {
	m.checksTotal.WithLabelValues(result, reason).Inc()
	m.checkDuration.WithLabelValues(result).Observe(duration.Seconds())
}

// ObserveSTSExchange implements authz.Metrics.
func (m *CheckRecorder) ObserveSTSExchange(duration time.Duration) {
	m.stsExchangeTime.Observe(duration.Seconds())
}

// ObserveResourceMap implements authz.Metrics.
func (m *CheckRecorder) ObserveResourceMap(duration time.Duration) {
	m.resourceMapTime.Observe(duration.Seconds())
}

// ObserveOpenFGACheck implements authz.Metrics.
func (m *CheckRecorder) ObserveOpenFGACheck(duration time.Duration) {
	m.openFGACheckTime.Observe(duration.Seconds())
}
