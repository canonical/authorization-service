// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package worker

// Metrics is the observability seam for worker processing outcomes. A no-op
// default is used unless a real (e.g. OTel-backed) implementation is injected.
type Metrics interface {
	// IncProcessed records a row successfully applied to OpenFGA and processed.
	IncProcessed(service string)
	// IncRetry records a transient failure that returned a row for retry.
	IncRetry(service string)
	// IncPermanentFailure records a row moved to 'failed', by service and code.
	IncPermanentFailure(service, messageID, code string)
}

// NoopMetrics is a Metrics implementation that records nothing.
type NoopMetrics struct{}

func (NoopMetrics) IncProcessed(string)                        {}
func (NoopMetrics) IncRetry(string)                            {}
func (NoopMetrics) IncPermanentFailure(string, string, string) {}
