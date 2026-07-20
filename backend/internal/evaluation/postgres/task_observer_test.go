package postgres_test

import (
	"context"
	"testing"
	"time"

	evdomain "agentguild.dev/agentguild/backend/internal/evaluation/domain"
	evpostgres "agentguild.dev/agentguild/backend/internal/evaluation/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func insertPlatformTask(t *testing.T, db *pgxpool.Pool, tenantID, taskID, status string) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO tasks (tenant_id, id, publisher_agent_version_id, type, title, problem, deadline, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tenantID, taskID, "publisher-version", "evaluation", "title", "problem",
		time.Now().Add(time.Hour), status,
	)
	require.NoError(t, err)
}

func insertPlatformExecution(t *testing.T, db *pgxpool.Pool, tenantID, executionID, taskID, agentVersionID, status string, createdAt time.Time, startedAt, submittedAt *time.Time) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO executions (tenant_id, id, task_id, agent_version_id, status, lease_generation, started_at, submitted_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		tenantID, executionID, taskID, agentVersionID, status, 1, startedAt, submittedAt, createdAt,
	)
	require.NoError(t, err)
}

func insertPlatformSubmission(t *testing.T, db *pgxpool.Pool, tenantID, submissionID, taskID, executionID, status string, createdAt time.Time) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO submissions (id, tenant_id, task_id, execution_id, branch, commit_sha, base_commit_sha, summary, diff_fingerprint, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		submissionID, tenantID, taskID, executionID, "agent/task", "deadbeef", "base0000", "summary", "fingerprint", status, createdAt, createdAt,
	)
	require.NoError(t, err)
}

func insertExecutionUsage(t *testing.T, db *pgxpool.Pool, tenantID, taskID, executionID, agentVersionID string, observedCost float64, observedAt time.Time) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO execution_usage (tenant_id, task_id, execution_id, agent_version_id, observed_cost, coverage, provider, source_cursor, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		tenantID, taskID, executionID, agentVersionID, observedCost, "complete", "langfuse", randomID(), observedAt,
	)
	require.NoError(t, err)
}

func TestEvaluationTaskObserverSnapshot(t *testing.T) {
	db := testdb.StartPostgres(t)
	observer := evpostgres.NewEvaluationTaskObserver(db)
	tenantID := "tenant-observer"

	started := time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	submitted := started.Add(2 * time.Second)

	// Task claimed by the evaluated version with a validated submission and an
	// observed cost.
	insertPlatformTask(t, db, tenantID, "pt-1", "in_progress")
	insertPlatformExecution(t, db, tenantID, "ex-1", "pt-1", "version-1", "reviewing", started.Add(-time.Second), &started, &submitted)
	insertPlatformSubmission(t, db, tenantID, "sub-1", "pt-1", "ex-1", "validated", submitted)
	insertExecutionUsage(t, db, tenantID, "pt-1", "ex-1", "version-1", 42, submitted)

	snapshot, err := observer.Observe(context.Background(), tenantID, "pt-1")
	require.NoError(t, err)
	require.Equal(t, "pt-1", snapshot.TaskID)
	require.Equal(t, "in_progress", snapshot.TaskStatus)
	require.Equal(t, "ex-1", snapshot.ExecutionID)
	require.Equal(t, "version-1", snapshot.ExecutionVersionID)
	require.Equal(t, "reviewing", snapshot.ExecutionStatus)
	require.NotNil(t, snapshot.ExecutionStartedAt)
	require.True(t, started.Equal(*snapshot.ExecutionStartedAt))
	require.NotNil(t, snapshot.ExecutionSubmittedAt)
	require.True(t, submitted.Equal(*snapshot.ExecutionSubmittedAt))
	require.Equal(t, "validated", snapshot.SubmissionStatus)
	require.NotNil(t, snapshot.ObservedCostCents)
	require.Equal(t, int64(42), *snapshot.ObservedCostCents)

	// The latest execution (and its latest submission) determines the outcome:
	// an older expired execution by another version is ignored.
	insertPlatformTask(t, db, tenantID, "pt-2", "in_progress")
	insertPlatformExecution(t, db, tenantID, "ex-old", "pt-2", "version-other", "expired", started.Add(-time.Hour), nil, nil)
	insertPlatformExecution(t, db, tenantID, "ex-new", "pt-2", "version-1", "validating", started, &started, &submitted)
	insertPlatformSubmission(t, db, tenantID, "sub-old", "pt-2", "ex-new", "validation_failed", submitted)
	insertPlatformSubmission(t, db, tenantID, "sub-new", "pt-2", "ex-new", "pending_verification", submitted.Add(time.Second))

	snapshot, err = observer.Observe(context.Background(), tenantID, "pt-2")
	require.NoError(t, err)
	require.Equal(t, "ex-new", snapshot.ExecutionID)
	require.Equal(t, "version-1", snapshot.ExecutionVersionID)
	require.Equal(t, "pending_verification", snapshot.SubmissionStatus)
	require.Nil(t, snapshot.ObservedCostCents)

	// A never-claimed task has no execution fields.
	insertPlatformTask(t, db, tenantID, "pt-3", "open")
	snapshot, err = observer.Observe(context.Background(), tenantID, "pt-3")
	require.NoError(t, err)
	require.Equal(t, "open", snapshot.TaskStatus)
	require.Equal(t, "", snapshot.ExecutionID)
	require.Equal(t, "", snapshot.ExecutionVersionID)
	require.Equal(t, "", snapshot.SubmissionStatus)
	require.Nil(t, snapshot.ExecutionStartedAt)
	require.Nil(t, snapshot.ExecutionSubmittedAt)
	require.Nil(t, snapshot.ObservedCostCents)

	// Unknown tasks are reported as not_found.
	_, err = observer.Observe(context.Background(), tenantID, "missing")
	require.ErrorIs(t, err, evdomain.ErrNotFound)

	// Tenant isolation: the same task id under another tenant is not visible.
	_, err = observer.Observe(context.Background(), "tenant-other", "pt-1")
	require.ErrorIs(t, err, evdomain.ErrNotFound)
}
