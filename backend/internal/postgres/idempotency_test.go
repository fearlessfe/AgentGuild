package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
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
	secondStarted := make(chan struct{})
	results := make(chan *application.IdempotencyRecord, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	defer release()
	wg.Add(2)

	go func() {
		defer wg.Done()
		err := store.WithTx(context.Background(), func(tx application.Tx) error {
			record, err := tx.AcquireIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
			if err != nil {
				return err
			}
			if !record.Acquired || record.Completed || record.OwnerToken == "" {
				return fmt.Errorf("first acquire state: %#v", record)
			}
			close(firstLocked)
			<-releaseFirst
			if err := tx.CompleteIdempotency(
				context.Background(), key, record.OwnerToken, 200, []byte(`{"execution_id":"exe-1"}`),
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
			close(secondStarted)
			record, err := tx.AcquireIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
			if err != nil {
				return err
			}
			results <- record
			return nil
		})
		errs <- err
	}()
	<-secondStarted
	waitForBlockedIdempotencyQuery(t, db)
	release()
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
		if record.Completed {
			completed++
			if record.Acquired || record.ResponseCode == nil || *record.ResponseCode != 200 ||
				string(record.ResponseBody) != `{"execution_id":"exe-1"}` {
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
		record, err := tx.AcquireIdempotency(context.Background(), key, first, time.Now().Add(time.Hour))
		if err != nil {
			return err
		}
		return tx.CompleteIdempotency(context.Background(), key, record.OwnerToken, 204, nil)
	}); err != nil {
		t.Fatal(err)
	}
	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		_, err := tx.AcquireIdempotency(context.Background(), key, second, time.Now().Add(time.Hour))
		return err
	})
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "idempotency_mismatch" {
		t.Fatalf("expected idempotency_mismatch domain error, got %v", err)
	}
}

