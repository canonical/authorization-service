// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"log/slog"
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
// blocking until ctx is cancelled. The loop is a single goroutine: processing is
// synchronous, so a tick that fires while a batch is still being processed is
// dropped by the ticker (it does not accumulate), and batches never overlap.
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
			w.processBatch(ctx)
		}
	}
}

// processBatch claims and processes at most one batch. Errors are logged rather
// than returned so a single bad row or transient DB blip does not stop the loop.
func (w *Worker) processBatch(ctx context.Context) {
	rows, err := w.repo.ClaimBatch(ctx, w.batchSize, w.retryBackoff)
	if err != nil {
		w.logger.Error("Failed to claim work batch", "error", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	w.logger.Debug("Claimed work batch", "rows", len(rows))
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		if err := w.processor.ProcessRow(ctx, row); err != nil {
			// ProcessRow only returns an error if it could not record the outcome
			// (e.g. DB unavailable). The row stays claimed and will be reclaimed by
			// the stale-row reaper once its processing timeout elapses.
			w.logger.Error("Failed to record processing outcome",
				"service", row.Service, "message_id", row.MessageID, "row_id", row.ID, "error", err)
		}
	}
}
