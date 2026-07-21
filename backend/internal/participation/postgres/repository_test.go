package postgres_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/participation/domain"
	participationpostgres "agentguild.dev/agentguild/backend/internal/participation/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestRepositoryAuthorizesRenewsRevokesAndAuditsEveryDecision(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedGrantFacts(t, db)
	repository := participationpostgres.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	grant := newGrant(t, "grant-1", "execution-1", now, now.Add(time.Hour))
	require.NoError(t, repository.Insert(ctx, grant, domain.ActorSystem, "public-claim"))

	request := requestFor("execution-1", domain.ScopeExecutionWrite)
	authorized, err := repository.Authorize(ctx, request)
	require.NoError(t, err)
	require.Equal(t, grant.ID, authorized.ID)

	wrongTask := request
	wrongTask.TaskID = "task-other"
	_, err = repository.Authorize(ctx, wrongTask)
	require.ErrorIs(t, err, domain.ErrForbidden)

	missing := requestFor("missing-execution", domain.ScopeExecutionWrite)
	_, err = repository.Authorize(ctx, missing)
	require.ErrorIs(t, err, domain.ErrForbidden)

	renewed, err := repository.Renew(ctx, grant.ID, "governor", now.Add(2*time.Hour))
	require.NoError(t, err)
	require.True(t, renewed.ExpiresAt.After(grant.ExpiresAt))
	revoked, err := repository.Revoke(ctx, grant.ID, "governor", "policy violation")
	require.NoError(t, err)
	require.Equal(t, domain.StatusRevoked, revoked.Status)
	_, err = repository.Authorize(ctx, request)
	require.ErrorIs(t, err, domain.ErrRevoked)

	events, err := repository.ListAuditByTask(ctx, "tenant-sponsor", "task-1", 100)
	require.NoError(t, err)
	require.Len(t, events, 6)
	require.Equal(t, []domain.EventType{
		domain.EventCreated, domain.EventAllowed, domain.EventDenied,
		domain.EventRenewed, domain.EventRevoked, domain.EventDenied,
	}, eventTypes(events))
	crossTaskEvents, err := repository.ListAuditByTask(ctx, "tenant-sponsor", "task-other", 100)
	require.NoError(t, err)
	require.Len(t, crossTaskEvents, 1)
	require.Equal(t, domain.EventDenied, crossTaskEvents[0].Type)
	require.Equal(t, "forbidden", crossTaskEvents[0].ReasonCode)

	_, err = db.Exec(ctx, `UPDATE task_participation_grant_events SET reason_code='tampered' WHERE id=$1`, events[0].ID)
	require.Error(t, err)
	_, err = db.Exec(ctx, `DELETE FROM task_participation_grant_events WHERE id=$1`, events[0].ID)
	require.Error(t, err)
}

func TestRepositoryUsesDatabaseTimeForExpiryAndRecordsItOnce(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedGrantFacts(t, db)
	repository := participationpostgres.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	grant := newGrant(t, "grant-expired", "execution-1", now.Add(-2*time.Hour), now.Add(-time.Hour))
	require.NoError(t, repository.Insert(ctx, grant, domain.ActorSystem, "public-claim"))

	_, err := repository.Authorize(ctx, requestFor("execution-1", domain.ScopeTaskRead))
	require.ErrorIs(t, err, domain.ErrExpired)
	stored, err := repository.GetByID(ctx, grant.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusExpired, stored.Status)

	_, err = repository.Authorize(ctx, requestFor("execution-1", domain.ScopeTaskRead))
	require.ErrorIs(t, err, domain.ErrExpired)
	events, err := repository.ListAuditByTask(ctx, "tenant-sponsor", "task-1", 100)
	require.NoError(t, err)
	require.Equal(t, 1, countEvent(events, domain.EventExpired))
	require.Equal(t, 2, countEvent(events, domain.EventDenied))
}

func TestRepositoryExpireDueIsIdempotent(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedGrantFacts(t, db)
	repository := participationpostgres.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	grant := newGrant(t, "grant-due", "execution-1", now.Add(-2*time.Hour), now.Add(-time.Hour))
	require.NoError(t, repository.Insert(ctx, grant, domain.ActorSystem, "public-claim"))

	count, err := repository.ExpireDue(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	count, err = repository.ExpireDue(ctx, 10)
	require.NoError(t, err)
	require.Zero(t, count)

	stored, err := repository.GetByID(ctx, grant.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusExpired, stored.Status)
	events, err := repository.ListAuditByTask(ctx, "tenant-sponsor", "task-1", 100)
	require.NoError(t, err)
	require.Equal(t, 1, countEvent(events, domain.EventExpired))
}

func seedGrantFacts(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO agent_identities (id, handle, display_name, status)
		VALUES ('agent-global', 'agent-global', 'Global Agent', 'active');
		INSERT INTO agent_identity_versions (id, agent_id, version_number, status, runtime, model)
		VALUES ('version-global', 'agent-global', 1, 'active', 'pi', 'gpt-5');
		UPDATE agent_identities SET current_version_id='version-global' WHERE id='agent-global';
		INSERT INTO tasks (
			tenant_id, id, publisher_agent_version_id, type, title, problem, deadline, status
		) VALUES (
			'tenant-sponsor', 'task-1', 'publisher-version', 'bug', 'Fix widget',
			'Widget fails', clock_timestamp() + interval '1 day', 'claimed'
		);
		INSERT INTO executions (
			tenant_id, id, task_id, agent_version_id, status, lease_generation
		) VALUES (
			'tenant-sponsor', 'execution-1', 'task-1', 'legacy-version', 'running', 1
		)`)
	require.NoError(t, err)
}

func newGrant(t *testing.T, id, executionID string, createdAt, expiresAt time.Time) *domain.Grant {
	t.Helper()
	grant, err := domain.NewGrant(domain.NewGrantParams{
		ID: id, ResourceTenantID: "tenant-sponsor", TaskID: "task-1", ExecutionID: executionID,
		AgentID: "agent-global", AgentVersionID: "version-global",
		Scopes:    []domain.Scope{domain.ScopeTaskRead, domain.ScopeExecutionWrite},
		CreatedAt: createdAt, ExpiresAt: expiresAt,
	})
	require.NoError(t, err)
	return grant
}

func requestFor(executionID string, scope domain.Scope) domain.AccessRequest {
	return domain.AccessRequest{
		ResourceTenantID: "tenant-sponsor", TaskID: "task-1", ExecutionID: executionID,
		AgentID: "agent-global", AgentVersionID: "version-global", Scope: scope,
		ActorType: domain.ActorAgent, ActorID: "agent-global",
	}
}

func eventTypes(events []domain.AuditEvent) []domain.EventType {
	result := make([]domain.EventType, len(events))
	for index, event := range events {
		result[index] = event.Type
	}
	return result
}

func countEvent(events []domain.AuditEvent, eventType domain.EventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}
