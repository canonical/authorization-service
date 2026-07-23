//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openfga/go-sdk/client"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/authorization-service/internal/model/permissions"
	"github.com/canonical/authorization-service/internal/repository"
	"github.com/canonical/authorization-service/internal/service/listen"
	"github.com/canonical/authorization-service/internal/service/worker"
	"google.golang.org/protobuf/proto"
)

// fakeApplier is an in-memory TupleApplier for the integration tests: OpenFGA is
// not part of the container suite, so we assert against the durable state
// (work-row status + authorization_tuples) rather than a real store. A per-call
// error can be injected to exercise the failure paths.
type fakeApplier struct {
	mu       sync.Mutex
	err      error
	applied  int
	lastKeys []client.ClientTupleKey
}

func (a *fakeApplier) ApplyTuples(_ context.Context, writes []client.ClientTupleKey, _ []client.ClientTupleKeyWithoutCondition) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return a.err
	}
	a.applied++
	a.lastKeys = writes
	return nil
}

// insertReceivedRow inserts a 'received' work row via the real repository and
// returns its idempotency key. The payload is a valid envelope with one write op.
func insertReceivedRow(t *testing.T, repo *repository.PostgresPermissionWorkRepository, slug, idem string, op *messagesv1.PermissionOperation) {
	t.Helper()
	env := &messagesv1.PermissionUpdateEnvelope{
		Version:        "1",
		Service:        slug,
		MessageId:      "msg-" + idem,
		IdempotencyKey: idem,
		Operations:     []*messagesv1.PermissionOperation{op},
	}
	payload, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	row := permissions.WorkRow{
		Service:        slug,
		MessageID:      env.GetMessageId(),
		IdempotencyKey: idem,
		Version:        "1",
		IngestionTime:  time.Now().UTC(),
		Payload:        payload,
	}
	if err := repo.Insert(context.Background(), row); err != nil {
		t.Fatalf("insert received row: %v", err)
	}
}

// workRowStatus returns the status of a work row by idempotency key.
func workRowStatus(t *testing.T, pool *pgxpool.Pool, slug, idem string) string {
	t.Helper()
	var status string
	err := pool.QueryRow(context.Background(),
		`SELECT status FROM permission_update_work WHERE service = $1 AND idempotency_key = $2`,
		slug, idem,
	).Scan(&status)
	if err != nil {
		t.Fatalf("workRowStatus: %v", err)
	}
	return status
}

// waitForStatus polls until a work row reaches the wanted status.
func waitForStatus(t *testing.T, pool *pgxpool.Pool, slug, idem, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if workRowStatus(t, pool, slug, idem) == want {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("row (service=%s idem=%s) did not reach status %q within 15s (last=%q)",
		slug, idem, want, workRowStatus(t, pool, slug, idem))
}

func countTuples(t *testing.T, pool *pgxpool.Pool, subject, relation, object string) int {
	t.Helper()
	baseSubject, userSetRelation := permissions.ParseSubject(subject)

	var n int
	var err error
	if userSetRelation == nil {
		err = pool.QueryRow(context.Background(),
			`SELECT count(*) FROM authorization_tuples WHERE subject = $1 AND user_set_subject_relation IS NULL AND relation = $2 AND object = $3`,
			baseSubject, relation, object,
		).Scan(&n)
	} else {
		err = pool.QueryRow(context.Background(),
			`SELECT count(*) FROM authorization_tuples WHERE subject = $1 AND user_set_subject_relation = $2 AND relation = $3 AND object = $4`,
			baseSubject, *userSetRelation, relation, object,
		).Scan(&n)
	}

	if err != nil {
		t.Fatalf("countTuples: %v", err)
	}
	return n
}

func writeOperation(subject, relation, object string) *messagesv1.PermissionOperation {
	return &messagesv1.PermissionOperation{
		Op:       messagesv1.PermissionOp_PERMISSION_OP_WRITE,
		Subject:  subject,
		Relation: relation,
		Object:   object,
	}
}

// startWorker runs a worker in the background and returns its error channel.
func startWorker(ctx context.Context, w *worker.Worker) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- w.Run(ctx) }()
	return ch
}

