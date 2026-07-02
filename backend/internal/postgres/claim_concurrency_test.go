package postgres_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConcurrentClaimHasExactlyOneWinner(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedClaimTask(t, db, "task-1", time.Now().Add(time.Hour))
	svc := integrationService(t, db)

	var successes atomic.Int64
	var conflicts atomic.Int64
	var unexpected error
	var unexpectedMu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.ClaimTask(context.Background(), agentPrincipal(i), application.ClaimTask{RequestID: fmt.Sprintf("req-%d", i), TaskID: "task-1"})
			switch domain.CodeOf(err) {
			case "":
				successes.Add(1)
			case "state_conflict":
				conflicts.Add(1)
			default:
				unexpectedMu.Lock()
				if unexpected == nil {
					unexpected = err
				}
				unexpectedMu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if unexpected != nil {
		t.Fatalf("unexpected claim error: %v", unexpected)
	}
	if successes.Load() != 1 || conflicts.Load() != 99 {
		t.Fatalf("successes=%d conflicts=%d", successes.Load(), conflicts.Load())
	}
}

func TestClaimRetryReturnsOneStableExecution(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedClaimTask(t, db, "task-1", time.Now().Add(time.Hour))
	svc := integrationService(t, db)
	p := agentPrincipal(1)
	command := application.ClaimTask{RequestID: "same", TaskID: "task-1"}
	first, err := svc.ClaimTask(context.Background(), p, command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.ClaimTask(context.Background(), p, command)
	if err != nil {
		t.Fatal(err)
	}
	if first.Data.ID != second.Data.ID || first.Data.LeaseGeneration != second.Data.LeaseGeneration || first.Meta != second.Meta {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	var count int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM executions WHERE task_id='task-1'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("execution count=%d err=%v", count, err)
	}
}

func TestHeartbeatGenerationIsFencedInPostgres(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedClaimTask(t, db, "task-1", time.Now().Add(time.Hour))
	svc := integrationService(t, db)
	p := agentPrincipal(1)
	claimed, err := svc.ClaimTask(context.Background(), p, application.ClaimTask{RequestID: "claim", TaskID: "task-1"})
	if err != nil {
		t.Fatal(err)
	}
	beat, err := svc.HeartbeatExecution(context.Background(), p, application.HeartbeatExecution{RequestID: "beat-1", ExecutionID: claimed.Data.ID, LeaseGeneration: 1})
	if err != nil {
		t.Fatal(err)
	}
	if beat.Data.LeaseGeneration != 2 {
		t.Fatalf("generation=%d", beat.Data.LeaseGeneration)
	}
	_, err = svc.HeartbeatExecution(context.Background(), p, application.HeartbeatExecution{RequestID: "beat-stale", ExecutionID: claimed.Data.ID, LeaseGeneration: 1})
	if domain.CodeOf(err) != "lease_expired" {
		t.Fatalf("stale heartbeat err=%v", err)
	}
}

func TestClaimMapsActiveExecutionUniqueViolationToStateConflict(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedClaimTask(t, db, "task-1", time.Now().Add(time.Hour))
	_, err := db.Exec(context.Background(), `
		INSERT INTO executions (
			tenant_id, id, task_id, agent_version_id, status, lease_secret_hash,
			lease_generation, lease_soft_expires_at, lease_hard_expires_at
		) VALUES ('tenant-1', 'existing', 'task-1', 'other-agent', 'leased', '\\x00', 1,
			clock_timestamp() + interval '10 minutes', clock_timestamp() + interval '11 minutes')`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = integrationService(t, db).ClaimTask(context.Background(), agentPrincipal(1), application.ClaimTask{
		RequestID: "unique-defense", TaskID: "task-1",
	})
	if domain.CodeOf(err) != "state_conflict" {
		t.Fatalf("claim error=%v, want state_conflict", err)
	}

	var status, activeExecutionID string
	var executionCount, idempotencyCount int
	err = db.QueryRow(context.Background(), `
		SELECT status, COALESCE(active_execution_id, ''),
		       (SELECT count(*) FROM executions WHERE tenant_id='tenant-1' AND task_id='task-1'),
		       (SELECT count(*) FROM idempotency_records WHERE tenant_id='tenant-1' AND request_id='unique-defense')
		FROM tasks WHERE tenant_id='tenant-1' AND id='task-1'`).Scan(
		&status, &activeExecutionID, &executionCount, &idempotencyCount,
	)
	if err != nil {
		t.Fatal(err)
	}
	if status != "open" || activeExecutionID != "" || executionCount != 1 || idempotencyCount != 0 {
		t.Fatalf("partial claim: status=%s active=%q executions=%d idempotency=%d", status, activeExecutionID, executionCount, idempotencyCount)
	}
}

func TestStartAndReaperUseExecutionThenTaskLockOrder(t *testing.T) {
	db := testdb.StartPostgres(t)
	hardExpiry := time.Now().Add(500 * time.Millisecond)
	seedReaperExecution(t, db, "task-1", "execution-1", time.Now().Add(time.Hour), hardExpiry)
	installLifecycleBlockTrigger(t, db, "tasks", "NEW.status = 'in_progress'", 41001)
	unblock := holdAdvisoryLock(t, db, 41001)
	svc := integrationService(t, db)

	startErr := make(chan error, 1)
	go func() {
		_, err := svc.StartExecution(context.Background(), workerPrincipal(), application.StartExecution{
			RequestID: "start-race", ExecutionID: "execution-1", LeaseGeneration: 1,
		})
		startErr <- err
	}()
	waitForAdvisoryWaiter(t, db, 41001)
	waitForDatabaseTime(t, db, hardExpiry)

	reaperResult := make(chan struct {
		count int
		err   error
	}, 1)
	go func() {
		count, err := postgres.NewReaper(db).RunBatch(context.Background(), 10)
		reaperResult <- struct {
			count int
			err   error
		}{count: count, err: err}
	}()

	select {
	case result := <-reaperResult:
		if result.err != nil || result.count != 0 {
			t.Fatalf("reaper while Start owns execution: count=%d err=%v", result.count, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reaper blocked behind Start; lock order is not Execution then Task")
	}
	unblock()
	if err := <-startErr; err != nil {
		t.Fatalf("start error=%v", err)
	}
	assertLifecycleState(t, db, "in_progress", "running", "execution-1")
}

func TestHeartbeatWaitsForReaperExecutionLockAndObservesExpiredState(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedReaperExecution(t, db, "task-1", "execution-1", time.Now().Add(time.Hour), time.Now().Add(-time.Second))
	installLifecycleBlockTrigger(t, db, "executions", "NEW.status = 'expired'", 41002)
	unblock := holdAdvisoryLock(t, db, 41002)
	svc := integrationService(t, db)

	reaperResult := make(chan struct {
		count int
		err   error
	}, 1)
	go func() {
		count, err := postgres.NewReaper(db).RunBatch(context.Background(), 10)
		reaperResult <- struct {
			count int
			err   error
		}{count: count, err: err}
	}()
	waitForAdvisoryWaiter(t, db, 41002)

	heartbeatErr := make(chan error, 1)
	go func() {
		_, err := svc.HeartbeatExecution(context.Background(), workerPrincipal(), application.HeartbeatExecution{
			RequestID: "heartbeat-race", ExecutionID: "execution-1", LeaseGeneration: 1,
		})
		heartbeatErr <- err
	}()
	waitForNonAdvisoryLockWaiter(t, db)

	unblock()
	result := <-reaperResult
	if result.err != nil || result.count != 1 {
		t.Fatalf("reaper: count=%d err=%v", result.count, result.err)
	}
	if err := <-heartbeatErr; domain.CodeOf(err) != "state_conflict" {
		t.Fatalf("heartbeat error=%v, want state_conflict after Reaper commits", err)
	}
	assertLifecycleState(t, db, "open", "expired", "")
}

func installLifecycleBlockTrigger(t *testing.T, db *pgxpool.Pool, table, condition string, key int64) {
	t.Helper()
	name := fmt.Sprintf("block_%s_%d", table, key)
	_, err := db.Exec(context.Background(), fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF %s THEN
				PERFORM pg_advisory_xact_lock(%d);
			END IF;
			RETURN NEW;
		END; $$`, name, condition, key,
	))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(context.Background(), fmt.Sprintf(
		`CREATE TRIGGER %s BEFORE UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION %s()`,
		name, table, name,
	))
	if err != nil {
		t.Fatal(err)
	}
}

func holdAdvisoryLock(t *testing.T, db *pgxpool.Pool, key int64) func() {
	t.Helper()
	conn, err := db.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), `SELECT pg_advisory_lock($1)`, key); err != nil {
		conn.Release()
		t.Fatal(err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			if _, err := conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, key); err != nil {
				t.Errorf("unlock advisory lock: %v", err)
			}
			conn.Release()
		})
	}
	t.Cleanup(release)
	return release
}

func waitForAdvisoryWaiter(t *testing.T, db *pgxpool.Pool, key int64) {
	t.Helper()
	waitForLockCount(t, db, `locktype='advisory' AND objid=$1 AND NOT granted`, key)
}

func waitForNonAdvisoryLockWaiter(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	waitForLockCount(t, db, `locktype='transactionid' AND NOT granted`, nil)
}

func waitForLockCount(t *testing.T, db *pgxpool.Pool, predicate string, argument any) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		var err error
		query := `SELECT count(*) FROM pg_locks WHERE ` + predicate
		if argument == nil {
			err = db.QueryRow(context.Background(), query).Scan(&count)
		} else {
			err = db.QueryRow(context.Background(), query, argument).Scan(&count)
		}
		if err != nil {
			t.Fatal(err)
		}
		if count > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for PostgreSQL lock predicate %q", predicate)
}

func waitForDatabaseTime(t *testing.T, db *pgxpool.Pool, target time.Time) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var reached bool
		if err := db.QueryRow(context.Background(), `SELECT clock_timestamp() >= $1`, target).Scan(&reached); err != nil {
			t.Fatal(err)
		}
		if reached {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("database clock did not reach %v", target)
}

func assertLifecycleState(t *testing.T, db *pgxpool.Pool, taskStatus, executionStatus, activeExecutionID string) {
	t.Helper()
	var gotTask, gotExecution, gotActive string
	err := db.QueryRow(context.Background(), `
		SELECT t.status, e.status, COALESCE(t.active_execution_id, '')
		FROM tasks t JOIN executions e ON e.tenant_id=t.tenant_id AND e.task_id=t.id
		WHERE t.tenant_id='tenant-1' AND t.id='task-1' AND e.id='execution-1'`).Scan(&gotTask, &gotExecution, &gotActive)
	if err != nil || gotTask != taskStatus || gotExecution != executionStatus || gotActive != activeExecutionID {
		t.Fatalf("task=%s execution=%s active=%q err=%v", gotTask, gotExecution, gotActive, err)
	}
}

func workerPrincipal() auth.Principal {
	return auth.Principal{TenantID: "tenant-1", AgentID: "worker", AgentVersionID: "worker", Scopes: []string{"tasks:claim", "tasks:execute"}}
}

func integrationService(t *testing.T, db *pgxpool.Pool) *application.Service {
	t.Helper()
	svc, err := application.NewService(postgres.NewStore(db), application.Options{CursorSecret: []byte("01234567890123456789012345678901")})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func agentPrincipal(i int) auth.Principal {
	id := fmt.Sprintf("agent-%d", i)
	return auth.Principal{TenantID: "tenant-1", AgentID: id, AgentVersionID: id, Scopes: []string{"tasks:claim", "tasks:execute"}}
}

func seedClaimTask(t *testing.T, db *pgxpool.Pool, id string, deadline time.Time) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO tasks (tenant_id, id, publisher_agent_version_id, type, title, problem, constraints, requirements, deadline, status)
		VALUES ('tenant-1', $1, 'publisher', 'code', 'title', 'problem', '[]', '[]', $2, 'open')`, id, deadline)
	if err != nil {
		t.Fatal(err)
	}
}
