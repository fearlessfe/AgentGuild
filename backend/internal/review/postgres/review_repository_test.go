package postgres_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	appdomain "agentguild.dev/agentguild/backend/internal/domain"
	reviewapplication "agentguild.dev/agentguild/backend/internal/review/application"
	"agentguild.dev/agentguild/backend/internal/review/domain"
	"agentguild.dev/agentguild/backend/internal/review/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestInsertReviewIsTenantScoped(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	reviewerID := insertReviewer(t, db, "tenant-1", "reviewer-1")
	rubricID := insertRubricVersion(t, db, "tenant-1", 1)
	review := newReview("review-1", "tenant-1", "submission-1", reviewerID, rubricID)

	store := postgres.NewStore(db)
	require.NoError(t, store.WithTx(ctx, func(tx reviewapplication.Tx) error {
		return tx.Reviews().Insert(ctx, review)
	}))

	got, err := postgres.NewReviewRepository(db).GetByID(ctx, "tenant-1", review.ID)
	require.NoError(t, err)
	require.Equal(t, review.ID, got.ID)
	require.Equal(t, review.TenantID, got.TenantID)
	require.Equal(t, review.SubmissionID, got.SubmissionID)

	_, err = postgres.NewReviewRepository(db).GetByID(ctx, "tenant-2", review.ID)
	require.ErrorIs(t, err, appdomain.ErrNotFound)
}

func TestReviewUpdatePersistsSubmissionAndScores(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	reviewerID := insertReviewer(t, db, "tenant-1", "reviewer-1")
	rubricID := insertRubricVersion(t, db, "tenant-1", 1)
	review := newReview("review-1", "tenant-1", "submission-1", reviewerID, rubricID)
	review.RubricScores = []domain.RubricScore{{Dimension: "quality", Score: 85}}

	repo := postgres.NewReviewRepository(db)
	require.NoError(t, repo.Insert(ctx, review))

	now := time.Now()
	rubric := newRubricVersion(rubricID, "tenant-1", 1)
	require.NoError(t, review.Submit(domain.DecisionAccepted, []domain.RubricScore{{Dimension: "quality", Score: 90}}, rubric, now))
	require.NoError(t, repo.Update(ctx, review))

	got, err := repo.GetByID(ctx, "tenant-1", review.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ReviewSubmitted, got.Status)
	require.Equal(t, domain.DecisionAccepted, got.FinalDecision)
	require.Len(t, got.RubricScores, 1)
	require.Equal(t, 90, got.RubricScores[0].Score)
	require.False(t, got.SubmittedAt.IsZero())
}

