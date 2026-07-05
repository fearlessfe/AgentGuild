// Package worker implements the asynchronous validation worker for submissions.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
)

// StepRunner executes a single validation step.
// Real implementations are provided by the validation/application layer.
type StepRunner interface {
	RunStep(ctx context.Context, job *gitdomain.ValidationJob, step gitdomain.ValidationStep) (gitdomain.Step, error)
}

// ValidationWorker consumes validation_jobs using PostgreSQL advisory leases.
type ValidationWorker struct {
	store      gitapp.Store
	workerID   string
	lease      time.Duration
	maxAttempts int
	runner     StepRunner
}

// NewValidationWorker creates a worker.
func NewValidationWorker(store gitapp.Store, workerID string, lease time.Duration, maxAttempts int, runner StepRunner) *ValidationWorker {
	if store == nil {
		panic("store is required")
	}
	if workerID == "" {
		workerID = "worker-" + time.Now().Format("20060102-150405")
	}
	if lease <= 0 {
		lease = 5 * time.Minute
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &ValidationWorker{
		store:       store,
		workerID:    workerID,
		lease:       lease,
		maxAttempts: maxAttempts,
		runner:      runner,
	}
}

// RunOnce attempts to claim and process one pending validation job per tenant.
// It returns the number of jobs processed (claimed, regardless of outcome).
func (w *ValidationWorker) RunOnce(ctx context.Context, tenantID string) (int, error) {
	if tenantID == "" {
		return 0, fmt.Errorf("tenant_id is required")
	}
	processed := 0
	for {
		done, err := w.runOne(ctx, tenantID)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return processed, err
			}
			slog.Error("validation worker iteration failed", "error", err, "tenant", tenantID)
			return processed, err
		}
		if done {
			return processed, nil
		}
		processed++
	}
}

func (w *ValidationWorker) runOne(ctx context.Context, tenantID string) (bool, error) {
	var job *gitdomain.ValidationJob
	err := w.store.WithTx(ctx, func(tx gitapp.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		until := now.Add(w.lease)
		job, err = tx.ValidationJobs().ClaimNextPending(ctx, tenantID, w.workerID, now, until)
		return err
	})
	if err != nil {
		return false, err
	}
	if job == nil {
		return true, nil
	}
	if job.Attempt > w.maxAttempts {
		return false, w.recordFailure(ctx, job, fmt.Errorf("max attempts %d exceeded", w.maxAttempts))
	}

	if err := w.process(ctx, job); err != nil {
		if errors.Is(err, context.Canceled) {
			return false, err
		}
		_ = w.recordFailure(ctx, job, err)
	}
	return false, nil
}

func (w *ValidationWorker) process(ctx context.Context, job *gitdomain.ValidationJob) error {
	for _, step := range job.Steps {
		if step.Status == gitdomain.ValidationStepStatusSucceeded || step.Status == gitdomain.ValidationStepStatusSkipped {
			continue
		}
		if err := w.runStep(ctx, job, step.Step); err != nil {
			return err
		}
		if job.Status == gitdomain.ValidationStatusFailed {
			return nil
		}
	}
	return nil
}

func (w *ValidationWorker) runStep(ctx context.Context, job *gitdomain.ValidationJob, step gitdomain.ValidationStep) error {
	return w.store.WithTx(ctx, func(tx gitapp.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		fresh, err := tx.ValidationJobs().GetByID(ctx, job.TenantID, job.ID)
		if err != nil {
			return err
		}
		if fresh.Status != gitdomain.ValidationStatusRunning {
			return errors.New("job is no longer running")
		}
		if err := fresh.StartStep(step, now); err != nil {
			return err
		}
		if err := tx.ValidationJobs().Update(ctx, fresh); err != nil {
			return err
		}

		// Run the step. In a future version the runner will execute outside this
		// transaction so long-running work does not hold the row lock.
		if w.runner == nil {
			result := gitdomain.Step{
				Step:       step,
				Status:     gitdomain.ValidationStepStatusSkipped,
				LogSummary: "no runner configured",
			}
			if err := fresh.FinishStep(step, gitdomain.ValidationStepStatusSkipped, result.LogSummary, nil, now); err != nil {
				return err
			}
			if err := tx.ValidationJobs().UpdateStep(ctx, fresh.TenantID, fresh.ID, result); err != nil {
				return err
			}
			return tx.ValidationJobs().Update(ctx, fresh)
		}

		result, err := w.runner.RunStep(ctx, fresh, step)
		if err != nil {
			return err
		}
		var usage []byte
		if len(result.ResourceUsage) > 0 {
			usage = result.ResourceUsage
		}
		if err := fresh.FinishStep(step, result.Status, result.LogSummary, usage, *result.FinishedAt); err != nil {
			return err
		}
		if err := tx.ValidationJobs().UpdateStep(ctx, fresh.TenantID, fresh.ID, result); err != nil {
			return err
		}
		return tx.ValidationJobs().Update(ctx, fresh)
	})
}

func (w *ValidationWorker) recordFailure(ctx context.Context, job *gitdomain.ValidationJob, cause error) error {
	return w.store.WithTx(ctx, func(tx gitapp.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		fresh, err := tx.ValidationJobs().GetByID(ctx, job.TenantID, job.ID)
		if err != nil {
			return err
		}
		if fresh.Status != gitdomain.ValidationStatusRunning {
			return nil
		}
		if fresh.Attempt >= w.maxAttempts {
			fresh.Status = gitdomain.ValidationStatusFailed
		} else {
			fresh.Status = gitdomain.ValidationStatusPending
		}
		fresh.ClaimedBy = nil
		fresh.ClaimedUntil = nil
		fresh.UpdatedAt = now
		return tx.ValidationJobs().Update(ctx, fresh)
	})
}

// ValidationJobError is a simple error type for worker configuration issues.
type ValidationJobError struct {
	Message string
}

func (e *ValidationJobError) Error() string {
	return fmt.Sprintf("validation job error: %s", e.Message)
}
