//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace/noop"

	kafkaintegration "github.com/canonical/authorization-service/internal/integration/kafka"
	"github.com/canonical/authorization-service/internal/integration/postgres"
	"github.com/canonical/authorization-service/internal/repository"
	"github.com/canonical/authorization-service/internal/service/listen"
)

// newTestPostgres opens a postgres.Client and a raw pool against the test DB.
func newTestPostgres(t *testing.T) (*postgres.Client, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), pgDSN)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	client, err := postgres.NewClient(pgConfig, testLogger, noop.NewTracerProvider().Tracer("test"))
	if err != nil {
		t.Fatalf("postgres client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client, pool
}

// newTestListener wires a Listener backed by the test containers, subscribing to
// all federated services within a unique consumer group.
func newTestListener(t *testing.T, group string, db postgres.DBClientInterface) *listen.Listener {
	t.Helper()

	registry, err := listen.NewServiceRegistry(federatedServices)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	kafkaClient, err := kafkaintegration.NewClient(kafkaintegration.Config{
		Brokers:       []string{kafkaBroker},
		ConsumerGroup: group,
		Topics:        registry.Topics(),
	}, testLogger)
	if err != nil {
		t.Fatalf("newTestListener kafka client: %v", err)
	}
	t.Cleanup(func() { kafkaClient.Close() })

	repo := repository.NewPostgresPermissionWorkRepository(db)
	ingestor := listen.NewIngestionService(
		registry, listen.NewDecoder(), listen.NewValidator(), repo, nil, testLogger,
	)
	return listen.NewListener(kafkaClient, ingestor, testLogger)
}

func startListener(ctx context.Context, l *listen.Listener) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- l.Run(ctx) }()
	return ch
}

// countWorkRows returns how many permission_update_work rows match.
func countWorkRows(t *testing.T, pool *pgxpool.Pool, service, idempotencyKey string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM permission_update_work WHERE service = $1 AND idempotency_key = $2`,
		service, idempotencyKey,
	).Scan(&n)
	if err != nil {
		t.Fatalf("countWorkRows: %v", err)
	}
	return n
}

// waitForWorkRow polls until a matching row appears in status 'received'.
func waitForWorkRow(t *testing.T, pool *pgxpool.Pool, service, idempotencyKey string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		err := pool.QueryRow(context.Background(),
			`SELECT status FROM permission_update_work WHERE service = $1 AND idempotency_key = $2`,
			service, idempotencyKey,
		).Scan(&status)
		if err == nil {
			if status != "received" {
				t.Fatalf("row status = %q, want received", status)
			}
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("work row (service=%s idempotency_key=%s) not found within 20s", service, idempotencyKey)
}

// TestListener_HappyPath publishes one valid envelope and asserts a 'received'
// row is durably persisted.
func TestListener_HappyPath(t *testing.T) {
	suffix := uniqueSuffix()
	idem := "idem-happy-" + suffix

	client, pool := newTestPostgres(t)
	publishEnvelope(t, "payments", sampleEnvelope("payments", idem))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := startListener(ctx, newTestListener(t, "happy-"+suffix, client))

	waitForWorkRow(t, pool, "payments", idem)

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("listener shutdown error: %v", err)
	}
}

// TestListener_DuplicateDelivery publishes the same idempotency key twice and
// asserts only a single row exists.
func TestListener_DuplicateDelivery(t *testing.T) {
	suffix := uniqueSuffix()
	idem := "idem-dup-" + suffix

	client, pool := newTestPostgres(t)
	publishEnvelope(t, "payments", sampleEnvelope("payments", idem))
	publishEnvelope(t, "payments", sampleEnvelope("payments", idem))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := startListener(ctx, newTestListener(t, "dup-"+suffix, client))

	waitForWorkRow(t, pool, "payments", idem)
	// Give the second delivery time to be processed as an idempotent no-op.
	time.Sleep(2 * time.Second)
	if n := countWorkRows(t, pool, "payments", idem); n != 1 {
		t.Fatalf("expected exactly 1 row, got %d", n)
	}

	cancel()
	<-errCh
}

// TestListener_ServiceMismatchNotPersisted publishes an envelope whose service
// field does not match the topic slug; it must fail validation and not persist.
func TestListener_ServiceMismatchNotPersisted(t *testing.T) {
	suffix := uniqueSuffix()
	idem := "idem-mismatch-" + suffix

	client, pool := newTestPostgres(t)

	env := sampleEnvelope("invoicing", idem) // service=invoicing...
	publishEnvelope(t, "payments", env)      // ...published to payments.permissions

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := startListener(ctx, newTestListener(t, "mismatch-"+suffix, client))

	// A permanent validation failure is acknowledged, not persisted.
	time.Sleep(3 * time.Second)
	if n := countWorkRows(t, pool, "invoicing", idem); n != 0 {
		t.Fatalf("expected 0 rows for mismatched service, got %d", n)
	}

	cancel()
	<-errCh
}
