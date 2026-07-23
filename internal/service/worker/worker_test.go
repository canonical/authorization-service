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

	w := NewWorker(repo, proc, 100, 5*time.Millisecond, time.Minute, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- w.Run(ctx) }()

	select {
	case <-proc.done:
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
}

func TestWorker_ClaimError_DoesNotStopLoop(t *testing.T) {
	repo := &claimRepo{claimErr: errors.New("db blip")}
	proc := &recordingProcessor{limit: 1}

	w := NewWorker(repo, proc, 100, 5*time.Millisecond, time.Minute, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Run must return nil (clean shutdown) even though every claim errors.
	if err := w.Run(ctx); err != nil {
		t.Fatalf("expected clean shutdown, got %v", err)
	}
	if len(proc.seen) != 0 {
		t.Fatalf("no rows should be processed when claims fail, got %v", proc.seen)
	}
}
