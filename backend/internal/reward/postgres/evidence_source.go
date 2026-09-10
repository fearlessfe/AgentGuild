package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	contributiondomain "agentguild.dev/agentguild/backend/internal/contribution/domain"
	"agentguild.dev/agentguild/backend/internal/reward/application"
	"agentguild.dev/agentguild/backend/internal/reward/domain"
	canonicaljson "github.com/gibson042/canonicaljson-go"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EvidenceSource 把公共任务规格、criterion 账本与贡献归属拼成生成决策所需
// 的证据摘要。
//
// 它只读取可公开的摘要与判定结果：Issue 正文、diff 与评审内容都不会进入
// RewardDecision（doc §10）。
type EvidenceSource struct {
	pool *pgxpool.Pool
}

func NewEvidenceSource(pool *pgxpool.Pool) *EvidenceSource {
	return &EvidenceSource{pool: pool}
}

func (s *EvidenceSource) DecisionEvidence(ctx context.Context, tenantID, executionID string) (application.DecisionEvidence, error) {
	specHash, spec, err := s.taskSpec(ctx, tenantID, executionID)
	if err != nil {
		return application.DecisionEvidence{}, err
	}
	latest, err := s.latestResults(ctx, tenantID, executionID)
	if err != nil {
		return application.DecisionEvidence{}, err
	}
	contributionHash, err := s.contributionHash(ctx, tenantID, executionID)
	if err != nil {
		return application.DecisionEvidence{}, err
	}
	return application.DecisionEvidence{
		TaskSpecHash: specHash, ContributionHash: contributionHash,
		Spec: spec, Latest: latest,
	}, nil
}

// acceptanceCriterion 只取参与奖励判定的两个字段。刻意不复用 publictask 的
// 领域结构：这里需要的是"哪些标准、是否必需"，不是完整规格。
type acceptanceCriterion struct {
	ID       string `json:"id"`
	Critical bool   `json:"critical"`
}

func (s *EvidenceSource) taskSpec(ctx context.Context, tenantID, executionID string) (string, []contributiondomain.SpecCriterion, error) {
	var specHash string
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT projection.spec_hash, projection.acceptance_criteria
		FROM executions AS execution
		JOIN public_task_projections AS projection
		  ON projection.resource_tenant_id = execution.tenant_id
		 AND projection.task_id = execution.task_id
		WHERE execution.tenant_id=$1 AND execution.id=$2`, tenantID, executionID,
	).Scan(&specHash, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, domain.ErrNotFound
	}
	if err != nil {
		return "", nil, err
	}
	if len(specHash) != 64 {
		// 没有规格摘要就无法生成可验证的决策，这里 fail closed。
		return "", nil, domain.ErrStateConflict
	}
	var criteria []acceptanceCriterion
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &criteria); err != nil {
			return "", nil, err
		}
	}
	spec := make([]contributiondomain.SpecCriterion, 0, len(criteria))
	for _, criterion := range criteria {
		spec = append(spec, contributiondomain.SpecCriterion{
			ID: criterion.ID, Critical: criterion.Critical,
		})
	}
	return specHash, spec, nil
}

func (s *EvidenceSource) latestResults(ctx context.Context, tenantID, executionID string) ([]contributiondomain.CriterionResult, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT criterion_id, critical, passed, observed_at
		FROM execution_criterion_latest
		WHERE resource_tenant_id=$1 AND execution_id=$2
		ORDER BY criterion_id`, tenantID, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]contributiondomain.CriterionResult, 0)
	for rows.Next() {
		result := contributiondomain.CriterionResult{
			ResourceTenantID: tenantID, ExecutionID: executionID,
		}
		if err := rows.Scan(&result.CriterionID, &result.Critical,
			&result.Passed, &result.ObservedAt); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

// contributionDocument 是 contribution_hash 的字段白名单：只含可公开的
// 归属标识，不含代码、评审或 Issue 内容。
type contributionDocument struct {
	ExecutionID         string `json:"execution_id"`
	Provider            string `json:"provider"`
	CanonicalRepository string `json:"canonical_repository"`
	PullRequestNumber   int64  `json:"pull_request_number"`
	CommitSHA           string `json:"commit_sha"`
}

// contributionHash 在贡献尚未归属时退化为只含 execution_id 的摘要。
// 它依然是稳定且可复算的，只是承载的证据更少。
func (s *EvidenceSource) contributionHash(ctx context.Context, tenantID, executionID string) (string, error) {
	document := contributionDocument{ExecutionID: executionID}
	err := s.pool.QueryRow(ctx, `
		SELECT provider, canonical_repository, pull_request_number, commit_sha
		FROM contributions
		WHERE resource_tenant_id=$1 AND execution_id=$2
		ORDER BY created_at DESC, id
		LIMIT 1`, tenantID, executionID,
	).Scan(&document.Provider, &document.CanonicalRepository,
		&document.PullRequestNumber, &document.CommitSHA)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	encoded, err := canonicaljson.Marshal(document)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

var _ application.EvidenceSource = (*EvidenceSource)(nil)
