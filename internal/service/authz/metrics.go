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
	// (e.g. "ok", "no_credentials", "conflicting_credentials", "no_cookie", "no_session", "openfga_denied") — never raw
	// request data; authType is "cookie", "client_credentials", or "none".
	RecordCheck(result, reason, authType string, duration time.Duration)
	// ObserveHydraVerify records the duration of the Ory Hydra token verification.
	ObserveHydraVerify(duration time.Duration)
	// ObserveSTSExchange records the duration of the STS exchange call, partitioned by exchangeType ("session" or "token").
	ObserveSTSExchange(exchangeType string, duration time.Duration)
	// ObserveResourceMap records the duration of the rule/tuple resolution step.
	ObserveResourceMap(duration time.Duration)
	// ObserveOpenFGACheck records the duration of the OpenFGA BatchCheck call.
	ObserveOpenFGACheck(duration time.Duration)
}

// NoopMetrics is a Metrics implementation that records nothing.
type NoopMetrics struct{}

func (NoopMetrics) RecordCheck(string, string, string, time.Duration) {}
func (NoopMetrics) ObserveHydraVerify(time.Duration)                  {}
func (NoopMetrics) ObserveSTSExchange(string, time.Duration)          {}
func (NoopMetrics) ObserveResourceMap(time.Duration)                  {}
func (NoopMetrics) ObserveOpenFGACheck(time.Duration)                 {}
