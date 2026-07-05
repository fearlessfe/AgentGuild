package worker_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/postgres"
	reputationworker "agentguild.dev/agentguild/backend/internal/reputation/worker"
	reviewapplication "agentguild.dev/agentguild/backend/internal/review/application"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
	reviewpostgres "agentguild.dev/agentguild/backend/internal/review/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestWorkerProcessesUnprojectedReviews(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	seedReviewableExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-v1", "code")
	reviewerID := insertReviewer(t, db, "tenant-1", "reviewer-1")
	rubricID := insertRubricVersion(t, db, "tenant-1", 1)
	review := submitReview(t, db, "tenant-1", "review-1", "exe-1", reviewerID, rubricID, reviewdomain.DecisionAccepted)

	w := reputationworker.NewWorker(postgres.NewStore(db), 10*time.Millisecond, 10, slog.Default())
	require.NoError(t, w.RunOnce(ctx))

	requireReviewMarkedProjected(t, db, "tenant-1", review.ID)

	var count int
	require.NoError(t, db.QueryRow(ctx, `
		SELECT count(*) FROM reputation_projections
		WHERE tenant_id='tenant-1' AND agent_version_id='agent-v1'`).Scan(&count))
	require.Equal(t, 1, count)
}

func TestWorkerDoesNotProcessPendingReviews(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	seedReviewableExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-v1", "code")
	reviewerID := insertReviewer(t, db, "tenant-1", "reviewer-1")
	rubricID := insertRubricVersion(t, db, "tenant-1", 1)
	review, err := reviewdomain.NewReview("review-1", "tenant-1", "exe-1", reviewerID, rubricID, time.Now())
	require.NoError(t, err)
	require.NoError(t, reviewpostgres.NewReviewRepository(db).Insert(ctx, review))

	w := reputationworker.NewWorker(postgres.NewStore(db), 10*time.Millisecond, 10, slog.Default())
	require.NoError(t, w.RunOnce(ctx))

	var projected bool
	require.NoError(t, db.QueryRow(ctx, `
		SELECT projected_at IS NOT NULL FROM reviews
		WHERE tenant_id='tenant-1' AND id=$1`, review.ID).Scan(&projected))
	require.False(t, projected)

	var count int
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*) FROM reputation_projections`).Scan(&count))
	require.Zero(t, count)
}

func TestWorkerGroupsByTenantAndKey(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	seedReviewableExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-v1", "code")
	seedReviewableExecution(t, db, "tenant-1", "task-2", "exe-2", "agent-v1", "code")
	seedReviewableExecution(t, db, "tenant-2", "task-3", "exe-3", "agent-v1", "code")
	reviewerID1 := insertReviewer(t, db, "tenant-1", "reviewer-1")
	reviewerID2 := insertReviewer(t, db, "tenant-2", "reviewer-2")
	rubricID1 := insertRubricVersion(t, db, "tenant-1", 1)
	rubricID2 := insertRubricVersion(t, db, "tenant-2", 1)
	submitReview(t, db, "tenant-1", "review-1", "exe-1", reviewerID1, rubricID1, reviewdomain.DecisionAccepted)
	submitReview(t, db, "tenant-1", "review-2", "exe-2", reviewerID1, rubricID1, reviewdomain.DecisionRejected)
	submitReview(t, db, "tenant-2", "review-3", "exe-3", reviewerID2, rubricID2, reviewdomain.DecisionAccepted)

	w := reputationworker.NewWorker(postgres.NewStore(db), 10*time.Millisecond, 10, slog.Default())
	require.NoError(t, w.RunOnce(ctx))

	var tenant1Total int
	require.NoError(t, db.QueryRow(ctx, `
		SELECT total_reviews FROM reputation_projections
		WHERE tenant_id='tenant-1' AND agent_version_id='agent-v1'`).Scan(&tenant1Total))
	require.Equal(t, 2, tenant1Total)

	var tenant2Total int
	require.NoError(t, db.QueryRow(ctx, `
		SELECT total_reviews FROM reputation_projections
		WHERE tenant_id='tenant-2' AND agent_version_id='agent-v1'`).Scan(&tenant2Total))
	require.Equal(t, 1, tenant2Total)
}

func TestWorkerRunRespectsContextCancellation(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx, cancel := context.WithCancel(context.Background())

	w := reputationworker.NewWorker(postgres.NewStore(db), time.Hour, 10, slog.Default())
	cancel()

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}

func seedReviewableExecution(t *testing.T, db *pgxpool.Pool, tenantID, taskID, executionID, agentVersionID, taskType string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO tasks (tenant_id, id, publisher_agent_version_id, type, title, problem, constraints, requirements, deadline, status)
		VALUES ($1, $2, 'publisher-1', $3, 'title', 'problem', '[]'::jsonb, '[]'::jsonb, $4, 'open')`,
		tenantID, taskID, taskType, time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = db.Exec(ctx, `
		INSERT INTO executions (tenant_id, id, task_id, agent_version_id, status, lease_generation, started_at)
		VALUES ($1, $2, $3, $4, 'reviewing', 1, clock_timestamp())`,
		tenantID, executionID, taskID, agentVersionID)
	require.NoError(t, err)
}

