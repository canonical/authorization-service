// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package authz

import "time"

// Metrics is the observability seam for the external authorization Check RPC.
// A no-op default is used unless a real (e.g. Prometheus-backed) implementation
// is injected.
type Metrics interface {
	// RecordCheck records the terminal outcome of a Check call and its total
	// duration. result is "allow", "deny" or "error"; reason is a fixed enum
	// (e.g. "ok", "no_cookie", "no_session", "openfga_denied") — never raw
	// request data.
	RecordCheck(result, reason string, duration time.Duration)
	// ObserveSTSExchange records the duration of the STS session-exchange call.
	ObserveSTSExchange(duration time.Duration)
	// ObserveResourceMap records the duration of the rule/tuple resolution step.
	ObserveResourceMap(duration time.Duration)
	// ObserveOpenFGACheck records the duration of the OpenFGA BatchCheck call.
	ObserveOpenFGACheck(duration time.Duration)
}

// NoopMetrics is a Metrics implementation that records nothing.
type NoopMetrics struct{}

func (NoopMetrics) RecordCheck(string, string, time.Duration) {}
func (NoopMetrics) ObserveSTSExchange(time.Duration)          {}
func (NoopMetrics) ObserveResourceMap(time.Duration)          {}
func (NoopMetrics) ObserveOpenFGACheck(time.Duration)         {}
