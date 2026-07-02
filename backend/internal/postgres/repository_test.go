package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOnlyOneNonTerminalExecutionPerTask(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedTask(t, db, "tenant-1", "task-1")
	insertExecution(t, db, "tenant-1", "exe-1", "task-1", "leased")

	_, err := db.Exec(context.Background(), `
		INSERT INTO executions
			(tenant_id, id, task_id, agent_version_id, status, lease_generation)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		"tenant-1", "exe-2", "task-1", "agent-1", "running", 1,
	)
	if err == nil {
		t.Fatal("expected a second active execution to violate the partial unique index")
	}
	if !contains(err.Error(), "executions_one_active_per_task") {
		t.Fatalf("expected executions_one_active_per_task violation, got %v", err)
	}
}

func TestTransactionUsesOneDatabaseTimestamp(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)

	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		first, err := tx.Now(context.Background())
		if err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
		second, err := tx.Now(context.Background())
		if err != nil {
			return err
		}
		if !first.Equal(second) {
			t.Fatalf("transaction database time changed: %v != %v", first, second)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTaskRepositoryIsTenantScopedAndClaimIsConditional(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedTask(t, db, "tenant-a", "shared")
	seedTask(t, db, "tenant-b", "shared")
	store := postgres.NewStore(db)
	rollbackClaim := errors.New("rollback claim fixture")

	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		task, version, err := tx.GetTask(context.Background(), "tenant-a", "shared")
		if err != nil {
			return err
		}
		if task.TenantID != "tenant-a" || version != 0 {
			t.Fatalf("unexpected task: %#v version=%d", task, version)
		}
		now, err := tx.Now(context.Background())
		if err != nil {
			return err
		}
		claimed, err := tx.ClaimTask(context.Background(), "tenant-a", "shared", version, "execution-a", now)
		if err != nil || !claimed {
			t.Fatalf("claim task: claimed=%v err=%v", claimed, err)
		}
		staleClaim, err := tx.ClaimTask(context.Background(), "tenant-a", "shared", version, "execution-b", now)
		if err != nil {
			return err
		}
		if staleClaim {
			t.Fatal("stale state version unexpectedly claimed task")
		}
		other, _, err := tx.GetTask(context.Background(), "tenant-b", "shared")
		if err != nil {
			return err
		}
		if other.Status != domain.TaskOpen {
			t.Fatalf("other tenant changed: %s", other.Status)
		}
		return rollbackClaim
	})
	if !errors.Is(err, rollbackClaim) {
		t.Fatalf("expected fixture rollback, got %v", err)
	}
}

func TestExecutionRepositoryIsTenantScoped(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedTask(t, db, "tenant-a", "task")
	seedTask(t, db, "tenant-b", "task")
	insertExecution(t, db, "tenant-a", "shared", "task", "expired")
	insertExecution(t, db, "tenant-b", "shared", "task", "accepted")
	store := postgres.NewStore(db)

	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		execution, _, err := tx.GetExecution(context.Background(), "tenant-a", "shared")
		if err != nil {
			return err
		}
		if execution.TenantID != "tenant-a" || execution.Status != domain.ExecutionExpired {
			t.Fatalf("unexpected execution: %#v", execution)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTransactionRollsBackTaskIdempotencyAuditAndOutbox(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedTask(t, db, "tenant-1", "task-1")
	store := postgres.NewStore(db)
	rollback := errors.New("force rollback")

	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		now, err := tx.Now(context.Background())
		if err != nil {
			return err
		}
		claimed, err := tx.ClaimTask(context.Background(), "tenant-1", "task-1", 0, "exe-1", now)
		if err != nil || !claimed {
			t.Fatalf("claim: claimed=%v err=%v", claimed, err)
		}
		execution, err := domain.NewLeasedExecution("exe-1", "task-1", "tenant-1", "agent-1", now, 1)
		if err != nil {
			return err
		}
		if err := tx.InsertExecution(context.Background(), execution, []byte("secret")); err != nil {
			return err
		}
		hash, err := postgres.CanonicalHash(map[string]any{"task_id": "task-1"})
		if err != nil {
			return err
		}
		key := application.IdempotencyKey{TenantID: "tenant-1", ActorID: "agent-1", Operation: "claim", RequestID: "request-1"}
		if _, err := tx.LockIdempotency(context.Background(), key, hash, now.Add(time.Hour)); err != nil {
			return err
		}
		if err := tx.SaveIdempotencyResponse(context.Background(), key, 200, []byte(`{"execution_id":"exe-1"}`), now); err != nil {
			return err
		}
		event := application.TaskEvent{
			TenantID: "tenant-1", TaskID: "task-1", ExecutionID: "exe-1",
			ActorType: "agent", ActorID: "agent-1", Intent: "claim",
			FromState: "open", ToState: "active", Payload: []byte(`{}`), CreatedAt: now,
		}
		if err := tx.AppendTaskEvent(context.Background(), event); err != nil {
			return err
		}
		outbox := application.OutboxEvent{
			TenantID: "tenant-1", ID: "outbox-1", EventType: "task.claimed",
			AggregateType: "task", AggregateID: "task-1", Payload: []byte(`{}`), AvailableAt: now,
		}
		if err := tx.AppendOutboxEvent(context.Background(), outbox); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("expected rollback error, got %v", err)
	}

	for table, want := range map[string]int64{
		"executions": 0, "idempotency_records": 0, "task_events": 0, "outbox_events": 0,
	} {
		var got int64
		if err := db.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s count=%d want=%d", table, got, want)
		}
	}
	var status string
	var version int64
	if err := db.QueryRow(context.Background(),
		`SELECT status, state_version FROM tasks WHERE tenant_id='tenant-1' AND id='task-1'`,
	).Scan(&status, &version); err != nil {
		t.Fatal(err)
	}
	if status != "open" || version != 0 {
		t.Fatalf("task was not rolled back: status=%s version=%d", status, version)
	}
}

func seedTask(t *testing.T, db *pgxpool.Pool, tenantID, taskID string) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO tasks
			(tenant_id, id, publisher_agent_version_id, type, title, problem, constraints, requirements, deadline, status)
		VALUES ($1, $2, $3, 'code', 'title', 'problem', '{}'::jsonb, '{}'::jsonb, $4, 'open')`,
		tenantID, taskID, "publisher-1", time.Now().Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
}

func insertExecution(t *testing.T, db *pgxpool.Pool, tenantID, executionID, taskID, status string) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO executions
			(tenant_id, id, task_id, agent_version_id, status, lease_generation)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		tenantID, executionID, taskID, "agent-1", status, 1,
	)
	if err != nil {
		t.Fatalf("insert execution: %v", err)
	}
}

func contains(value, fragment string) bool {
	return len(fragment) == 0 || len(value) >= len(fragment) && index(value, fragment) >= 0
}

func index(value, fragment string) int {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return i
		}
	}
	return -1
}