func submitReview(t *testing.T, db *pgxpool.Pool, tenantID, reviewID, submissionID, reviewerID, rubricVersionID string, decision reviewdomain.Decision) *reviewdomain.Review {
	t.Helper()
	ctx := context.Background()
	review, err := reviewdomain.NewReview(reviewID, tenantID, submissionID, reviewerID, rubricVersionID, time.Now())
	require.NoError(t, err)
	rubric := newRubricVersion(rubricVersionID, tenantID, 1)
	require.NoError(t, review.Submit(decision, []reviewdomain.RubricScore{{Dimension: "quality", Score: 80}}, rubric, time.Now()))
	require.NoError(t, reviewpostgres.NewReviewRepository(db).Insert(ctx, review))
	return review
}

func requireReviewMarkedProjected(t *testing.T, db *pgxpool.Pool, tenantID, reviewID string) {
	t.Helper()
	ctx := context.Background()
	var projected bool
	require.NoError(t, db.QueryRow(ctx, `
		SELECT projected_at IS NOT NULL FROM reviews
		WHERE tenant_id=$1 AND id=$2`, tenantID, reviewID).Scan(&projected))
	require.True(t, projected)
}

func insertReviewer(t *testing.T, db *pgxpool.Pool, tenantID, reviewerID string) string {
	t.Helper()
	ctx := context.Background()
	profile, err := reviewdomain.NewReviewerProfile(reviewerID, tenantID, "user-"+reviewerID, []string{"go"}, time.Now())
	require.NoError(t, err)
	require.NoError(t, reviewpostgres.NewStore(db).WithTx(ctx, func(tx reviewapplication.Tx) error {
		return tx.Reviewers().Insert(ctx, profile)
	}))
	return reviewerID
}

func insertRubricVersion(t *testing.T, db *pgxpool.Pool, tenantID string, versionNumber int) string {
	t.Helper()
	ctx := context.Background()
	version, err := reviewdomain.NewRubricVersion(
		"rubric-"+tenantID+"-"+string(rune('0'+versionNumber)),
		tenantID,
		"Rubric "+string(rune('0'+versionNumber)),
		versionNumber,
		[]reviewdomain.RubricDimension{{ID: "quality", Name: "Quality"}},
		map[string]float64{"quality": 1.0},
		"v1",
		time.Now(),
	)
	require.NoError(t, err)
	require.NoError(t, reviewpostgres.NewStore(db).WithTx(ctx, func(tx reviewapplication.Tx) error {
		return tx.Rubrics().CreateVersion(ctx, version)
	}))
	return version.ID
}

func newRubricVersion(id, tenantID string, versionNumber int) *reviewdomain.RubricVersion {
	version, err := reviewdomain.NewRubricVersion(
		id, tenantID, "Rubric "+id, versionNumber,
		[]reviewdomain.RubricDimension{{ID: "quality", Name: "Quality"}},
		map[string]float64{"quality": 1.0},
		"v1", time.Now(),
	)
	if err != nil {
		panic(err)
	}
	return version
}
