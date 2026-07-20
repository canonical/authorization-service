// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package listen

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/authorization-service/internal/model/permissions"
	"github.com/canonical/authorization-service/internal/repository"
)

// PermanentError marks a failure that will never succeed on retry (bad payload,
// validation failure, topic/service mismatch). The consumer should acknowledge
// (commit) such a message rather than redeliver it.
type PermanentError struct {
	Code string
	Err  error
}

func (e *PermanentError) Error() string { return fmt.Sprintf("%s: %v", e.Code, e.Err) }
func (e *PermanentError) Unwrap() error { return e.Err }

func permanent(code string, err error) *PermanentError {
	return &PermanentError{Code: code, Err: err}
}

// IsPermanent reports whether err is a PermanentError.
func IsPermanent(err error) bool {
	var p *PermanentError
	return errors.As(err, &p)
}

// Message is the transport-agnostic view of a consumed Kafka record that the
// Ingestor needs: the source topic, the raw value, and the partition/offset
// provenance persisted for reconciliation.
type Message struct {
	Topic     string
	Partition int
	Offset    int64
	Value     []byte
}

// Ingestor decodes, validates and durably persists a single Kafka message.
type Ingestor interface {
	// Ingest handles one raw message. On success the message has been durably
	// persisted (or was a recognised duplicate). A PermanentError means the
	// message is unprocessable and should be acknowledged; any other error is
	// transient and the message must not be treated as consumed.
	Ingest(ctx context.Context, msg Message) error
}

// Metrics is the observability seam for ingestion outcomes. A no-op default is
// used unless a real implementation is injected.
type Metrics interface {
	// IncPermanentFailure records a permanently-failed message for a service.
	IncPermanentFailure(service, messageID, code string)
}

// NoopMetrics is a Metrics implementation that records nothing.
type NoopMetrics struct{}

func (NoopMetrics) IncPermanentFailure(string, string, string) {}

// IngestionService implements Ingestor: it resolves the source service from the
// topic, decodes and validates the protobuf envelope, and appends a 'received'
// row to the durable work table. It never writes to OpenFGA.
type IngestionService struct {
	registry  *ServiceRegistry
	decoder   *Decoder
	validator *Validator
	repo      repository.PermissionWorkRepository
	metrics   Metrics
	logger    *slog.Logger
	// now is injectable for tests; defaults to time.Now.
	now func() time.Time
}

// NewIngestionService constructs an IngestionService. If metrics is nil a no-op
// implementation is used.
func NewIngestionService(
	registry *ServiceRegistry,
	decoder *Decoder,
	validator *Validator,
	repo repository.PermissionWorkRepository,
	metrics Metrics,
	logger *slog.Logger,
) *IngestionService {
	if metrics == nil {
		metrics = NoopMetrics{}
	}
	return &IngestionService{
		registry:  registry,
		decoder:   decoder,
		validator: validator,
		repo:      repo,
		metrics:   metrics,
		logger:    logger,
		now:       time.Now,
	}
}

// Ingest resolves, decodes, validates and persists a single message.
func (s *IngestionService) Ingest(ctx context.Context, msg Message) error {
	slug, ok := s.registry.ResolveService(msg.Topic)
	if !ok {
		// The consumer only subscribes to registered topics, so an unknown topic
		// is a permanent routing error rather than something to retry.
		return s.permanentFailure("", "", "unknown_topic",
			fmt.Errorf("no federated service resolves topic %q", msg.Topic))
	}

	env, err := s.decoder.Decode(msg.Value)
	if err != nil {
		return s.permanentFailure(slug, "", "decode_failed", err)
	}

	if err := s.validator.Validate(env, slug); err != nil {
		return s.permanentFailure(slug, env.GetMessageId(), "validation_failed", err)
	}

	row := permissions.WorkRow{
		Service:        env.GetService(),
		MessageID:      env.GetMessageId(),
		IdempotencyKey: env.GetIdempotencyKey(),
		Version:        env.GetVersion(),
		EventTime:      optionalTime(env.GetEventTime()),
		IngestionTime:  s.now().UTC(),
		CorrelationID:  optionalString(env.GetCorrelationId()),
		Payload:        msg.Value,
		Partition:      msg.Partition,
		Offset:         msg.Offset,
	}

	switch err := s.repo.Insert(ctx, row); {
	case err == nil:
		s.logger.Debug("Permission update ingested",
			"service", slug, "message_id", env.GetMessageId(), "idempotency_key", env.GetIdempotencyKey())
		return nil
	case errors.Is(err, repository.ErrDuplicate):
		// At-least-once delivery: a duplicate is a successful, idempotent no-op.
		s.logger.Debug("Duplicate permission update ignored",
			"service", slug, "message_id", env.GetMessageId(), "idempotency_key", env.GetIdempotencyKey())
		return nil
	default:
		// Treat any other DB error as transient so the message is redelivered.
		return fmt.Errorf("persist permission update (service=%s message_id=%s): %w",
			slug, env.GetMessageId(), err)
	}
}

// permanentFailure logs, records a metric, and returns a PermanentError.
func (s *IngestionService) permanentFailure(service, messageID, code string, err error) error {
	s.logger.Error("Permanent ingestion failure",
		"service", service, "message_id", messageID, "code", code, "error", err)
	s.metrics.IncPermanentFailure(service, messageID, code)
	return permanent(code, err)
}

// optionalTime converts a protobuf timestamp to a *time.Time, nil when unset.
func optionalTime(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

// optionalString returns nil for the empty string, otherwise a pointer to it.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Decoder decodes raw protobuf bytes into a PermissionUpdateEnvelope.
type Decoder struct{}

// NewDecoder creates a new Decoder.
func NewDecoder() *Decoder {
	return &Decoder{}
}

// Decode unmarshals the envelope, returning an error on malformed input.
func (d *Decoder) Decode(value []byte) (*messagesv1.PermissionUpdateEnvelope, error) {
	var env messagesv1.PermissionUpdateEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		return nil, fmt.Errorf("proto unmarshal: %w", err)
	}
	return &env, nil
}
