// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package repository

import "time"

// Metrics is the observability seam for repository query operations. A no-op
// default is used unless a real implementation is injected.
type Metrics interface {
	// ObserveQuery records the outcome and duration of a repository call.
	// operation is a fixed method name (e.g. "insert", "claim_batch",
	// "find_candidates"); err is nil on success.
	ObserveQuery(operation string, err error, duration time.Duration)
}

// NoopMetrics is a Metrics implementation that records nothing.
type NoopMetrics struct{}

func (NoopMetrics) ObserveQuery(string, error, time.Duration) {}