// TestWorker_HappyPath seeds a received row and asserts the worker applies it,
// mirrors the tuple into authorization_tuples, and marks the row processed.
func TestWorker_HappyPath(t *testing.T) {
	client, pool := newTestPostgres(t)
	repo := repository.NewPostgresPermissionWorkRepository(client)

	suffix := uniqueSuffix()
	idem := "worker-happy-" + suffix
	subject := "user:u-" + suffix
	insertReceivedRow(t, repo, "payments", idem, writeOperation(subject, "member", "group:g1"))

	applier := &fakeApplier{}
	proc := worker.NewProcessor(repo, applier, listen.NewDecoder(), 5, nil, testLogger)
	w := worker.NewWorker(repo, proc, 50, 100*time.Millisecond, time.Minute, testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := startWorker(ctx, w)

	waitForStatus(t, pool, "payments", idem, "processed")

	if n := countTuples(t, pool, subject, "member", "group:g1"); n != 1 {
		t.Fatalf("expected tuple mirrored into authorization_tuples, got %d", n)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("worker shutdown error: %v", err)
	}
}

// TestWorker_UsersetSubject seeds a received row with a userset subject (e.g. role:admin#assignee)
// and asserts the worker applies it, mirrors the tuple correctly, and marks the row processed.
func TestWorker_UsersetSubject(t *testing.T) {
	client, pool := newTestPostgres(t)
	repo := repository.NewPostgresPermissionWorkRepository(client)

	suffix := uniqueSuffix()
	idem := "worker-userset-" + suffix
	subject := "role:admin#assignee-" + suffix
	insertReceivedRow(t, repo, "payments", idem, writeOperation(subject, "member", "group:g1"))

	applier := &fakeApplier{}
	proc := worker.NewProcessor(repo, applier, listen.NewDecoder(), 5, nil, testLogger)
	w := worker.NewWorker(repo, proc, 50, 100*time.Millisecond, time.Minute, testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := startWorker(ctx, w)

	waitForStatus(t, pool, "payments", idem, "processed")

	if n := countTuples(t, pool, subject, "member", "group:g1"); n != 1 {
		t.Fatalf("expected userset tuple mirrored into authorization_tuples, got %d", n)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("worker shutdown error: %v", err)
	}
}

// TestWorker_PermanentFailure seeds a row whose payload cannot be decoded and
// asserts the worker moves it straight to 'failed'.
func TestWorker_PermanentFailure(t *testing.T) {
	client, pool := newTestPostgres(t)
	repo := repository.NewPostgresPermissionWorkRepository(client)

	suffix := uniqueSuffix()
	idem := "worker-bad-" + suffix
	// Insert a row with a deliberately corrupt payload (not a valid envelope).
	row := permissions.WorkRow{
		Service:        "payments",
		MessageID:      "msg-" + idem,
		IdempotencyKey: idem,
		Version:        "1",
		IngestionTime:  time.Now().UTC(),
		Payload:        []byte{0xff, 0xff, 0xff, 0xff},
	}
	if err := repo.Insert(context.Background(), row); err != nil {
		t.Fatalf("insert: %v", err)
	}

	applier := &fakeApplier{}
	proc := worker.NewProcessor(repo, applier, listen.NewDecoder(), 5, nil, testLogger)
	w := worker.NewWorker(repo, proc, 50, 100*time.Millisecond, time.Minute, testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := startWorker(ctx, w)

	waitForStatus(t, pool, "payments", idem, "failed")

	cancel()
	<-errCh
}

// TestReaper_Integration verifies that the Reaper background loop correctly identifies
// and reclaims a row stuck in 'processing' status.
func TestReaper_Integration(t *testing.T) {
	client, pool := newTestPostgres(t)
	repo := repository.NewPostgresPermissionWorkRepository(client)

	suffix := uniqueSuffix()
	idem := "reaper-stale-" + suffix
	subject := "user:u-" + suffix

	// 1. Insert a row in 'received' status
	insertReceivedRow(t, repo, "payments", idem, writeOperation(subject, "member", "group:g1"))

	// 2. Claim it to move it to 'processing'
	rows, err := repo.ClaimBatch(context.Background(), 1, time.Minute)
	if err != nil {
		t.Fatalf("failed to claim: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row claimed, got %d", len(rows))
	}

	// 3. Update processing_started_at to be in the past
	_, err = pool.Exec(context.Background(),
		`UPDATE permission_update_work SET processing_started_at = now() - INTERVAL '30 minutes' WHERE idempotency_key = $1`,
		idem,
	)
	if err != nil {
		t.Fatalf("failed to update processing_started_at: %v", err)
	}

	// 4. Run the Reaper background loop with a 15-minute timeout
	r := worker.NewReaper(repo, 15*time.Minute, 50*time.Millisecond, testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- r.Run(ctx) }()

	// 5. Verify the row is reverted back to 'received'
	waitForStatus(t, pool, "payments", idem, "received")

	cancel()
	<-errCh
}

// TestReaper_LockCoordination verifies that concurrent executions of ReclaimStale
// are coordinated using a PostgreSQL advisory lock, preventing overlapping runs.
func TestReaper_LockCoordination(t *testing.T) {
	client, _ := newTestPostgres(t)
	repo := repository.NewPostgresPermissionWorkRepository(client)

	// Begin a transaction and lock the advisory key manually to simulate another node holding it
	ctx := context.Background()
	tx, err := client.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const lockKey = 118871
	var acquired bool
	err = tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock($1)", lockKey).Scan(&acquired)
	if err != nil || !acquired {
		t.Fatalf("failed to acquire test lock: %v (acquired=%t)", err, acquired)
	}

	// Now try to run ReclaimStale on the repository concurrently.
	// Since the lock is held in tx, ReclaimStale must exit immediately with 0 rows and NO error.
	n, err := repo.ReclaimStale(ctx, 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error from ReclaimStale when locked: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 rows reclaimed, got %d", n)
	}
}

