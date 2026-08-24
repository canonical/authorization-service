//go:build integration

package listener

import (
	"context"
	"fmt"
	"testing"

	"github.com/canonical/authorization-service/tests/integration/suite"
)

// TestListener_CorruptedMessageResilience verifies that invalid or corrupted
// messages published to Kafka are safely rejected by the listener without
// crashing the consumption loop or inserting corrupted rows into PostgreSQL.
func TestListener_CorruptedMessageResilience(t *testing.T) {
	suffix := suite.UniqueSuffix()
	group := "corrupted-" + suffix

	client, pool := newTestPostgres(t)

	// Publish corrupted non-protobuf payload to topic "payments.permissions"
	corruptPayload := []byte("invalid-corrupted-raw-bytes-payload")
	suite.PublishRaw(t, kafkaBroker, "payments", []byte("corrupt-key-"+suffix), corruptPayload)

	// Also publish a valid envelope afterwards to verify the listener continues processing.
	validIdem := "valid-after-corrupt-" + suffix
	suite.PublishEnvelope(t, kafkaBroker, "payments", suite.SampleEnvelope("payments", validIdem))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := startListener(ctx, newTestListener(t, group, client))

	// Wait for the valid envelope row to appear in PostgreSQL
	waitForWorkRow(t, pool, "payments", validIdem)

	// Confirm corrupt payload was ignored and no row with idempotency key "corrupt-key-..." was inserted
	if n := countWorkRows(t, pool, "payments", "corrupt-key-"+suffix); n != 0 {
		t.Fatalf("expected 0 rows for corrupted payload, got %d", n)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("listener shutdown error: %v", err)
	}
}

// TestListener_RestartAndAtLeastOnceDelivery verifies that if a Listener instance
// is interrupted/restarted mid-stream, the next Listener in the same consumer group
// seamlessly resumes consumption and all messages are durably stored in PostgreSQL.
func TestListener_RestartAndAtLeastOnceDelivery(t *testing.T) {
	suffix := suite.UniqueSuffix()
	group := "restart-" + suffix

	client, pool := newTestPostgres(t)

	const totalEnvelopes = 5
	idems := make([]string, totalEnvelopes)
	for i := 0; i < totalEnvelopes; i++ {
		idems[i] = fmt.Sprintf("restart-idem-%d-%s", i, suffix)
		suite.PublishEnvelope(t, kafkaBroker, "payments", suite.SampleEnvelope("payments", idems[i]))
	}

	// 1. Start Listener 1
	ctx1, cancel1 := context.WithCancel(context.Background())
	l1 := newTestListener(t, group, client)
	errCh1 := startListener(ctx1, l1)

	// Wait for at least 1 message to be stored
	waitForWorkRow(t, pool, "payments", idems[0])

	// Abruptly cancel Listener 1 to simulate crash / restart
	cancel1()
	<-errCh1

	// 2. Start Listener 2 with the SAME consumer group
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	l2 := newTestListener(t, group, client)
	errCh2 := startListener(ctx2, l2)

	// Verify all remaining messages are processed by Listener 2
	for i := 0; i < totalEnvelopes; i++ {
		waitForWorkRow(t, pool, "payments", idems[i])
	}

	cancel2()
	if err := <-errCh2; err != nil {
		t.Fatalf("listener 2 shutdown error: %v", err)
	}
}
