package postgres_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"sync"
	"testing"
	"time"

	core "agentguild.dev/agentguild/backend/internal/domain"
	participationdomain "agentguild.dev/agentguild/backend/internal/participation/domain"
	publictaskapp "agentguild.dev/agentguild/backend/internal/publictask/application"
	publictaskdomain "agentguild.dev/agentguild/backend/internal/publictask/domain"
	publictaskpostgres "agentguild.dev/agentguild/backend/internal/publictask/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestPublicClaimAtomicallyBindsExecutionGrantAndIdempotentReplay(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	seedGlobalAgent(t, db, "agent-1", "version-1", "active", "active", 1)
	repository := publictaskpostgres.NewRepository(db)
	seedPublicClaimTask(t, db, repository, "task-1", "public-1", "spec-1")

	request := publicClaimRequest("public-1", "agent-1", "version-1", "request-1", "execution-1", "grant-1", "outbox-1")
	result, err := repository.Claim(ctx, request)
	require.NoError(t, err)
	require.Equal(t, "execution-1", result.Data.ExecutionID)
	require.Equal(t, "spec-1", result.Data.TaskSpecificationVersionID)
	require.Equal(t, "agent-1", result.Data.AgentID)
	require.Equal(t, "version-1", result.Data.AgentVersionID)
	require.Equal(t, "grant-1", result.Data.Grant.ID)
	require.Equal(t, int64(1), result.Data.LeaseGeneration)
	require.NotZero(t, result.Data.ClaimedAt)
	body, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(body), "tenant-sponsor")
	require.NotContains(t, string(body), "resource_tenant")

	var taskStatus, executionAgentID, executionVersionID, specificationVersionID string
	err = db.QueryRow(ctx, `SELECT status FROM tasks WHERE tenant_id='tenant-sponsor' AND id='task-1'`).Scan(&taskStatus)
	require.NoError(t, err)
	require.Equal(t, "claimed", taskStatus)
	err = db.QueryRow(ctx, `
		SELECT agent_id, agent_version_id, task_specification_version_id
		FROM executions
		WHERE tenant_id='tenant-sponsor' AND id='execution-1'`,
	).Scan(&executionAgentID, &executionVersionID, &specificationVersionID)
	require.NoError(t, err)
	require.Equal(t, "agent-1", executionAgentID)
	require.Equal(t, "version-1", executionVersionID)
	require.Equal(t, "spec-1", specificationVersionID)

	var grantEvents int
	err = db.QueryRow(ctx, `
		SELECT count(*) FROM task_participation_grant_events
		WHERE grant_id='grant-1' AND event_type='created' AND reason_code='public_claim'`,
	).Scan(&grantEvents)
	require.NoError(t, err)
	require.Equal(t, 1, grantEvents)

	replayRequest := request
	replayRequest.ExecutionID = "execution-retry-must-not-be-used"
	replayRequest.GrantID = "grant-retry-must-not-be-used"
	replayRequest.OutboxEventID = "outbox-retry-must-not-be-used"
	replay, err := repository.Claim(ctx, replayRequest)
	require.NoError(t, err)
	require.Equal(t, result, replay)

	var executions, grants int
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*) FROM executions WHERE tenant_id='tenant-sponsor' AND task_id='task-1'`).Scan(&executions))
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*) FROM task_participation_grants WHERE resource_tenant_id='tenant-sponsor' AND task_id='task-1'`).Scan(&grants))
	require.Equal(t, 1, executions)
	require.Equal(t, 1, grants)
}

func TestPublicClaimAllowsExactlyOneConcurrentGlobalAgent(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedGlobalAgent(t, db, "agent-1", "version-1", "active", "active", 1)
	seedGlobalAgent(t, db, "agent-2", "version-2", "active", "active", 1)
	repository := publictaskpostgres.NewRepository(db)
	seedPublicClaimTask(t, db, repository, "task-1", "public-1", "spec-1")

	requests := []publictaskapp.PublicClaimRequest{
		publicClaimRequest("public-1", "agent-1", "version-1", "request-1", "execution-1", "grant-1", "outbox-1"),
		publicClaimRequest("public-1", "agent-2", "version-2", "request-2", "execution-2", "grant-2", "outbox-2"),
	}
	start := make(chan struct{})
	errs := make([]error, len(requests))
	var wg sync.WaitGroup
	for index := range requests {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, errs[index] = repository.Claim(context.Background(), requests[index])
		}(index)
	}
	close(start)
	wg.Wait()

	successes, conflicts := 0, 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		if core.CodeOf(err) == "state_conflict" {
			conflicts++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
}

