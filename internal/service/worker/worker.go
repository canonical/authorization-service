// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/canonical/authorization-service/internal/model/permissions"
	"github.com/canonical/authorization-service/internal/repository"
)

// RowProcessor applies a single claimed row. Processor implements it; the
// interface keeps the Worker loop testable in isolation.
type RowProcessor interface {
	ProcessRow(ctx context.Context, row permissions.ClaimedRow) error
}

// Worker periodically claims batches of permission-update rows and applies each
// one to OpenFGA. Rows are processed serially within an instance; horizontal
// scaling is achieved by running multiple instances (FOR UPDATE SKIP LOCKED
// assigns disjoint rows), exactly as the listener scales.
type Worker struct {
	repo         repository.PermissionWorkRepository
	processor    RowProcessor
	batchSize    int
	pollInterval time.Duration
	retryBackoff time.Duration
	logger       *slog.Logger
}

// NewWorker constructs a Worker.
func NewWorker(
	repo repository.PermissionWorkRepository,
	processor RowProcessor,
	batchSize int,
	pollInterval time.Duration,
	retryBackoff time.Duration,
	logger *slog.Logger,
) *Worker {
	return &Worker{
		repo:         repo,
		processor:    processor,
		batchSize:    batchSize,
		pollInterval: pollInterval,
		retryBackoff: retryBackoff,
		logger:       logger,
	}
}

// Run polls the work table on pollInterval and processes one batch per tick,
// blocking until ctx is cancelled or an error occurs.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("Starting permission-update worker",
		"batch_size", w.batchSize, "poll_interval", w.pollInterval.String())

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.processBatch(ctx); err != nil {
				return err
			}
		}
	}
}

// processBatch claims and processes at most one batch.
func (w *Worker) processBatch(ctx context.Context) error {
	rows, err := w.repo.ClaimBatch(ctx, w.batchSize, w.retryBackoff)
	if err != nil {
		w.logger.Error("Failed to claim work batch", "error", err)
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	w.logger.Debug("Claimed work batch", "rows", len(rows))

	// Sort the batch by event_timestamp (EventTime) as a best-effort local ordering step.
	sort.Slice(rows, func(i, j int) bool {
		ti := rows[i].EventTime
		tj := rows[j].EventTime
		if ti == nil && tj == nil {
			return false
		}
		if ti == nil {
			return true // Treat nil EventTime as older
		}
		if tj == nil {
			return false
		}
		return ti.Before(*tj)
	})

	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := w.processor.ProcessRow(ctx, row); err != nil {
			// ProcessRow only returns an error if it could not record the outcome
			// (e.g. DB unavailable). The row stays claimed and will be reclaimed by
			// the stale-row reaper once its processing timeout elapses.
			w.logger.Error("Failed to record processing outcome",
				"service", row.Service, "message_id", row.MessageID, "row_id", row.ID, "error", err)
			return err
		}
	}
	return nil
}
