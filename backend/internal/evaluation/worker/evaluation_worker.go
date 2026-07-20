// Package worker implements the asynchronous evaluation harvest worker: it
// resolves platform-executor run tasks from real platform task outcomes and
// completes evaluation runs once every benchmark task is resolved (or the run
// times out).
package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"agentguild.dev/agentguild/backend/internal/evaluation/application"
	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
)

// Resolution reasons recorded in a run-task row's details under "resolution".
const (
	resolutionValidated        = "validated"
	resolutionValidationFailed = "validation_failed"
	resolutionPublishFailed    = "publish_failed"
	resolutionTaskCancelled    = "task_cancelled"
	resolutionTaskExpired      = "task_expired"
	resolutionVersionMismatch  = "version_mismatch"
	resolutionRunTimeout       = "run_timeout"
)

// Worker harvests evaluation run-task outcomes and completes evaluation runs.
type Worker struct {
	store         application.Store
	runs          application.EvaluationRunRepository
	runTasks      application.RunTaskRepository
	benchmarkSets application.BenchmarkSetRepository
	versions      application.VersionLifecyclePort
	observer      application.EvaluationTaskObserver
	interval      time.Duration
	runTimeout    time.Duration
	batchSize     int
	logger        *slog.Logger
}

// NewWorker creates the evaluation harvest worker.
func NewWorker(
	store application.Store,
	runs application.EvaluationRunRepository,
	runTasks application.RunTaskRepository,
	benchmarkSets application.BenchmarkSetRepository,
	versions application.VersionLifecyclePort,
	observer application.EvaluationTaskObserver,
	interval, runTimeout time.Duration,
	batchSize int,
	logger *slog.Logger,
) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if runTimeout <= 0 {
		runTimeout = 24 * time.Hour
	}
	if batchSize <= 0 {
		batchSize = 10
	}
	return &Worker{
		store:         store,
		runs:          runs,
		runTasks:      runTasks,
		benchmarkSets: benchmarkSets,
		versions:      versions,
		observer:      observer,
		interval:      interval,
		runTimeout:    runTimeout,
		batchSize:     batchSize,
		logger:        logger,
	}
}

// Run starts the worker loop. It stops when ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				w.logger.Error("evaluation worker batch failed", "error", err)
			}
		}
	}
}

// RunOnce processes one batch of running evaluation runs. Each run is
// harvested in its own transaction so a failing run neither blocks nor rolls
// back the others; per-run failures are logged and retried on the next tick.
func (w *Worker) RunOnce(ctx context.Context) error {
	runs, err := w.runs.ListRunning(ctx, w.batchSize)
	if err != nil {
		return err
	}
	for i := range runs {
		if err := w.harvestRun(ctx, runs[i].TenantID(), runs[i].ID()); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			w.logger.Error("evaluation run harvest failed",
				"tenant_id", runs[i].TenantID(), "run_id", runs[i].ID(), "error", err)
		}
	}
	return nil
}

// harvestRun resolves the run's outstanding run-task rows and, once every row
// is resolved (or the run timed out), completes the run and drives the version
// lifecycle — all inside one transaction.
func (w *Worker) harvestRun(ctx context.Context, tenantID, runID string) error {
	return w.store.WithTx(ctx, func(tx application.Tx) error {
		run, err := w.runs.LockRunningForUpdate(ctx, tx, tenantID, runID)
		if errors.Is(err, domain.ErrNotFound) {
			// Completed by a concurrent tick (or never existed): nothing to do.
			return nil
		}
		if err != nil {
			return err
		}
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}

		unresolved, err := w.runTasks.ListUnresolved(ctx, tx, tenantID, runID)
		if err != nil {
			return err
		}
		for _, runTask := range unresolved {
			resolution, resolved, err := w.harvestOutcome(ctx, run, runTask)
			if err != nil {
				return err
			}
			if !resolved {
				continue
			}
			resolution.ResolvedAt = now
			if err := w.runTasks.Resolve(ctx, tx, tenantID, runID, runTask.TaskRef, resolution); err != nil {
				return err
			}
		}

		remaining, err := w.runTasks.ListUnresolved(ctx, tx, tenantID, runID)
		if err != nil {
			return err
		}
		if len(remaining) > 0 && !run.StartedAt().Add(w.runTimeout).After(now) {
			// The run outlived its timeout: every outstanding task is scored as
			// failed so the run can complete.
			for _, runTask := range remaining {
				resolution := application.RunTaskResolution{
					Passed:     false,
					Details:    map[string]any{"resolution": resolutionRunTimeout},
					ResolvedAt: now,
				}
				if err := w.runTasks.Resolve(ctx, tx, tenantID, runID, runTask.TaskRef, resolution); err != nil {
					return err
				}
			}
			remaining = nil
		}
		if len(remaining) > 0 {
			// Outcomes still in flight; the run stays running.
			return nil
		}

		all, err := w.runTasks.ListByRunTx(ctx, tx, tenantID, runID)
		if err != nil {
			return err
		}
		return w.completeRun(ctx, tx, run, all, now)
	})
}

