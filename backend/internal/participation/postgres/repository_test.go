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

func TestRepositoryAuthorizesBoundTaskExecutionSubmissionAndReviewResources(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedGrantFacts(t, db)
	seedSubmissionReviewFacts(t, db)
	repository := participationpostgres.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	grant := newGrant(t, "grant-resource", "execution-1", now, now.Add(time.Hour))
	require.NoError(t, repository.Insert(ctx, grant, domain.ActorSystem, "public-claim"))

	for _, test := range []struct {
		name       string
		kind       domain.ResourceKind
		resourceID string
		scope      domain.Scope
	}{
		{name: "task", kind: domain.ResourceTask, resourceID: "task-1", scope: domain.ScopeTaskRead},
		{name: "execution", kind: domain.ResourceExecution, resourceID: "execution-1", scope: domain.ScopeExecutionRead},
		{name: "submission", kind: domain.ResourceSubmission, resourceID: "submission-1", scope: domain.ScopeExecutionRead},
		{name: "review", kind: domain.ResourceReview, resourceID: "review-1", scope: domain.ScopeReviewRead},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := repository.AuthorizeResource(ctx, resourceRequest(test.kind, test.resourceID, test.scope))
			require.NoError(t, err)
			require.Equal(t, grant.ID, got.ID)
			require.Equal(t, "tenant-sponsor", got.ResourceTenantID)
		})
	}

	wrongVersion := resourceRequest(domain.ResourceExecution, "execution-1", domain.ScopeExecutionRead)
	wrongVersion.AgentVersionID = "wrong-version"
	_, err := repository.AuthorizeResource(ctx, wrongVersion)
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = repository.AuthorizeResource(ctx, resourceRequest(domain.ResourceExecution, "execution-1", domain.ScopeGitWrite))
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = repository.AuthorizeResource(ctx, resourceRequest(domain.ResourceExecution, "another-execution", domain.ScopeExecutionRead))
	require.ErrorIs(t, err, domain.ErrForbidden)

	events, err := repository.ListAuditByTask(ctx, "tenant-sponsor", "task-1", 100)
	require.NoError(t, err)
	require.Equal(t, 4, countEvent(events, domain.EventAllowed))
	require.Equal(t, 2, countEvent(events, domain.EventDenied))
	var unresolved int
	require.NoError(t, db.QueryRow(ctx, `
		SELECT count(*) FROM task_participation_grant_events
		WHERE agent_id='agent-global' AND reason_code='resource_not_found'`).Scan(&unresolved))
	require.Equal(t, 1, unresolved)
}

func TestRepositoryResourceAuthorizationUsesDatabaseTimeForExpiry(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedGrantFacts(t, db)
	repository := participationpostgres.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	grant := newGrant(t, "grant-resource-expired", "execution-1", now.Add(-2*time.Hour), now.Add(-time.Hour))
	require.NoError(t, repository.Insert(ctx, grant, domain.ActorSystem, "public-claim"))

	_, err := repository.AuthorizeResource(ctx, resourceRequest(domain.ResourceExecution, "execution-1", domain.ScopeExecutionRead))
	require.ErrorIs(t, err, domain.ErrExpired)
	stored, err := repository.GetByID(ctx, grant.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusExpired, stored.Status)
	events, err := repository.ListAuditByTask(ctx, "tenant-sponsor", "task-1", 100)
	require.NoError(t, err)
	require.Equal(t, 1, countEvent(events, domain.EventExpired))
	require.Equal(t, 1, countEvent(events, domain.EventDenied))
}

func TestRepositoryResourceAuthorizationRejectsInactiveAgentVersionAndAudits(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedGrantFacts(t, db)
	repository := participationpostgres.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	grant := newGrant(t, "grant-inactive", "execution-1", now, now.Add(time.Hour))
	require.NoError(t, repository.Insert(ctx, grant, domain.ActorSystem, "public-claim"))
	_, err := db.Exec(ctx, `UPDATE agent_identity_versions SET status='retired' WHERE id='version-global'`)
	require.NoError(t, err)

	_, err = repository.AuthorizeResource(ctx, resourceRequest(domain.ResourceExecution, "execution-1", domain.ScopeExecutionRead))
	require.ErrorIs(t, err, domain.ErrForbidden)
	events, err := repository.ListAuditByTask(ctx, "tenant-sponsor", "task-1", 100)
	require.NoError(t, err)
	require.Equal(t, "agent_or_version_inactive", events[len(events)-1].ReasonCode)
}

