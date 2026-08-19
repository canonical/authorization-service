// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"sync"
	"testing"
	"time"
)

// permanentFailureCall captures a single IncPermanentFailure invocation.
type permanentFailureCall struct {
	service   string
	messageID string
	code      string
}

// fakeMetrics is a hand-rolled fake implementing Metrics, WorkerMetrics and
// ReaperMetrics, recording every call so tests can assert instrumentation
// placement and labels. Safe for concurrent use since it is exercised from
// background Run loops while the test goroutine reads it.
type fakeMetrics struct {
	mu sync.Mutex

	processedCalls []string
	retryCalls     []string
	permanentCalls []permanentFailureCall

	batchClaimedCalls []int
	rowDurationCalls  int

	reclaimCounts         []int64
	lastRunTimestampCalls int
}

func (f *fakeMetrics) IncProcessed(service string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.processedCalls = append(f.processedCalls, service)
}

func (f *fakeMetrics) IncRetry(service string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retryCalls = append(f.retryCalls, service)
}

func (f *fakeMetrics) IncPermanentFailure(service, messageID, code string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.permanentCalls = append(f.permanentCalls, permanentFailureCall{service: service, messageID: messageID, code: code})
}

func (f *fakeMetrics) ObserveBatchClaimed(size int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batchClaimedCalls = append(f.batchClaimedCalls, size)
}

func (f *fakeMetrics) ObserveRowDuration(_ time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rowDurationCalls++
}

func (f *fakeMetrics) ObserveReclaim(count int64, _ time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reclaimCounts = append(f.reclaimCounts, count)
}

func (f *fakeMetrics) SetLastRunTimestamp() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastRunTimestampCalls++
}

func (f *fakeMetrics) processedServices() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.processedCalls...)
}

func (f *fakeMetrics) retryServices() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.retryCalls...)
}

func (f *fakeMetrics) permanentFailures() []permanentFailureCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]permanentFailureCall(nil), f.permanentCalls...)
}

func (f *fakeMetrics) batchClaimedSizes() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.batchClaimedCalls...)
}

func (f *fakeMetrics) rowDurationCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rowDurationCalls
}

func (f *fakeMetrics) reclaimedCounts() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.reclaimCounts...)
}

func (f *fakeMetrics) lastRunCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastRunTimestampCalls
}

// assertIncProcessed asserts IncProcessed was called exactly once with wantService.
func assertIncProcessed(t *testing.T, m *fakeMetrics, wantService string) {
	t.Helper()
	got := m.processedServices()
	if len(got) != 1 || got[0] != wantService {
		t.Fatalf("IncProcessed calls = %v, want exactly one call with %q", got, wantService)
	}
}

// assertIncRetry asserts IncRetry was called exactly once with wantService.
func assertIncRetry(t *testing.T, m *fakeMetrics, wantService string) {
	t.Helper()
	got := m.retryServices()
	if len(got) != 1 || got[0] != wantService {
		t.Fatalf("IncRetry calls = %v, want exactly one call with %q", got, wantService)
	}
}

// assertIncPermanentFailure asserts IncPermanentFailure was called exactly once with want.
func assertIncPermanentFailure(t *testing.T, m *fakeMetrics, want permanentFailureCall) {
	t.Helper()
	got := m.permanentFailures()
	if len(got) != 1 || got[0] != want {
		t.Fatalf("IncPermanentFailure calls = %v, want exactly one call %+v", got, want)
	}
}
