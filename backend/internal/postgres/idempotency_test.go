package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
)

func TestCanonicalHashIgnoresObjectKeyOrder(t *testing.T) {
	left, err := postgres.CanonicalHash(map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatal(err)
	}
	right, err := postgres.CanonicalHash(map[string]any{"a": 1, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	if left != right {
		t.Fatalf("hash differs by object key order: %x != %x", left, right)
	}
}

func TestConcurrentFirstUseOfIdempotencyKeySerializes(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	key := application.IdempotencyKey{
		TenantID: "tenant-1", ActorID: "agent-1", Operation: "claim", RequestID: "request-1",
	}
	hash, err := postgres.CanonicalHash(map[string]any{"task_id": "task-1"})
	if err != nil {
		t.Fatal(err)
	}

	firstLocked := make(chan struct{})
	releaseFirst := make(chan struct{})
	results := make(chan *application.IdempotencyRecord, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		err := store.WithTx(context.Background(), func(tx application.Tx) error {
			record, err := tx.LockIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
			if err != nil {
				return err
			}
			close(firstLocked)
			<-releaseFirst
			if err := tx.SaveIdempotencyResponse(
				context.Background(), key, 200, []byte(`{"execution_id":"exe-1"}`), time.Now(),
			); err != nil {
				return err
			}
			results <- record
			return nil
		})
		errs <- err
	}()

	<-firstLocked
	go func() {
		defer wg.Done()
		err := store.WithTx(context.Background(), func(tx application.Tx) error {
			record, err := tx.LockIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
			if err != nil {
				return err
			}
			results <- record
			return nil
		})
		errs <- err
	}()
	time.Sleep(50 * time.Millisecond)
	close(releaseFirst)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var completed int
	for record := range results {
		if record.ResponseCode != nil {
			completed++
			if *record.ResponseCode != 200 || string(record.ResponseBody) != `{"execution_id":"exe-1"}` {
				t.Fatalf("unexpected stable response: %#v", record)
			}
		}
	}
	if completed != 1 {
		t.Fatalf("completed responses=%d want=1", completed)
	}
}

func TestIdempotencyHashMismatchReturnsStableDomainError(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	key := application.IdempotencyKey{
		TenantID: "tenant-1", ActorID: "agent-1", Operation: "claim", RequestID: "request-1",
	}
	first, _ := postgres.CanonicalHash(map[string]any{"task_id": "task-1"})
	second, _ := postgres.CanonicalHash(map[string]any{"task_id": "task-2"})

	if err := store.WithTx(context.Background(), func(tx application.Tx) error {
		_, err := tx.LockIdempotency(context.Background(), key, first, time.Now().Add(time.Hour))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		_, err := tx.LockIdempotency(context.Background(), key, second, time.Now().Add(time.Hour))
		return err
	})
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "idempotency_mismatch" {
		t.Fatalf("expected idempotency_mismatch domain error, got %v", err)
	}
}
