// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	fgaSdk "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/authorization-service/internal/model/permissions"
	"github.com/canonical/authorization-service/internal/repository"
	"github.com/canonical/authorization-service/internal/service/listen"
)

func permanent(code string, err error) *permissions.PermanentError {
	return permissions.NewPermanentError(code, err)
}

// TupleApplier applies a batch of tuple writes and deletes to OpenFGA. The
// application must be idempotent (duplicate writes and missing deletes are not
// errors). It is an interface so the Processor's mapping, classification and
// bookkeeping logic can be unit-tested without the OpenFGA SDK; openFGAApplier is
// the production implementation.
type TupleApplier interface {
	ApplyTuples(ctx context.Context, writes []client.ClientTupleKey, deletes []client.ClientTupleKeyWithoutCondition) error
}

// Processor applies a single claimed work row to OpenFGA and records the outcome.
type Processor struct {
	repo        repository.PermissionWorkRepository
	applier     TupleApplier
	decoder     *listen.Decoder
	metrics     Metrics
	maxAttempts int
	logger      *slog.Logger
	// now is injectable for tests; defaults to time.Now.
	now func() time.Time
}

// NewProcessor constructs a Processor. If metrics is nil a no-op is used.
func NewProcessor(
	repo repository.PermissionWorkRepository,
	applier TupleApplier,
	decoder *listen.Decoder,
	maxAttempts int,
	metrics Metrics,
	logger *slog.Logger,
) *Processor {
	if metrics == nil {
		metrics = NoopMetrics{}
	}
	return &Processor{
		repo:        repo,
		applier:     applier,
		decoder:     decoder,
		metrics:     metrics,
		maxAttempts: maxAttempts,
		logger:      logger,
		now:         time.Now,
	}
}

// ProcessRow decodes a claimed row, applies its operations to OpenFGA
// idempotently, and records the outcome. It never returns an error for a row it
// has resolved (processed, retried or failed); it returns an error only if it
// could not even record the outcome, so the caller can surface it.
func (p *Processor) ProcessRow(ctx context.Context, row permissions.ClaimedRow) error {
	writes, deletes, err := p.buildTuples(row)
	if err != nil {
		// Undecodable payload / bad operation: permanent, cannot succeed on retry.
		return p.fail(ctx, row, err)
	}

	if err := p.applier.ApplyTuples(ctx, writes, deletes); err != nil {
		return p.classifyAndRecord(ctx, row, err)
	}

	// OpenFGA write succeeded. Mirror the change locally and mark processed. A
	// failure here happened after the authorisation change was already applied, so
	// the row stays retryable WITHOUT counting against the retry limit (spec §201):
	// a subsequent idempotent re-apply will re-run bookkeeping safely.
	if err := p.repo.RecordProcessed(ctx, row.ID, row.Service, toModelTuples(writes), toModelDeletes(deletes)); err != nil {
		p.logger.Error("Bookkeeping failed after successful OpenFGA write; row remains retryable",
			"service", row.Service, "message_id", row.MessageID, "row_id", row.ID, "error", err)
		p.metrics.IncRetry(row.Service)
		return p.repo.MarkRetry(ctx, row.ID, "bookkeeping_failed", err.Error(), false)
	}

	p.logger.Debug("Permission update processed",
		"service", row.Service, "message_id", row.MessageID, "row_id", row.ID,
		"writes", len(writes), "deletes", len(deletes))
	p.metrics.IncProcessed(row.Service)
	return nil
}

// buildTuples decodes the stored envelope and splits its operations into OpenFGA
// write and delete tuples. The subject/relation/object are already in "type:id"
// form and were validated at ingestion, so the mapping is 1:1.
func (p *Processor) buildTuples(row permissions.ClaimedRow) ([]client.ClientTupleKey, []client.ClientTupleKeyWithoutCondition, error) {
	env, err := p.decoder.Decode(row.Payload)
	if err != nil {
		return nil, nil, permanent("decode_failed", err)
	}

	var (
		writes  []client.ClientTupleKey
		deletes []client.ClientTupleKeyWithoutCondition
	)
	for _, op := range env.GetOperations() {
		switch op.GetOp() {
		case messagesv1.PermissionOp_PERMISSION_OP_WRITE:
			writes = append(writes, client.ClientTupleKey{
				User:     op.GetSubject(),
				Relation: op.GetRelation(),
				Object:   op.GetObject(),
			})
		case messagesv1.PermissionOp_PERMISSION_OP_DELETE:
			deletes = append(deletes, client.ClientTupleKeyWithoutCondition{
				User:     op.GetSubject(),
				Relation: op.GetRelation(),
				Object:   op.GetObject(),
			})
		default:
			return nil, nil, permanent("invalid_operation",
				fmt.Errorf("unsupported permission op %v for message %s", op.GetOp(), env.GetMessageId()))
		}
	}
	return writes, deletes, nil
}

