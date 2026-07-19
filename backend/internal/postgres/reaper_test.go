package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReaperReopensHardExpiredTaskAndIsIdempotent(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedReaperExecution(t, db, "task-1", "execution-1", time.Now().Add(time.Hour), time.Now().Add(-time.Second))
	reaper := postgres.NewReaper(db)

	count, err := reaper.RunBatch(context.Background(), 10)
	if err != nil || count != 1 {
		t.Fatalf("RunBatch()=%d, %v", count, err)
	}
	count, err = reaper.RunBatch(context.Background(), 10)
	if err != nil || count != 0 {
		t.Fatalf("second RunBatch()=%d, %v", count, err)
	}
	assertReapedState(t, db, "task-1", "execution-1", "open", "expired", 1)
}

func TestReaperSkipsInconsistentTaskRowAndContinues(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedReaperExecution(t, db, "task-1", "execution-1", time.Now().Add(time.Hour), time.Now().Add(-time.Second))
	seedReaperExecution(t, db, "task-2", "execution-2", time.Now().Add(time.Hour), time.Now().Add(-time.Second))

	// Simulate a concurrent state change that makes the task row inconsistent.
	if _, err := db.Exec(context.Background(), `
		UPDATE tasks SET status='open', active_execution_id=NULL
		WHERE tenant_id='tenant-1' AND id='task-1'`); err != nil {
		t.Fatal(err)
	}

	reaper := postgres.NewReaper(db)
	count, err := reaper.RunBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunBatch() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("RunBatch()=%d, want 1", count)
	}

	var events, outbox int
	if err := db.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM task_events WHERE task_id='task-2' AND intent='expire'),
			(SELECT count(*) FROM outbox_events WHERE aggregate_id='task-2')`).Scan(&events, &outbox); err != nil {
		t.Fatal(err)
	}
	if events != 1 || outbox != 1 {
		t.Fatalf("task-2 events=%d outbox=%d", events, outbox)
	}
}

func TestReaperExpiresTaskAtDeadlineEvenWithFutureLease(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedReaperExecution(t, db, "task-1", "execution-1", time.Now().Add(-time.Second), time.Now().Add(time.Hour))

	count, err := postgres.NewReaper(db).RunBatch(context.Background(), 10)
	if err != nil || count != 1 {
		t.Fatalf("RunBatch()=%d, %v", count, err)
	}
	assertReapedState(t, db, "task-1", "execution-1", "expired", "expired", 1)
}

func TestMultipleReapersSkipLockedAndProcessEachExecutionOnce(t *testing.T) {
	db := testdb.StartPostgres(t)
	for i := 0; i < 20; i++ {
		id := time.Now().Add(time.Duration(i) * time.Nanosecond).Format("150405.000000000")
		seedReaperExecution(t, db, "task-"+id, "execution-"+id, time.Now().Add(time.Hour), time.Now().Add(-time.Second))
	}
	var wg sync.WaitGroup
	counts := make(chan int, 4)
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			count, err := postgres.NewReaper(db).RunBatch(context.Background(), 20)
			counts <- count
			errs <- err
		}()
	}
	wg.Wait()
	close(counts)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	total := 0
	for count := range counts {
		total += count
	}
	if total != 20 {
		t.Fatalf("processed=%d", total)
	}
	var events int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM task_events WHERE intent='expire'`).Scan(&events); err != nil || events != 20 {
		t.Fatalf("events=%d err=%v", events, err)
	}
}

func TestReaperExpiresUnclaimedOpenTaskPastDeadline(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedReaperOpenTask(t, db, "task-open-1", time.Now().Add(-time.Second))
	reaper := postgres.NewReaper(db)

	count, err := reaper.RunBatch(context.Background(), 10)
	if err != nil || count != 1 {
		t.Fatalf("RunBatch()=%d, %v", count, err)
	}
	count, err = reaper.RunBatch(context.Background(), 10)
	if err != nil || count != 0 {
		t.Fatalf("second RunBatch()=%d, %v", count, err)
	}

	var status, actorType, actorID, fromState, toState string
	var executionID *string
	var eventCount, outboxCount int
	err = db.QueryRow(context.Background(), `
		SELECT t.status,
		       (SELECT count(*) FROM task_events WHERE tenant_id='tenant-1' AND task_id=t.id AND intent='expire'),
		       (SELECT count(*) FROM outbox_events WHERE tenant_id='tenant-1' AND aggregate_id=t.id AND event_type='task.expired')
		FROM tasks t WHERE t.tenant_id='tenant-1' AND t.id='task-open-1'`).Scan(&status, &eventCount, &outboxCount)
	if err != nil {
		t.Fatal(err)
	}
	if status != "expired" || eventCount != 1 || outboxCount != 1 {
		t.Fatalf("status=%s events=%d outbox=%d", status, eventCount, outboxCount)
	}
	err = db.QueryRow(context.Background(), `
		SELECT execution_id, actor_type, actor_id, from_state, to_state
		FROM task_events WHERE tenant_id='tenant-1' AND task_id='task-open-1' AND intent='expire'`).
		Scan(&executionID, &actorType, &actorID, &fromState, &toState)
	if err != nil {
		t.Fatal(err)
	}
	if executionID != nil || actorType != "system" || actorID != "reaper" || fromState != "open" || toState != "expired" {
		t.Fatalf("event execution=%v actor=%s/%s transition=%s->%s", executionID, actorType, actorID, fromState, toState)
	}
}

