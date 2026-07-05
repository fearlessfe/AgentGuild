package domain_test

import (
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"github.com/stretchr/testify/require"
)

func TestNewValidationJobRequiresFields(t *testing.T) {
	now := time.Now()
	newID := func() string { return "job-1" }

	_, err := gitdomain.NewValidationJob("", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, newID)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))

	_, err = gitdomain.NewValidationJob("tenant-1", "", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, newID)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))

	_, err = gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "", "agentguild/exec-1", "head-sha", "v1", now, newID)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))

	_, err = gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "", "head-sha", "v1", now, newID)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))

	_, err = gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "", "v1", now, newID)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))

	_, err = gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "", now, newID)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))
}

func TestNewValidationJobCreatesPendingJobWithDefaultSteps(t *testing.T) {
	now := time.Now()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	require.Equal(t, "job-1", job.ID)
	require.Equal(t, gitdomain.ValidationStatusPending, job.Status)
	require.Len(t, job.Steps, len(gitdomain.DefaultValidationSteps))
	for i, step := range gitdomain.DefaultValidationSteps {
		require.Equal(t, step, job.Steps[i].Step)
		require.Equal(t, gitdomain.ValidationStepStatusPending, job.Steps[i].Status)
	}
}

func TestClaimTransitionsToRunning(t *testing.T) {
	now := time.Now()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)

	until := now.Add(5 * time.Minute)
	err = job.Claim("worker-1", until, now)
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStatusRunning, job.Status)
	require.Equal(t, 1, job.Attempt)
	require.Equal(t, "worker-1", *job.ClaimedBy)
	require.True(t, job.ClaimedUntil.Equal(until))
}

func TestClaimRejectsActiveLease(t *testing.T) {
	now := time.Now()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, job.Claim("worker-1", now.Add(5*time.Minute), now))

	err = job.Claim("worker-2", now.Add(10*time.Minute), now)
	require.ErrorIs(t, err, domain.ErrStateConflict)
}

func TestClaimAllowsExpiredLease(t *testing.T) {
	now := time.Now()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, job.Claim("worker-1", now.Add(5*time.Minute), now))

	later := now.Add(6 * time.Minute)
	require.NoError(t, job.Claim("worker-2", later.Add(5*time.Minute), later))
	require.Equal(t, "worker-2", *job.ClaimedBy)
	require.Equal(t, 2, job.Attempt)
}

func TestStartStepRequiresRunningState(t *testing.T) {
	now := time.Now()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)

	err = job.StartStep(gitdomain.ValidationStepBuild, now)
	require.ErrorIs(t, err, domain.ErrStateConflict)
}

func TestFinishStepAdvancesState(t *testing.T) {
	now := time.Now()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, job.Claim("worker-1", now.Add(5*time.Minute), now))
	require.NoError(t, job.StartStep(gitdomain.ValidationStepBuild, now))

	err = job.FinishStep(gitdomain.ValidationStepBuild, gitdomain.ValidationStepStatusSucceeded, "built", nil, now)
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStepStatusSucceeded, job.Steps[0].Status)
	require.Equal(t, gitdomain.ValidationStatusRunning, job.Status)
}

func TestFinishStepFailsHardGate(t *testing.T) {
	now := time.Now()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, job.Claim("worker-1", now.Add(5*time.Minute), now))
	require.NoError(t, job.StartStep(gitdomain.ValidationStepBuild, now))

	job.Steps[0].HardGate = true
	err = job.FinishStep(gitdomain.ValidationStepBuild, gitdomain.ValidationStepStatusFailed, "compile error", nil, now)
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStatusFailed, job.Status)
	require.Nil(t, job.ClaimedBy)
}

func TestFinishStepSoftFailureDoesNotFailJobImmediately(t *testing.T) {
	now := time.Now()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, job.Claim("worker-1", now.Add(5*time.Minute), now))
	require.NoError(t, job.StartStep(gitdomain.ValidationStepBuild, now))

	job.Steps[0].HardGate = false
	require.NoError(t, job.FinishStep(gitdomain.ValidationStepBuild, gitdomain.ValidationStepStatusFailed, "lint warning", nil, now))
	require.Equal(t, gitdomain.ValidationStatusRunning, job.Status)

	for i := 1; i < len(job.Steps); i++ {
		require.NoError(t, job.StartStep(job.Steps[i].Step, now))
		require.NoError(t, job.FinishStep(job.Steps[i].Step, gitdomain.ValidationStepStatusSucceeded, "ok", nil, now))
	}
	require.Equal(t, gitdomain.ValidationStatusFailed, job.Status)
	require.Nil(t, job.ClaimedBy)
}

func TestFinishStepSucceedsWhenAllStepsDone(t *testing.T) {
	now := time.Now()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, job.Claim("worker-1", now.Add(5*time.Minute), now))

	for _, step := range gitdomain.DefaultValidationSteps {
		require.NoError(t, job.StartStep(step, now))
		require.NoError(t, job.FinishStep(step, gitdomain.ValidationStepStatusSucceeded, "ok", nil, now))
	}

	require.Equal(t, gitdomain.ValidationStatusSucceeded, job.Status)
	require.Nil(t, job.ClaimedBy)
}

func TestCancelPendingJob(t *testing.T) {
	now := time.Now()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)

	require.NoError(t, job.Cancel(now))
	require.Equal(t, gitdomain.ValidationStatusCancelled, job.Status)
}

func TestCancelCompletedJobFails(t *testing.T) {
	now := time.Now()
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, job.Claim("worker-1", now.Add(5*time.Minute), now))
	for _, step := range gitdomain.DefaultValidationSteps {
		require.NoError(t, job.StartStep(step, now))
		require.NoError(t, job.FinishStep(step, gitdomain.ValidationStepStatusSucceeded, "ok", nil, now))
	}

	err = job.Cancel(now)
	require.ErrorIs(t, err, domain.ErrStateConflict)
}
