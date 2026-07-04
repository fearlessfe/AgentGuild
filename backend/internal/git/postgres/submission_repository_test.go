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
	record := sampleSubmission("tenant-1", "task-1", "exec-1", "sub-1")

	repo := postgres.NewSubmissionRepository(db)
	require.NoError(t, repo.Save(ctx, record))

	got, err := repo.GetByID(ctx, record.TenantID, record.ID)
	require.NoError(t, err)
	require.Equal(t, record.ID, got.ID)
	require.Equal(t, record.TaskID, got.TaskID)
	require.Equal(t, record.ExecutionID, got.ExecutionID)
	require.Equal(t, record.Branch, got.Branch)
	require.Equal(t, record.CommitSHA, got.CommitSHA)
	require.Equal(t, record.BaseCommitSHA, got.BaseCommitSHA)
	require.Equal(t, record.Summary, got.Summary)
	require.Equal(t, record.TestDeclaration, got.TestDeclaration)
	require.Equal(t, record.DiffFingerprint, got.DiffFingerprint)
	require.Equal(t, record.Status, got.Status)
	require.Nil(t, got.ValidationJobID)
	require.True(t, record.CreatedAt.Equal(got.CreatedAt))
	require.False(t, got.UpdatedAt.Before(record.UpdatedAt))

	byExecution, err := repo.GetByExecutionID(ctx, record.TenantID, record.ExecutionID)
	require.NoError(t, err)
	require.Len(t, byExecution, 1)
	require.Equal(t, record.ID, byExecution[0].ID)
}

func TestSubmissionRepositoryTenantIsolation(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	record := sampleSubmission("tenant-1", "task-1", "exec-1", "sub-1")
	require.NoError(t, postgres.NewSubmissionRepository(db).Save(ctx, record))

	repo := postgres.NewSubmissionRepository(db)
	_, err := repo.GetByID(ctx, "tenant-2", record.ID)
	require.ErrorIs(t, err, git.ErrSubmissionNotFound)

	got, err := repo.GetByExecutionID(ctx, "tenant-2", record.ExecutionID)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestSubmissionRepositoryUpdateStatus(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	record := sampleSubmission("tenant-1", "task-1", "exec-1", "sub-1")

	store := postgres.NewStore(db)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.Submissions().Save(ctx, record)
	}))

	record.Status = gitdomain.SubmissionStatusValidated
	jobID := "job-1"
	record.ValidationJobID = &jobID
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.Submissions().Save(ctx, record)
	}))

	got, err := postgres.NewSubmissionRepository(db).GetByID(ctx, "tenant-1", "sub-1")
	require.NoError(t, err)
	require.Equal(t, gitdomain.SubmissionStatusValidated, got.Status)
	require.Equal(t, "job-1", *got.ValidationJobID)
	require.True(t, got.UpdatedAt.After(record.CreatedAt))
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
	record := sampleSubmission("tenant-1", "task-1", "exec-1", "sub-1")
	record.Evidence = []byte(`{"tests":42,"coverage":0.8}`)

	require.NoError(t, postgres.NewSubmissionRepository(db).Save(ctx, record))

	got, err := postgres.NewSubmissionRepository(db).GetByID(ctx, "tenant-1", "sub-1")
	require.NoError(t, err)
	require.JSONEq(t, string(record.Evidence), string(got.Evidence))
}

func TestSubmissionRepositoryUpsertRespectsTenantIsolation(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	repo := postgres.NewSubmissionRepository(db)

	recordTenant1 := sampleSubmission("tenant-1", "task-1", "exec-1", "sub-shared")
	recordTenant2 := sampleSubmission("tenant-2", "task-2", "exec-2", "sub-shared")
	require.NoError(t, repo.Save(ctx, recordTenant1))
	require.NoError(t, repo.Save(ctx, recordTenant2))

	got1, err := repo.GetByID(ctx, "tenant-1", "sub-shared")
	require.NoError(t, err)
	require.Equal(t, "tenant-1", got1.TenantID)

	got2, err := repo.GetByID(ctx, "tenant-2", "sub-shared")
	require.NoError(t, err)
	require.Equal(t, "tenant-2", got2.TenantID)
}

func sampleSubmission(tenantID, taskID, executionID, id string) *application.SubmissionRecord {
	testDecl := "go test ./..."
	return &application.SubmissionRecord{
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
		CreatedAt:       time.Now().UTC().Truncate(time.Microsecond),
		UpdatedAt:       time.Now().UTC().Truncate(time.Microsecond),
	}
}
