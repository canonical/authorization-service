// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// CheckRecorder implements authz.Metrics. Cardinality is bounded: result,
// reason, and auth_type are small fixed enums, never raw request data.
type CheckRecorder struct {
	checksTotal      *prometheus.CounterVec
	checkDuration    *prometheus.HistogramVec
	hydraVerifyTime  prometheus.Histogram
	stsExchangeTime  *prometheus.HistogramVec
	resourceMapTime  prometheus.Histogram
	openFGACheckTime prometheus.Histogram
}

// NewCheckRecorder registers the external-authz Check collectors on reg.
func NewCheckRecorder(reg *prometheus.Registry) *CheckRecorder {
	m := &CheckRecorder{
		checksTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "authz_check_total",
			Help: "Total number of external authorization Check calls, by result, reason, and auth_type.",
		}, []string{"result", "reason", "auth_type"}),
		checkDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "authz_check_duration_seconds",
			Help:    "Total duration of an external authorization Check call, by result and auth_type.",
			Buckets: prometheus.DefBuckets,
		}, []string{"result", "auth_type"}),
		hydraVerifyTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "authz_check_hydra_verify_duration_seconds",
			Help:    "Duration of the Ory Hydra token verification within Check.",
			Buckets: prometheus.DefBuckets,
		}),
		stsExchangeTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "authz_check_sts_exchange_duration_seconds",
			Help:    "Duration of the STS exchange call within Check, by exchange_type.",
			Buckets: prometheus.DefBuckets,
		}, []string{"exchange_type"}),
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
	reg.MustRegister(m.checksTotal, m.checkDuration, m.hydraVerifyTime, m.stsExchangeTime, m.resourceMapTime, m.openFGACheckTime)
	return m
}

// RecordCheck implements authz.Metrics.
func (m *CheckRecorder) RecordCheck(result, reason, authType string, duration time.Duration) {
	m.checksTotal.WithLabelValues(result, reason, authType).Inc()
	m.checkDuration.WithLabelValues(result, authType).Observe(duration.Seconds())
}

// ObserveHydraVerify implements authz.Metrics.
func (m *CheckRecorder) ObserveHydraVerify(duration time.Duration) {
	m.hydraVerifyTime.Observe(duration.Seconds())
}

// ObserveSTSExchange implements authz.Metrics.
func (m *CheckRecorder) ObserveSTSExchange(exchangeType string, duration time.Duration) {
	m.stsExchangeTime.WithLabelValues(exchangeType).Observe(duration.Seconds())
}

// ObserveResourceMap implements authz.Metrics.
func (m *CheckRecorder) ObserveResourceMap(duration time.Duration) {
	m.resourceMapTime.Observe(duration.Seconds())
}

// ObserveOpenFGACheck implements authz.Metrics.
func (m *CheckRecorder) ObserveOpenFGACheck(duration time.Duration) {
	m.openFGACheckTime.Observe(duration.Seconds())
}
