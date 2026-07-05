package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"

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
	return &EvaluationService{
		store:         store,
		benchmarkSets: benchmarkSets,
		runs:          runs,
		versions:      versions,
		executor:      executor,
		policy:        policy,
		newID:         options.NewID,
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

		benchmarkSet, err := s.benchmarkSets.GetByID(ctx, cmd.TenantID, cmd.BenchmarkSetID)
		if err != nil {
			return err
		}

		ruleVersion := cmd.ScoringRuleVersion
		if ruleVersion == "" {
			ruleVersion = domain.ScoringRuleVersionV1
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

		// Synchronous execution for the initial implementation.
		taskResults, execErr := s.executor.Execute(ctx, benchmarkSet, cmd.EnvironmentDigest)
		if execErr != nil {
			return execErr
		}
		thresholdResults, summary := domain.ApplyScoringRule(taskResults, ruleVersion)

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
	return &StartEvaluationRunResponse{EvaluationRun: run}, nil
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
