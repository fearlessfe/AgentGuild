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

func TestValidationJobRepositoryRoundTrip(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", time.Now(), func() string { return "job-1" })
	require.NoError(t, err)

	repo := postgres.NewValidationJobRepository(db)
	require.NoError(t, repo.Insert(ctx, job))

	got, err := repo.GetByID(ctx, "tenant-1", "job-1")
	require.NoError(t, err)
	require.Equal(t, job.ID, got.ID)
	require.Equal(t, job.SubmissionID, got.SubmissionID)
	require.Equal(t, gitdomain.ValidationStatusPending, got.Status)
	require.Len(t, got.Steps, len(gitdomain.DefaultValidationSteps))

	bySub, err := repo.GetBySubmissionID(ctx, "tenant-1", "sub-1")
	require.NoError(t, err)
	require.Equal(t, job.ID, bySub.ID)
}

func TestValidationJobRepositoryTenantIsolation(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", time.Now(), func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, postgres.NewValidationJobRepository(db).Insert(ctx, job))

	_, err = postgres.NewValidationJobRepository(db).GetByID(ctx, "tenant-2", "job-1")
	require.ErrorIs(t, err, git.ErrValidationJobNotFound)
}

func TestValidationJobRepositoryClaimNextPending(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", time.Now(), func() string { return "job-1" })
	require.NoError(t, err)

	store := postgres.NewStore(db)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.ValidationJobs().Insert(ctx, job)
	}))

	var claimed *gitdomain.ValidationJob
	now := time.Now()
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		var err error
		claimed, err = tx.ValidationJobs().ClaimNextPending(ctx, "tenant-1", "worker-1", now, now.Add(5*time.Minute))
		return err
	}))
	require.NotNil(t, claimed)
	require.Equal(t, gitdomain.ValidationStatusRunning, claimed.Status)
	require.Equal(t, "worker-1", *claimed.ClaimedBy)
	require.Equal(t, 1, claimed.Attempt)

	// Second claim with same time should not find anything because lease is active.
	var second *gitdomain.ValidationJob
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		var err error
		second, err = tx.ValidationJobs().ClaimNextPending(ctx, "tenant-1", "worker-2", now, now.Add(5*time.Minute))
		return err
	}))
	require.Nil(t, second)
}

func TestValidationJobRepositoryClaimNextPendingAfterExpiry(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", time.Now(), func() string { return "job-1" })
	require.NoError(t, err)

	store := postgres.NewStore(db)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.ValidationJobs().Insert(ctx, job)
	}))

	now := time.Now()
	leaseUntil := now.Add(5 * time.Minute)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		_, err := tx.ValidationJobs().ClaimNextPending(ctx, "tenant-1", "worker-1", now, leaseUntil)
		return err
	}))

	afterLease := leaseUntil.Add(time.Second)
	var claimed *gitdomain.ValidationJob
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		var err error
		claimed, err = tx.ValidationJobs().ClaimNextPending(ctx, "tenant-1", "worker-2", afterLease, afterLease.Add(5*time.Minute))
		return err
	}))
	require.NotNil(t, claimed)
	require.Equal(t, "worker-2", *claimed.ClaimedBy)
	require.Equal(t, 2, claimed.Attempt)
}

func TestValidationJobRepositoryRejectsDuplicateSubmissionJob(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", time.Now(), func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, postgres.NewValidationJobRepository(db).Insert(ctx, job))

	job2, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", time.Now(), func() string { return "job-2" })
	require.NoError(t, err)
	err = postgres.NewValidationJobRepository(db).Insert(ctx, job2)
	require.Error(t, err)
}

func TestValidationJobRepositoryUpdateStep(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", time.Now(), func() string { return "job-1" })
	require.NoError(t, err)
	now := time.Now()
	require.NoError(t, job.Claim("worker-1", now.Add(5*time.Minute), now))
	require.NoError(t, job.StartStep(gitdomain.ValidationStepBuild, now))

	store := postgres.NewStore(db)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		if err := tx.ValidationJobs().Insert(ctx, job); err != nil {
			return err
		}
		step := job.Steps[0]
		step.Status = gitdomain.ValidationStepStatusSucceeded
		step.LogSummary = "built"
		step.FinishedAt = &now
		return tx.ValidationJobs().UpdateStep(ctx, job.TenantID, job.ID, step)
	}))

	got, err := postgres.NewValidationJobRepository(db).GetByID(ctx, "tenant-1", "job-1")
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStepStatusSucceeded, got.Steps[0].Status)
	require.Equal(t, "built", got.Steps[0].LogSummary)
}
