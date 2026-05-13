package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	kafka "github.com/segmentio/kafka-go"
)

// ConsumerInterface defines the interface for consuming Kafka messages.
type ConsumerInterface interface {
	Consume(ctx context.Context, handler func(ctx context.Context, msg kafka.Message) error) error
	Close() error
}

// PublisherInterface defines the interface for publishing Kafka messages.
type PublisherInterface interface {
	Publish(ctx context.Context, topic string, key, value []byte, headers ...kafka.Header) error
	Close() error
}

// Compile-time checks.
var _ ConsumerInterface = (*Client)(nil)
var _ PublisherInterface = (*Client)(nil)

// Config holds Kafka client configuration.
type Config struct {
	Brokers       []string
	ConsumerGroup string
	Topic         string
	Workers       int
}

// Client wraps a Kafka reader and writer.
type Client struct {
	reader    *kafka.Reader
	writer    *kafka.Writer
	workers   int
	logger    *slog.Logger
	closeOnce sync.Once
	closeErr  error
}

// NewClient creates a new Kafka client.
func NewClient(cfg Config, logger *slog.Logger) (*Client, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers must not be empty")
	}

	workers := cfg.Workers
	if workers < 1 {
		workers = 1
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  cfg.Brokers,
		GroupID:  cfg.ConsumerGroup,
		Topic:    cfg.Topic,
		MinBytes: 1,
		MaxBytes: 10e6,
	})

	writer := &kafka.Writer{
		Addr:                   kafka.TCP(cfg.Brokers...),
		Balancer:               &kafka.LeastBytes{},
		AllowAutoTopicCreation: true,
	}

	logger.Info("Kafka client created", "brokers", cfg.Brokers, "topic", cfg.Topic, "group", cfg.ConsumerGroup, "workers", workers)
	return &Client{
		reader:  reader,
		writer:  writer,
		workers: workers,
		logger:  logger,
	}, nil
}

// Consume starts workers goroutines that fetch and handle messages until ctx is cancelled.
// A single fetcher goroutine feeds a shared channel; workers process messages in parallel.
func (c *Client) Consume(ctx context.Context, handler func(ctx context.Context, msg kafka.Message) error) error {
	msgChan := make(chan kafka.Message, c.workers*2)
	fetchErrChan := make(chan error, 1)

	// Single fetcher goroutine — kafka.Reader.FetchMessage must only be called from one goroutine.
	go func() {
		defer close(msgChan)
		for {
			msg, err := c.reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				select {
				case fetchErrChan <- fmt.Errorf("kafka fetch error: %w", err):
				default:
				}
				return
			}
			select {
			case msgChan <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for range c.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for msg := range msgChan {
				if err := handler(ctx, msg); err != nil {
					c.logger.Error("Kafka message handler error", "error", err)
				}
				if err := c.reader.CommitMessages(ctx, msg); err != nil {
					if ctx.Err() != nil {
						return
					}
					c.logger.Error("Failed to commit Kafka message", "error", err)
				}
			}
		}()
	}

	wg.Wait()

	select {
	case err := <-fetchErrChan:
		return err
	default:
	}
	return nil
}

// Publish writes a message to the given topic.
func (c *Client) Publish(ctx context.Context, topic string, key, value []byte, headers ...kafka.Header) error {
	msg := kafka.Message{
		Topic:   topic,
		Key:     key,
		Value:   value,
		Headers: headers,
	}
	if err := c.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("failed to publish to topic %s: %w", topic, err)
	}
	c.logger.Debug("Published Kafka message", "topic", topic, "size", len(value))
	return nil
}

// Close closes the reader and writer. Safe to call multiple times.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		var errs []error
		if err := c.reader.Close(); err != nil {
			errs = append(errs, fmt.Errorf("reader: %w", err))
		}
		if err := c.writer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("writer: %w", err))
		}
		c.closeErr = errors.Join(errs...)
		c.logger.Info("Kafka client closed")
	})
	return c.closeErr
}
