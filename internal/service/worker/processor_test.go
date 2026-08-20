// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	fgaSdk "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	"google.golang.org/protobuf/proto"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/authorization-service/internal/logging"
	"github.com/canonical/authorization-service/internal/model/permissions"
	"github.com/canonical/authorization-service/internal/service/listen"
	"github.com/canonical/authorization-service/internal/testutil"
)

// listenDecoder returns a real Decoder; decoding is pure so no mock is needed.
func listenDecoder() *listen.Decoder { return listen.NewDecoder() }

// fakeRepo records the lifecycle calls the processor makes. Only the methods the
// processor uses are meaningful; the rest satisfy the interface.
type fakeRepo struct {
	processedID      string
	processedService string
	processedWrites  []permissions.Tuple
	processedDeletes []permissions.Tuple

	failedID   string
	failedCode string

	retryID         string
	retryCode       string
	retryIncAttempt bool

	recordProcessedErr error

	calls []string
}

func (f *fakeRepo) Insert(context.Context, permissions.WorkRow) error { return nil }

func (f *fakeRepo) ClaimBatch(context.Context, int, time.Duration) ([]permissions.ClaimedRow, error) {
	return nil, nil
}

func (f *fakeRepo) RecordProcessed(_ context.Context, id, service string, writes, deletes []permissions.Tuple) error {
	f.calls = append(f.calls, "RecordProcessed")
	if f.recordProcessedErr != nil {
		return f.recordProcessedErr
	}
	f.processedID = id
	f.processedService = service
	f.processedWrites = writes
	f.processedDeletes = deletes
	return nil
}

func (f *fakeRepo) MarkFailed(_ context.Context, id, errCode, _ string) error {
	f.calls = append(f.calls, "MarkFailed")
	f.failedID = id
	f.failedCode = errCode
	return nil
}

func (f *fakeRepo) MarkRetry(_ context.Context, id, errCode, _ string, incAttempt bool) error {
	f.calls = append(f.calls, "MarkRetry")
	f.retryID = id
	f.retryCode = errCode
	f.retryIncAttempt = incAttempt
	return nil
}

func (f *fakeRepo) ReclaimStale(context.Context, time.Duration) (int64, error) { return 0, nil }

// fakeApplier records the tuples applied and returns a configurable error.
type fakeApplier struct {
	err     error
	writes  []client.ClientTupleKey
	deletes []client.ClientTupleKeyWithoutCondition
	called  bool
}

func (a *fakeApplier) ApplyTuples(_ context.Context, writes []client.ClientTupleKey, deletes []client.ClientTupleKeyWithoutCondition) error {
	a.called = true
	a.writes = writes
	a.deletes = deletes
	return a.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// debugCapturingLogger is like testutil.CapturingLogger but captures Debug
// level too, since the success-path log line is logged at Debug.
func debugCapturingLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	handler := logging.NewTraceHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return slog.New(handler), &buf
}

