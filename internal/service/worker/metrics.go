// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package worker

import "time"

// Metrics is the observability seam for worker row-processing outcomes. A
// no-op default is used unless a real (e.g. Prometheus-backed) implementation
// is injected.
type Metrics interface {
	// IncProcessed records a row successfully applied to OpenFGA and processed.
	IncProcessed(service string)
	// IncRetry records a transient failure that returned a row for retry.
	IncRetry(service string)
	// IncPermanentFailure records a row moved to 'failed', by service and code.
	IncPermanentFailure(service, messageID, code string)
}

// WorkerMetrics is the observability seam for the Worker's batch-claim loop. A
// no-op default is used unless a real implementation is injected.
type WorkerMetrics interface {
	// ObserveBatchClaimed records the size of a claimed batch (0 when idle).
	ObserveBatchClaimed(size int)
	// ObserveRowDuration records the time taken to process a single row,
	// including ProcessRow's own bookkeeping.
	ObserveRowDuration(duration time.Duration)
}

// ReaperMetrics is the observability seam for the stale-row reaper. A no-op
// default is used unless a real implementation is injected.
type ReaperMetrics interface {
	// ObserveReclaim records the number of rows reclaimed and the duration of a
	// reclaim tick.
	ObserveReclaim(count int64, duration time.Duration)
	// SetLastRunTimestamp records the wall-clock time of the most recent tick,
	// so staleness can be alerted on even when nothing was reclaimed.
	SetLastRunTimestamp()
}

// NoopMetrics is a Metrics, WorkerMetrics and ReaperMetrics implementation
// that records nothing.
type NoopMetrics struct{}

func (NoopMetrics) IncProcessed(string)                        {}
func (NoopMetrics) IncRetry(string)                            {}
func (NoopMetrics) IncPermanentFailure(string, string, string) {}
func (NoopMetrics) ObserveBatchClaimed(int)                    {}
func (NoopMetrics) ObserveRowDuration(time.Duration)           {}
func (NoopMetrics) ObserveReclaim(int64, time.Duration)        {}
func (NoopMetrics) SetLastRunTimestamp()                       {}
