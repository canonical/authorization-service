package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	kafka "github.com/segmentio/kafka-go"
)

// ConsumerInterface defines the interface for consuming Kafka messages.
type ConsumerInterface interface {
	Consume(ctx context.Context, handler func(ctx context.Context, msg kafka.Message) error) error
	Close() error
	// Stats returns a snapshot of the underlying reader's statistics, for
	// exposing consumer lag and throughput as metrics.
	Stats() kafka.ReaderStats
}

// Compile-time check.
var _ ConsumerInterface = (*Client)(nil)

// Config holds Kafka client configuration.
//
// Topics lists all permission-update topics the consumer subscribes to within
// the single ConsumerGroup. Partitions across these topics may be reassigned at
// any time due to rebalance or scaling; delivery is at-least-once. Parallelism
// is achieved by running multiple instances, not multiple in-process workers, so
// that per-partition ordering and safe offset committing are preserved.
type Config struct {
	Brokers       []string
	ConsumerGroup string
	Topics        []string
}

// Client wraps a Kafka consumer-group reader.
type Client struct {
	reader    *kafka.Reader
	logger    *slog.Logger
	closeOnce sync.Once
	closeErr  error
}

// NewClient creates a new Kafka client.
func NewClient(cfg Config, logger *slog.Logger) (*Client, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers must not be empty")
	}

	if cfg.ConsumerGroup == "" {
		return nil, fmt.Errorf("kafka consumer group must not be empty")
	}
	if len(cfg.Topics) == 0 {
		return nil, fmt.Errorf("kafka topics must not be empty")
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     cfg.Brokers,
		GroupID:     cfg.ConsumerGroup,
		GroupTopics: cfg.Topics,
		MinBytes:    1,
		MaxBytes:    10e6,
	})

	logger.Info("Kafka client created", "brokers", cfg.Brokers, "topics", cfg.Topics, "group", cfg.ConsumerGroup)
	return &Client{
		reader: reader,
		logger: logger,
	}, nil
}

// Consume fetches and handles messages in order until ctx is cancelled.
//
// Processing is deliberately serial. Kafka offsets are a per-partition
// high-water mark, not per-message acknowledgements: committing offset N marks
// every offset up to N as consumed. A parallel fan-out could therefore commit a
// later offset while an earlier one is still failing, silently dropping the
// earlier message; it would also break the per-partition ordering the ingestion
// stage relies on. To scale, run multiple listener instances — the consumer
// group assigns disjoint partitions to each, and every instance processes its
// partitions in order.
//
// The handler's return value drives progress:
//   - nil: the message is fully handled (persisted, a recognised duplicate, or a
//     permanent failure that has been logged/metered). The offset is committed
//     and processing advances.
//   - non-nil: a transient failure. The same message is retried in place with
//     backoff — the reader is NOT advanced, because leaving an offset uncommitted
//     does not cause a running reader to re-fetch it; only serial retry (or, on
//     restart/rebalance, redelivery from the last committed offset) guarantees
//     the message is not skipped.
func (c *Client) Consume(ctx context.Context, handler func(ctx context.Context, msg kafka.Message) error) error {
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("kafka fetch error: %w", err)
		}

		if err := c.handleWithRetry(ctx, handler, msg); err != nil {
			// Only returns non-nil on context cancellation mid-retry: exit without
			// committing so the message is redelivered from the committed offset.
			return nil
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.logger.Error("Failed to commit Kafka message",
				"topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset, "error", err)
		}
	}
}

// handleWithRetry invokes handler for msg, retrying transient failures in place
// with capped exponential backoff. It returns nil once the message is handled
// (handler returned nil), or a non-nil error only if ctx is cancelled while
// waiting to retry.
func (c *Client) handleWithRetry(ctx context.Context, handler func(ctx context.Context, msg kafka.Message) error, msg kafka.Message) error {
	const (
		baseBackoff = 200 * time.Millisecond
		maxBackoff  = 30 * time.Second
	)

	backoff := baseBackoff
	for attempt := 1; ; attempt++ {
		if err := handler(ctx, msg); err == nil {
			return nil
		} else {
			c.logger.Error("Transient handler failure; retrying same message in place",
				"topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset,
				"attempt", attempt, "backoff", backoff.String(), "error", err)
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}

		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// Stats returns a snapshot of the underlying reader's statistics.
func (c *Client) Stats() kafka.ReaderStats {
	return c.reader.Stats()
}

// Close closes the reader. Safe to call multiple times.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		if err := c.reader.Close(); err != nil {
			c.closeErr = fmt.Errorf("reader: %w", err)
		}
		c.logger.Info("Kafka client closed")
	})
	return c.closeErr
}
