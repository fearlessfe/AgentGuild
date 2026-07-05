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
