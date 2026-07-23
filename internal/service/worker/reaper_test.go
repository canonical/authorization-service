// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type reclaimRepo struct {
	fakeRepo
	mu           sync.Mutex
	reclaims     []time.Duration
	reclaimCount int64
	reclaimErr   error
	done         chan struct{}
	limit        int
}

func (r *reclaimRepo) ReclaimStale(_ context.Context, staleAfter time.Duration) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reclaims = append(r.reclaims, staleAfter)

	if len(r.reclaims) >= r.limit && r.done != nil {
		close(r.done)
		r.done = nil
	}

	if r.reclaimErr != nil {
		return 0, r.reclaimErr
	}

	return r.reclaimCount, nil
}

func TestReaper_ReclaimsStaleRows(t *testing.T) {
	repo := &reclaimRepo{
		reclaimCount: 3,
		limit:        1,
		done:         make(chan struct{}),
	}
	doneCh := repo.done
	r := NewReaper(repo, 15*time.Minute, 5*time.Millisecond, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- r.Run(ctx) }()

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reaper to tick")
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("reaper returned error: %v", err)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.reclaims) == 0 {
		t.Fatal("expected at least one reclaim call, got 0")
	}
	if repo.reclaims[0] != 15*time.Minute {
		t.Fatalf("expected stale timeout 15m, got %s", repo.reclaims[0])
	}
}

func TestReaper_ErrorStopsLoop(t *testing.T) {
	expectedErr := errors.New("db disconnect")
	repo := &reclaimRepo{
		reclaimErr: expectedErr,
		limit:      1,
		done:       make(chan struct{}),
	}
	r := NewReaper(repo, 10*time.Minute, 5*time.Millisecond, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- r.Run(ctx) }()

	select {
	case err := <-errCh:
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected error %v, got %v", expectedErr, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reaper to fail and exit")
	}
}