// classifyAndRecord decides whether an OpenFGA write error is permanent or
// transient and records the appropriate outcome.
func (p *Processor) classifyAndRecord(ctx context.Context, row permissions.ClaimedRow, err error) error {
	if isTransient(err) {
		return p.retryOrFail(ctx, row, "openfga_write_failed", err)
	}
	// Not classified as transient: treat as permanent (invalid input, business
	// rule violation, impossible mapping) and fail the row.
	return p.fail(ctx, row, permanent("openfga_write_rejected", err))
}

// retryOrFail returns the row for retry if it still has attempts left, otherwise
// moves it to 'failed'. This is the standard transient-failure path.
func (p *Processor) retryOrFail(ctx context.Context, row permissions.ClaimedRow, code string, err error) error {
	if row.AttemptCount+1 >= p.maxAttempts {
		p.logger.Warn("Retry limit exhausted; marking row failed",
			"service", row.Service, "message_id", row.MessageID, "row_id", row.ID,
			"attempts", row.AttemptCount+1, "error", err)
		p.metrics.IncPermanentFailure(row.Service, row.MessageID, code)
		return p.repo.MarkFailed(ctx, row.ID, code, err.Error())
	}
	p.logger.Info("Transient processing failure; scheduling retry",
		"service", row.Service, "message_id", row.MessageID, "row_id", row.ID,
		"attempt", row.AttemptCount+1, "error", err)
	p.metrics.IncRetry(row.Service)
	return p.repo.MarkRetry(ctx, row.ID, code, err.Error(), true)
}

// fail records a permanent failure: logs, meters, and moves the row to 'failed'.
func (p *Processor) fail(ctx context.Context, row permissions.ClaimedRow, err error) error {
	code := "permanent_failure"
	if perm, ok := permissions.AsPermanent(err); ok {
		code = perm.Code
	}
	p.logger.Error("Permanent processing failure",
		"service", row.Service, "message_id", row.MessageID, "row_id", row.ID,
		"code", code, "error", err)
	p.metrics.IncPermanentFailure(row.Service, row.MessageID, code)
	return p.repo.MarkFailed(ctx, row.ID, code, err.Error())
}

// isTransient reports whether an OpenFGA error is worth retrying: server-side 5xx
// that the SDK flags retryable, rate limiting, and any error not recognised as a
// definitive client-side rejection (network/timeouts fall here). Validation and
// authentication errors are treated as permanent.
func isTransient(err error) bool {
	var internal fgaSdk.FgaApiInternalError
	if errors.As(err, &internal) {
		return internal.ShouldRetry()
	}
	var rate fgaSdk.FgaApiRateLimitExceededError
	if errors.As(err, &rate) {
		return true
	}
	var validation fgaSdk.FgaApiValidationError
	if errors.As(err, &validation) {
		return false
	}
	var auth fgaSdk.FgaApiAuthenticationError
	if errors.As(err, &auth) {
		return false
	}
	// Unknown error (connection refused, context deadline, DNS, etc.): transient.
	return true
}

// toModelTuples converts SDK write tuples to the repository's tuple model.
func toModelTuples(writes []client.ClientTupleKey) []permissions.Tuple {
	if len(writes) == 0 {
		return nil
	}
	out := make([]permissions.Tuple, len(writes))
	for i, w := range writes {
		out[i] = permissions.Tuple{Subject: w.User, Relation: w.Relation, Object: w.Object}
	}
	return out
}

// toModelDeletes converts SDK delete tuples to the repository's tuple model.
func toModelDeletes(deletes []client.ClientTupleKeyWithoutCondition) []permissions.Tuple {
	if len(deletes) == 0 {
		return nil
	}
	out := make([]permissions.Tuple, len(deletes))
	for i, d := range deletes {
		out[i] = permissions.Tuple{Subject: d.User, Relation: d.Relation, Object: d.Object}
	}
	return out
}
