// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package listen

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/authorization-service/internal/model/permissions"
	"github.com/canonical/authorization-service/internal/repository"
)

// fakeRepo is a hand-rolled PermissionWorkRepository for ingestor tests. Only
// Insert is exercised here; the worker-side methods are stubbed to satisfy the
// interface.
type fakeRepo struct {
	inserted []permissions.WorkRow
	err      error
}

func (f *fakeRepo) Insert(_ context.Context, row permissions.WorkRow) error {
	if f.err != nil {
		return f.err
	}
	f.inserted = append(f.inserted, row)
	return nil
}

func (f *fakeRepo) ClaimBatch(context.Context, int, time.Duration) ([]permissions.ClaimedRow, error) {
	return nil, nil
}

func (f *fakeRepo) RecordProcessed(context.Context, string, string, []permissions.Tuple, []permissions.Tuple) error {
	return nil
}

func (f *fakeRepo) MarkFailed(context.Context, string, string, string) error { return nil }

func (f *fakeRepo) MarkRetry(context.Context, string, string, string, bool) error { return nil }

func (f *fakeRepo) ReclaimStale(context.Context, time.Duration) (int64, error) { return 0, nil }

// countingMetrics records permanent-failure calls.
type countingMetrics struct {
	calls []struct{ service, messageID, code string }
}

func (m *countingMetrics) IncPermanentFailure(service, messageID, code string) {
	m.calls = append(m.calls, struct{ service, messageID, code string }{service, messageID, code})
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newIngestor(t *testing.T, repo repository.PermissionWorkRepository, m Metrics) *IngestionService {
	t.Helper()
	registry, err := NewServiceRegistry([]string{"payments"})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return NewIngestionService(registry, NewDecoder(), NewValidator(), repo, m, testLogger())
}

func marshalEnvelope(t *testing.T, env *messagesv1.PermissionUpdateEnvelope) []byte {
	t.Helper()
	b, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// msg builds a Message for the given topic and raw value.
func msg(topic string, value []byte) Message {
	return Message{Topic: topic, Partition: 0, Offset: 0, Value: value}
}

func TestIngest_HappyPath(t *testing.T) {
	repo := &fakeRepo{}
	ing := newIngestor(t, repo, nil)

	value := marshalEnvelope(t, validEnvelope())
	if err := ing.Ingest(context.Background(), msg("payments.permissions", value)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.inserted) != 1 {
		t.Fatalf("expected 1 row inserted, got %d", len(repo.inserted))
	}
	row := repo.inserted[0]
	if row.Service != "payments" || row.IdempotencyKey != "idem-1" {
		t.Errorf("unexpected row %+v", row)
	}
	if row.IngestionTime.IsZero() {
		t.Errorf("ingestion time not set")
	}
	if len(row.Payload) == 0 {
		t.Errorf("payload not retained")
	}
}

func TestIngest_DuplicateIsSuccess(t *testing.T) {
	repo := &fakeRepo{err: repository.ErrDuplicate}
	ing := newIngestor(t, repo, nil)

	value := marshalEnvelope(t, validEnvelope())
	if err := ing.Ingest(context.Background(), msg("payments.permissions", value)); err != nil {
		t.Fatalf("duplicate should be nil, got %v", err)
	}
}

func TestIngest_DBErrorIsTransient(t *testing.T) {
	repo := &fakeRepo{err: errors.New("connection refused")}
	ing := newIngestor(t, repo, nil)

	value := marshalEnvelope(t, validEnvelope())
	err := ing.Ingest(context.Background(), msg("payments.permissions", value))
	if err == nil {
		t.Fatal("expected transient error, got nil")
	}
	if permissions.IsPermanent(err) {
		t.Errorf("DB error must be transient, got permanent: %v", err)
	}
}

func TestIngest_DecodeErrorIsPermanent(t *testing.T) {
	repo := &fakeRepo{}
	metrics := &countingMetrics{}
	ing := newIngestor(t, repo, metrics)

	err := ing.Ingest(context.Background(), msg("payments.permissions", []byte("not-a-proto")))
	if err == nil || !permissions.IsPermanent(err) {
		t.Fatalf("expected permanent error, got %v", err)
	}
	if len(metrics.calls) != 1 || metrics.calls[0].code != "decode_failed" {
		t.Errorf("expected decode_failed metric, got %+v", metrics.calls)
	}
	if len(repo.inserted) != 0 {
		t.Errorf("nothing should be persisted on decode failure")
	}
}

func TestIngest_ServiceMismatchIsPermanent(t *testing.T) {
	repo := &fakeRepo{}
	metrics := &countingMetrics{}
	ing := newIngestor(t, repo, metrics)

	env := validEnvelope()
	env.Service = "invoicing" // mismatches the payments.permissions topic
	value := marshalEnvelope(t, env)

	err := ing.Ingest(context.Background(), msg("payments.permissions", value))
	if err == nil || !permissions.IsPermanent(err) {
		t.Fatalf("expected permanent error, got %v", err)
	}
	if len(metrics.calls) != 1 || metrics.calls[0].code != "validation_failed" {
		t.Errorf("expected validation_failed metric, got %+v", metrics.calls)
	}
}

func TestIngest_UnknownTopicIsPermanent(t *testing.T) {
	repo := &fakeRepo{}
	ing := newIngestor(t, repo, nil)

	value := marshalEnvelope(t, validEnvelope())
	err := ing.Ingest(context.Background(), msg("other.permissions", value))
	if err == nil || !permissions.IsPermanent(err) {
		t.Fatalf("expected permanent error for unknown topic, got %v", err)
	}
}
