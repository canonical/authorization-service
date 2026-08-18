// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

import "time"

// Metrics is the observability seam for permission registration operations. A
// no-op default is used unless a real implementation is injected.
type Metrics interface {
	// ObserveOperation records the outcome and duration of a Register/Get/Delete
	// call. operation is one of "register", "get", "delete"; err is nil on
	// success.
	ObserveOperation(operation string, err error, duration time.Duration)
}

// NoopMetrics is a Metrics implementation that records nothing.
type NoopMetrics struct{}

func (NoopMetrics) ObserveOperation(string, error, time.Duration) {}
