// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/canonical/authorization-service/internal/model/permissions"
	"github.com/canonical/authorization-service/internal/repository"
)

// compile-time check that fakeRepo satisfies the repository interface.
var _ repository.PermissionWorkRepository = (*fakeRepo)(nil)

// claimRepo is a fakeRepo variant that returns a fixed batch on the first claim
// and then empties, so the worker loop has bounded work.
type claimRepo struct {
	fakeRepo
	mu       sync.Mutex
	batches  [][]permissions.ClaimedRow
	claimErr error
}

func (r *claimRepo) ClaimBatch(context.Context, int, time.Duration) ([]permissions.ClaimedRow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.claimErr != nil {
		return nil, r.claimErr
	}
	if len(r.batches) == 0 {
		return nil, nil
	}
	batch := r.batches[0]
	r.batches = r.batches[1:]
	return batch, nil
}

// recordingProcessor counts processed rows and signals when the target is met.
type recordingProcessor struct {
	mu    sync.Mutex
	seen  []string
	done  chan struct{}
	limit int
}

func (p *recordingProcessor) ProcessRow(_ context.Context, row permissions.ClaimedRow) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen = append(p.seen, row.ID)
	if len(p.seen) >= p.limit && p.done != nil {
		close(p.done)
		p.done = nil
	}
	return nil
}

func TestWorker_ProcessesClaimedBatch(t *testing.T) {
	repo := &claimRepo{batches: [][]permissions.ClaimedRow{{
		{ID: "row-1", Service: "payments"},
		{ID: "row-2", Service: "payments"},
	}}}
	proc := &recordingProcessor{done: make(chan struct{}), limit: 2}
	metrics := &fakeMetrics{}

	doneCh := proc.done
	w := NewWorker(repo, proc, 100, 5*time.Millisecond, time.Minute, metrics, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- w.Run(ctx) }()

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rows to be processed")
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("worker returned error: %v", err)
	}

	if len(proc.seen) < 2 {
		t.Fatalf("expected at least 2 processed rows, got %v", proc.seen)
	}

	batchSizes := metrics.batchClaimedSizes()
	if len(batchSizes) == 0 || batchSizes[0] != 2 {
		t.Fatalf("expected first ObserveBatchClaimed call with size 2, got %v", batchSizes)
	}
	if got := metrics.rowDurationCount(); got < 2 {
		t.Fatalf("expected ObserveRowDuration called at least twice, got %d", got)
	}
}

func TestWorker_ClaimError_StopsLoop(t *testing.T) {
	expectedErr := errors.New("db blip")
	repo := &claimRepo{claimErr: expectedErr}
	proc := &recordingProcessor{limit: 1}

	w := NewWorker(repo, proc, 100, 5*time.Millisecond, time.Minute, nil, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- w.Run(ctx) }()

	select {
	case err := <-errCh:
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected error %v, got %v", expectedErr, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for worker to fail and exit")
	}

	if len(proc.seen) != 0 {
		t.Fatalf("no rows should be processed when claims fail, got %v", proc.seen)
	}
}

func TestWorker_BatchSortingByEventTime(t *testing.T) {
	t1 := time.Now().Add(-10 * time.Minute)
	t2 := time.Now().Add(-5 * time.Minute)
	t3 := time.Now().Add(-20 * time.Minute)

	repo := &claimRepo{batches: [][]permissions.ClaimedRow{{
		{ID: "row-latest", EventTime: &t2},
		{ID: "row-nil-time", EventTime: nil},
		{ID: "row-earliest", EventTime: &t3},
		{ID: "row-middle", EventTime: &t1},
	}}}

	proc := &recordingProcessor{done: make(chan struct{}), limit: 4}
	doneCh := proc.done

	w := NewWorker(repo, proc, 100, 5*time.Millisecond, time.Minute, nil, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- w.Run(ctx) }()

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rows to be processed")
	}

	cancel()
	_ = <-errCh

	// The order of processing must be:
	// 1. row-nil-time (nil EventTime is treated as older and comes first)
	// 2. row-earliest (t3 = -20m)
	// 3. row-middle (t1 = -10m)
	// 4. row-latest (t2 = -5m)
	expectedOrder := []string{"row-nil-time", "row-earliest", "row-middle", "row-latest"}
	if len(proc.seen) != len(expectedOrder) {
		t.Fatalf("expected %d processed rows, got %d: %v", len(expectedOrder), len(proc.seen), proc.seen)
	}
	for i, id := range expectedOrder {
		if proc.seen[i] != id {
			t.Errorf("at index %d: expected %q, got %q", i, id, proc.seen[i])
		}
	}
}
