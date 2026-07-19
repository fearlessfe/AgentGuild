package postgres

import (
	"context"
	"testing"

	"agentguild.dev/agentguild/backend/internal/agentexperience/application"
	"agentguild.dev/agentguild/backend/internal/agentexperience/domain"
	agentversionpostgres "agentguild.dev/agentguild/backend/internal/agentversion/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestExtractCandidateFromPersistedAcceptedSubmission(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	pool := testdb.StartPostgres(t)
	seedExperienceSources(t, pool, "tenant-1")

	candidateRepo := NewExperienceCandidateRepository(pool)
	service := newCandidateService(t, pool, func() string { return "cand-1" })

	resp, err := service.ExtractCandidate(ctx, application.ExtractCandidate{
		TenantID:      "tenant-1",
		AgentID:       "agent-1",
		SubmissionID:  "sub-1",
		EvidenceBytes: []byte("evidence"),
		CreatedBy:     "owner-1",
	})
	require.NoError(t, err)
	require.Equal(t, "cand-1", resp.Candidate.ID)
	require.Equal(t, "tenant-1", resp.Candidate.TenantID)
	require.Equal(t, "agent-1", resp.Candidate.AgentID)
	require.Equal(t, "task-1", resp.Candidate.SourceTaskID)
	require.Equal(t, "sub-1", resp.Candidate.SourceSubmissionID)
	require.Equal(t, "rev-1", resp.Candidate.SourceReviewID)
	require.Equal(t, []string{"code", "go"}, resp.Candidate.ApplicableCapabilities)
	require.Equal(t, domain.StatusPendingReview, resp.Candidate.Status)

	loaded, err := candidateRepo.GetByID(ctx, "tenant-1", "agent-1", "cand-1")
	require.NoError(t, err)
	require.Equal(t, "rev-1", loaded.SourceReviewID)
	require.NotEmpty(t, loaded.ContentHash)
}

func TestSourceStoresTenantIsolationAndAcceptanceGate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	pool := testdb.StartPostgres(t)
	seedExperienceSources(t, pool, "tenant-1")

	submissions := NewSubmissionStore(pool)
	executions := NewExecutionStore(pool)

	// Cross-tenant reads must not find tenant-1 rows.
	_, err := submissions.GetAcceptedSubmission(ctx, "tenant-2", "sub-1")
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = executions.GetExecution(ctx, "tenant-2", "exe-1")
	require.ErrorIs(t, err, domain.ErrNotFound)

	// A submission whose review was rejected is not an accepted submission.
	_, err = submissions.GetAcceptedSubmission(ctx, "tenant-1", "sub-2")
	require.ErrorIs(t, err, domain.ErrNotFound)

	// End-to-end: extraction scoped to another tenant fails with not-found.
	service := newCandidateService(t, pool, func() string { return "cand-x" })
	_, err = service.ExtractCandidate(ctx, application.ExtractCandidate{
		TenantID:      "tenant-2",
		AgentID:       "agent-1",
		SubmissionID:  "sub-1",
		EvidenceBytes: []byte("evidence"),
		CreatedBy:     "owner-2",
		IsAdmin:       true,
	})
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func newCandidateService(t *testing.T, pool *pgxpool.Pool, newID func() string) *application.CandidateService {
	t.Helper()
	service, err := application.NewCandidateService(
		NewStore(pool),
		NewExperienceCandidateRepository(pool),
		NewSubmissionStore(pool),
		NewExecutionStore(pool),
		application.NewPolicy(agentversionpostgres.NewVersionRepository(pool)),
		domain.NewRuleBasedSensitivityPolicy(),
		application.CandidateOptions{NewID: newID},
	)
	require.NoError(t, err)
	return service
}

// seedExperienceSources persists the full chain agent -> agent version ->
// task -> execution -> submission -> review for tenantID: sub-1 has an
// accepted review, sub-2 a rejected one.
func seedExperienceSources(t *testing.T, pool *pgxpool.Pool, tenantID string) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO agents (id, tenant_id, owner_id, owner_email, name, status)
		VALUES ('agent-1', $1, 'owner-1', 'owner@example.com', 'Agent One', 'active')`, tenantID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO agent_versions (id, tenant_id, agent_id, version_number, runtime, model, capabilities)
		VALUES ('version-1', $1, 'agent-1', 1, 'runtime', 'model', ARRAY['code', 'go'])`, tenantID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO tasks (tenant_id, id, publisher_agent_version_id, type, title, problem, deadline, status)
		VALUES ($1, 'task-1', 'version-1', 'code', 'title', 'problem', clock_timestamp(), 'in_progress')`, tenantID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO executions (tenant_id, id, task_id, agent_version_id, status, lease_generation)
		VALUES ($1, 'exe-1', 'task-1', 'version-1', 'accepted', 1)`, tenantID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO submissions (id, tenant_id, task_id, execution_id, branch, commit_sha, base_commit_sha, summary, diff_fingerprint, status, created_at, updated_at)
		VALUES
			('sub-1', $1, 'task-1', 'exe-1', 'main', 'commit-1', 'base-1', 'summary', 'fp-1', 'validated', clock_timestamp(), clock_timestamp()),
			('sub-2', $1, 'task-1', 'exe-1', 'main', 'commit-2', 'base-1', 'summary', 'fp-2', 'validated', clock_timestamp(), clock_timestamp())`, tenantID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO reviewer_profiles (tenant_id, id, user_id)
		VALUES ($1, 'reviewer-1', 'user-1')`, tenantID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO rubric_versions (tenant_id, id, version_number, name, dimensions, weights, algorithm_version)
		VALUES ($1, 'rubric-1', 1, 'default', '[{"id":"quality","name":"Quality"}]'::jsonb, '{"quality":1}'::jsonb, 'v1')`, tenantID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO reviews (tenant_id, id, submission_id, reviewer_id, rubric_version_id, capability, status, final_decision, submitted_at)
		VALUES
			($1, 'rev-1', 'sub-1', 'reviewer-1', 'rubric-1', 'code', 'submitted', 'accepted', clock_timestamp()),
			($1, 'rev-2', 'sub-2', 'reviewer-1', 'rubric-1', 'code', 'submitted', 'rejected', clock_timestamp())`, tenantID)
	require.NoError(t, err)
}
