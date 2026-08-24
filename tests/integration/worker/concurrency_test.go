//go:build integration

package worker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/canonical/authorization-service/internal/repository"
	"github.com/canonical/authorization-service/internal/service/listen"
	"github.com/canonical/authorization-service/internal/service/worker"
	"github.com/canonical/authorization-service/tests/integration/suite"
)

// TestWorker_ParallelConcurrency seeds a large batch of work rows and spawns
// multiple parallel worker instances. It verifies that SKIP LOCKED prevents
// duplicate work, no deadlocks occur, and all rows transition to 'processed'.
func TestWorker_ParallelConcurrency(t *testing.T) {
	client, pool := newTestPostgres(t)
	repo := repository.NewPostgresPermissionWorkRepository(client, nil)

	suffix := suite.UniqueSuffix()
	const totalRows = 60
	const numWorkers = 4

	for i := 0; i < totalRows; i++ {
		idem := fmt.Sprintf("worker-conc-%d-%s", i, suffix)
		subject := fmt.Sprintf("user:u-%d-%s", i, suffix)
		insertReceivedRow(t, repo, "payments", idem, writeOperation(subject, "member", "group:g1"))
	}

	applier := &fakeApplier{}
	proc := worker.NewProcessor(repo, applier, listen.NewDecoder(), 10, false, nil, suite.TestLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		w := worker.NewWorker(repo, proc, 10, 20*time.Millisecond, time.Minute, nil, suite.TestLogger)
		go func() {
			defer wg.Done()
			if err := w.Run(ctx); err != nil {
				errCh <- err
			}
		}()
	}

	// Poll until all totalRows rows are marked 'processed' in Postgres.
	pattern := fmt.Sprintf("worker-conc-%%-%s", suffix)
	deadline := time.Now().Add(20 * time.Second)
	var processedCount int
	for time.Now().Before(deadline) {
		var n int
		err := pool.QueryRow(context.Background(),
			`SELECT count(*) FROM permission_update_work WHERE service = 'payments' AND status = 'processed' AND idempotency_key LIKE $1`,
			pattern,
		).Scan(&n)
		if err == nil && n == totalRows {
			processedCount = n
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	cancel()
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("worker error under concurrency: %v", err)
		}
	}

	if processedCount != totalRows {
		t.Fatalf("expected %d rows processed concurrently, got %d", totalRows, processedCount)
	}

	// Verify thread-safe applier received totalRows tuple writes across batches (no double-processing)
	applier.mu.Lock()
	applied := applier.applied
	applier.mu.Unlock()

	if applied != totalRows {
		t.Fatalf("expected applier to apply %d batches, got %d", totalRows, applied)
	}

	// Verify authorization_tuples count in database
	subjPattern := fmt.Sprintf("user:u-%%-%s", suffix)
	var tupleCount int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM authorization_tuples WHERE relation = 'member' AND object = 'group:g1' AND subject LIKE $1`,
		subjPattern,
	).Scan(&tupleCount)
	if err != nil {
		t.Fatalf("query authorization_tuples count: %v", err)
	}
	if tupleCount != totalRows {
		t.Fatalf("expected %d mirrored tuples in DB, got %d", totalRows, tupleCount)
	}
}