// harvestOutcome decides the resolution for one run-task row from the observed
// platform state. resolved=false means the outcome is not final yet and the
// row stays unresolved for a later tick (or the run timeout).
func (w *Worker) harvestOutcome(
	ctx context.Context,
	run *domain.EvaluationRun,
	runTask domain.EvaluationRunTask,
) (application.RunTaskResolution, bool, error) {
	fail := func(reason string, details map[string]any) application.RunTaskResolution {
		if details == nil {
			details = map[string]any{}
		}
		details["resolution"] = reason
		return application.RunTaskResolution{Passed: false, Details: details}
	}

	if _, failed := runTask.Details["publish_error"]; failed {
		// The platform task was never published successfully.
		return fail(resolutionPublishFailed, nil), true, nil
	}
	if runTask.TaskID == "" {
		// Publication is still in flight (or its backfill was lost); wait for
		// the task id backfill or the publish_error marker.
		return application.RunTaskResolution{}, false, nil
	}

	snapshot, err := w.observer.Observe(ctx, runTask.TenantID, runTask.TaskID)
	if errors.Is(err, domain.ErrNotFound) {
		// Platform tasks are never deleted, so this indicates data corruption;
		// leave the row unresolved and let the run timeout close it.
		w.logger.Warn("evaluation platform task missing",
			"tenant_id", runTask.TenantID, "run_id", runTask.RunID,
			"task_ref", runTask.TaskRef, "task_id", runTask.TaskID)
		return application.RunTaskResolution{}, false, nil
	}
	if err != nil {
		return application.RunTaskResolution{}, false, err
	}

	switch snapshot.TaskStatus {
	case "cancelled":
		return fail(resolutionTaskCancelled, nil), true, nil
	case "expired":
		return fail(resolutionTaskExpired, nil), true, nil
	}

	if snapshot.ExecutionID == "" {
		// Not claimed yet.
		return application.RunTaskResolution{}, false, nil
	}

	// Version guard: only the execution claimed by the evaluated version
	// counts. A task claimed by any other version is contaminated evidence and
	// is scored as failed.
	if snapshot.ExecutionVersionID != run.AgentVersionID() {
		return fail(resolutionVersionMismatch, map[string]any{
			"expected_agent_version_id": run.AgentVersionID(),
			"actual_agent_version_id":   snapshot.ExecutionVersionID,
			"execution_id":              snapshot.ExecutionID,
		}), true, nil
	}

	switch snapshot.SubmissionStatus {
	case "validated":
		// passed = the objective automated validation signal; no human review
		// is required for evaluation evidence.
		details := map[string]any{
			"resolution":       resolutionValidated,
			"submission_state": snapshot.SubmissionStatus,
			"execution_id":     snapshot.ExecutionID,
		}
		resolution := application.RunTaskResolution{Passed: true, Details: details}
		// Latency = execution start -> submission, taken from the execution
		// timestamps (executions.started_at / executions.submitted_at).
		if snapshot.ExecutionStartedAt != nil && snapshot.ExecutionSubmittedAt != nil {
			latencyMs := float64(snapshot.ExecutionSubmittedAt.Sub(*snapshot.ExecutionStartedAt)) / float64(time.Millisecond)
			resolution.LatencyMs = &latencyMs
		} else {
			details["latency_unavailable"] = true
		}
		// Cost = latest observed execution cost; missing observations count as
		// 0 and are annotated.
		var costCents int64
		if snapshot.ObservedCostCents != nil {
			costCents = *snapshot.ObservedCostCents
		} else {
			details["cost_observed"] = false
		}
		resolution.CostCents = &costCents
		return resolution, true, nil
	case "validation_failed", "invalid":
		// Terminal automated-validation failure ('invalid' covers submissions
		// rejected before validation ran).
		return fail(resolutionValidationFailed, map[string]any{
			"submission_state": snapshot.SubmissionStatus,
			"execution_id":     snapshot.ExecutionID,
		}), true, nil
	case "pending_verification":
		return application.RunTaskResolution{}, false, nil
	}

	// No submission yet: only a terminally failed execution resolves the row.
	switch snapshot.ExecutionStatus {
	case "expired", "cancelled", "rejected":
		return fail("execution_"+snapshot.ExecutionStatus, map[string]any{
			"execution_id": snapshot.ExecutionID,
		}), true, nil
	}
	return application.RunTaskResolution{}, false, nil
}

