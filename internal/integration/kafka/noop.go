package kafka

import (
	"context"
	"log/slog"

	kafka "github.com/segmentio/kafka-go"
)

// Compile-time checks.
var _ ConsumerInterface = (*NoopClient)(nil)
var _ PublisherInterface = (*NoopClient)(nil)

// NoopClient is a no-operation implementation of ConsumerInterface and PublisherInterface.
type NoopClient struct {
	logger *slog.Logger
}

// NewNoopClient creates a new no-op Kafka client.
func NewNoopClient(logger *slog.Logger) *NoopClient {
	return &NoopClient{logger: logger}
}

// Consume blocks until ctx is cancelled, simulating a consumer with no messages.
func (c *NoopClient) Consume(ctx context.Context, _ func(ctx context.Context, msg kafka.Message) error) error {
	c.logger.Info("Noop Kafka consumer started")
	<-ctx.Done()
	return nil
}

// Publish logs the attempt but does not send any message.
func (c *NoopClient) Publish(_ context.Context, topic string, _ []byte, value []byte, _ ...kafka.Header) error {
	c.logger.Debug("Noop Kafka publish", "topic", topic, "size", len(value))
	return nil
}

// Close is a no-op.
func (c *NoopClient) Close() error {
	c.logger.Info("Noop Kafka client closed")
	return nil
}