func TestReviewListBySubmissionFiltersByTenant(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	reviewerID1 := insertReviewer(t, db, "tenant-1", "reviewer-1")
	rubricID1 := insertRubricVersion(t, db, "tenant-1", 1)
	reviewerID2 := insertReviewer(t, db, "tenant-2", "reviewer-2")
	rubricID2 := insertRubricVersion(t, db, "tenant-2", 1)
	require.NoError(t, postgres.NewReviewRepository(db).Insert(ctx, newReview("review-1", "tenant-1", "sub-1", reviewerID1, rubricID1)))
	require.NoError(t, postgres.NewReviewRepository(db).Insert(ctx, newReview("review-2", "tenant-1", "sub-2", reviewerID1, rubricID1)))
	require.NoError(t, postgres.NewReviewRepository(db).Insert(ctx, newReview("review-3", "tenant-2", "sub-1", reviewerID2, rubricID2)))

	got, err := postgres.NewReviewRepository(db).ListBySubmission(ctx, "tenant-1", "sub-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "review-1", got[0].ID)
}

func TestReviewOnePerSubmissionConstraint(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	reviewerID := insertReviewer(t, db, "tenant-1", "reviewer-1")
	rubricID := insertRubricVersion(t, db, "tenant-1", 1)
	require.NoError(t, postgres.NewReviewRepository(db).Insert(ctx, newReview("review-1", "tenant-1", "sub-1", reviewerID, rubricID)))

	err := postgres.NewReviewRepository(db).Insert(ctx, newReview("review-2", "tenant-1", "sub-1", reviewerID, rubricID))
	require.Error(t, err)
}

func TestLineCommentRepositoryIsTenantScoped(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	reviewerID := insertReviewer(t, db, "tenant-1", "reviewer-1")
	rubricID := insertRubricVersion(t, db, "tenant-1", 1)
	review := newReview("review-1", "tenant-1", "submission-1", reviewerID, rubricID)
	require.NoError(t, postgres.NewReviewRepository(db).Insert(ctx, review))

	comment, err := domain.NewLineComment("comment-1", "tenant-1", review.ID, review.SubmissionID, "main.go", domain.SideRight, 10, "hunk", "fp", "fix this", time.Now())
	require.NoError(t, err)
	require.NoError(t, postgres.NewLineCommentRepository(db).Insert(ctx, comment))

	got, err := postgres.NewLineCommentRepository(db).ListByReview(ctx, "tenant-1", review.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, comment.Text, got[0].Text)

	got2, err := postgres.NewLineCommentRepository(db).ListByReview(ctx, "tenant-2", review.ID)
	require.NoError(t, err)
	require.Empty(t, got2)
}

func TestRubricRepositoryActiveAndList(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	version1 := newRubricVersion("rubric-v1", "tenant-1", 1)
	version2 := newRubricVersion("rubric-v2", "tenant-1", 2)
	version2.IsActive = false
	repo := postgres.NewRubricRepository(db)
	require.NoError(t, repo.CreateVersion(ctx, version1))
	require.NoError(t, repo.CreateVersion(ctx, version2))

	active, err := repo.GetActive(ctx, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, version1.ID, active.ID)

	versions, err := repo.ListVersions(ctx, "tenant-1")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	require.Equal(t, 2, versions[0].VersionNumber)

	_, err = repo.GetActive(ctx, "tenant-2")
	require.ErrorIs(t, err, appdomain.ErrNotFound)
}

func TestRubricActiveUniquePerTenant(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	version1 := newRubricVersion("rubric-v1", "tenant-1", 1)
	version2 := newRubricVersion("rubric-v2", "tenant-1", 2)
	repo := postgres.NewRubricRepository(db)
	require.NoError(t, repo.CreateVersion(ctx, version1))

	err := repo.CreateVersion(ctx, version2)
	require.Error(t, err)
}

func TestReviewerRepositoryLoadOperations(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	insertReviewer(t, db, "tenant-1", "reviewer-1")
	repo := postgres.NewReviewerRepository(db)

	require.NoError(t, repo.IncrementLoad(ctx, "tenant-1", "reviewer-1"))
	got, err := repo.GetByID(ctx, "tenant-1", "reviewer-1")
	require.NoError(t, err)
	require.Equal(t, 1, got.CurrentLoad)

	require.NoError(t, repo.DecrementLoad(ctx, "tenant-1", "reviewer-1"))
	got, err = repo.GetByID(ctx, "tenant-1", "reviewer-1")
	require.NoError(t, err)
	require.Equal(t, 0, got.CurrentLoad)

	err = repo.DecrementLoad(ctx, "tenant-1", "reviewer-1")
	require.ErrorIs(t, err, appdomain.ErrNotFound)
}

func TestReviewerListActiveOrdersByLoad(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	insertReviewerWithLoad(t, db, "tenant-1", "reviewer-1", 2)
	insertReviewerWithLoad(t, db, "tenant-1", "reviewer-2", 0)
	insertReviewerWithLoad(t, db, "tenant-1", "reviewer-3", 1)
	insertReviewerWithLoad(t, db, "tenant-2", "reviewer-4", 0)

	got, err := postgres.NewReviewerRepository(db).ListActive(ctx, "tenant-1", 10)
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, "reviewer-2", got[0].ID)
	require.Equal(t, "reviewer-3", got[1].ID)
	require.Equal(t, "reviewer-1", got[2].ID)
}

func TestReviewerIncrementLoadDistinguishesInactiveFromNotFound(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	insertReviewer(t, db, "tenant-1", "reviewer-1")
	_, err := db.Exec(ctx, "UPDATE reviewer_profiles SET is_active=false WHERE tenant_id=$1 AND id=$2", "tenant-1", "reviewer-1")
	require.NoError(t, err)

	repo := postgres.NewReviewerRepository(db)
	require.ErrorIs(t, repo.IncrementLoad(ctx, "tenant-1", "reviewer-1"), appdomain.ErrStateConflict)
	require.ErrorIs(t, repo.IncrementLoad(ctx, "tenant-1", "missing"), appdomain.ErrNotFound)
}

