// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package repository

import (
	"testing"
	"time"
)

// queryMetricsCall captures a single ObserveQuery invocation.
type queryMetricsCall struct {
	operation string
	err       error
}

// fakeQueryMetrics is a hand-rolled Metrics fake recording every ObserveQuery
// call so tests can assert operation label and outcome.
type fakeQueryMetrics struct {
	calls []queryMetricsCall
}

func (f *fakeQueryMetrics) ObserveQuery(operation string, err error, _ time.Duration) {
	f.calls = append(f.calls, queryMetricsCall{operation: operation, err: err})
}

// assertObserveQuery asserts ObserveQuery was called exactly once with the
// given operation and error-presence.
func assertObserveQuery(t *testing.T, m *fakeQueryMetrics, wantOperation string, wantErr bool) {
	t.Helper()
	if len(m.calls) != 1 {
		t.Fatalf("expected exactly 1 ObserveQuery call, got %d: %+v", len(m.calls), m.calls)
	}
	got := m.calls[0]
	if got.operation != wantOperation {
		t.Errorf("ObserveQuery operation = %q, want %q", got.operation, wantOperation)
	}
	if (got.err != nil) != wantErr {
		t.Errorf("ObserveQuery err = %v, want non-nil = %t", got.err, wantErr)
	}
}
