package postgres_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"agentguild.dev/agentguild/backend/internal/git/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestSubmissionRepositoryRoundTrip(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	sub := sampleSubmission("tenant-1", "task-1", "exec-1", "sub-1")

	repo := postgres.NewSubmissionRepository(db)
	require.NoError(t, repo.Save(ctx, sub))

	got, err := repo.GetByID(ctx, sub.TenantID, sub.ID)
	require.NoError(t, err)
	require.Equal(t, sub.ID, got.ID)
	require.Equal(t, sub.TaskID, got.TaskID)
	require.Equal(t, sub.ExecutionID, got.ExecutionID)
	require.Equal(t, sub.Branch, got.Branch)
	require.Equal(t, sub.CommitSHA, got.CommitSHA)
	require.Equal(t, sub.BaseCommitSHA, got.BaseCommitSHA)
	require.Equal(t, sub.Summary, got.Summary)
	require.Equal(t, sub.TestDeclaration, got.TestDeclaration)
	require.Equal(t, sub.DiffFingerprint, got.DiffFingerprint)
	require.Equal(t, sub.Status, got.Status)
	require.Nil(t, got.ValidationJobID)
	require.True(t, sub.CreatedAt.Equal(got.CreatedAt))
	require.False(t, got.UpdatedAt.Before(sub.UpdatedAt))

	byExecution, err := repo.GetByExecutionID(ctx, sub.TenantID, sub.ExecutionID)
	require.NoError(t, err)
	require.Len(t, byExecution, 1)
	require.Equal(t, sub.ID, byExecution[0].ID)
}

func TestSubmissionRepositoryTenantIsolation(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	sub := sampleSubmission("tenant-1", "task-1", "exec-1", "sub-1")
	require.NoError(t, postgres.NewSubmissionRepository(db).Save(ctx, sub))

	repo := postgres.NewSubmissionRepository(db)
	_, err := repo.GetByID(ctx, "tenant-2", sub.ID)
	require.ErrorIs(t, err, git.ErrSubmissionNotFound)

	got, err := repo.GetByExecutionID(ctx, "tenant-2", sub.ExecutionID)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestSubmissionRepositoryUpdateStatus(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	sub := sampleSubmission("tenant-1", "task-1", "exec-1", "sub-1")

	store := postgres.NewStore(db)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.Submissions().Save(ctx, sub)
	}))

	now := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, sub.MarkValidated(now))
	jobID := "job-1"
	sub.SetValidationJobID(jobID, now)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.Submissions().Save(ctx, sub)
	}))

	got, err := postgres.NewSubmissionRepository(db).GetByID(ctx, "tenant-1", "sub-1")
	require.NoError(t, err)
	require.Equal(t, gitdomain.SubmissionStatusValidated, got.Status)
	require.Equal(t, "job-1", *got.ValidationJobID)
	require.True(t, got.UpdatedAt.After(sub.CreatedAt))
}

func TestSubmissionRepositoryListsMultipleSubmissionsPerExecution(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	repo := postgres.NewSubmissionRepository(db)

	require.NoError(t, repo.Save(ctx, sampleSubmission("tenant-1", "task-1", "exec-1", "sub-1")))
	require.NoError(t, repo.Save(ctx, sampleSubmission("tenant-1", "task-1", "exec-1", "sub-2")))
	require.NoError(t, repo.Save(ctx, sampleSubmission("tenant-1", "task-2", "exec-2", "sub-3")))

	got, err := repo.GetByExecutionID(ctx, "tenant-1", "exec-1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "sub-1", got[0].ID)
	require.Equal(t, "sub-2", got[1].ID)
}

func TestSubmissionRepositoryEvidenceJSONB(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	sub := sampleSubmission("tenant-1", "task-1", "exec-1", "sub-1")
	sub.Evidence = []byte(`{"tests":42,"coverage":0.8}`)

	require.NoError(t, postgres.NewSubmissionRepository(db).Save(ctx, sub))

	got, err := postgres.NewSubmissionRepository(db).GetByID(ctx, "tenant-1", "sub-1")
	require.NoError(t, err)
	require.JSONEq(t, string(sub.Evidence), string(got.Evidence))
}

func TestSubmissionRepositoryEvidenceNilOrEmpty(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	repo := postgres.NewSubmissionRepository(db)

	// nil evidence
	subNil := sampleSubmission("tenant-1", "task-1", "exec-1", "sub-nil")
	subNil.Evidence = nil
	require.NoError(t, repo.Save(ctx, subNil))

	gotNil, err := repo.GetByID(ctx, subNil.TenantID, subNil.ID)
	require.NoError(t, err)
	require.Nil(t, gotNil.Evidence)

	// empty evidence
	subEmpty := sampleSubmission("tenant-1", "task-1", "exec-1", "sub-empty")
	subEmpty.Evidence = []byte{}
	require.NoError(t, repo.Save(ctx, subEmpty))

	gotEmpty, err := repo.GetByID(ctx, subEmpty.TenantID, subEmpty.ID)
	require.NoError(t, err)
	require.Empty(t, gotEmpty.Evidence)
}

func TestSubmissionRepositoryUpsertRespectsTenantIsolation(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	repo := postgres.NewSubmissionRepository(db)

	subTenant1 := sampleSubmission("tenant-1", "task-1", "exec-1", "sub-shared")
	subTenant2 := sampleSubmission("tenant-2", "task-2", "exec-2", "sub-shared")
	require.NoError(t, repo.Save(ctx, subTenant1))
	require.NoError(t, repo.Save(ctx, subTenant2))

	got1, err := repo.GetByID(ctx, "tenant-1", "sub-shared")
	require.NoError(t, err)
	require.Equal(t, "tenant-1", got1.TenantID)

	got2, err := repo.GetByID(ctx, "tenant-2", "sub-shared")
	require.NoError(t, err)
	require.Equal(t, "tenant-2", got2.TenantID)
}

func sampleSubmission(tenantID, taskID, executionID, id string) *gitdomain.Submission {
	testDecl := "go test ./..."
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &gitdomain.Submission{
		ID:              id,
		TenantID:        tenantID,
		TaskID:          taskID,
		ExecutionID:     executionID,
		Branch:          "agentguild/" + executionID,
		CommitSHA:       "head-" + id,
		BaseCommitSHA:   "base-" + id,
		Summary:         "submission " + id,
		TestDeclaration: &testDecl,
		Evidence:        []byte(`{"ok": true}`),
		DiffFingerprint: gitdomain.DiffFingerprint("agentguild/"+executionID, "base-"+id, "head-"+id, []string{"main.go"}),
		Status:          gitdomain.SubmissionStatusPendingVerification,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}