func TestReviewerInsertUsesTransactionTime(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	profile, err := domain.NewReviewerProfile("reviewer-time", "tenant-1", "user-time", []string{"go"}, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)

	var txNow time.Time
	store := postgres.NewStore(db)
	require.NoError(t, store.WithTx(ctx, func(tx reviewapplication.Tx) error {
		var err error
		txNow, err = tx.Now(ctx)
		if err != nil {
			return err
		}
		return tx.Reviewers().Insert(ctx, profile)
	}))

	var createdAt, updatedAt time.Time
	err = db.QueryRow(ctx, "SELECT created_at, updated_at FROM reviewer_profiles WHERE tenant_id=$1 AND id=$2", profile.TenantID, profile.ID).Scan(&createdAt, &updatedAt)
	require.NoError(t, err)
	require.WithinDuration(t, txNow, createdAt, 0)
	require.WithinDuration(t, txNow, updatedAt, 0)
}

func TestCodeReviewMigrationCanRollbackAndReapply(t *testing.T) {
	db := testdb.StartPostgres(t)

	testdb.ApplyDownMigration(t, db)
	testdb.ApplyUpMigration(t, db)

	reviewerID := insertReviewer(t, db, "tenant-1", "reviewer-1")
	rubricID := insertRubricVersion(t, db, "tenant-1", 1)
	review := newReview("review-1", "tenant-1", "submission-1", reviewerID, rubricID)
	require.NoError(t, postgres.NewReviewRepository(db).Insert(context.Background(), review))
}

func TestCodeReviewPrimaryAndUniqueConstraintsIncludeTenantID(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	rows, err := db.Query(ctx, `
		SELECT c.conname, rel.relname
		FROM pg_constraint c
		JOIN pg_class rel ON rel.oid = c.conrelid
		WHERE rel.relname = ANY($1::text[])
		  AND c.contype IN ('p', 'u')
		  AND NOT EXISTS (
		      SELECT 1
		      FROM unnest(c.conkey) AS key(attnum)
		      JOIN pg_attribute attr
		        ON attr.attrelid = c.conrelid
		       AND attr.attnum = key.attnum
		      WHERE attr.attname = 'tenant_id'
		  )
		ORDER BY rel.relname, c.conname`,
		[]string{"reviews", "line_comments", "rubric_versions", "reviewer_profiles", "reputation_projections"},
	)
	require.NoError(t, err)
	defer rows.Close()

	var violations []string
	for rows.Next() {
		var constraintName, tableName string
		require.NoError(t, rows.Scan(&constraintName, &tableName))
		violations = append(violations, tableName+"."+constraintName)
	}
	require.NoError(t, rows.Err())
	require.Empty(t, violations)
}

func TestReviewNotFoundReturnsDomainError(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	_, err := postgres.NewReviewRepository(db).GetByID(ctx, "tenant-1", "missing")
	require.ErrorIs(t, err, appdomain.ErrNotFound)
	require.Equal(t, "not_found", appdomain.CodeOf(err))
}

func TestRubricNotFoundReturnsDomainError(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	_, err := postgres.NewRubricRepository(db).GetActive(ctx, "tenant-1")
	require.ErrorIs(t, err, appdomain.ErrNotFound)
	require.Equal(t, "not_found", appdomain.CodeOf(err))
}

func TestReviewerNotFoundReturnsDomainError(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	err := postgres.NewReviewerRepository(db).DecrementLoad(ctx, "tenant-1", "missing")
	require.ErrorIs(t, err, appdomain.ErrNotFound)
	require.Equal(t, "not_found", appdomain.CodeOf(err))
}

func TestReviewInsertUsesTransactionTime(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	reviewerID := insertReviewer(t, db, "tenant-1", "reviewer-1")
	rubricID := insertRubricVersion(t, db, "tenant-1", 1)
	past := time.Now().Add(-24 * time.Hour)
	review, err := domain.NewReview("review-time", "tenant-1", "sub-time", reviewerID, rubricID, past)
	require.NoError(t, err)

	var txNow time.Time
	store := postgres.NewStore(db)
	require.NoError(t, store.WithTx(ctx, func(tx reviewapplication.Tx) error {
		txNow, err = tx.Now(ctx)
		if err != nil {
			return err
		}
		return tx.Reviews().Insert(ctx, review)
	}))

	var createdAt, updatedAt time.Time
	err = db.QueryRow(ctx, "SELECT created_at, updated_at FROM reviews WHERE tenant_id=$1 AND id=$2", review.TenantID, review.ID).Scan(&createdAt, &updatedAt)
	require.NoError(t, err)
	require.WithinDuration(t, txNow, createdAt, 0)
	require.WithinDuration(t, txNow, updatedAt, 0)
	require.NotEqual(t, past.Truncate(time.Microsecond), createdAt)
}

