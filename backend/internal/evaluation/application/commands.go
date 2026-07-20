package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
)

// NewEvaluationService creates the evaluation service.
func NewEvaluationService(
	store Store,
	benchmarkSets BenchmarkSetRepository,
	runs EvaluationRunRepository,
	versions VersionLifecyclePort,
	executor BenchmarkExecutor,
	policy *Policy,
	options EvaluationOptions,
) (*EvaluationService, error) {
	if store == nil {
		return nil, invalidArg("store")
	}
	if benchmarkSets == nil {
		return nil, invalidArg("benchmark_sets")
	}
	if runs == nil {
		return nil, invalidArg("runs")
	}
	if versions == nil {
		return nil, invalidArg("versions")
	}
	if executor == nil {
		return nil, invalidArg("executor")
	}
	if policy == nil {
		return nil, invalidArg("policy")
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	if (options.TaskPublisher == nil) != (options.RunTasks == nil) {
		return nil, invalidArg("task_publisher")
	}
	if options.TaskPublisher != nil && options.TaskDeadline <= 0 {
		options.TaskDeadline = 2 * time.Hour
	}
	return &EvaluationService{
		store:         store,
		benchmarkSets: benchmarkSets,
		runs:          runs,
		versions:      versions,
		executor:      executor,
		policy:        policy,
		newID:         options.NewID,
		taskPublisher: options.TaskPublisher,
		runTasks:      options.RunTasks,
		taskDeadline:  options.TaskDeadline,
	}, nil
}

// CreateBenchmarkSet creates a new versioned benchmark set. If is_active is
// true it atomically deactivates any other active benchmark set for the tenant.
func (s *EvaluationService) CreateBenchmarkSet(
	ctx context.Context,
	cmd CreateBenchmarkSet,
) (*CreateBenchmarkSetResponse, error) {
	principal := identityapp.Principal{
		TenantID: cmd.TenantID,
		OwnerID:  cmd.CreatedBy,
		IsAdmin:  cmd.IsAdmin,
	}
	if err := s.policy.RequireTenantOwnerOrAdmin(principal, cmd.TenantID); err != nil {
		return nil, err
	}

	var bs *domain.BenchmarkSet
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		versionNumber, err := s.benchmarkSets.NextVersionNumber(ctx, tx, cmd.TenantID)
		if err != nil {
			return err
		}
		bs, err = domain.NewBenchmarkSetWithTasks(
			s.newID(), cmd.TenantID, cmd.CreatedBy, versionNumber,
			cmd.Name, cmd.Description, cmd.Tasks, now,
		)
		if err != nil {
			return err
		}
		if cmd.IsActive {
			if err := s.benchmarkSets.SetInactiveAll(ctx, tx, cmd.TenantID); err != nil {
				return err
			}
			bs.SetActive()
		}
		return s.benchmarkSets.Create(ctx, tx, bs)
	})
	if err != nil {
		return nil, err
	}
	return &CreateBenchmarkSetResponse{BenchmarkSet: bs}, nil
}

// StartEvaluationRun validates the version and benchmark set belong to the
// tenant, creates a running evaluation run, and transitions the version to
// evaluating.
func (s *EvaluationService) StartEvaluationRun(
	ctx context.Context,
	cmd StartEvaluationRun,
) (*StartEvaluationRunResponse, error) {
	principal := identityapp.Principal{
		TenantID: cmd.TenantID,
		OwnerID:  cmd.ActorID,
		IsAdmin:  cmd.IsAdmin,
	}
	if err := s.policy.RequireOwnerOrAdmin(ctx, principal, cmd.TenantID, cmd.AgentID); err != nil {
		return nil, err
	}

	var run *domain.EvaluationRun
	var benchmarkSet *domain.BenchmarkSet
	var startedAt time.Time
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}

		version, err := s.versions.GetByID(ctx, cmd.TenantID, cmd.VersionID)
		if err != nil {
			return err
		}
		if version.AgentID != cmd.AgentID {
			return domain.ErrNotFound
		}
		if version.Status != "draft" {
			return domain.ErrStateConflict
		}

		benchmarkSet, err = s.benchmarkSets.GetByID(ctx, cmd.TenantID, cmd.BenchmarkSetID)
		if err != nil {
			return err
		}

		ruleVersion := cmd.ScoringRuleVersion
		if ruleVersion == "" {
			ruleVersion = domain.ScoringRuleVersionV1
		}
		if !domain.IsKnownScoringRuleVersion(ruleVersion) {
			return invalidArg("scoring_rule_version")
		}

		run, err = domain.NewEvaluationRun(
			s.newID(), cmd.TenantID, cmd.VersionID, benchmarkSet.ID(),
			cmd.EnvironmentDigest, ruleVersion, now,
		)
		if err != nil {
			return err
		}
		if err := s.runs.Create(ctx, tx, run); err != nil {
			return err
		}

		// Transition version to evaluating through the agentversion port.
		if err := s.versions.MarkEvaluating(ctx, tx, cmd.TenantID, cmd.AgentID, cmd.VersionID); err != nil {
			return err
		}

		if s.taskPublisher != nil {
			// Platform executor: insert the run-task plan rows and commit. The
			// real tasks are published after commit (see publishEvaluationTasks)
			// and the run stays running until the harvest worker (phase 2)
			// resolves every run-task row.
			startedAt = now
			plan := make([]domain.EvaluationRunTask, 0, len(benchmarkSet.Tasks()))
			for _, task := range benchmarkSet.Tasks() {
				plan = append(plan, domain.EvaluationRunTask{
					TenantID: cmd.TenantID,
					RunID:    run.ID(),
					TaskRef:  task.TaskRef,
					Ordering: task.Ordering,
					Details:  map[string]any{},
				})
			}
			return s.runTasks.Insert(ctx, tx, plan)
		}

		// Synchronous execution for the initial implementation.
		taskResults, execErr := s.executor.Execute(ctx, benchmarkSet, cmd.EnvironmentDigest)
		if execErr != nil {
			return execErr
		}
		thresholdResults, summary := domain.ApplyScoringRule(taskResults, ruleVersion)
		summary.Executor = s.executor.ExecutorID()

		if err := run.CompleteAt(thresholdResults, summary, now); err != nil {
			return err
		}
		if err := s.runs.Complete(ctx, tx, run); err != nil {
			return err
		}
		for _, tr := range taskResults {
			if err := s.runs.CreateResult(ctx, tx, &domain.EvaluationRunResult{
				EvaluationRunID: run.ID(),
				TenantID:        cmd.TenantID,
				TaskRef:         tr.TaskRef,
				Score:           tr.Score,
				Passed:          tr.Passed,
				Details:         tr.Details,
			}); err != nil {
				return err
			}
		}

		if run.IsPassed() {
			return s.versions.MarkEligible(ctx, tx, cmd.TenantID, cmd.AgentID, cmd.VersionID)
		}
		return s.versions.MarkRejected(ctx, tx, cmd.TenantID, cmd.AgentID, cmd.VersionID, "hard threshold failure")
	})
	if err != nil {
		return nil, err
	}
	if s.taskPublisher != nil {
		s.publishEvaluationTasks(ctx, run, benchmarkSet, startedAt)
	}
	return &StartEvaluationRunResponse{EvaluationRun: run}, nil
}

