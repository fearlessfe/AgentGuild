package postgres_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/contribution/domain"
	contributionpostgres "agentguild.dev/agentguild/backend/internal/contribution/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

const (
	firstCommit  = "1111111111111111111111111111111111111111"
	secondCommit = "2222222222222222222222222222222222222222"
)

func TestRepositoryPersistsContributionAndAppendOnlyIdempotentEvents(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedContributionFacts(t, db)
	repository := contributionpostgres.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	contribution := newContribution(t, now)
	require.NoError(t, repository.Insert(ctx, contribution))
	stored, err := repository.GetByID(ctx, contribution.ID)
	require.NoError(t, err)
	require.Equal(t, contribution.AgentID, stored.AgentID)
	require.Equal(t, contribution.AgentVersionID, stored.AgentVersionID)
	require.Equal(t, contribution.TaskSpecificationVersionID, stored.TaskSpecificationVersionID)
	require.Equal(t, contribution.CanonicalRepository, stored.CanonicalRepository)

	opened := newEvent(t, domain.EventPROpened, domain.OutcomeAttempt, firstCommit,
		"delivery-opened", "pull:52:opened", now)
	inserted, created, err := repository.AppendEvent(ctx, opened)
	require.NoError(t, err)
	require.True(t, created)
	require.Positive(t, inserted.ID)

	replayed, created, err := repository.AppendEvent(ctx, opened)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, inserted.ID, replayed.ID)

	redelivered := *opened
	redelivered.ProviderDeliveryID = "delivery-opened-redelivery"
	redelivered.ReceivedAt = now.Add(time.Second)
	replayed, created, err = repository.AppendEvent(ctx, &redelivered)
	require.NoError(t, err)
	require.False(t, created, "stable object version must deduplicate a new provider delivery")
	require.Equal(t, inserted.ID, replayed.ID)

	synchronized := newEvent(t, domain.EventPRSynchronized, domain.OutcomeAttempt, secondCommit,
		"delivery-synchronize", "pull:52:head:"+secondCommit, now.Add(time.Minute))
	_, created, err = repository.AppendEvent(ctx, synchronized)
	require.NoError(t, err)
	require.True(t, created)

	events, err := repository.ListEvents(ctx, contribution.ID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, firstCommit, events[0].CommitSHA)
	require.Equal(t, secondCommit, events[1].CommitSHA)

	conflictingReplay := *opened
	conflictingReplay.Payload = json.RawMessage(`{"action":"tampered"}`)
	_, _, err = repository.AppendEvent(ctx, &conflictingReplay)
	require.ErrorIs(t, err, domain.ErrStateConflict)

	_, err = db.Exec(ctx, `UPDATE contribution_events SET outcome='approved' WHERE id=$1`, inserted.ID)
	require.Error(t, err, "event updates must be rejected by the database")
	_, err = db.Exec(ctx, `DELETE FROM contribution_events WHERE id=$1`, inserted.ID)
	require.Error(t, err, "event deletes must be rejected by the database")
}

func TestRepositoryRejectsMismatchedGlobalAgentVersionAndDuplicatePullRequest(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedContributionFacts(t, db)
	repository := contributionpostgres.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	contribution := newContribution(t, now)
	require.NoError(t, repository.Insert(ctx, contribution))

	duplicate := *contribution
	duplicate.ID = "contribution-duplicate"
	require.ErrorIs(t, repository.Insert(ctx, &duplicate), domain.ErrStateConflict)

	mismatched := *contribution
	mismatched.ID = "contribution-mismatch"
	mismatched.PullRequestNumber = 53
	mismatched.PullRequestURL = "https://github.com/acme/widgets/pull/53"
	mismatched.AgentID = "agent-other"
	require.Error(t, repository.Insert(ctx, &mismatched), "version must belong to the same global Agent")
}

func seedContributionFacts(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO agent_identities (id, handle, display_name, status)
		VALUES
			('agent-global', 'agent-global', 'Global Agent', 'active'),
			('agent-other', 'agent-other', 'Other Agent', 'active');
		INSERT INTO agent_identity_versions (
			id, agent_id, version_number, status, runtime, model
		) VALUES ('version-global', 'agent-global', 1, 'active', 'pi', 'gpt-5');
		UPDATE agent_identities
		SET current_version_id='version-global'
		WHERE id='agent-global';
		INSERT INTO tasks (
			tenant_id, id, publisher_agent_version_id, type, title, problem,
			deadline, status
		) VALUES (
			'tenant-sponsor', 'task-1', 'publisher-version', 'bug',
			'Fix widget', 'Widget fails', clock_timestamp() + interval '1 day', 'open'
		);
		INSERT INTO executions (
			tenant_id, id, task_id, agent_version_id, status, lease_generation
		) VALUES (
			'tenant-sponsor', 'execution-1', 'task-1', 'legacy-executor-version',
			'accepted', 0
		)`)
	require.NoError(t, err)
}

func newContribution(t *testing.T, now time.Time) *domain.Contribution {
	t.Helper()
	contribution, err := domain.NewContribution(domain.NewContributionParams{
		ID:                         "contribution-1",
		ResourceTenantID:           "tenant-sponsor",
		AgentID:                    "agent-global",
		AgentVersionID:             "version-global",
		TaskID:                     "task-1",
		ExecutionID:                "execution-1",
		TaskSpecificationVersionID: "specification-1",
		CanonicalRepository:        "acme/widgets",
		IssueNumber:                41,
		IssueURL:                   "https://github.com/acme/widgets/issues/41",
		Provider:                   domain.ProviderGitHub,
		PullRequestNumber:          52,
		PullRequestURL:             "https://github.com/acme/widgets/pull/52",
		CommitSHA:                  firstCommit,
		AttributionStatus:          domain.AttributionVerified,
		Outcome:                    domain.OutcomeAttempt,
		CreatedAt:                  now,
	})
	require.NoError(t, err)
	return contribution
}

func newEvent(t *testing.T, eventType domain.EventType, outcome domain.Outcome, commit, delivery, objectVersion string, now time.Time) *domain.ContributionEvent {
	t.Helper()
	event, err := domain.NewContributionEvent(domain.NewContributionEventParams{
		ContributionID:     "contribution-1",
		Provider:           domain.ProviderGitHub,
		ProviderDeliveryID: delivery,
		ObjectVersion:      objectVersion,
		Type:               eventType,
		Outcome:            outcome,
		CommitSHA:          commit,
		Payload:            json.RawMessage(`{"action":"received"}`),
		OccurredAt:         now,
		ReceivedAt:         now,
	})
	require.NoError(t, err)
	return event
}
