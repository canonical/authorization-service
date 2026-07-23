// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"

	"github.com/openfga/go-sdk/client"

	"github.com/canonical/authorization-service/internal/integration/openfga"
)

// openFGAApplier is the production TupleApplier: it issues a single transactional
// Write to OpenFGA carrying the batch of writes and deletes for one work row.
//
// Idempotency is delegated to the server via the conflict options: duplicate
// writes and missing deletes are ignored rather than erroring, so re-processing
// the same logical event (on retry or stale-row reclaim) never produces an
// inconsistent tuple state. The authorization model ID is specified explicitly,
// as recommended for consistency and performance.
type openFGAApplier struct {
	fga     openfga.OpenFGAClientInterface
	modelID string
}

// NewOpenFGAApplier constructs the production TupleApplier.
func NewOpenFGAApplier(fga openfga.OpenFGAClientInterface, modelID string) TupleApplier {
	return &openFGAApplier{fga: fga, modelID: modelID}
}

// ApplyTuples applies the batch to OpenFGA. An empty batch is a no-op.
func (a *openFGAApplier) ApplyTuples(ctx context.Context, writes []client.ClientTupleKey, deletes []client.ClientTupleKeyWithoutCondition) error {
	if len(writes) == 0 && len(deletes) == 0 {
		return nil
	}

	modelID := a.modelID
	req := a.fga.Write(ctx).
		Body(client.ClientWriteRequest{Writes: writes, Deletes: deletes}).
		Options(client.ClientWriteOptions{
			AuthorizationModelId: &modelID,
			Conflict: client.ClientWriteConflictOptions{
				OnDuplicateWrites: client.CLIENT_WRITE_REQUEST_ON_DUPLICATE_WRITES_IGNORE,
				OnMissingDeletes:  client.CLIENT_WRITE_REQUEST_ON_MISSING_DELETES_IGNORE,
			},
		})
	if _, err := a.fga.WriteExecute(req); err != nil {
		return err
	}
	return nil
}
