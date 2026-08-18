// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	kafka "github.com/segmentio/kafka-go"
)

func TestIngestRecorder(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewIngestRecorder(reg)

	m.IncIngested("dummy")
	m.IncDuplicate("dummy")
	m.IncPermanentFailure("dummy", "msg-id-ignored", "decode_failed")
	m.ObserveIngestDuration("dummy", 10*time.Millisecond)

	if got := testutil.ToFloat64(m.ingestedTotal.WithLabelValues("dummy")); got != 1 {
		t.Errorf("ingestedTotal{dummy} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.duplicateTotal.WithLabelValues("dummy")); got != 1 {
		t.Errorf("duplicateTotal{dummy} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.failuresTotal.WithLabelValues("dummy", "decode_failed")); got != 1 {
		t.Errorf("failuresTotal{dummy,decode_failed} = %v, want 1", got)
	}

	snap := histogramSnapshot(t, m.ingestDuration.WithLabelValues("dummy"))
	if snap.GetSampleCount() != 1 {
		t.Errorf("ingestDuration{dummy} sample count = %d, want 1", snap.GetSampleCount())
	}
}

func TestKafkaStatsRecorder_Record(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewKafkaStatsRecorder(reg)

	m.Record(kafka.ReaderStats{
		Messages: 10, Bytes: 100, Errors: 1, Timeouts: 0, Rebalances: 0,
		Offset: 10, Lag: 5, QueueLength: 2,
	})
	m.Record(kafka.ReaderStats{
		Messages: 5, Bytes: 50, Errors: 0, Timeouts: 1, Rebalances: 1,
		Offset: 15, Lag: 1, QueueLength: 0,
	})

	// Counters accumulate deltas across calls.
	if got := testutil.ToFloat64(m.messagesTotal); got != 15 {
		t.Errorf("messagesTotal = %v, want 15", got)
	}
	if got := testutil.ToFloat64(m.bytesTotal); got != 150 {
		t.Errorf("bytesTotal = %v, want 150", got)
	}
	if got := testutil.ToFloat64(m.errorsTotal); got != 1 {
		t.Errorf("errorsTotal = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.timeoutsTotal); got != 1 {
		t.Errorf("timeoutsTotal = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.rebalancesTotal); got != 1 {
		t.Errorf("rebalancesTotal = %v, want 1", got)
	}

	// Gauges reflect only the latest snapshot's absolute values.
	if got := testutil.ToFloat64(m.offset); got != 15 {
		t.Errorf("offset = %v, want 15 (latest snapshot)", got)
	}
	if got := testutil.ToFloat64(m.lag); got != 1 {
		t.Errorf("lag = %v, want 1 (latest snapshot)", got)
	}
	if got := testutil.ToFloat64(m.queueLength); got != 0 {
		t.Errorf("queueLength = %v, want 0 (latest snapshot)", got)
	}
}

type fakeStatsProvider struct {
	stats kafka.ReaderStats
	calls chan struct{}
}

func (f *fakeStatsProvider) Stats() kafka.ReaderStats {
	select {
	case f.calls <- struct{}{}:
	default:
	}
	return f.stats
}

func TestPollKafkaStats(t *testing.T) {
	reg := prometheus.NewRegistry()
	recorder := NewKafkaStatsRecorder(reg)
	provider := &fakeStatsProvider{
		stats: kafka.ReaderStats{Messages: 1},
		calls: make(chan struct{}, 10),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- PollKafkaStats(ctx, provider, recorder, 5*time.Millisecond)
	}()

	// Wait for at least two ticks to have fired.
	for i := 0; i < 2; i++ {
		select {
		case <-provider.calls:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for PollKafkaStats to call Stats()")
		}
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("PollKafkaStats returned err = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for PollKafkaStats to return after cancellation")
	}

	if got := testutil.ToFloat64(recorder.messagesTotal); got < 2 {
		t.Errorf("messagesTotal = %v, want >= 2 (at least two ticks recorded)", got)
	}
}