func TestReviewUpdateUsesTransactionTime(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	reviewerID := insertReviewer(t, db, "tenant-1", "reviewer-1")
	rubricID := insertRubricVersion(t, db, "tenant-1", 1)
	rubric := newRubricVersion(rubricID, "tenant-1", 1)
	review := newReview("review-update-time", "tenant-1", "sub-update", reviewerID, rubricID)
	require.NoError(t, postgres.NewReviewRepository(db).Insert(ctx, review))

	var txNow time.Time
	store := postgres.NewStore(db)
	var err error
	require.NoError(t, store.WithTx(ctx, func(tx reviewapplication.Tx) error {
		txNow, err = tx.Now(ctx)
		if err != nil {
			return err
		}
		require.NoError(t, review.Submit(domain.DecisionAccepted, []domain.RubricScore{{Dimension: "quality", Score: 80}}, rubric, txNow))
		return tx.Reviews().Update(ctx, review)
	}))

	var updatedAt time.Time
	err = db.QueryRow(ctx, "SELECT updated_at FROM reviews WHERE tenant_id=$1 AND id=$2", review.TenantID, review.ID).Scan(&updatedAt)
	require.NoError(t, err)
	require.WithinDuration(t, txNow, updatedAt, 0)
}

func TestListUnprojectedReturnsSubmittedReviewsWithExecutionMetadata(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	insertExecutionAndTask(t, db, "tenant-1", "task-1", "exe-1", "agent-v1", "code")
	reviewerID := insertReviewer(t, db, "tenant-1", "reviewer-1")
	rubricID := insertRubricVersion(t, db, "tenant-1", 1)
	review := submitReviewWithRubric(t, db, "tenant-1", "review-1", "exe-1", reviewerID, rubricID, domain.DecisionAccepted)

	store := postgres.NewStore(db)
	var records []application.ReviewSignalRecord
	require.NoError(t, store.WithTx(ctx, func(tx reviewapplication.Tx) error {
		var err error
		records, err = tx.Reviews().ListUnprojected(ctx, 10)
		return err
	}))

	require.Len(t, records, 1)
	require.Equal(t, "tenant-1", records[0].TenantID)
	require.Equal(t, review.ID, records[0].ReviewID)
	require.Equal(t, "agent-v1", records[0].AgentVersionID)
	require.Equal(t, "code", records[0].TaskType)
	require.Equal(t, domain.DecisionAccepted, records[0].Decision)
}

func TestListUnprojectedSkipsPendingAndAlreadyProjected(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	insertExecutionAndTask(t, db, "tenant-1", "task-1", "exe-1", "agent-v1", "code")
	insertExecutionAndTask(t, db, "tenant-1", "task-2", "exe-2", "agent-v1", "code")
	reviewerID := insertReviewer(t, db, "tenant-1", "reviewer-1")
	rubricID := insertRubricVersion(t, db, "tenant-1", 1)
	pending := newReview("review-pending", "tenant-1", "exe-1", reviewerID, rubricID)
	require.NoError(t, postgres.NewReviewRepository(db).Insert(ctx, pending))
	submitted := submitReviewWithRubric(t, db, "tenant-1", "review-submitted", "exe-2", reviewerID, rubricID, domain.DecisionAccepted)

	_, err := db.Exec(ctx, "UPDATE reviews SET projected_at=clock_timestamp() WHERE tenant_id='tenant-1' AND id=$1", submitted.ID)
	require.NoError(t, err)

	store := postgres.NewStore(db)
	var records []application.ReviewSignalRecord
	require.NoError(t, store.WithTx(ctx, func(tx reviewapplication.Tx) error {
		records, err = tx.Reviews().ListUnprojected(ctx, 10)
		return err
	}))
	require.Empty(t, records)
}

