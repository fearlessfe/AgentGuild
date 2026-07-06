package worker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"agentguild.dev/agentguild/backend/internal/git/worker"
	"github.com/stretchr/testify/require"
)

func TestValidationWorkerClaimsPendingJob(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	store := newMemoryStore(now)
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.ValidationJobs().Insert(ctx, job)
	}))

	w := worker.NewValidationWorker(store, "worker-1", 5*time.Minute, 3, nil, nil)
	processed, err := w.RunOnce(ctx, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	got, err := store.getJob("job-1")
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStatusSucceeded, got.Status)
	require.Nil(t, got.ClaimedBy)
}

func TestValidationWorkerReturnsZeroWhenNoJobs(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	store := newMemoryStore(now)
	w := worker.NewValidationWorker(store, "worker-1", 5*time.Minute, 3, nil, nil)
	processed, err := w.RunOnce(ctx, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, 0, processed)
}

func TestValidationWorkerRespectsActiveLease(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	store := newMemoryStore(now)
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, job.Claim("worker-1", now.Add(5*time.Minute), now))
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.ValidationJobs().Insert(ctx, job)
	}))

	w := worker.NewValidationWorker(store, "worker-2", 5*time.Minute, 3, nil, nil)
	processed, err := w.RunOnce(ctx, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, 0, processed)
}

func TestValidationWorkerSkipsNoRunner(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	store := newMemoryStore(now)
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.ValidationJobs().Insert(ctx, job)
	}))

	w := worker.NewValidationWorker(store, "worker-1", 5*time.Minute, 3, nil, nil)
	_, err = w.RunOnce(ctx, "tenant-1")
	require.NoError(t, err)

	got, err := store.getJob("job-1")
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStatusSucceeded, got.Status)
	for _, step := range got.Steps {
		require.Equal(t, gitdomain.ValidationStepStatusSkipped, step.Status)
	}
}

func TestValidationWorkerRecordFailureAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	store := newMemoryStore(now)
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	job.Attempt = 3
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.ValidationJobs().Insert(ctx, job)
	}))

	// Manually mark as running without claiming to simulate a failed run.
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		job.Status = gitdomain.ValidationStatusRunning
		return tx.ValidationJobs().Update(ctx, job)
	}))

	recorder := &recordingNotifier{}
	w := worker.NewValidationWorker(store, "worker-1", 5*time.Minute, 3, nil, recorder)
	_, err = w.RunOnce(ctx, "tenant-1")
	require.NoError(t, err)

	got, err := store.getJob("job-1")
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStatusFailed, got.Status)
	require.Len(t, recorder.calls, 1)
	require.Equal(t, domain.IntentFailValidation, recorder.calls[0].Intent)
	require.Equal(t, "exec-1", recorder.calls[0].ExecutionID)
}

func TestValidationWorkerRecordsFailureWhenRunnerFails(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	store := newMemoryStore(now)
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.ValidationJobs().Insert(ctx, job)
	}))

	runner := &fakeRunner{failStep: gitdomain.ValidationStepBuild}
	recorder := &recordingNotifier{}
	w := worker.NewValidationWorker(store, "worker-1", 5*time.Minute, 3, runner, recorder)
	_, err = w.RunOnce(ctx, "tenant-1")
	require.NoError(t, err)

	got, err := store.getJob("job-1")
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStatusFailed, got.Status)
	require.Equal(t, gitdomain.ValidationStepStatusFailed, got.Steps[0].Status)
	require.Len(t, recorder.calls, 2)
	require.Equal(t, domain.IntentStartValidation, recorder.calls[0].Intent)
	require.Equal(t, domain.IntentFailValidation, recorder.calls[1].Intent)
}

func TestValidationWorkerSucceedsWhenRunnerPassesAllSteps(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	store := newMemoryStore(now)
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "exec-1", "owner/repo", "agentguild/exec-1", "head-sha", "v1", now, func() string { return "job-1" })
	require.NoError(t, err)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.ValidationJobs().Insert(ctx, job)
	}))

	w := worker.NewValidationWorker(store, "worker-1", 5*time.Minute, 3, &fakeRunner{}, nil)
	processed, err := w.RunOnce(ctx, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	got, err := store.getJob("job-1")
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStatusSucceeded, got.Status)
	for _, step := range got.Steps {
		require.Equal(t, gitdomain.ValidationStepStatusSucceeded, step.Status)
	}
}

