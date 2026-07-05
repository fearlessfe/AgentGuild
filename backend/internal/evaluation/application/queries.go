package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
)

// GetBenchmarkSet returns a benchmark set by ID.
func (s *EvaluationService) GetBenchmarkSet(ctx context.Context, tenantID, id string) (*domain.BenchmarkSet, error) {
	return s.benchmarkSets.GetByID(ctx, tenantID, id)
}

// ListBenchmarkSets returns all benchmark sets for a tenant.
func (s *EvaluationService) ListBenchmarkSets(ctx context.Context, tenantID string) ([]domain.BenchmarkSet, error) {
	return s.benchmarkSets.ListByTenant(ctx, tenantID)
}

// GetEvaluationRun returns an evaluation run by ID.
func (s *EvaluationService) GetEvaluationRun(ctx context.Context, tenantID, id string) (*domain.EvaluationRun, error) {
	return s.runs.GetByID(ctx, tenantID, id)
}

// ListEvaluationRuns returns all evaluation runs for an agent version.
func (s *EvaluationService) ListEvaluationRuns(ctx context.Context, tenantID, agentVersionID string) ([]domain.EvaluationRun, error) {
	return s.runs.ListByAgentVersion(ctx, tenantID, agentVersionID)
}

// GetActiveBenchmarkSet returns the active benchmark set for a tenant.
func (s *EvaluationService) GetActiveBenchmarkSet(ctx context.Context, tenantID string) (*domain.BenchmarkSet, error) {
	return s.benchmarkSets.GetActiveByTenant(ctx, tenantID)
}

// SetActiveBenchmarkSet marks a benchmark set as the active one for a tenant.
func (s *EvaluationService) SetActiveBenchmarkSet(
	ctx context.Context,
	tenantID, benchmarkSetID, actorID string,
	isAdmin bool,
) error {
	principal := identityapp.Principal{
		TenantID: tenantID,
		OwnerID:  actorID,
		IsAdmin:  isAdmin,
	}
	if err := s.policy.RequireTenantAdmin(ctx, principal, tenantID); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(tx Tx) error {
		if err := s.benchmarkSets.SetInactiveAll(ctx, tx, tenantID); err != nil {
			return err
		}
		return s.benchmarkSets.SetActive(ctx, tx, tenantID, benchmarkSetID)
	})
}
