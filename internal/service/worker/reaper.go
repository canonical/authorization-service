// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/canonical/authorization-service/internal/repository"
)

// Reaper periodically scans the work table for rows stuck in 'processing'
// state for longer than a configured timeout and returns them to 'received'.
type Reaper struct {
	repo         repository.PermissionWorkRepository
	staleTimeout time.Duration
	interval     time.Duration
	metrics      ReaperMetrics
	logger       *slog.Logger
}

// NewReaper constructs a Reaper. If metrics is nil a no-op is used.
func NewReaper(
	repo repository.PermissionWorkRepository,
	staleTimeout time.Duration,
	interval time.Duration,
	metrics ReaperMetrics,
	logger *slog.Logger,
) *Reaper {
	if metrics == nil {
		metrics = NoopMetrics{}
	}
	return &Reaper{
		repo:         repo,
		staleTimeout: staleTimeout,
		interval:     interval,
		metrics:      metrics,
		logger:       logger,
	}
}

// Run starts the reaper ticker loop, which periodically reclaims stale rows.
// It runs in a background goroutine until the context is cancelled or an error occurs.
func (r *Reaper) Run(ctx context.Context) error {
	r.logger.Info("Starting stale-row reaper background loop",
		"stale_timeout", r.staleTimeout.String(),
		"interval", r.interval.String())

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Stopping stale-row reaper background loop")
			return nil
		case <-ticker.C:
			if err := r.reclaim(ctx); err != nil {
				return err
			}
		}
	}
}

func (r *Reaper) reclaim(ctx context.Context) error {
	r.logger.Debug("Reaper checking for stale rows")
	r.metrics.SetLastRunTimestamp()

	start := time.Now()
	count, err := r.repo.ReclaimStale(ctx, r.staleTimeout)
	if err != nil {
		r.logger.Error("Failed to reclaim stale rows", "error", err)
		return err
	}
	r.metrics.ObserveReclaim(count, time.Since(start))
	if count > 0 {
		r.logger.Info("Reclaimed stale permission-update rows",
			"count", count,
			"stale_timeout", r.staleTimeout.String())
	}
	return nil
}