func TestPublicClaimRejectsWrongOrInactiveIdentityAndRevokedProjection(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	seedGlobalAgent(t, db, "agent-active", "version-active", "active", "active", 1)
	seedGlobalAgent(t, db, "agent-other", "version-other", "active", "active", 1)
	seedGlobalAgent(t, db, "agent-revoked", "version-revoked", "revoked", "active", 1)
	seedGlobalAgent(t, db, "agent-retired", "version-retired", "active", "retired", 1)
	repository := publictaskpostgres.NewRepository(db)
	for index := 1; index <= 4; index++ {
		seedPublicClaimTask(t, db, repository,
			"task-"+string(rune('0'+index)), "public-"+string(rune('0'+index)), "spec-"+string(rune('0'+index)))
	}
	revoked, err := repository.GetByID(ctx, "public-4")
	require.NoError(t, err)
	require.NoError(t, revoked.Revoke("governor", "source revoked", time.Now().UTC()))
	require.NoError(t, repository.Update(ctx, revoked))

	tests := []struct {
		name, publicTaskID, agentID, versionID string
	}{
		{"version belongs to another agent", "public-1", "agent-active", "version-other"},
		{"agent revoked", "public-2", "agent-revoked", "version-revoked"},
		{"version retired", "public-3", "agent-retired", "version-retired"},
		{"projection revoked", "public-4", "agent-active", "version-active"},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := publicClaimRequest(
				test.publicTaskID, test.agentID, test.versionID,
				"request-"+test.publicTaskID, "denied-execution-"+string(rune('0'+index)),
				"denied-grant-"+string(rune('0'+index)), "denied-outbox-"+string(rune('0'+index)),
			)
			_, err := repository.Claim(ctx, request)
			require.ErrorIs(t, err, publictaskdomain.ErrForbidden)
		})
	}
	var executions, grants int
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*) FROM executions WHERE agent_id IS NOT NULL`).Scan(&executions))
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*) FROM task_participation_grants`).Scan(&grants))
	require.Zero(t, executions)
	require.Zero(t, grants)
}

func TestPublicClaimRollsBackEveryFactWhenFinalEventFails(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	seedGlobalAgent(t, db, "agent-1", "version-1", "active", "active", 1)
	repository := publictaskpostgres.NewRepository(db)
	seedPublicClaimTask(t, db, repository, "task-1", "public-1", "spec-1")
	_, err := db.Exec(ctx, `
		INSERT INTO outbox_events (
			tenant_id, id, event_type, aggregate_type, aggregate_id, payload
		) VALUES ('tenant-sponsor','duplicate-outbox','seed','seed','seed','{}'::jsonb)`)
	require.NoError(t, err)

	request := publicClaimRequest("public-1", "agent-1", "version-1", "request-rollback", "execution-rollback", "grant-rollback", "duplicate-outbox")
	_, err = repository.Claim(ctx, request)
	require.Error(t, err)

	var status string
	require.NoError(t, db.QueryRow(ctx, `SELECT status FROM tasks WHERE tenant_id='tenant-sponsor' AND id='task-1'`).Scan(&status))
	require.Equal(t, "open", status)
	for _, query := range []string{
		`SELECT count(*) FROM executions WHERE id='execution-rollback'`,
		`SELECT count(*) FROM task_participation_grants WHERE id='grant-rollback'`,
		`SELECT count(*) FROM task_participation_grant_events WHERE grant_id='grant-rollback'`,
		`SELECT count(*) FROM idempotency_records WHERE request_id='request-rollback'`,
	} {
		var count int
		require.NoError(t, db.QueryRow(ctx, query).Scan(&count))
		require.Zero(t, count)
	}
}

func seedGlobalAgent(t *testing.T, db *pgxpool.Pool, agentID, versionID, agentStatus, versionStatus string, versionNumber int) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO agent_identities (id, handle, display_name, status)
		VALUES ($1,$1,$1,$2)`, agentID, agentStatus)
	require.NoError(t, err)
	_, err = db.Exec(context.Background(), `
		INSERT INTO agent_identity_versions (
			id, agent_id, version_number, status, runtime, model
		) VALUES ($1,$2,$3,$4,'pi','gpt-5')`,
		versionID, agentID, versionNumber, versionStatus,
	)
	require.NoError(t, err)
}

func seedPublicClaimTask(t *testing.T, db *pgxpool.Pool, repository *publictaskpostgres.Repository, taskID, publicTaskID, specificationVersionID string) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO tasks (
			tenant_id, id, publisher_agent_version_id, type, title, problem,
			deadline, status
		) VALUES (
			'tenant-sponsor',$1,'publisher-version','bug','Fix widget',
			'Widget fails',clock_timestamp() + interval '1 day','open'
		)`, taskID)
	require.NoError(t, err)
	projection := newProjection(t)
	projection.ID = publicTaskID
	projection.TaskID = taskID
	projection.TaskSpecificationVersionID = specificationVersionID
	require.NoError(t, repository.Insert(context.Background(), projection))
}

func publicClaimRequest(publicTaskID, agentID, versionID, requestID, executionID, grantID, outboxID string) publictaskapp.PublicClaimRequest {
	return publictaskapp.PublicClaimRequest{
		PublicTaskID: publicTaskID, AgentID: agentID, AgentVersionID: versionID,
		RequestID: requestID, RequestHash: sha256.Sum256([]byte(publicTaskID + "|" + agentID + "|" + versionID)),
		ExecutionID: executionID, GrantID: grantID, OutboxEventID: outboxID,
		GrantTTL: time.Hour,
		GrantScopes: []participationdomain.Scope{
			participationdomain.ScopeTaskRead,
			participationdomain.ScopeExecutionRead,
			participationdomain.ScopeExecutionWrite,
			participationdomain.ScopeSubmissionCreate,
			participationdomain.ScopeReviewRead,
			participationdomain.ScopeGitWrite,
		},
	}
}
