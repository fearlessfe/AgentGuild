package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/contribution/application"
	"agentguild.dev/agentguild/backend/internal/contribution/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CriterionRepository struct {
	pool *pgxpool.Pool
}

func NewCriterionRepository(pool *pgxpool.Pool) application.CriterionRepository {
	return &CriterionRepository{pool: pool}
}

const criterionColumns = `id, resource_tenant_id, task_id, execution_id, criterion_id,
	critical, verifier_kind, passed, source_kind, source_id, verified_by,
	verifier_version, evidence_uri, evidence_hash, observed_at, recorded_at`

// Record 追加一条 criterion 验证事实。同一 (execution, criterion, source)
// 的重复投递不会插入第二行，返回已存在的事实且 inserted=false；内容与已
// 存事实不一致时返回 state_conflict，避免同一来源改写结论。
func (r *CriterionRepository) Record(ctx context.Context, result *domain.CriterionResult) (*domain.CriterionResult, bool, error) {
	if result == nil {
		return nil, false, domain.ErrInvalidArgument
	}
	inserted, err := scanCriterionResult(r.pool.QueryRow(ctx, `
		INSERT INTO execution_criterion_results (
			resource_tenant_id, task_id, execution_id, criterion_id, critical,
			verifier_kind, passed, source_kind, source_id, verified_by,
			verifier_version, evidence_uri, evidence_hash, observed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT DO NOTHING
		RETURNING `+criterionColumns,
		result.ResourceTenantID, result.TaskID, result.ExecutionID, result.CriterionID,
		result.Critical, result.VerifierKind, result.Passed, result.SourceKind,
		result.SourceID, result.VerifiedBy, result.VerifierVersion,
		result.EvidenceURI, result.EvidenceHash, result.ObservedAt,
	))
	if err == nil {
		return inserted, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, writeError(err)
	}

	existing, err := scanCriterionResult(r.pool.QueryRow(ctx, `
		SELECT `+criterionColumns+`
		FROM execution_criterion_results
		WHERE resource_tenant_id=$1 AND execution_id=$2 AND criterion_id=$3
		  AND source_kind=$4 AND source_id=$5`,
		result.ResourceTenantID, result.ExecutionID, result.CriterionID,
		result.SourceKind, result.SourceID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, domain.ErrStateConflict
		}
		return nil, false, err
	}
	if existing.Passed != result.Passed ||
		existing.Critical != result.Critical ||
		existing.VerifierKind != result.VerifierKind ||
		!existing.ObservedAt.Equal(result.ObservedAt) {
		return nil, false, domain.ErrStateConflict
	}
	return existing, false, nil
}

// ListLatest 返回某个 Execution 上每条 criterion 的最新态。
func (r *CriterionRepository) ListLatest(ctx context.Context, tenantID, executionID string) ([]domain.CriterionResult, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+criterionColumns+`
		FROM execution_criterion_latest
		WHERE resource_tenant_id=$1 AND execution_id=$2
		ORDER BY criterion_id`, tenantID, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectCriterionResults(rows)
}

// ListHistory 返回某个 Execution 的完整验证时间线，供审计与重算使用。
func (r *CriterionRepository) ListHistory(ctx context.Context, tenantID, executionID string) ([]domain.CriterionResult, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+criterionColumns+`
		FROM execution_criterion_results
		WHERE resource_tenant_id=$1 AND execution_id=$2
		ORDER BY observed_at, id`, tenantID, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectCriterionResults(rows)
}

func collectCriterionResults(rows pgx.Rows) ([]domain.CriterionResult, error) {
	results := make([]domain.CriterionResult, 0)
	for rows.Next() {
		result, err := scanCriterionResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *result)
	}
	return results, rows.Err()
}

func scanCriterionResult(row scanner) (*domain.CriterionResult, error) {
	var result domain.CriterionResult
	err := row.Scan(
		&result.ID, &result.ResourceTenantID, &result.TaskID, &result.ExecutionID,
		&result.CriterionID, &result.Critical, &result.VerifierKind, &result.Passed,
		&result.SourceKind, &result.SourceID, &result.VerifiedBy,
		&result.VerifierVersion, &result.EvidenceURI, &result.EvidenceHash,
		&result.ObservedAt, &result.RecordedAt,
	)
	return &result, err
}

var _ application.CriterionRepository = (*CriterionRepository)(nil)