func TestMarkProjectedSetsTimestamp(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	insertExecutionAndTask(t, db, "tenant-1", "task-1", "exe-1", "agent-v1", "code")
	reviewerID := insertReviewer(t, db, "tenant-1", "reviewer-1")
	rubricID := insertRubricVersion(t, db, "tenant-1", 1)
	review := submitReviewWithRubric(t, db, "tenant-1", "review-1", "exe-1", reviewerID, rubricID, domain.DecisionAccepted)

	store := postgres.NewStore(db)
	var txNow time.Time
	require.NoError(t, store.WithTx(ctx, func(tx reviewapplication.Tx) error {
		var err error
		txNow, err = tx.Now(ctx)
		if err != nil {
			return err
		}
		return tx.Reviews().MarkProjected(ctx, "tenant-1", review.ID)
	}))

	var projectedAt time.Time
	require.NoError(t, db.QueryRow(ctx, "SELECT projected_at FROM reviews WHERE tenant_id=$1 AND id=$2", review.TenantID, review.ID).Scan(&projectedAt))
	require.WithinDuration(t, txNow, projectedAt, 0)
}

func TestMarkProjectedNotFoundReturnsDomainError(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	store := postgres.NewStore(db)
	err := store.WithTx(ctx, func(tx reviewapplication.Tx) error {
		return tx.Reviews().MarkProjected(ctx, "tenant-1", "missing")
	})
	require.ErrorIs(t, err, appdomain.ErrNotFound)
	require.Equal(t, "not_found", appdomain.CodeOf(err))
}

func insertExecutionAndTask(t *testing.T, db *pgxpool.Pool, tenantID, taskID, executionID, agentVersionID, taskType string) {
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

func submitReviewWithRubric(t *testing.T, db *pgxpool.Pool, tenantID, reviewID, submissionID, reviewerID, rubricVersionID string, decision domain.Decision) *domain.Review {
	t.Helper()
	ctx := context.Background()
	rubric := newRubricVersion(rubricVersionID, tenantID, 1)
	review := newReview(reviewID, tenantID, submissionID, reviewerID, rubricVersionID)
	require.NoError(t, review.Submit(decision, []domain.RubricScore{{Dimension: "quality", Score: 80}}, rubric, time.Now()))
	require.NoError(t, postgres.NewReviewRepository(db).Insert(ctx, review))
	return review
}

func newReview(id, tenantID, submissionID, reviewerID, rubricVersionID string) *domain.Review {
	review, err := domain.NewReview(id, tenantID, submissionID, reviewerID, rubricVersionID, time.Now())
	if err != nil {
		panic(err)
	}
	return review
}

func newRubricVersion(id, tenantID string, versionNumber int) *domain.RubricVersion {
	version, err := domain.NewRubricVersion(
		id, tenantID, "Rubric "+id, versionNumber,
		[]domain.RubricDimension{{ID: "quality", Name: "Quality"}},
		map[string]float64{"quality": 1.0},
		"v1", time.Now(),
	)
	if err != nil {
		panic(err)
	}
	return version
}

func insertReviewer(t *testing.T, db *pgxpool.Pool, tenantID, reviewerID string) string {
	t.Helper()
	ctx := context.Background()
	profile, err := domain.NewReviewerProfile(reviewerID, tenantID, "user-"+reviewerID, []string{"go"}, time.Now())
	require.NoError(t, err)
	require.NoError(t, postgres.NewStore(db).WithTx(ctx, func(tx reviewapplication.Tx) error {
		return tx.Reviewers().Insert(ctx, profile)
	}))
	return reviewerID
}

func insertReviewerWithLoad(t *testing.T, db *pgxpool.Pool, tenantID, reviewerID string, load int) {
	t.Helper()
	ctx := context.Background()
	profile, err := domain.NewReviewerProfile(reviewerID, tenantID, "user-"+reviewerID, []string{"go"}, time.Now())
	require.NoError(t, err)
	require.NoError(t, profile.SetLoad(load, time.Now()))
	require.NoError(t, postgres.NewReviewerRepository(db).Insert(ctx, profile))
}

func insertRubricVersion(t *testing.T, db *pgxpool.Pool, tenantID string, versionNumber int) string {
	t.Helper()
	ctx := context.Background()
	version, err := domain.NewRubricVersion(
		"rubric-"+tenantID+"-"+string(rune('0'+versionNumber)),
		tenantID,
		"Rubric "+string(rune('0'+versionNumber)),
		versionNumber,
		[]domain.RubricDimension{{ID: "quality", Name: "Quality"}},
		map[string]float64{"quality": 1.0},
		"v1",
		time.Now(),
	)
	require.NoError(t, err)
	require.NoError(t, postgres.NewStore(db).WithTx(ctx, func(tx reviewapplication.Tx) error {
		return tx.Rubrics().CreateVersion(ctx, version)
	}))
	return version.ID
}
