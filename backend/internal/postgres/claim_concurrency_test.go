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