func TestReaperLeavesUnclaimedOpenTaskBeforeDeadline(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedReaperOpenTask(t, db, "task-open-1", time.Now().Add(time.Hour))

	count, err := postgres.NewReaper(db).RunBatch(context.Background(), 10)
	if err != nil || count != 0 {
		t.Fatalf("RunBatch()=%d, %v", count, err)
	}

	var status string
	var eventCount, outboxCount int
	err = db.QueryRow(context.Background(), `
		SELECT t.status,
		       (SELECT count(*) FROM task_events WHERE tenant_id='tenant-1' AND task_id=t.id),
		       (SELECT count(*) FROM outbox_events WHERE tenant_id='tenant-1' AND aggregate_id=t.id)
		FROM tasks t WHERE t.tenant_id='tenant-1' AND t.id='task-open-1'`).Scan(&status, &eventCount, &outboxCount)
	if err != nil {
		t.Fatal(err)
	}
	if status != "open" || eventCount != 0 || outboxCount != 0 {
		t.Fatalf("status=%s events=%d outbox=%d", status, eventCount, outboxCount)
	}
}

func seedReaperOpenTask(t *testing.T, db *pgxpool.Pool, taskID string, deadline time.Time) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO tasks (tenant_id, id, publisher_agent_version_id, type, title, problem, constraints, requirements, deadline, status)
		VALUES ('tenant-1', $1, 'publisher', 'code', 'title', 'problem', '[]', '[]', $2, 'open');
		`, taskID, deadline)
	if err != nil {
		t.Fatal(err)
	}
}

func seedReaperExecution(t *testing.T, db *pgxpool.Pool, taskID, executionID string, deadline, hardExpiry time.Time) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO tasks (tenant_id, id, publisher_agent_version_id, type, title, problem, constraints, requirements, deadline, status)
		VALUES ('tenant-1', $1, 'publisher', 'code', 'title', 'problem', '[]', '[]', $2, 'claimed');
		`, taskID, deadline)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(context.Background(), `
		INSERT INTO executions (tenant_id, id, task_id, agent_version_id, status, lease_secret_hash, lease_generation, lease_soft_expires_at, lease_hard_expires_at)
		VALUES ('tenant-1', $1, $2, 'worker', 'leased', '\\x00', 1, $3, $4)`, executionID, taskID, hardExpiry.Add(-30*time.Second), hardExpiry)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(context.Background(), `
		UPDATE tasks SET active_execution_id=$1 WHERE tenant_id='tenant-1' AND id=$2`, executionID, taskID)
	if err != nil {
		t.Fatal(err)
	}
}

func assertReapedState(t *testing.T, db *pgxpool.Pool, taskID, executionID, taskStatus, executionStatus string, events int) {
	t.Helper()
	var gotTask, gotExecution, activeExecutionID string
	var eventCount, outboxCount int
	err := db.QueryRow(context.Background(), `
		SELECT t.status, e.status, COALESCE(t.active_execution_id, ''),
		       (SELECT count(*) FROM task_events WHERE task_id=$1 AND execution_id=$2 AND intent='expire'),
		       (SELECT count(*) FROM outbox_events WHERE aggregate_id=$1)
		FROM tasks t JOIN executions e ON e.tenant_id=t.tenant_id AND e.task_id=t.id
		WHERE t.tenant_id='tenant-1' AND t.id=$1 AND e.id=$2`, taskID, executionID).Scan(&gotTask, &gotExecution, &activeExecutionID, &eventCount, &outboxCount)
	if err != nil || gotTask != taskStatus || gotExecution != executionStatus || activeExecutionID != "" || eventCount != events || outboxCount != events {
		t.Fatalf("task=%s execution=%s active=%q events=%d outbox=%d err=%v", gotTask, gotExecution, activeExecutionID, eventCount, outboxCount, err)
	}
}