func TestIdempotencyCompletionRequiresOwnerAndIsOneShot(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	key := application.IdempotencyKey{
		TenantID: "tenant-1", ActorID: "agent-1", Operation: "claim", RequestID: "owner-test",
	}
	hash, _ := postgres.CanonicalHash(map[string]any{"task_id": "task-1"})

	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		record, err := tx.AcquireIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
		if err != nil {
			return err
		}
		if err := tx.CompleteIdempotency(context.Background(), key, "not-the-owner", 200, []byte("wrong")); err == nil {
			t.Fatal("non-owner completed idempotency record")
		}
		if err := tx.CompleteIdempotency(context.Background(), key, record.OwnerToken, 200, []byte("stable")); err != nil {
			return err
		}
		if err := tx.CompleteIdempotency(context.Background(), key, record.OwnerToken, 201, []byte("overwrite")); err == nil {
			t.Fatal("completed idempotency response was overwritten")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTransactionCannotCommitAcquiredPendingIdempotency(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	key := application.IdempotencyKey{
		TenantID: "tenant-1", ActorID: "agent-1", Operation: "claim", RequestID: "pending-test",
	}
	hash, _ := postgres.CanonicalHash(map[string]any{"task_id": "task-1"})

	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		record, err := tx.AcquireIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
		if err != nil {
			return err
		}
		if !record.Acquired || record.Completed {
			t.Fatalf("unexpected acquire state: %#v", record)
		}
		return nil
	})
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "idempotency_incomplete" {
		t.Fatalf("expected idempotency_incomplete, got %v", err)
	}

	var count int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM idempotency_records`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("pending idempotency record committed: count=%d", count)
	}
}

func TestStorePersistsPendingAndCompletesAcrossShortTransactions(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	key := application.IdempotencyKey{TenantID: "tenant-1", ActorID: "owner-1", Operation: "repository.delete", RequestID: "request-1"}
	hash, _ := postgres.CanonicalHash(map[string]any{"repository_id": "repo-1"})

	acquired, err := store.AcquireIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !acquired.Acquired || acquired.Completed {
		t.Fatalf("unexpected acquire state: %#v", acquired)
	}
	pending, err := store.AcquireIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if pending.Acquired || pending.Completed {
		t.Fatalf("expected committed pending state, got %#v", pending)
	}
	if err := store.CompleteIdempotency(context.Background(), key, acquired.OwnerToken, 200, []byte(`{"data":{"deleted":true}}`)); err != nil {
		t.Fatal(err)
	}
	replay, err := store.AcquireIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Completed || replay.ResponseCode == nil || *replay.ResponseCode != 200 || string(replay.ResponseBody) != `{"data":{"deleted":true}}` {
		t.Fatalf("unexpected replay: %#v", replay)
	}
}

func TestPendingIdempotencyBeforeLeaseExpiryIsNotAcquired(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	key := application.IdempotencyKey{TenantID: "tenant-1", ActorID: "owner-1", Operation: "repository.create", RequestID: "pending-lease"}
	hash, _ := postgres.CanonicalHash(map[string]any{"repo": "octo/one"})

	first, err := store.AcquireIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AcquireIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !first.Acquired || second.Acquired || second.Completed || second.OwnerToken != "" {
		t.Fatalf("unexpected pending lease states: first=%#v second=%#v", first, second)
	}
}

func TestExpiredPendingIdempotencyLeaseCanBeTakenOver(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	key := application.IdempotencyKey{TenantID: "tenant-1", ActorID: "owner-1", Operation: "repository.create", RequestID: "lease-takeover"}
	hash, _ := postgres.CanonicalHash(map[string]any{"repo": "octo/one"})

	first, err := store.AcquireIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(context.Background(), `UPDATE idempotency_records SET updated_at=clock_timestamp()-interval '6 minutes'`); err != nil {
		t.Fatal(err)
	}
	takeover, err := store.AcquireIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !takeover.Acquired || takeover.Completed || takeover.OwnerToken == "" || takeover.OwnerToken == first.OwnerToken {
		t.Fatalf("unexpected takeover state: first=%#v takeover=%#v", first, takeover)
	}
}

func TestConcurrentExpiredPendingLeaseHasExactlyOneTakeoverOwner(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	key := application.IdempotencyKey{TenantID: "tenant-1", ActorID: "owner-1", Operation: "repository.create", RequestID: "concurrent-takeover"}
	hash, _ := postgres.CanonicalHash(map[string]any{"repo": "octo/one"})
	if _, err := store.AcquireIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(context.Background(), `UPDATE idempotency_records SET updated_at=clock_timestamp()-interval '6 minutes'`); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan *application.IdempotencyRecord, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			record, err := store.AcquireIdempotency(context.Background(), key, hash, time.Now().Add(time.Hour))
			results <- record
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var acquired int
	var owner string
	for record := range results {
		if record.Acquired {
			acquired++
			owner = record.OwnerToken
		}
	}
	if acquired != 1 || owner == "" {
		t.Fatalf("takeover acquisitions=%d owner=%q want exactly one", acquired, owner)
	}
}

func TestPendingIdempotencyHashMismatchDuringRetention(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	key := application.IdempotencyKey{TenantID: "tenant-1", ActorID: "owner-1", Operation: "repository.create", RequestID: "pending-mismatch"}
	first, _ := postgres.CanonicalHash(map[string]any{"repo": "octo/one"})
	second, _ := postgres.CanonicalHash(map[string]any{"repo": "octo/two"})
	if _, err := store.AcquireIdempotency(context.Background(), key, first, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	_, err := store.AcquireIdempotency(context.Background(), key, second, time.Now().Add(time.Hour))
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "idempotency_mismatch" {
		t.Fatalf("expected idempotency_mismatch, got %v", err)
	}
}

func TestExpiredIdempotencyRetentionAllowsAtomicKeyReuse(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	key := application.IdempotencyKey{TenantID: "tenant-1", ActorID: "owner-1", Operation: "repository.create", RequestID: "retention-reuse"}
	firstHash, _ := postgres.CanonicalHash(map[string]any{"repo": "octo/one"})
	secondHash, _ := postgres.CanonicalHash(map[string]any{"repo": "octo/two"})
	first, err := store.AcquireIdempotency(context.Background(), key, firstHash, time.Now().Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	reused, err := store.AcquireIdempotency(context.Background(), key, secondHash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !reused.Acquired || reused.Completed || reused.OwnerToken == "" || reused.OwnerToken == first.OwnerToken || reused.RequestHash != secondHash {
		t.Fatalf("unexpected reused state: first=%#v reused=%#v", first, reused)
	}
}

func waitForBlockedIdempotencyQuery(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var blocked bool
		err := db.QueryRow(context.Background(), `
			SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity
				WHERE datname=current_database()
				  AND wait_event_type='Lock'
				  AND query LIKE '%idempotency_records%'
			)`).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("second idempotency acquire never became lock-blocked")
		}
	}
}
