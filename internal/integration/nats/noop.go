// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package nats

import (
    "context"
    "log/slog"

    "github.com/nats-io/nats.go"
)

// Compile-time check to ensure NoopClient implements EventClientInterface
var _ EventClientInterface = (*NoopClient)(nil)

// NoopClient is a no-operation implementation of EventClientInterface
// that doesn't actually publish or subscribe to any message broker.
// Useful for testing and development without NATS infrastructure.
type NoopClient struct {
    logger      *slog.Logger
    subscribers map[string][]func(msg *nats.Msg) error
    connected   bool
}

// NewNoopClient creates a new noop NATS client
func NewNoopClient(logger *slog.Logger) *NoopClient {
    return &NoopClient{
        logger:      logger,
        subscribers: make(map[string][]func(msg *nats.Msg) error),
        connected:   true,
    }
}

// Publish logs the publish attempt but doesn't send any message
func (c *NoopClient) Publish(ctx context.Context, subject string, data []byte) error {
    c.logger.Debug("Noop publish - no message sent",
        "subject", subject,
        "size", len(data),
    )
    return nil
}

// Subscribe stores the handler but doesn't actually subscribe to anything
func (c *NoopClient) Subscribe(ctx context.Context, subject string, handler func(msg *nats.Msg) error) error {
    c.logger.Debug("Noop subscribe - handler stored but not active",
        "subject", subject,
    )

    if c.subscribers[subject] == nil {
        c.subscribers[subject] = make([]func(msg *nats.Msg) error, 0)
    }
    c.subscribers[subject] = append(c.subscribers[subject], handler)

    c.logger.Info("Noop subscription registered",
        "subject", subject,
        "total_handlers", len(c.subscribers[subject]),
    )
    return nil
}

// IsConnected always returns true for noop client
func (c *NoopClient) IsConnected() bool {
    return c.connected
}

// Close marks the client as disconnected
func (c *NoopClient) Close() error {
    c.logger.Info("Noop NATS client closed")
    c.connected = false
    c.subscribers = make(map[string][]func(msg *nats.Msg) error)
    return nil
}