func TestRepositoryResourceAuthorizationFailsClosedOnCrossTenantIDCollision(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedGrantFacts(t, db)
	repository := participationpostgres.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, repository.Insert(ctx, newGrant(t, "grant-sponsor", "execution-1", now, now.Add(time.Hour)), domain.ActorSystem, "public-claim"))
	_, err := db.Exec(ctx, `
		INSERT INTO tasks (
			tenant_id, id, publisher_agent_version_id, type, title, problem, deadline, status
		) VALUES (
			'tenant-other', 'task-other', 'publisher-version', 'bug', 'Other task',
			'Other problem', clock_timestamp() + interval '1 day', 'claimed'
		);
		INSERT INTO executions (
			tenant_id, id, task_id, agent_version_id, status, lease_generation
		) VALUES (
			'tenant-other', 'execution-1', 'task-other', 'legacy-version', 'running', 1
		)`)
	require.NoError(t, err)
	other, err := domain.NewGrant(domain.NewGrantParams{
		ID: "grant-other", ResourceTenantID: "tenant-other", TaskID: "task-other", ExecutionID: "execution-1",
		AgentID: "agent-global", AgentVersionID: "version-global",
		Scopes: []domain.Scope{domain.ScopeExecutionRead}, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, repository.Insert(ctx, other, domain.ActorSystem, "public-claim"))

	_, err = repository.AuthorizeResource(ctx, resourceRequest(domain.ResourceExecution, "execution-1", domain.ScopeExecutionRead))
	require.ErrorIs(t, err, domain.ErrForbidden)
	var ambiguous int
	require.NoError(t, db.QueryRow(ctx, `
		SELECT count(*) FROM task_participation_grant_events
		WHERE agent_id='agent-global' AND reason_code='ambiguous_resource'`).Scan(&ambiguous))
	require.Equal(t, 1, ambiguous)
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

func seedSubmissionReviewFacts(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO submissions (
			tenant_id, id, task_id, execution_id, repo, branch, commit_sha,
			base_commit_sha, summary, diff_fingerprint, status, created_at, updated_at
		) VALUES (
			'tenant-sponsor', 'submission-1', 'task-1', 'execution-1', 'owner/repo',
			'agentguild/execution-1', 'head-sha', 'base-sha', 'fix', 'fingerprint',
			'validated', clock_timestamp(), clock_timestamp()
		);
		INSERT INTO reviewer_profiles (tenant_id, id, user_id)
		VALUES ('tenant-sponsor', 'reviewer-1', 'reviewer-user-1');
		INSERT INTO rubric_versions (
			tenant_id, id, version_number, name, dimensions, weights,
			algorithm_version, is_active
		) VALUES (
			'tenant-sponsor', 'rubric-1', 1, 'Default', '[]', '{}', 'v1', true
		);
		INSERT INTO reviews (
			tenant_id, id, submission_id, reviewer_id, rubric_version_id, capability, status
		) VALUES (
			'tenant-sponsor', 'review-1', 'submission-1', 'reviewer-1', 'rubric-1', 'go', 'pending'
		)`)
	require.NoError(t, err)
}

func newGrant(t *testing.T, id, executionID string, createdAt, expiresAt time.Time) *domain.Grant {
	t.Helper()
	grant, err := domain.NewGrant(domain.NewGrantParams{
		ID: id, ResourceTenantID: "tenant-sponsor", TaskID: "task-1", ExecutionID: executionID,
		AgentID: "agent-global", AgentVersionID: "version-global",
		Scopes: []domain.Scope{
			domain.ScopeTaskRead, domain.ScopeExecutionRead, domain.ScopeExecutionWrite,
			domain.ScopeSubmissionCreate, domain.ScopeReviewRead,
		},
		CreatedAt: createdAt, ExpiresAt: expiresAt,
	})
	require.NoError(t, err)
	return grant
}

func resourceRequest(kind domain.ResourceKind, resourceID string, scope domain.Scope) domain.ResourceAccessRequest {
	return domain.ResourceAccessRequest{
		Kind: kind, ResourceID: resourceID,
		AgentID: "agent-global", AgentVersionID: "version-global", Scope: scope,
		ActorType: domain.ActorAgent, ActorID: "agent-global",
	}
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