// completeRun finishes the run inside the worker transaction: it applies the
// scoring rule to the resolved run-task rows, persists the run completion and
// per-task results, and transitions the agent version to eligible or rejected.
func (w *Worker) completeRun(
	ctx context.Context,
	tx application.Tx,
	run *domain.EvaluationRun,
	all []domain.EvaluationRunTask,
	now time.Time,
) error {
	// is_security comes from the benchmark task definitions, not from the
	// run-task rows, so read the frozen benchmark set.
	benchmarkSet, err := w.benchmarkSets.GetByID(ctx, run.TenantID(), run.BenchmarkSetID())
	if err != nil {
		return err
	}
	securityByRef := make(map[string]bool, len(benchmarkSet.Tasks()))
	for _, task := range benchmarkSet.Tasks() {
		securityByRef[task.TaskRef] = task.IsSecurity
	}

	taskResults := make([]domain.TaskResult, 0, len(all))
	for _, runTask := range all {
		result := domain.TaskResult{
			TaskRef:    runTask.TaskRef,
			IsSecurity: securityByRef[runTask.TaskRef],
			Details:    runTask.Details,
		}
		if runTask.Passed != nil && *runTask.Passed {
			result.Passed = true
			result.Score = 1.0
		}
		if runTask.LatencyMs != nil {
			result.LatencyMs = *runTask.LatencyMs
		}
		if runTask.CostCents != nil {
			result.CostCents = *runTask.CostCents
		}
		taskResults = append(taskResults, result)
	}

	thresholdResults, summary := domain.ApplyScoringRule(taskResults, run.ScoringRuleVersion())
	summary.Executor = application.PlatformBenchmarkExecutorID

	if err := run.CompleteAt(thresholdResults, summary, now); err != nil {
		return err
	}
	// The repository update is guarded on status='running', so a concurrent
	// completion loses the race and conflicts instead of completing twice.
	if err := w.runs.Complete(ctx, tx, run); err != nil {
		return err
	}
	for _, taskResult := range taskResults {
		if err := w.runs.CreateResult(ctx, tx, &domain.EvaluationRunResult{
			EvaluationRunID: run.ID(),
			TenantID:        run.TenantID(),
			TaskRef:         taskResult.TaskRef,
			Score:           taskResult.Score,
			Passed:          taskResult.Passed,
			Details:         taskResult.Details,
		}); err != nil {
			return err
		}
	}

	version, err := w.versions.GetByID(ctx, run.TenantID(), run.AgentVersionID())
	if err != nil {
		return err
	}
	if run.IsPassed() {
		return w.versions.MarkEligible(ctx, tx, run.TenantID(), version.AgentID, version.ID)
	}
	return w.versions.MarkRejected(ctx, tx, run.TenantID(), version.AgentID, version.ID, "hard threshold failure")
}
