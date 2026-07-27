//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/openfga/go-sdk/client"

	"github.com/canonical/authorization-service/internal/repository"
	"github.com/canonical/authorization-service/internal/service/listen"
	"github.com/canonical/authorization-service/internal/service/worker"
)

type multitenancyFakeApplier struct {
	mu     sync.Mutex
	writes []client.ClientTupleKey
	called bool
}

func (a *multitenancyFakeApplier) ApplyTuples(_ context.Context, writes []client.ClientTupleKey, _ []client.ClientTupleKeyWithoutCondition) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.called = true
	a.writes = writes
	return nil
}

func TestWorker_MultitenancyEnabled_Integration(t *testing.T) {
	client, pool := newTestPostgres(t)
	repo := repository.NewPostgresPermissionWorkRepository(client)

	suffix := uniqueSuffix()
	idem := "worker-multitenancy-" + suffix
	subject := "user:u-" + suffix
	serviceName := "payments"

	insertReceivedRow(t, repo, serviceName, idem, writeOperation(subject, "member", "group:g1"))

	applier := &multitenancyFakeApplier{}
	// Create Processor with multitenancyEnabled = true
	proc := worker.NewProcessor(repo, applier, listen.NewDecoder(), 5, true, nil, testLogger)
	w := worker.NewWorker(repo, proc, 50, 100*time.Millisecond, time.Minute, testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := startWorker(ctx, w)

	waitForStatus(t, pool, serviceName, idem, "processed")

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("worker shutdown error: %v", err)
	}

	applier.mu.Lock()
	defer applier.mu.Unlock()

	if !applier.called {
		t.Fatal("expected applier to be called, but it wasn't")
	}
	if len(applier.writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(applier.writes))
	}
	wTuple := applier.writes[0]
	if wTuple.Condition == nil {
		t.Fatal("expected written tuple to have a non-nil Condition")
	}
	if wTuple.Condition.Name != "tenant_match" {
		t.Errorf("expected Condition Name 'tenant_match', got %q", wTuple.Condition.Name)
	}
	if wTuple.Condition.Context == nil {
		t.Fatal("expected Condition Context to be non-nil")
	}
	ctxMap := *wTuple.Condition.Context
	if ctxMap["tenant"] != serviceName {
		t.Errorf("expected Condition tenant context to be %q, got %v", serviceName, ctxMap["tenant"])
	}
}

func TestWorker_MultitenancyDisabled_Integration(t *testing.T) {
	client, pool := newTestPostgres(t)
	repo := repository.NewPostgresPermissionWorkRepository(client)

	suffix := uniqueSuffix()
	idem := "worker-no-multitenancy-" + suffix
	subject := "user:u-" + suffix
	serviceName := "payments"

	insertReceivedRow(t, repo, serviceName, idem, writeOperation(subject, "member", "group:g1"))

	applier := &multitenancyFakeApplier{}
	// Create Processor with multitenancyEnabled = false
	proc := worker.NewProcessor(repo, applier, listen.NewDecoder(), 5, false, nil, testLogger)
	w := worker.NewWorker(repo, proc, 50, 100*time.Millisecond, time.Minute, testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := startWorker(ctx, w)

	waitForStatus(t, pool, serviceName, idem, "processed")

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("worker shutdown error: %v", err)
	}

	applier.mu.Lock()
	defer applier.mu.Unlock()

	if !applier.called {
		t.Fatal("expected applier to be called, but it wasn't")
	}
	if len(applier.writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(applier.writes))
	}
	wTuple := applier.writes[0]
	if wTuple.Condition != nil {
		t.Errorf("expected written tuple to have a nil Condition, got %+v", wTuple.Condition)
	}
}
