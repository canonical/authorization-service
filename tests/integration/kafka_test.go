package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
	kafkaintegration "github.com/canonical/authorization-service/internal/integration/kafka"
)

// TestKafkaPublishConsume verifies that a single published WriteRequest proto
// is received verbatim by the consumer handler.
func TestKafkaPublishConsume(t *testing.T) {
	t.Parallel()

	suffix := uniqueSuffix()
	group := "kafka-single-" + suffix

	want := &messagesv1.WriteRequest{
		SequenceId:   77,
		UserType:     "user",
		UserId:       "alice-" + suffix,
		ResourceType: "group",
		ResourceId:   "eng-" + suffix,
		Organization: "canonical",
		Permission:   &messagesv1.WriteRequest_Entitlement{Entitlement: "member"},
	}

	w := newTestWriter(t)
	publishWriteRequest(t, w, want, "svc-"+suffix)

	c := newKafkaClient(t, group)

	var (
		got     *messagesv1.WriteRequest
		mu      sync.Mutex
		gotOnce sync.Once
		done    = make(chan struct{})
	)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	go func() {
		_ = c.Consume(ctx, func(_ context.Context, msg kafkago.Message) error {
			var req messagesv1.WriteRequest
			if err := proto.Unmarshal(msg.Value, &req); err != nil {
				return nil // skip messages from other tests
			}
			if req.GetUserId() != want.GetUserId() {
				return nil // belongs to a different test
			}
			gotOnce.Do(func() {
				mu.Lock()
				got = &req
				mu.Unlock()
				close(done)
				cancel()
			})
			return nil
		})
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("message not received within deadline")
	}

	mu.Lock()
	defer mu.Unlock()

	if got.GetSequenceId() != want.GetSequenceId() {
		t.Errorf("sequence_id: got %d, want %d", got.GetSequenceId(), want.GetSequenceId())
	}
	if got.GetUserId() != want.GetUserId() {
		t.Errorf("user_id: got %q, want %q", got.GetUserId(), want.GetUserId())
	}
	if got.GetResourceId() != want.GetResourceId() {
		t.Errorf("resource_id: got %q, want %q", got.GetResourceId(), want.GetResourceId())
	}
	if got.GetEntitlement() != want.GetEntitlement() {
		t.Errorf("entitlement: got %q, want %q", got.GetEntitlement(), want.GetEntitlement())
	}
}

// TestKafkaConsume_MultipleWorkers publishes N messages and verifies that all N
// sequence IDs are received across the worker pool (order is unimportant).
func TestKafkaConsume_MultipleWorkers(t *testing.T) {
	t.Parallel()

	const N = 10
	suffix := uniqueSuffix()
	group := "kafka-multi-" + suffix

	w := newTestWriter(t)
	wantIDs := make(map[uint32]struct{}, N)
	for i := range N {
		seqID := uint32(i + 1)
		wantIDs[seqID] = struct{}{}
		publishWriteRequest(t, w, &messagesv1.WriteRequest{
			SequenceId:   seqID,
			UserType:     "user",
			UserId:       fmt.Sprintf("user-%s-%d", suffix, i),
			ResourceType: "group",
			ResourceId:   fmt.Sprintf("grp-%s-%d", suffix, i),
			Permission:   &messagesv1.WriteRequest_Entitlement{Entitlement: "member"},
		}, "svc-multi-"+suffix)
	}

	c, err := kafkaintegration.NewClient(kafkaintegration.Config{
		Brokers:       []string{kafkaBroker},
		ConsumerGroup: group,
		Topic:         ingestTopic,
		Workers:       4,
	}, testLogger)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	var (
		mu       sync.Mutex
		received = make(map[uint32]struct{})
	)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	go func() {
		_ = c.Consume(ctx, func(_ context.Context, msg kafkago.Message) error {
			var req messagesv1.WriteRequest
			if err := proto.Unmarshal(msg.Value, &req); err != nil {
				return nil
			}
			// Only track IDs published by this test instance
			if _, ok := wantIDs[req.GetSequenceId()]; !ok {
				return nil
			}
			mu.Lock()
			received[req.GetSequenceId()] = struct{}{}
			count := len(received)
			mu.Unlock()
			if count == N {
				cancel()
			}
			return nil
		})
	}()

	<-ctx.Done()

	mu.Lock()
	defer mu.Unlock()

	for id := range wantIDs {
		if _, ok := received[id]; !ok {
			t.Errorf("sequence_id %d was not received", id)
		}
	}
}

// TestNoopConsume verifies that the NoopClient.Consume blocks until ctx is
// cancelled and returns nil — no Docker required.
func TestNoopConsume(t *testing.T) {
	t.Parallel()

	noop := kafkaintegration.NewNoopClient(testLogger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Should block until ctx expires and then return nil.
	err := noop.Consume(ctx, func(_ context.Context, _ kafkago.Message) error {
		t.Error("NoopClient handler must never be called")
		return nil
	})
	if err != nil {
		t.Errorf("NoopClient.Consume returned non-nil error: %v", err)
	}
}
