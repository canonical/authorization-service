//go:build integration
// +build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/canonical/authorization-service/internal/testutil"
)

func TestNATSIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start NATS container
	container, natsClient := testutil.StartNATSContainer(ctx, t)
	defer testutil.StopContainer(ctx, container)

	// Test publish
	err := natsClient.Publish(ctx, "test.subject", []byte("test message"))
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Test connection
	if !natsClient.IsConnected() {
		t.Fatal("Expected NATS to be connected")
	}

	// Close
	err = natsClient.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
