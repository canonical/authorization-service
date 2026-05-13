package integration

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
	kafkaintegration "github.com/canonical/authorization-service/internal/integration/kafka"
	"github.com/canonical/authorization-service/internal/service/listen"
)

// listenerConfig returns a listen.Config with a configurable flush interval.
func listenerConfig(flushInterval time.Duration) listen.Config {
	return listen.Config{
		ErrorTopic:      errorTopic,
		ServiceIdHeader: "service",
		BatchSize:       100,
		FlushInterval:   flushInterval,
		ShutdownTimeout: 10 * time.Second,
	}
}

// newTestListener wires a Listener backed by the test containers.
// Each call uses a unique consumer group so tests don't share committed offsets.
func newTestListener(t *testing.T, group string, cfg listen.Config) *listen.Listener {
	t.Helper()

	kafkaClient, err := kafkaintegration.NewClient(kafkaintegration.Config{
		Brokers:       []string{kafkaBroker},
		ConsumerGroup: group,
		Topic:         ingestTopic,
		Workers:       2,
	}, testLogger)
	if err != nil {
		t.Fatalf("newTestListener kafka client: %v", err)
	}
	t.Cleanup(func() { kafkaClient.Close() })

	return listen.NewListener(kafkaClient, kafkaClient, newOpenFGAClient(t), &listen.Base64Encoder{}, cfg, testLogger)
}

// startListener runs listener.Run in a goroutine and returns the error channel.
func startListener(ctx context.Context, l *listen.Listener) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- l.Run(ctx) }()
	return ch
}

// rawWriter returns a *kafkago.Writer that publishes arbitrary bytes to ingestTopic.
func rawWriter(t *testing.T) *kafkago.Writer {
	t.Helper()
	w := &kafkago.Writer{
		Addr:                   kafkago.TCP(kafkaBroker),
		Topic:                  ingestTopic,
		AllowAutoTopicCreation: true,
	}
	t.Cleanup(func() { w.Close() })
	return w
}

// TestListener_HappyPath publishes one unique WriteRequest and asserts the
// resulting tuple appears in OpenFGA after the flush interval.
func TestListener_HappyPath(t *testing.T) {
	t.Parallel()

	suffix := uniqueSuffix()
	userID := "alice-" + suffix
	resourceID := "eng-" + suffix

	publishWriteRequest(t, newTestWriter(t), &messagesv1.WriteRequest{
		SequenceId:   1,
		UserType:     "user",
		UserId:       userID,
		ResourceType: "group",
		ResourceId:   resourceID,
		Organization: "canonical",
		Permission:   &messagesv1.WriteRequest_Entitlement{Entitlement: "member"},
	}, "svc-happy-"+suffix)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := startListener(ctx, newTestListener(t, "happy-"+suffix, listenerConfig(200*time.Millisecond)))

	waitForTuple(t, newOpenFGAClient(t), "user:"+userID, "member", "group:"+resourceID)

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("listener shutdown error: %v", err)
	}
}

