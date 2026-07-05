package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
)

// GetBenchmarkSet returns a benchmark set by ID.
func (s *EvaluationService) GetBenchmarkSet(ctx context.Context, principal identityapp.Principal, tenantID, id string) (*domain.BenchmarkSet, error) {
	if err := s.policy.RequireTenantOwnerOrAdmin(principal, tenantID); err != nil {
		return nil, err
	}
	return s.benchmarkSets.GetByID(ctx, tenantID, id)
}

// ListBenchmarkSets returns all benchmark sets for a tenant.
func (s *EvaluationService) ListBenchmarkSets(ctx context.Context, principal identityapp.Principal, tenantID string) ([]domain.BenchmarkSet, error) {
	if err := s.policy.RequireTenantOwnerOrAdmin(principal, tenantID); err != nil {
		return nil, err
	}
	return s.benchmarkSets.ListByTenant(ctx, tenantID)
}

// GetBenchmarkSetSummary returns a summary of a benchmark set by ID.
func (s *EvaluationService) GetBenchmarkSetSummary(ctx context.Context, principal identityapp.Principal, tenantID, id string) (*BenchmarkSetSummary, error) {
	bs, err := s.GetBenchmarkSet(ctx, principal, tenantID, id)
	if err != nil {
		return nil, err
	}
	summary := toBenchmarkSetSummary(bs)
	return &summary, nil
}

// ListBenchmarkSetSummaries returns summaries of all benchmark sets for a tenant.
func (s *EvaluationService) ListBenchmarkSetSummaries(ctx context.Context, principal identityapp.Principal, tenantID string) ([]BenchmarkSetSummary, error) {
	sets, err := s.ListBenchmarkSets(ctx, principal, tenantID)
	if err != nil {
		return nil, err
	}
	summaries := make([]BenchmarkSetSummary, 0, len(sets))
	for i := range sets {
		summaries = append(summaries, toBenchmarkSetSummary(&sets[i]))
	}
	return summaries, nil
}

func toBenchmarkSetSummary(bs *domain.BenchmarkSet) BenchmarkSetSummary {
	return BenchmarkSetSummary{
		ID:            bs.ID(),
		TenantID:      bs.TenantID(),
		VersionNumber: bs.VersionNumber(),
		Name:          bs.Name(),
		Description:   bs.Description(),
		IsActive:      bs.IsActive(),
		CreatedBy:     bs.CreatedBy(),
		CreatedAt:     bs.CreatedAt(),
	}
}

// GetEvaluationRun returns an evaluation run by ID.
func (s *EvaluationService) GetEvaluationRun(ctx context.Context, principal identityapp.Principal, tenantID, id string) (*domain.EvaluationRun, error) {
	run, err := s.runs.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	version, err := s.versions.GetByID(ctx, tenantID, run.AgentVersionID())
	if err != nil {
		return nil, err
	}
	if err := s.policy.RequireOwnerOrAdmin(ctx, principal, tenantID, version.AgentID); err != nil {
		return nil, err
	}
	return run, nil
}

// ListEvaluationRuns returns all evaluation runs for an agent version.
func (s *EvaluationService) ListEvaluationRuns(ctx context.Context, principal identityapp.Principal, tenantID, agentVersionID string) ([]domain.EvaluationRun, error) {
	version, err := s.versions.GetByID(ctx, tenantID, agentVersionID)
	if err != nil {
		return nil, err
	}
	if err := s.policy.RequireOwnerOrAdmin(ctx, principal, tenantID, version.AgentID); err != nil {
		return nil, err
	}
	return s.runs.ListByAgentVersion(ctx, tenantID, agentVersionID)
}

// GetEvaluationRunSummary returns a summary of an evaluation run by ID.
func (s *EvaluationService) GetEvaluationRunSummary(ctx context.Context, principal identityapp.Principal, tenantID, id string) (*EvaluationRunSummary, error) {
	run, err := s.GetEvaluationRun(ctx, principal, tenantID, id)
	if err != nil {
		return nil, err
	}
	summary := toEvaluationRunSummary(run)
	return &summary, nil
}

// ListEvaluationRunSummaries returns summaries of all evaluation runs for an agent version.
func (s *EvaluationService) ListEvaluationRunSummaries(ctx context.Context, principal identityapp.Principal, tenantID, agentVersionID string) ([]EvaluationRunSummary, error) {
	runs, err := s.ListEvaluationRuns(ctx, principal, tenantID, agentVersionID)
	if err != nil {
		return nil, err
	}
	summaries := make([]EvaluationRunSummary, 0, len(runs))
	for i := range runs {
		summaries = append(summaries, toEvaluationRunSummary(&runs[i]))
	}
	return summaries, nil
}

func toEvaluationRunSummary(run *domain.EvaluationRun) EvaluationRunSummary {
	return EvaluationRunSummary{
		ID:                 run.ID(),
		TenantID:           run.TenantID(),
		AgentVersionID:     run.AgentVersionID(),
		BenchmarkSetID:     run.BenchmarkSetID(),
		Status:             string(run.Status()),
		EnvironmentDigest:  run.EnvironmentDigest(),
		ScoringRuleVersion: run.ScoringRuleVersion(),
		StartedAt:          run.StartedAt(),
		CompletedAt:        run.CompletedAt(),
	}
}

// GetActiveBenchmarkSet returns the active benchmark set for a tenant.
func (s *EvaluationService) GetActiveBenchmarkSet(ctx context.Context, principal identityapp.Principal, tenantID string) (*domain.BenchmarkSet, error) {
	if err := s.policy.RequireTenantOwnerOrAdmin(principal, tenantID); err != nil {
		return nil, err
	}
	return s.benchmarkSets.GetActiveByTenant(ctx, tenantID)
}

// SetActiveBenchmarkSet marks a benchmark set as the active one for a tenant.
func (s *EvaluationService) SetActiveBenchmarkSet(
	ctx context.Context,
	principal identityapp.Principal,
	tenantID, benchmarkSetID string,
) error {
	if err := s.policy.RequireTenantOwnerOrAdmin(principal, tenantID); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(tx Tx) error {
		if err := s.benchmarkSets.SetInactiveAll(ctx, tx, tenantID); err != nil {
			return err
		}
		return s.benchmarkSets.SetActive(ctx, tx, tenantID, benchmarkSetID)
	})
}
