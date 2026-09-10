package postgres

import (
	"context"
	"encoding/json"
	"time"

	"agentguild.dev/agentguild/backend/internal/reward/domain"
)

type policyRepository struct{ q queryer }

const policyColumns = `
	resource_tenant_id, id, task_id, policy_hash, currency, settlement_provider,
	gross_amount_minor, funded_amount_minor, criterion_weights_bps,
	maintainer_share_bps, reviewer_pool_share_bps, platform_fee_bps,
	dispute_reserve_bps, quality_multiplier_min_bps, quality_multiplier_max_bps,
	challenge_period_seconds, expires_at, status, created_at,
	funded_at, exhausted_at, cancelled_at`

func (r *policyRepository) Insert(ctx context.Context, policy *domain.Policy) error {
	weights, err := marshalWeights(policy.CriterionWeights)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO reward_policies (`+policyColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`,
		policy.ResourceTenantID, policy.ID, policy.TaskID, policy.PolicyHash,
		string(policy.Currency), policy.SettlementProvider, policy.GrossAmountMinor,
		policy.FundedAmountMinor, weights, policy.MaintainerShareBps,
		policy.ReviewerPoolShareBps, policy.PlatformFeeBps, policy.DisputeReserveBps,
		policy.QualityMultiplierMinBps, policy.QualityMultiplierMaxBps,
		int64(policy.ChallengePeriod/time.Second), policy.ExpiresAt,
		string(policy.Status), policy.CreatedAt,
		policy.FundedAt, policy.ExhaustedAt, policy.CancelledAt,
	)
	return writeError(err)
}

func (r *policyRepository) GetByID(ctx context.Context, tenantID, id string) (*domain.Policy, error) {
	return r.scanOne(ctx, `
		SELECT`+policyColumns+`
		FROM reward_policies
		WHERE resource_tenant_id=$1 AND id=$2`, tenantID, id)
}

func (r *policyRepository) GetActiveByTask(ctx context.Context, tenantID, taskID string) (*domain.Policy, error) {
	return r.scanOne(ctx, `
		SELECT`+policyColumns+`
		FROM reward_policies
		WHERE resource_tenant_id=$1 AND task_id=$2
		  AND status IN ('unfunded','funded')`, tenantID, taskID)
}

// GetFundedByTaskForUpdate 在 Claim 事务里对 policy 行取排他锁。
// 没有这把锁，两个并发 claim 会各自读到同一份余额并双双通过预检查。
func (r *policyRepository) GetFundedByTaskForUpdate(ctx context.Context, tenantID, taskID string) (*domain.Policy, error) {
	return r.scanOne(ctx, `
		SELECT`+policyColumns+`
		FROM reward_policies
		WHERE resource_tenant_id=$1 AND task_id=$2 AND status='funded'
		FOR UPDATE`, tenantID, taskID)
}

func (r *policyRepository) Save(ctx context.Context, policy *domain.Policy) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE reward_policies
		SET funded_amount_minor=$3, status=$4, funded_at=$5,
		    exhausted_at=$6, cancelled_at=$7
		WHERE resource_tenant_id=$1 AND id=$2`,
		policy.ResourceTenantID, policy.ID, policy.FundedAmountMinor,
		string(policy.Status), policy.FundedAt, policy.ExhaustedAt, policy.CancelledAt,
	)
	if err != nil {
		return writeError(err)
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}

// LockedAmount 汇总该 policy 上仍占用资金的锁：locked / releasable / disputed。
// 终态的锁（released / refunded / expired）已经把资金交割完毕。
func (r *policyRepository) LockedAmount(ctx context.Context, tenantID, policyID string) (int64, error) {
	var total int64
	err := r.q.QueryRow(ctx, `
		SELECT COALESCE(SUM(locked_amount_minor), 0)
		FROM reward_locks
		WHERE resource_tenant_id=$1 AND policy_id=$2
		  AND status IN ('locked','releasable','disputed')`, tenantID, policyID,
	).Scan(&total)
	return total, err
}

func (r *policyRepository) scanOne(ctx context.Context, query string, args ...any) (*domain.Policy, error) {
	var policy domain.Policy
	var currency, status string
	var weights []byte
	var challengeSeconds int64
	err := r.q.QueryRow(ctx, query, args...).Scan(
		&policy.ResourceTenantID, &policy.ID, &policy.TaskID, &policy.PolicyHash,
		&currency, &policy.SettlementProvider, &policy.GrossAmountMinor,
		&policy.FundedAmountMinor, &weights, &policy.MaintainerShareBps,
		&policy.ReviewerPoolShareBps, &policy.PlatformFeeBps, &policy.DisputeReserveBps,
		&policy.QualityMultiplierMinBps, &policy.QualityMultiplierMaxBps,
		&challengeSeconds, &policy.ExpiresAt, &status, &policy.CreatedAt,
		&policy.FundedAt, &policy.ExhaustedAt, &policy.CancelledAt,
	)
	if notFound(err) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	policy.Currency = domain.Currency(currency)
	policy.Status = domain.PolicyStatus(status)
	policy.ChallengePeriod = time.Duration(challengeSeconds) * time.Second
	parsed, err := unmarshalWeights(weights)
	if err != nil {
		return nil, err
	}
	policy.CriterionWeights = parsed
	return &policy, nil
}

// marshalWeights 把权重存成 {criterion_id: weight_bps} 对象，让 SQL 侧可以
// 直接按 key 检索，同时与迁移里的 jsonb object CHECK 相符。
func marshalWeights(weights []domain.CriterionWeight) ([]byte, error) {
	object := make(map[string]int, len(weights))
	for _, weight := range weights {
		object[weight.CriterionID] = weight.WeightBps
	}
	return json.Marshal(object)
}

func unmarshalWeights(raw []byte) ([]domain.CriterionWeight, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var object map[string]int
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	weights := make([]domain.CriterionWeight, 0, len(object))
	for criterionID, bps := range object {
		weights = append(weights, domain.CriterionWeight{CriterionID: criterionID, WeightBps: bps})
	}
	// 摘要要求稳定顺序，交给领域层的归一化逻辑保证。
	return sortWeights(weights), nil
}

func sortWeights(weights []domain.CriterionWeight) []domain.CriterionWeight {
	for i := 1; i < len(weights); i++ {
		for j := i; j > 0 && weights[j].CriterionID < weights[j-1].CriterionID; j-- {
			weights[j], weights[j-1] = weights[j-1], weights[j]
		}
	}
	return weights
}