// TestListener_DeserializeFailed publishes garbage bytes and asserts that the
// error topic receives a message with error_code="deserialize_failed", seq=0,
// and a non-empty base64 snapshot.
func TestListener_DeserializeFailed(t *testing.T) {
	t.Parallel()

	suffix := uniqueSuffix()

	w := rawWriter(t)
	if err := w.WriteMessages(context.Background(), kafkago.Message{
		Value:   []byte("not-a-proto-" + suffix),
		Headers: []kafkago.Header{{Key: "service", Value: []byte("svc-deser-" + suffix)}},
	}); err != nil {
		t.Fatalf("write garbage: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := startListener(ctx, newTestListener(t, "deser-"+suffix, listenerConfig(200*time.Millisecond)))

	got := waitForErrorMessage(t, newErrorReader(t, "deser-reader-"+suffix), 20*time.Second, func(e *messagesv1.WriteRequestError) bool {
		return e.GetErrorCode() == "deserialize_failed"
	})

	if got.GetErrorCode() != "deserialize_failed" {
		t.Errorf("error_code: got %q, want deserialize_failed", got.GetErrorCode())
	}
	if got.GetSequenceId() != 0 {
		t.Errorf("sequence_id: got %d, want 0", got.GetSequenceId())
	}
	if got.GetTupleSnapshot() == "" {
		t.Error("tuple_snapshot must not be empty")
	}
	if _, err := base64.StdEncoding.DecodeString(got.GetTupleSnapshot()); err != nil {
		t.Errorf("tuple_snapshot is not valid base64: %v", err)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("listener shutdown error: %v", err)
	}
}

// TestListener_OpenFGAWriteFailed publishes a WriteRequest with an entitlement
// that doesn't exist in the model and asserts that the error topic receives a
// message with the correct error_code, echoed sequence_id, and a snapshot that
// round-trips back to the original request.
func TestListener_OpenFGAWriteFailed(t *testing.T) {
	t.Parallel()

	suffix := uniqueSuffix()
	const seqID = uint32(999)

	orig := &messagesv1.WriteRequest{
		SequenceId:   seqID,
		UserType:     "user",
		UserId:       "charlie-" + suffix,
		ResourceType: "group",
		ResourceId:   "ops-" + suffix,
		Organization: "canonical",
		Permission:   &messagesv1.WriteRequest_Entitlement{Entitlement: "owner"}, // not in model
	}

	publishWriteRequest(t, newTestWriter(t), orig, "svc-bad-rel-"+suffix)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := startListener(ctx, newTestListener(t, "bad-rel-"+suffix, listenerConfig(200*time.Millisecond)))

	got := waitForErrorMessage(t, newErrorReader(t, "bad-rel-reader-"+suffix), 20*time.Second, func(e *messagesv1.WriteRequestError) bool {
		return e.GetErrorCode() == "openfga_write_failed" && e.GetSequenceId() == seqID
	})

	if got.GetErrorCode() != "openfga_write_failed" {
		t.Errorf("error_code: got %q, want openfga_write_failed", got.GetErrorCode())
	}
	if got.GetSequenceId() != seqID {
		t.Errorf("sequence_id: got %d, want %d", got.GetSequenceId(), seqID)
	}
	if got.GetTupleSnapshot() == "" {
		t.Fatal("tuple_snapshot must not be empty")
	}

	raw, err := base64.StdEncoding.DecodeString(got.GetTupleSnapshot())
	if err != nil {
		t.Fatalf("decoding snapshot: %v", err)
	}
	var decoded messagesv1.WriteRequest
	if err := proto.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshalling snapshot: %v", err)
	}
	if decoded.GetUserId() != orig.GetUserId() {
		t.Errorf("snapshot user_id: got %q, want %q", decoded.GetUserId(), orig.GetUserId())
	}
	if decoded.GetEntitlement() != orig.GetEntitlement() {
		t.Errorf("snapshot entitlement: got %q, want %q", decoded.GetEntitlement(), orig.GetEntitlement())
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("listener shutdown error: %v", err)
	}
}

// TestListener_RoleSkipped asserts that a WriteRequest carrying a role (the
// currently unimplemented oneof branch) is consumed without producing an OpenFGA
// tuple or an error message.
func TestListener_RoleSkipped(t *testing.T) {
	t.Parallel()

	suffix := uniqueSuffix()
	userID := "dave-" + suffix
	resourceID := "platform-" + suffix

	publishWriteRequest(t, newTestWriter(t), &messagesv1.WriteRequest{
		SequenceId:   42,
		UserType:     "user",
		UserId:       userID,
		ResourceType: "group",
		ResourceId:   resourceID,
		Organization: "canonical",
		Permission:   &messagesv1.WriteRequest_Role{Role: "admin"},
	}, "svc-role-"+suffix)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startListener(ctx, newTestListener(t, "role-"+suffix, listenerConfig(200*time.Millisecond)))

	// Give the listener time to consume and flush before asserting absence.
	waitForAbsenceTuple(t, newOpenFGAClient(t), "user:"+userID, "member", "group:"+resourceID, 4*time.Second)

	cancel()
}

// TestListener_BatchSizeFlush publishes exactly BatchSize messages to a single
// service and asserts they all land in OpenFGA via the size trigger — the flush
// interval is set to a minute so only a full batch can trigger the write.
func TestListener_BatchSizeFlush(t *testing.T) {
	t.Parallel()

	suffix := uniqueSuffix()
	const batchSize = 3

	type tuple struct{ user, resource string }
	var want []tuple
	w := newTestWriter(t)

	for i := range batchSize {
		u := "batch-u" + suffix + string(rune('a'+i))
		r := "batch-r" + suffix + string(rune('a'+i))
		want = append(want, tuple{u, r})
		publishWriteRequest(t, w, &messagesv1.WriteRequest{
			SequenceId:   uint32(i + 1),
			UserType:     "user",
			UserId:       u,
			ResourceType: "group",
			ResourceId:   r,
			Organization: "canonical",
			Permission:   &messagesv1.WriteRequest_Entitlement{Entitlement: "member"},
		}, "svc-batch-"+suffix)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := startListener(ctx, newTestListener(t, "batch-"+suffix, listen.Config{
		ErrorTopic:      errorTopic,
		ServiceIdHeader: "service",
		BatchSize:       batchSize,
		FlushInterval:   60 * time.Second, // only size trigger should fire
		ShutdownTimeout: 10 * time.Second,
	}))

	fga := newOpenFGAClient(t)
	for _, tp := range want {
		waitForTuple(t, fga, "user:"+tp.user, "member", "group:"+tp.resource)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("listener shutdown error: %v", err)
	}
}

// TestListener_GracefulShutdown verifies that cancelling the listener context
// before the flush interval fires causes the batcher to flush in-flight buffers
// during shutdown, so all published tuples land in OpenFGA.
func TestListener_GracefulShutdown(t *testing.T) {
	t.Parallel()

	suffix := uniqueSuffix()

	type tuple struct{ user, resource string }
	tuples := []tuple{
		{"grace-u1-" + suffix, "grace-r1-" + suffix},
		{"grace-u2-" + suffix, "grace-r2-" + suffix},
	}

	ctx, cancel := context.WithCancel(context.Background())

	// FlushInterval is deliberately long — only the shutdown flush should write.
	l := newTestListener(t, "grace-"+suffix, listen.Config{
		ErrorTopic:      errorTopic,
		ServiceIdHeader: "service",
		BatchSize:       100,
		FlushInterval:   30 * time.Second,
		ShutdownTimeout: 15 * time.Second,
	})
	errCh := startListener(ctx, l)

	// Wait long enough for the consumer group to join before publishing so the
	// messages enter the batcher (but don't flush due to the long interval).
	time.Sleep(10 * time.Second)

	w := newTestWriter(t)
	for i, tp := range tuples {
		publishWriteRequest(t, w, &messagesv1.WriteRequest{
			SequenceId:   uint32(i + 1),
			UserType:     "user",
			UserId:       tp.user,
			ResourceType: "group",
			ResourceId:   tp.resource,
			Organization: "canonical",
			Permission:   &messagesv1.WriteRequest_Entitlement{Entitlement: "member"},
		}, "svc-grace-"+suffix)
	}

	// Short pause so the consumer fetches and adds messages to the batcher.
	time.Sleep(3 * time.Second)

	// Trigger graceful shutdown.
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("listener shutdown error: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("listener did not stop within 20s")
	}

	fga := newOpenFGAClient(t)
	for _, tp := range tuples {
		waitForTuple(t, fga, "user:"+tp.user, "member", "group:"+tp.resource)
	}
}
