package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTaskPersistenceRecordRoundTripsWithoutInventedContent(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	want := application.TaskRecord{
		ID: "task-lossless", TenantID: "tenant-1", PublisherAgentVersionID: "publisher-7",
		Type: "code-review", Title: "Review the scheduler", Problem: "Find lifecycle races",
		Constraints:  []byte(`{"network":"offline","max_minutes":15}`),
		Requirements: []byte(`{"language":"go","tests":true}`),
		Deadline:     time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond),
		Status:       domain.TaskStatus("active"),
	}

	if err := store.WithTx(context.Background(), func(tx application.Tx) error {
		if err := tx.InsertTask(context.Background(), want); err != nil {
			return err
		}
		got, err := tx.GetTask(context.Background(), want.TenantID, want.ID)
		if err != nil {
			return err
		}
		if got.ID != want.ID || got.TenantID != want.TenantID ||
			got.PublisherAgentVersionID != want.PublisherAgentVersionID ||
			got.Type != want.Type || got.Title != want.Title || got.Problem != want.Problem ||
			!got.Deadline.Equal(want.Deadline) || got.Status != want.Status {
			t.Fatalf("task record changed: got=%#v want=%#v", got, want)
		}
		assertJSONEqual(t, got.Constraints, want.Constraints)
		assertJSONEqual(t, got.Requirements, want.Requirements)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

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

func TestActiveExecutionCannotBelongToAnotherTask(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedTask(t, db, "tenant-1", "task-a")
	seedTask(t, db, "tenant-1", "task-b")
	insertExecution(t, db, "tenant-1", "exe-a", "task-a", "leased")

	_, err := db.Exec(context.Background(), `
		UPDATE tasks SET active_execution_id='exe-a'
		WHERE tenant_id='tenant-1' AND id='task-b'`)
	if err == nil {
		t.Fatal("active_execution_id accepted an execution belonging to another task")
	}
}

func TestTaskEventExecutionCannotBelongToAnotherTask(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedTask(t, db, "tenant-1", "task-a")
	seedTask(t, db, "tenant-1", "task-b")
	insertExecution(t, db, "tenant-1", "exe-a", "task-a", "accepted")

	_, err := db.Exec(context.Background(), `
		INSERT INTO task_events
			(tenant_id, task_id, execution_id, actor_type, actor_id, intent, from_state, to_state)
		VALUES ('tenant-1', 'task-b', 'exe-a', 'system', 'scheduler', 'test', 'open', 'open')`)
	if err == nil {
		t.Fatal("task event accepted an execution belonging to another task")
	}
}

func TestExecutionUsageCannotBelongToAnotherTask(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedTask(t, db, "tenant-1", "task-a")
	seedTask(t, db, "tenant-1", "task-b")
	insertExecution(t, db, "tenant-1", "exe-a", "task-a", "accepted")

	_, err := db.Exec(context.Background(), `
		INSERT INTO execution_usage
			(tenant_id, task_id, execution_id, agent_version_id, coverage, provider, source_cursor, observed_at)
		VALUES ('tenant-1', 'task-b', 'exe-a', 'agent-1', 'complete', 'test', 'cursor-1', clock_timestamp())`)
	if err == nil {
		t.Fatal("execution usage accepted an execution belonging to another task")
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

func TestRepositoriesReuseTransactionDatabaseTimestamp(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	var want time.Time

	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		var err error
		want, err = tx.Now(context.Background())
		if err != nil {
			return err
		}
		task := application.TaskRecord{
			ID: "task-time", TenantID: "tenant-1", PublisherAgentVersionID: "publisher-1",
			Type: "code", Title: "title", Problem: "problem",
			Constraints: []byte(`{}`), Requirements: []byte(`{}`),
			Deadline: want.Add(time.Hour), Status: domain.TaskOpen,
		}
		if err := tx.InsertTask(context.Background(), task); err != nil {
			return err
		}
		execution, err := domain.NewLeasedExecution(
			"exe-time", task.ID, task.TenantID, "agent-1", want, 1,
		)
		if err != nil {
			return err
		}
		if err := tx.InsertExecution(context.Background(), execution, []byte("secret")); err != nil {
			return err
		}
		claimed, err := tx.ClaimTask(
			context.Background(), task.TenantID, task.ID, 0, execution.ID,
		)
		if err != nil || !claimed {
			t.Fatalf("claim task: claimed=%v err=%v", claimed, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var taskCreated, taskUpdated, executionCreated, executionUpdated, claimedAt time.Time
	if err := db.QueryRow(context.Background(), `
		SELECT t.created_at, t.updated_at, e.created_at, e.updated_at, e.claimed_at
		FROM tasks t JOIN executions e
		  ON e.tenant_id=t.tenant_id AND e.task_id=t.id
		WHERE t.tenant_id='tenant-1' AND t.id='task-time'`,
	).Scan(&taskCreated, &taskUpdated, &executionCreated, &executionUpdated, &claimedAt); err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]time.Time{
		"task.created_at": taskCreated, "task.updated_at": taskUpdated,
		"execution.created_at": executionCreated, "execution.updated_at": executionUpdated,
		"execution.claimed_at": claimedAt,
	} {
		if !got.Equal(want) {
			t.Errorf("%s=%v, want cached transaction time %v", name, got, want)
		}
	}
}

func TestTaskRepositoryIsTenantScopedAndClaimIsConditional(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedTask(t, db, "tenant-a", "shared")
	seedTask(t, db, "tenant-b", "shared")
	store := postgres.NewStore(db)
	rollbackClaim := errors.New("rollback claim fixture")

	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		task, err := tx.GetTask(context.Background(), "tenant-a", "shared")
		if err != nil {
			return err
		}
		if task.TenantID != "tenant-a" || task.StateVersion != 0 {
			t.Fatalf("unexpected task: %#v", task)
		}
		claimed, err := tx.ClaimTask(context.Background(), "tenant-a", "shared", task.StateVersion, "execution-a")
		if err != nil || !claimed {
			t.Fatalf("claim task: claimed=%v err=%v", claimed, err)
		}
		staleClaim, err := tx.ClaimTask(context.Background(), "tenant-a", "shared", task.StateVersion, "execution-b")
		if err != nil {
			return err
		}
		if staleClaim {
			t.Fatal("stale state version unexpectedly claimed task")
		}
		other, err := tx.GetTask(context.Background(), "tenant-b", "shared")
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

func TestTaskListIsTenantFilteredAndKeysetOrdered(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedTask(t, db, "tenant-a", "task-1")
	seedTask(t, db, "tenant-a", "task-2")
	seedTask(t, db, "tenant-b", "task-3")
	for id, createdAt := range map[string]time.Time{"task-1": fixtureTime(1), "task-2": fixtureTime(2)} {
		if _, err := db.Exec(context.Background(), `UPDATE tasks SET created_at=$1 WHERE tenant_id='tenant-a' AND id=$2`, createdAt, id); err != nil {
			t.Fatal(err)
		}
	}
	store := postgres.NewStore(db)
	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		first, err := tx.ListTaskRecords(context.Background(), application.TaskListQuery{TenantID: "tenant-a", Statuses: []domain.TaskStatus{domain.TaskOpen}, Limit: 1})
		if err != nil || len(first) != 1 || first[0].ID != "task-2" {
			t.Fatalf("first page=%#v err=%v", first, err)
		}
		second, err := tx.ListTaskRecords(context.Background(), application.TaskListQuery{TenantID: "tenant-a", Statuses: []domain.TaskStatus{domain.TaskOpen}, AfterCreatedAt: first[0].CreatedAt, AfterID: first[0].ID, Limit: 2})
		if err != nil || len(second) != 1 || second[0].ID != "task-1" {
			t.Fatalf("second page=%#v err=%v", second, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func fixtureTime(minute int) time.Time {
	return time.Date(2026, 7, 2, 10, minute, 0, 0, time.UTC)
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

func TestListActiveExecutionsIsTenantTaskScopedAndComplete(t *testing.T) {
	db := testdb.StartPostgres(t)
	active := []string{"leased", "running", "submitted", "validating", "reviewing", "revision_requested"}
	for i, status := range active {
		taskID := fmt.Sprintf("task-%d", i)
		seedTask(t, db, "tenant-a", taskID)
		seedTask(t, db, "tenant-b", taskID)
		insertExecution(t, db, "tenant-a", fmt.Sprintf("e-%d", i), taskID, status)
		insertExecution(t, db, "tenant-b", fmt.Sprintf("other-%d", i), taskID, "running")
	}
	store := postgres.NewStore(db)
	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		for i := range active {
			records, err := tx.ListActiveExecutions(context.Background(), "tenant-a", fmt.Sprintf("task-%d", i))
			if err != nil {
				return err
			}
			if len(records) != 1 || records[0].Execution.ID != fmt.Sprintf("e-%d", i) {
				t.Fatalf("status %s records=%#v", active[i], records)
			}
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
		claimed, err := tx.ClaimTask(context.Background(), "tenant-1", "task-1", 0, "exe-1")
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
		record, err := tx.AcquireIdempotency(context.Background(), key, hash, now.Add(time.Hour))
		if err != nil {
			return err
		}
		if err := tx.CompleteIdempotency(
			context.Background(), key, record.OwnerToken, 200, []byte(`{"execution_id":"exe-1"}`),
		); err != nil {
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

func assertJSONEqual(t *testing.T, got, want []byte) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decode got JSON %q: %v", got, err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("decode want JSON %q: %v", want, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON differs: got=%s want=%s", got, want)
	}
}
