// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ProcessorRecorder implements worker.Metrics. Cardinality is bounded: code is
// a fixed set of permanent-failure codes; the message ID is deliberately never
// used as a label.
type ProcessorRecorder struct {
	processedTotal *prometheus.CounterVec
	retriedTotal   *prometheus.CounterVec
	failuresTotal  *prometheus.CounterVec
}

// NewProcessorRecorder registers the row-processing collectors on reg.
func NewProcessorRecorder(reg *prometheus.Registry) *ProcessorRecorder {
	m := &ProcessorRecorder{
		processedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "authz_worker_processed_total",
			Help: "Total number of permission-update rows successfully applied to OpenFGA, by service.",
		}, []string{"service"}),
		retriedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "authz_worker_retried_total",
			Help: "Total number of transient row-processing failures returned for retry, by service.",
		}, []string{"service"}),
		failuresTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "authz_worker_permanent_failures_total",
			Help: "Total number of rows moved to 'failed', by service and error code.",
		}, []string{"service", "code"}),
	}
	reg.MustRegister(m.processedTotal, m.retriedTotal, m.failuresTotal)
	return m
}

// IncProcessed implements worker.Metrics.
func (m *ProcessorRecorder) IncProcessed(service string) {
	m.processedTotal.WithLabelValues(service).Inc()
}

// IncRetry implements worker.Metrics.
func (m *ProcessorRecorder) IncRetry(service string) {
	m.retriedTotal.WithLabelValues(service).Inc()
}

// IncPermanentFailure implements worker.Metrics. messageID is intentionally
// dropped from the label set to avoid unbounded cardinality.
func (m *ProcessorRecorder) IncPermanentFailure(service, _, code string) {
	m.failuresTotal.WithLabelValues(service, code).Inc()
}

// WorkerLoopRecorder implements worker.WorkerMetrics.
type WorkerLoopRecorder struct {
	batchClaimed prometheus.Histogram
	rowDuration  prometheus.Histogram
}

// NewWorkerLoopRecorder registers the Worker batch-claim-loop collectors on reg.
func NewWorkerLoopRecorder(reg *prometheus.Registry) *WorkerLoopRecorder {
	m := &WorkerLoopRecorder{
		batchClaimed: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "authz_worker_batch_claimed_size",
			Help:    "Number of rows claimed per poll tick (0 when idle).",
			Buckets: []float64{0, 1, 5, 10, 25, 50, 100, 250, 500},
		}),
		rowDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "authz_worker_row_duration_seconds",
			Help:    "Duration of processing a single claimed row.",
			Buckets: prometheus.DefBuckets,
		}),
	}
	reg.MustRegister(m.batchClaimed, m.rowDuration)
	return m
}

// ObserveBatchClaimed implements worker.WorkerMetrics.
func (m *WorkerLoopRecorder) ObserveBatchClaimed(size int) {
	m.batchClaimed.Observe(float64(size))
}

// ObserveRowDuration implements worker.WorkerMetrics.
func (m *WorkerLoopRecorder) ObserveRowDuration(duration time.Duration) {
	m.rowDuration.Observe(duration.Seconds())
}

// ReaperRecorder implements worker.ReaperMetrics.
type ReaperRecorder struct {
	reclaimedTotal  prometheus.Counter
	reclaimDuration prometheus.Histogram
	lastRunSeconds  prometheus.Gauge
}

// NewReaperRecorder registers the stale-row reaper collectors on reg.
func NewReaperRecorder(reg *prometheus.Registry) *ReaperRecorder {
	m := &ReaperRecorder{
		reclaimedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "authz_reaper_reclaimed_total",
			Help: "Total number of stale rows returned to 'received' by the reaper.",
		}),
		reclaimDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "authz_reaper_reclaim_duration_seconds",
			Help:    "Duration of a single reaper reclaim tick.",
			Buckets: prometheus.DefBuckets,
		}),
		lastRunSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "authz_reaper_last_run_timestamp_seconds",
			Help: "Unix timestamp of the reaper's most recent tick, so staleness can be alerted on.",
		}),
	}
	reg.MustRegister(m.reclaimedTotal, m.reclaimDuration, m.lastRunSeconds)
	return m
}

// ObserveReclaim implements worker.ReaperMetrics.
func (m *ReaperRecorder) ObserveReclaim(count int64, duration time.Duration) {
	m.reclaimedTotal.Add(float64(count))
	m.reclaimDuration.Observe(duration.Seconds())
}

// SetLastRunTimestamp implements worker.ReaperMetrics.
func (m *ReaperRecorder) SetLastRunTimestamp() {
	m.lastRunSeconds.Set(float64(time.Now().Unix()))
}
