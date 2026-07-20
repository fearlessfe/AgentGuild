package application

import (
	"context"
	"errors"
	"log/slog"

	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
)

// AutoEvaluator implements the agentversion module's DraftCreatedHook port.
// When EVALUATION_AUTO is enabled, every newly created draft version
// automatically starts an evaluation run against the tenant's active
// benchmark set, acting as the draft's creator. The hook is best-effort:
// skips (no active benchmark set, duplicate or conflicting start) are
// debug-logged and never fail draft creation.
type AutoEvaluator struct {
	enabled       bool
	runs          *EvaluationService
	benchmarkSets BenchmarkSetRepository
	environments  VersionEnvironmentProvider
	logger        *slog.Logger
}

// NewAutoEvaluator creates the automatic evaluation hook behind EVALUATION_AUTO.
func NewAutoEvaluator(
	enabled bool,
	runs *EvaluationService,
	benchmarkSets BenchmarkSetRepository,
	environments VersionEnvironmentProvider,
	logger *slog.Logger,
) *AutoEvaluator {
	if logger == nil {
		logger = slog.Default()
	}
	return &AutoEvaluator{
		enabled:       enabled,
		runs:          runs,
		benchmarkSets: benchmarkSets,
		environments:  environments,
		logger:        logger,
	}
}

// OnDraftCreated starts an evaluation run for the new draft version when
// automatic evaluation is enabled and the tenant has an active benchmark set.
func (a *AutoEvaluator) OnDraftCreated(ctx context.Context, tenantID, agentID, versionID, actorID string) error {
	if !a.enabled {
		return nil
	}
	benchmarkSet, err := a.benchmarkSets.GetActiveByTenant(ctx, tenantID)
	if errors.Is(err, domain.ErrNotFound) {
		a.logger.Debug("auto evaluation skipped: no active benchmark set",
			"tenant_id", tenantID, "agent_id", agentID, "version_id", versionID)
		return nil
	}
	if err != nil {
		return err
	}
	environmentDigest, err := a.environments.GetVersionEnvironmentDigest(ctx, tenantID, versionID)
	if err != nil {
		return err
	}
	_, err = a.runs.StartEvaluationRun(ctx, StartEvaluationRun{
		TenantID:          tenantID,
		AgentID:           agentID,
		VersionID:         versionID,
		BenchmarkSetID:    benchmarkSet.ID(),
		EnvironmentDigest: environmentDigest,
		ActorID:           actorID,
	})
	if err != nil {
		switch domain.CodeOf(err) {
		case "state_conflict", "not_found", "invalid_argument":
			// Duplicate hook delivery (the draft is already evaluating), a
			// creator without start permission, or a version without a usable
			// environment digest: skip quietly — the draft->evaluating state
			// machine already makes repeated starts idempotent.
			a.logger.Debug("auto evaluation skipped",
				"tenant_id", tenantID, "agent_id", agentID, "version_id", versionID,
				"benchmark_set_id", benchmarkSet.ID(), "error", err)
			return nil
		}
		return err
	}
	a.logger.Info("auto evaluation started",
		"tenant_id", tenantID, "agent_id", agentID, "version_id", versionID,
		"benchmark_set_id", benchmarkSet.ID())
	return nil
}