// publishEvaluationTasks publishes one real platform task per benchmark task
// and backfills the run-task mapping with the returned task identifier.
//
// Ordering: this runs strictly after the evaluation transaction committed the
// run and its plan rows. The publisher adapter delegates to the core task
// service, which opens its own transaction — calling it inside the evaluation
// transaction would nest two independent transactions and could deadlock or
// publish tasks for a run that later rolls back. Publication failures are
// recorded in the row's details and do not fail the run: the phase-2 harvest
// worker treats unpublished rows as failed results.
func (s *EvaluationService) publishEvaluationTasks(
	ctx context.Context,
	run *domain.EvaluationRun,
	benchmarkSet *domain.BenchmarkSet,
	startedAt time.Time,
) {
	deadline := startedAt.Add(s.taskDeadline)
	for _, task := range benchmarkSet.Tasks() {
		constraints := append([]string(nil), task.Constraints...)
		constraints = append(constraints, "eval_run:"+run.ID())
		taskID, err := s.taskPublisher.PublishEvaluationTask(ctx, PublishEvaluationTaskCommand{
			TenantID:     run.TenantID(),
			RequestID:    "eval:" + run.ID() + ":" + task.TaskRef,
			Type:         "evaluation",
			Title:        task.Title,
			Problem:      task.Problem,
			Constraints:  constraints,
			Requirements: task.Requirements,
			Deadline:     deadline,
		})
		if err != nil {
			if recordErr := s.store.WithTx(ctx, func(tx Tx) error {
				return s.runTasks.RecordPublishFailure(ctx, tx, run.TenantID(), run.ID(), task.TaskRef, err.Error())
			}); recordErr != nil {
				slog.Error("record evaluation task publish failure", "run_id", run.ID(), "task_ref", task.TaskRef, "error", recordErr)
			}
			continue
		}
		if err := s.store.WithTx(ctx, func(tx Tx) error {
			return s.runTasks.SetTaskID(ctx, tx, run.TenantID(), run.ID(), task.TaskRef, taskID)
		}); err != nil {
			slog.Error("backfill evaluation run task id", "run_id", run.ID(), "task_ref", task.TaskRef, "error", err)
		}
	}
}

// CompleteEvaluationRun allows external callers to complete a running
// evaluation run directly. It updates the run status and transitions the
// associated version based on the result.
func (s *EvaluationService) CompleteEvaluationRun(
	ctx context.Context,
	cmd CompleteEvaluationRun,
) error {
	principal := identityapp.Principal{
		TenantID: cmd.TenantID,
		OwnerID:  cmd.ActorID,
		IsAdmin:  cmd.IsAdmin,
	}
	if err := s.policy.RequireOwnerOrAdmin(ctx, principal, cmd.TenantID, cmd.AgentID); err != nil {
		return err
	}

	return s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		run, err := s.runs.GetByID(ctx, cmd.TenantID, cmd.EvaluationRunID)
		if err != nil {
			return err
		}
		if run.AgentVersionID() != "" && run.AgentVersionID() != cmd.VersionID {
			return domain.ErrStateConflict
		}
		if err := run.CompleteAt(cmd.ThresholdResults, cmd.Summary, now); err != nil {
			return err
		}
		if err := s.runs.Complete(ctx, tx, run); err != nil {
			return err
		}
		if run.IsPassed() {
			return s.versions.MarkEligible(ctx, tx, cmd.TenantID, cmd.AgentID, cmd.VersionID)
		}
		return s.versions.MarkRejected(ctx, tx, cmd.TenantID, cmd.AgentID, cmd.VersionID, "hard threshold failure")
	})
}

func invalidArg(field string) error {
	return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
