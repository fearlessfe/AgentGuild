// Package worker implements the asynchronous validation worker for submissions.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
)

// StepRunner executes a single validation step.
// Real implementations are provided by the validation/application layer.
type StepRunner interface {
	RunStep(ctx context.Context, job *gitdomain.ValidationJob, step gitdomain.ValidationStep) (gitdomain.Step, error)
}

type IntegrityChecker interface {
	CheckIntegrity(context.Context, *gitdomain.ValidationJob) (bool, error)
}

// ValidationWorker consumes validation_jobs using PostgreSQL advisory leases.
type ValidationWorker struct {
	store       gitapp.Store
	workerID    string
	lease       time.Duration
	maxAttempts int
	runner      StepRunner
	notifier    gitapp.ExecutionNotifier
}

// NewValidationWorker creates a worker.
func NewValidationWorker(store gitapp.Store, workerID string, lease time.Duration, maxAttempts int, runner StepRunner, notifier gitapp.ExecutionNotifier) *ValidationWorker {
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
	if notifier == nil {
		notifier = gitapp.NopExecutionNotifier{}
	}
	return &ValidationWorker{
		store:       store,
		workerID:    workerID,
		lease:       lease,
		maxAttempts: maxAttempts,
		runner:      runner,
		notifier:    notifier,
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
	if job.Attempt > w.maxAttempts && job.Status != gitdomain.ValidationStatusSucceeded && job.Status != gitdomain.ValidationStatusFailed {
		return false, w.recordFailure(ctx, job, fmt.Errorf("max attempts %d exceeded", w.maxAttempts))
	}

	if job.Status != gitdomain.ValidationStatusSucceeded && job.Status != gitdomain.ValidationStatusFailed {
		if err := w.notifier.Notify(ctx, gitapp.ExecutionStateCommand{
			TenantID:    job.TenantID,
			ExecutionID: job.ExecutionID,
			Intent:      domain.IntentStartValidation,
			Actor:       domain.Actor{Type: domain.ActorSystem, ID: "validation-worker"},
		}, time.Now()); err != nil {
			return false, w.recordFailure(ctx, job, err)
		}
	}

	if err := w.process(ctx, job); err != nil {
		if errors.Is(err, context.Canceled) {
			return false, err
		}
		if recordErr := w.recordFailure(ctx, job, err); recordErr != nil {
			return false, recordErr
		}
		if job.Status == gitdomain.ValidationStatusSucceeded || job.Status == gitdomain.ValidationStatusFailed {
			return false, err
		}
	}
	return false, nil
}

func (w *ValidationWorker) process(ctx context.Context, job *gitdomain.ValidationJob) error {
	if job.Status == gitdomain.ValidationStatusSucceeded || job.Status == gitdomain.ValidationStatusFailed {
		return w.syncTerminalState(ctx, job)
	}
	for _, step := range job.Steps {
		if step.Status == gitdomain.ValidationStepStatusSucceeded || step.Status == gitdomain.ValidationStepStatusSkipped {
			continue
		}
		if err := w.runStep(ctx, job, step.Step); err != nil {
			return err
		}
		if job.Status == gitdomain.ValidationStatusFailed {
			return w.syncTerminalState(ctx, job)
		}
	}
	if job.Status == gitdomain.ValidationStatusSucceeded {
		return w.syncTerminalState(ctx, job)
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
		for i, s := range fresh.Steps {
			if s.Step == step {
				fresh.Steps[i].HardGate = result.HardGate
				break
			}
		}
		if err := fresh.FinishStep(step, result.Status, result.LogSummary, usage, *result.FinishedAt); err != nil {
			return err
		}
		if err := tx.ValidationJobs().UpdateStep(ctx, fresh.TenantID, fresh.ID, result); err != nil {
			return err
		}
		if err := tx.ValidationJobs().Update(ctx, fresh); err != nil {
			return err
		}
		// Keep the in-memory job pointer in sync with the persisted state so
		// callers can inspect the updated status after the step finishes.
		*job = *fresh
		return nil
	})
}

func (w *ValidationWorker) recordFailure(ctx context.Context, job *gitdomain.ValidationJob, cause error) error {
	err := w.store.WithTx(ctx, func(tx gitapp.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		fresh, err := tx.ValidationJobs().GetByID(ctx, job.TenantID, job.ID)
		if err != nil {
			return err
		}
		if fresh.Status != gitdomain.ValidationStatusRunning {
			if fresh.Status == gitdomain.ValidationStatusFailed || fresh.Status == gitdomain.ValidationStatusSucceeded {
				fresh.ClaimedBy = nil
				fresh.ClaimedUntil = nil
				fresh.UpdatedAt = now
				return tx.ValidationJobs().Update(ctx, fresh)
			}
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
	if err != nil {
		return err
	}
	return nil
}

func (w *ValidationWorker) syncTerminalState(ctx context.Context, job *gitdomain.ValidationJob) error {
	var intent domain.Intent
	var target gitdomain.SubmissionStatus
	invalidated := false
	if job.Status == gitdomain.ValidationStatusSucceeded {
		if checker, ok := w.runner.(IntegrityChecker); ok {
			reachable, err := checker.CheckIntegrity(ctx, job)
			if err != nil {
				return err
			}
			if !reachable {
				job.Status = gitdomain.ValidationStatusFailed
				invalidated = true
			}
		}
	}
	switch job.Status {
	case gitdomain.ValidationStatusSucceeded:
		intent = domain.IntentMarkReviewing
		target = gitdomain.SubmissionStatusValidated
	case gitdomain.ValidationStatusFailed:
		intent = domain.IntentFailValidation
		if invalidated {
			target = gitdomain.SubmissionStatusInvalid
		} else {
			target = gitdomain.SubmissionStatusValidationFailed
		}
	default:
		return fmt.Errorf("validation job %s is not terminal", job.ID)
	}
	if err := w.notifier.Notify(ctx, gitapp.ExecutionStateCommand{
		TenantID: job.TenantID, ExecutionID: job.ExecutionID, Intent: intent,
		Actor: domain.Actor{Type: domain.ActorSystem, ID: "validation-worker"},
	}, time.Now()); err != nil {
		return err
	}
	return w.store.WithTx(ctx, func(tx gitapp.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		submission, err := tx.Submissions().GetByID(ctx, job.TenantID, job.SubmissionID)
		if err != nil {
			return err
		}
		if submission.Status != target {
			switch target {
			case gitdomain.SubmissionStatusValidated:
				err = submission.MarkValidated(now)
			case gitdomain.SubmissionStatusValidationFailed:
				err = submission.MarkValidationFailed(now)
			case gitdomain.SubmissionStatusInvalid:
				err = submission.MarkInvalid(now)
			}
			if err != nil {
				return err
			}
			if err := tx.Submissions().Save(ctx, submission); err != nil {
				return err
			}
		}
		fresh, err := tx.ValidationJobs().GetByID(ctx, job.TenantID, job.ID)
		if err != nil {
			return err
		}
		fresh.ExecutionStateSynced = true
		fresh.Status = job.Status
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
