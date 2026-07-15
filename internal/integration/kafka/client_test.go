// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package kafka

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	kafka "github.com/segmentio/kafka-go"
)

func testClient() *Client {
	return &Client{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

// TestHandleWithRetry_RetriesUntilSuccess verifies a transient failure is retried
// in place (same message) until the handler eventually succeeds.
func TestHandleWithRetry_RetriesUntilSuccess(t *testing.T) {
	c := testClient()

	var calls int
	handler := func(_ context.Context, _ kafka.Message) error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	}

	err := c.handleWithRetry(context.Background(), handler, kafka.Message{})
	if err != nil {
		t.Fatalf("expected nil after success, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
}

// TestHandleWithRetry_StopsOnContextCancel verifies retry stops (without treating
// the message as handled) when the context is cancelled mid-backoff.
func TestHandleWithRetry_StopsOnContextCancel(t *testing.T) {
	c := testClient()

	ctx, cancel := context.WithCancel(context.Background())
	handler := func(_ context.Context, _ kafka.Message) error {
		return errors.New("always transient")
	}

	// Cancel shortly after starting so the first backoff wait is interrupted.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := c.handleWithRetry(ctx, handler, kafka.Message{})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestHandleWithRetry_SuccessFirstTry verifies no retry when the handler succeeds
// immediately.
func TestHandleWithRetry_SuccessFirstTry(t *testing.T) {
	c := testClient()

	var calls int
	handler := func(_ context.Context, _ kafka.Message) error {
		calls++
		return nil
	}

	if err := c.handleWithRetry(context.Background(), handler, kafka.Message{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 attempt, got %d", calls)
	}
}
