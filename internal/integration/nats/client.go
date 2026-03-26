package nats

import (
    "context"
    "fmt"
    "log/slog"
    "time"

    "github.com/nats-io/nats.go"
    "github.com/nats-io/nats.go/jetstream"
)

// EventClientInterface defines the interface for event streaming operations
type EventClientInterface interface {
    Publish(ctx context.Context, subject string, data []byte) error
    Subscribe(ctx context.Context, subject string, handler func(msg *nats.Msg) error) error
    IsConnected() bool
    Close() error
}

// Compile-time check to ensure Client implements EventClientInterface
var _ EventClientInterface = (*Client)(nil)

// Client wraps NATS functionality
type Client struct {
    conn      *nats.Conn
    js        jetstream.JetStream
    streamCfg jetstream.StreamConfig
    logger    *slog.Logger
}

// Config holds NATS client configuration
type Config struct {
    URL             string
    ClusterID       string
    ClientID        string
    EnableJetStream bool
    StreamName      string
    MaxReconnects   int
    ReconnectWait   time.Duration
    Timeout         time.Duration
}

// NewClient creates a new NATS client
func NewClient(cfg Config, logger *slog.Logger) (*Client, error) {
    opts := []nats.Option{
        nats.Name(cfg.ClientID),
        nats.MaxReconnects(cfg.MaxReconnects),
        nats.ReconnectWait(cfg.ReconnectWait),
        nats.Timeout(cfg.Timeout),
        nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
            if err != nil {
                logger.Error("NATS disconnected", "error", err)
            }
        }),
        nats.ReconnectHandler(func(nc *nats.Conn) {
            logger.Info("NATS reconnected", "url", nc.ConnectedUrl())
        }),
    }

    conn, err := nats.Connect(cfg.URL, opts...)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to NATS: %w", err)
    }

    client := &Client{
        conn:   conn,
        logger: logger,
    }

    if cfg.EnableJetStream {
        js, err := jetstream.New(conn)
        if err != nil {
            conn.Close()
            return nil, fmt.Errorf("failed to create JetStream context: %w", err)
        }
        client.js = js

        // Create or update stream
        streamCfg := jetstream.StreamConfig{
            Name:      cfg.StreamName,
            Subjects:  []string{fmt.Sprintf("%s.>", cfg.StreamName)},
            Retention: jetstream.InterestPolicy,
            Storage:   jetstream.MemoryStorage,
        }
        client.streamCfg = streamCfg

        _, err = js.CreateOrUpdateStream(context.Background(), streamCfg)
        if err != nil {
            conn.Close()
            return nil, fmt.Errorf("failed to create JetStream stream: %w", err)
        }

        logger.Info("JetStream stream created/updated", "stream", cfg.StreamName)
    }

    logger.Info("NATS client connected", "url", cfg.URL, "jetstream", cfg.EnableJetStream)
    return client, nil
}

// Publish publishes a message to a subject
func (c *Client) Publish(ctx context.Context, subject string, data []byte) error {
    if c.js != nil {
        _, err := c.js.Publish(ctx, subject, data)
        if err != nil {
            c.logger.Error("Failed to publish to JetStream", "subject", subject, "error", err)
            return fmt.Errorf("failed to publish to JetStream: %w", err)
        }
    } else {
        err := c.conn.Publish(subject, data)
        if err != nil {
            c.logger.Error("Failed to publish to NATS", "subject", subject, "error", err)
            return fmt.Errorf("failed to publish to NATS: %w", err)
        }
    }

    c.logger.Debug("Published message", "subject", subject, "size", len(data))
    return nil
}

// Subscribe subscribes to a subject with a handler function
func (c *Client) Subscribe(ctx context.Context, subject string, handler func(msg *nats.Msg) error) error {
    if c.js != nil {
        consumer, err := c.js.CreateOrUpdateConsumer(ctx, c.streamCfg.Name, jetstream.ConsumerConfig{
            Durable:       fmt.Sprintf("%s-consumer", subject),
            FilterSubject: subject,
            AckPolicy:     jetstream.AckExplicitPolicy,
        })
        if err != nil {
            return fmt.Errorf("failed to create JetStream consumer: %w", err)
        }

        _, err = consumer.Consume(func(msg jetstream.Msg) {
            // Convert jetstream.Msg to *nats.Msg for the handler
            natsMsg := &nats.Msg{
                Subject: msg.Subject(),
                Reply:   msg.Reply(),
                Data:    msg.Data(),
                Sub:     nil,
            }
            if err := handler(natsMsg); err != nil {
                c.logger.Error("Handler error", "subject", subject, "error", err)
                msg.Nak()
            } else {
                msg.Ack()
            }
        })
        if err != nil {
            return fmt.Errorf("failed to start JetStream consumer: %w", err)
        }
    } else {
        _, err := c.conn.Subscribe(subject, func(msg *nats.Msg) {
            if err := handler(msg); err != nil {
                c.logger.Error("Handler error", "subject", subject, "error", err)
            }
        })
        if err != nil {
            return fmt.Errorf("failed to subscribe to NATS: %w", err)
        }
    }

    c.logger.Info("Subscribed to subject", "subject", subject)
    return nil
}

// Close closes the NATS connection
func (c *Client) Close() error {
    if c.conn != nil {
        c.conn.Close()
        c.logger.Info("NATS connection closed")
    }
    return nil
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
    return c.conn != nil && c.conn.IsConnected()
}