type fakeRunner struct {
	failStep gitdomain.ValidationStep
}

func (r *fakeRunner) RunStep(_ context.Context, _ *gitdomain.ValidationJob, step gitdomain.ValidationStep) (gitdomain.Step, error) {
	now := time.Now()
	if step == r.failStep {
		return gitdomain.Step{
			Step:       step,
			Status:     gitdomain.ValidationStepStatusFailed,
			LogSummary: "step failed",
			StartedAt:  &now,
			FinishedAt: &now,
		}, nil
	}
	return gitdomain.Step{
		Step:       step,
		Status:     gitdomain.ValidationStepStatusSucceeded,
		LogSummary: "ok",
		StartedAt:  &now,
		FinishedAt: &now,
	}, nil
}

type memoryStore struct {
	now            time.Time
	validationJobs map[string]*gitdomain.ValidationJob
}

func newMemoryStore(now time.Time) *memoryStore {
	return &memoryStore{now: now, validationJobs: map[string]*gitdomain.ValidationJob{}}
}

func (s *memoryStore) getJob(id string) (*gitdomain.ValidationJob, error) {
	job := s.validationJobs[id]
	if job == nil {
		return nil, errors.New("validation job not found")
	}
	return job, nil
}

func (s *memoryStore) WithTx(ctx context.Context, fn func(application.Tx) error) error {
	return fn(&memoryTx{store: s})
}

type memoryTx struct {
	store *memoryStore
}

func (tx *memoryTx) Credentials() application.CredentialRepository { return nil }
func (tx *memoryTx) Submissions() application.SubmissionRepository { return nil }
func (tx *memoryTx) GitHubApps() application.GitHubAppRepository    { return nil }
func (tx *memoryTx) ValidationJobs() application.ValidationJobRepository {
	return &memoryValidationJobRepository{store: tx.store}
}
func (tx *memoryTx) Now(context.Context) (time.Time, error) { return tx.store.now, nil }

type memoryValidationJobRepository struct {
	store *memoryStore
}

func (r *memoryValidationJobRepository) Insert(_ context.Context, job *gitdomain.ValidationJob) error {
	r.store.validationJobs[job.ID] = job
	return nil
}

func (r *memoryValidationJobRepository) GetByID(_ context.Context, tenantID, id string) (*gitdomain.ValidationJob, error) {
	job := r.store.validationJobs[id]
	if job == nil || job.TenantID != tenantID {
		return nil, nil
	}
	return job, nil
}

func (r *memoryValidationJobRepository) GetBySubmissionID(_ context.Context, tenantID, submissionID string) (*gitdomain.ValidationJob, error) {
	for _, job := range r.store.validationJobs {
		if job.TenantID == tenantID && job.SubmissionID == submissionID {
			return job, nil
		}
	}
	return nil, nil
}

func (r *memoryValidationJobRepository) ClaimNextPending(_ context.Context, tenantID, workerID string, now, until time.Time) (*gitdomain.ValidationJob, error) {
	for _, job := range r.store.validationJobs {
		if job.TenantID != tenantID {
			continue
		}
		if job.Status != gitdomain.ValidationStatusPending && job.Status != gitdomain.ValidationStatusRunning {
			continue
		}
		if job.ClaimedUntil != nil && job.ClaimedUntil.After(now) {
			continue
		}
		if err := job.Claim(workerID, until, now); err != nil {
			continue
		}
		return job, nil
	}
	return nil, nil
}

func (r *memoryValidationJobRepository) Update(_ context.Context, job *gitdomain.ValidationJob) error {
	r.store.validationJobs[job.ID] = job
	return nil
}

func (r *memoryValidationJobRepository) UpdateStep(_ context.Context, tenantID, jobID string, step gitdomain.Step) error {
	job, err := r.store.getJob(jobID)
	if err != nil || job.TenantID != tenantID {
		return nil
	}
	for i, s := range job.Steps {
		if s.Step == step.Step {
			job.Steps[i] = step
			return nil
		}
	}
	return nil
}


type recordingNotifier struct {
	calls []application.ExecutionStateCommand
}

func (r *recordingNotifier) Notify(_ context.Context, cmd application.ExecutionStateCommand, _ time.Time) error {
	r.calls = append(r.calls, cmd)
	return nil
}
