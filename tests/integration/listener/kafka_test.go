//go:build integration

package listener

import (
	"context"
	"sync"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
	kafkaintegration "github.com/canonical/authorization-service/internal/integration/kafka"
	"github.com/canonical/authorization-service/internal/service/listen"
	"github.com/canonical/authorization-service/tests/integration/suite"
)

// TestKafkaConsumerGroup_MultiTopic verifies the consumer group receives messages
// published across multiple "permissions.<slug>" topics, and that msg.Topic
// carries the originating topic (used for service resolution).
func TestKafkaConsumerGroup_MultiTopic(t *testing.T) {
	suffix := suite.UniqueSuffix()
	group := "kafka-multitopic-" + suffix

	registry, err := listen.NewServiceRegistry(suite.FederatedServices)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	// Publish one envelope to each federated service's topic.
	want := map[string]string{} // topic -> idempotency key
	for _, slug := range suite.FederatedServices {
		idem := "idem-" + slug + "-" + suffix
		suite.PublishEnvelope(t, kafkaBroker, slug, suite.SampleEnvelope(slug, idem))
		want[suite.TopicFor(slug)] = idem
	}

	c, err := kafkaintegration.NewClient(kafkaintegration.Config{
		Brokers:       []string{kafkaBroker},
		ConsumerGroup: group,
		Topics:        registry.Topics(),
	}, suite.TestLogger)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	var (
		mu   sync.Mutex
		seen = map[string]string{}
	)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	go func() {
		_ = c.Consume(ctx, func(_ context.Context, msg kafkago.Message) error {
			var env messagesv1.PermissionUpdateEnvelope
			if err := proto.Unmarshal(msg.Value, &env); err != nil {
				return nil
			}
			if want[msg.Topic] != env.GetIdempotencyKey() {
				return nil // belongs to another test run
			}
			mu.Lock()
			seen[msg.Topic] = env.GetIdempotencyKey()
			count := len(seen)
			mu.Unlock()
			if count == len(want) {
				cancel()
			}
			return nil
		})
	}()

	<-ctx.Done()

	mu.Lock()
	defer mu.Unlock()
	for topic, idem := range want {
		if seen[topic] != idem {
			t.Errorf("topic %s: did not receive idempotency key %s", topic, idem)
		}
	}
}

// TestNoopConsume verifies NoopClient.Consume blocks until ctx is cancelled and
// returns nil — no Docker required.
func TestNoopConsume(t *testing.T) {
	t.Parallel()

	noop := kafkaintegration.NewNoopClient(suite.TestLogger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := noop.Consume(ctx, func(_ context.Context, _ kafkago.Message) error {
		t.Error("NoopClient handler must never be called")
		return nil
	})
	if err != nil {
		t.Errorf("NoopClient.Consume returned non-nil error: %v", err)
	}
}