// decodeLogLines decodes every JSON log line captured in buf.
func decodeLogLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("failed to decode log line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

// findLogLine returns the single log line with the given msg, failing the
// test if there isn't exactly one.
func findLogLine(t *testing.T, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, entry := range decodeLogLines(t, buf) {
		if entry["msg"] == msg {
			found = append(found, entry)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly 1 %q log line, got %d", msg, len(found))
	}
	return found[0]
}

// encodeEnvelope builds a valid protobuf payload with the given operations.
func encodeEnvelope(t *testing.T, ops ...*messagesv1.PermissionOperation) []byte {
	t.Helper()
	env := &messagesv1.PermissionUpdateEnvelope{
		Service:    "payments",
		MessageId:  "msg-1",
		Operations: ops,
	}
	b, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("failed to marshal envelope: %v", err)
	}
	return b
}

func writeOp(subject, relation, object string) *messagesv1.PermissionOperation {
	return &messagesv1.PermissionOperation{
		Op:       messagesv1.PermissionOp_PERMISSION_OP_WRITE,
		Subject:  subject,
		Relation: relation,
		Object:   object,
	}
}

func deleteOp(subject, relation, object string) *messagesv1.PermissionOperation {
	return &messagesv1.PermissionOperation{
		Op:       messagesv1.PermissionOp_PERMISSION_OP_DELETE,
		Subject:  subject,
		Relation: relation,
		Object:   object,
	}
}

func newTestProcessor(repo *fakeRepo, applier *fakeApplier, maxAttempts int, metrics Metrics) *Processor {
	return NewProcessor(repo, applier, listenDecoder(), maxAttempts, false, metrics, testLogger())
}

func TestProcessRow_Success_Multitenancy(t *testing.T) {
	repo := &fakeRepo{}
	applier := &fakeApplier{}
	metrics := &fakeMetrics{}
	p := NewProcessor(repo, applier, listenDecoder(), 5, true, metrics, testLogger())

	row := permissions.ClaimedRow{
		ID:      "row-1",
		Service: "payments",
		Payload: encodeEnvelope(t,
			writeOp("user:u1", "viewer", "doc:d1"),
		),
	}
	if err := p.ProcessRow(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !applier.called || len(applier.writes) != 1 {
		t.Fatalf("applier not called with expected writes: %+v", applier)
	}
	writeTuple := applier.writes[0]
	if writeTuple.User != "user:u1" || writeTuple.Relation != "viewer" || writeTuple.Object != "doc:d1" {
		t.Fatalf("unexpected mapped tuple: %+v", writeTuple)
	}
	if writeTuple.Condition == nil {
		t.Fatal("expected tuple to have a condition but got nil")
	}
	if writeTuple.Condition.Name != "tenant_match" {
		t.Errorf("expected condition name 'tenant_match', got %q", writeTuple.Condition.Name)
	}
	ctxMap := *writeTuple.Condition.Context
	if ctxMap["tenant"] != "payments" {
		t.Errorf("expected condition context tenant to be 'payments', got %v", ctxMap["tenant"])
	}
	assertIncProcessed(t, metrics, "payments")
}

func TestProcessRow_Success_WritesAndDeletes(t *testing.T) {
	repo := &fakeRepo{}
	applier := &fakeApplier{}
	metrics := &fakeMetrics{}
	p := newTestProcessor(repo, applier, 5, metrics)

	row := permissions.ClaimedRow{
		ID:      "row-1",
		Service: "payments",
		Payload: encodeEnvelope(t,
			writeOp("user:u1", "viewer", "doc:d1"),
			deleteOp("user:u2", "editor", "doc:d2"),
		),
	}
	if err := p.ProcessRow(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !applier.called || len(applier.writes) != 1 || len(applier.deletes) != 1 {
		t.Fatalf("applier not called with expected tuples: %+v", applier)
	}
	if applier.writes[0].User != "user:u1" || applier.deletes[0].Object != "doc:d2" {
		t.Fatalf("unexpected mapped tuples: %+v", applier)
	}
	if repo.processedID != "row-1" || repo.processedService != "payments" {
		t.Fatalf("RecordProcessed not called correctly: %+v", repo)
	}
	if len(repo.processedWrites) != 1 || len(repo.processedDeletes) != 1 {
		t.Fatalf("bookkeeping tuples mismatch: %+v", repo)
	}
	assertIncProcessed(t, metrics, "payments")
}

func TestProcessRow_DecodeFailure_IsPermanent(t *testing.T) {
	repo := &fakeRepo{}
	applier := &fakeApplier{}
	metrics := &fakeMetrics{}
	p := newTestProcessor(repo, applier, 5, metrics)

	row := permissions.ClaimedRow{ID: "row-1", Service: "payments", Payload: []byte{0xff, 0xff, 0xff}}
	if err := p.ProcessRow(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if applier.called {
		t.Fatal("applier should not be called on decode failure")
	}
	if repo.failedID != "row-1" || repo.failedCode != "decode_failed" {
		t.Fatalf("expected permanent failure with decode_failed, got %+v", repo)
	}
	assertIncPermanentFailure(t, metrics, permanentFailureCall{service: "payments", messageID: "", code: "decode_failed"})
}

func TestProcessRow_TransientFGAError_Retries(t *testing.T) {
	repo := &fakeRepo{}
	applier := &fakeApplier{err: fgaSdk.FgaApiRateLimitExceededError{}}
	metrics := &fakeMetrics{}
	p := newTestProcessor(repo, applier, 5, metrics)

	row := permissions.ClaimedRow{
		ID:           "row-1",
		Service:      "payments",
		AttemptCount: 1,
		Payload:      encodeEnvelope(t, writeOp("user:u1", "viewer", "doc:d1")),
	}
	if err := p.ProcessRow(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.retryID != "row-1" || !repo.retryIncAttempt {
		t.Fatalf("expected retry with attempt increment, got %+v", repo)
	}
	if repo.failedID != "" {
		t.Fatalf("row should not be failed on transient error: %+v", repo)
	}
	assertIncRetry(t, metrics, "payments")
}

func TestProcessRow_TransientButRetriesExhausted_Fails(t *testing.T) {
	repo := &fakeRepo{}
	applier := &fakeApplier{err: fgaSdk.FgaApiRateLimitExceededError{}}
	metrics := &fakeMetrics{}
	p := newTestProcessor(repo, applier, 3, metrics)

	// AttemptCount 2, +1 = 3 == maxAttempts → fail.
	row := permissions.ClaimedRow{
		ID:           "row-1",
		Service:      "payments",
		AttemptCount: 2,
		Payload:      encodeEnvelope(t, writeOp("user:u1", "viewer", "doc:d1")),
	}
	if err := p.ProcessRow(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.failedID != "row-1" || repo.failedCode != "openfga_write_failed" {
		t.Fatalf("expected failed after exhausting retries, got %+v", repo)
	}
	if repo.retryID != "" {
		t.Fatalf("row should not be retried when exhausted: %+v", repo)
	}
	assertIncPermanentFailure(t, metrics, permanentFailureCall{service: "payments", messageID: "", code: "openfga_write_failed"})
}

func TestProcessRow_ValidationError_IsPermanent(t *testing.T) {
	repo := &fakeRepo{}
	applier := &fakeApplier{err: fgaSdk.FgaApiValidationError{}}
	metrics := &fakeMetrics{}
	p := newTestProcessor(repo, applier, 5, metrics)

	row := permissions.ClaimedRow{
		ID:      "row-1",
		Service: "payments",
		Payload: encodeEnvelope(t, writeOp("user:u1", "viewer", "doc:d1")),
	}
	if err := p.ProcessRow(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.failedID != "row-1" || repo.failedCode != "openfga_write_rejected" {
		t.Fatalf("expected permanent failure on validation error, got %+v", repo)
	}
	if repo.retryID != "" {
		t.Fatalf("validation error must not be retried: %+v", repo)
	}
	assertIncPermanentFailure(t, metrics, permanentFailureCall{service: "payments", messageID: "", code: "openfga_write_rejected"})
}

func TestProcessRow_BookkeepingFailure_RetriesWithoutAttemptIncrement(t *testing.T) {
	repo := &fakeRepo{recordProcessedErr: errors.New("db down")}
	applier := &fakeApplier{}
	metrics := &fakeMetrics{}
	p := newTestProcessor(repo, applier, 5, metrics)

	row := permissions.ClaimedRow{
		ID:           "row-1",
		Service:      "payments",
		AttemptCount: 4, // near the limit: must still not fail
		Payload:      encodeEnvelope(t, writeOp("user:u1", "viewer", "doc:d1")),
	}
	if err := p.ProcessRow(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.retryID != "row-1" || repo.retryIncAttempt {
		t.Fatalf("expected retry WITHOUT attempt increment after bookkeeping failure, got %+v", repo)
	}
	if repo.failedID != "" {
		t.Fatalf("row must not be failed after a successful OpenFGA write: %+v", repo)
	}
	if repo.retryCode != "bookkeeping_failed" {
		t.Fatalf("expected bookkeeping_failed code, got %q", repo.retryCode)
	}
	assertIncRetry(t, metrics, "payments")
}

func TestProcessRow_InvalidOperation_IsPermanent(t *testing.T) {
	repo := &fakeRepo{}
	applier := &fakeApplier{}
	metrics := &fakeMetrics{}
	p := newTestProcessor(repo, applier, 5, metrics)

	// An operation with an unspecified op type cannot be mapped.
	row := permissions.ClaimedRow{
		ID:      "row-1",
		Service: "payments",
		Payload: encodeEnvelope(t, &messagesv1.PermissionOperation{
			Op:       messagesv1.PermissionOp_PERMISSION_OP_UNSPECIFIED,
			Subject:  "user:u1",
			Relation: "viewer",
			Object:   "doc:d1",
		}),
	}
	if err := p.ProcessRow(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if applier.called {
		t.Fatal("applier should not be called when an operation cannot be mapped")
	}
	if repo.failedID != "row-1" || repo.failedCode != "invalid_operation" {
		t.Fatalf("expected permanent failure invalid_operation, got %+v", repo)
	}
	assertIncPermanentFailure(t, metrics, permanentFailureCall{service: "payments", messageID: "", code: "invalid_operation"})
}

func TestProcessRow_LogsCorrelationID_OnSuccess(t *testing.T) {
	repo := &fakeRepo{}
	applier := &fakeApplier{}
	metrics := &fakeMetrics{}
	logger, buf := debugCapturingLogger(t)
	p := NewProcessor(repo, applier, listenDecoder(), 5, false, metrics, logger)

	corrID := "corr-1"
	row := permissions.ClaimedRow{
		ID:            "row-1",
		Service:       "payments",
		CorrelationID: &corrID,
		Payload:       encodeEnvelope(t, writeOp("user:u1", "viewer", "doc:d1")),
	}
	if err := p.ProcessRow(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entry := findLogLine(t, buf, "Permission update processed")
	if got := entry["correlation_id"]; got != corrID {
		t.Errorf("expected correlation_id %q, got %v", corrID, got)
	}
}

func TestProcessRow_LogsCorrelationID_OnPermanentFailure(t *testing.T) {
	repo := &fakeRepo{}
	applier := &fakeApplier{}
	metrics := &fakeMetrics{}
	logger, buf := testutil.CapturingLogger(t)
	p := NewProcessor(repo, applier, listenDecoder(), 5, false, metrics, logger)

	corrID := "corr-2"
	row := permissions.ClaimedRow{
		ID:            "row-1",
		Service:       "payments",
		CorrelationID: &corrID,
		Payload:       []byte{0xff, 0xff, 0xff},
	}
	if err := p.ProcessRow(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entry := findLogLine(t, buf, "Permanent processing failure")
	if got := entry["correlation_id"]; got != corrID {
		t.Errorf("expected correlation_id %q, got %v", corrID, got)
	}
}

func TestProcessRow_LogsCorrelationID_OnRetry(t *testing.T) {
	repo := &fakeRepo{}
	applier := &fakeApplier{err: fgaSdk.FgaApiRateLimitExceededError{}}
	metrics := &fakeMetrics{}
	logger, buf := testutil.CapturingLogger(t)
	p := NewProcessor(repo, applier, listenDecoder(), 5, false, metrics, logger)

	corrID := "corr-3"
	row := permissions.ClaimedRow{
		ID:            "row-1",
		Service:       "payments",
		AttemptCount:  1,
		CorrelationID: &corrID,
		Payload:       encodeEnvelope(t, writeOp("user:u1", "viewer", "doc:d1")),
	}
	if err := p.ProcessRow(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entry := findLogLine(t, buf, "Transient processing failure; scheduling retry")
	if got := entry["correlation_id"]; got != corrID {
		t.Errorf("expected correlation_id %q, got %v", corrID, got)
	}
}
