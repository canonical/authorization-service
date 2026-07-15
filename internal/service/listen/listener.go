// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package listen

import (
	"context"
	"log/slog"

	kafka "github.com/segmentio/kafka-go"

	kafkaintegration "github.com/canonical/authorization-service/internal/integration/kafka"
)

// Listener consumes permission-update messages from the federated services'
// Kafka topics and hands each one to the Ingestor for durable persistence.
// It never writes to OpenFGA: tuple application is performed by a separate
// worker pool reading from the work table.
type Listener struct {
	consumer kafkaintegration.ConsumerInterface
	ingestor Ingestor
	logger   *slog.Logger
}

// NewListener constructs a Listener.
func NewListener(
	consumer kafkaintegration.ConsumerInterface,
	ingestor Ingestor,
	logger *slog.Logger,
) *Listener {
	return &Listener{
		consumer: consumer,
		ingestor: ingestor,
		logger:   logger,
	}
}

// Run starts the consumer and blocks until ctx is cancelled.
func (l *Listener) Run(ctx context.Context) error {
	l.logger.Info("Starting Kafka permission-update listener")
	err := l.consumer.Consume(ctx, l.handleMessage)
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// handleMessage is the per-message Kafka handler. It delegates to the Ingestor.
//
// The returned error drives the consumer: returning nil lets it commit the
// offset and advance. A successful ingest, a recognised duplicate, and a
// PermanentError all resolve to nil here — a permanently-unprocessable message
// is acknowledged (already logged and metered by the Ingestor) rather than
// blocking the partition forever. Only transient errors are propagated, so the
// consumer retries the same message in place without advancing.
func (l *Listener) handleMessage(ctx context.Context, msg kafka.Message) error {
	err := l.ingestor.Ingest(ctx, Message{
		Topic:     msg.Topic,
		Partition: msg.Partition,
		Offset:    msg.Offset,
		Value:     msg.Value,
	})
	if err != nil && IsPermanent(err) {
		return nil
	}
	return err
}
