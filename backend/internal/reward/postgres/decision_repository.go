package postgres

import (
	"context"
	"encoding/json"
	"time"

	"agentguild.dev/agentguild/backend/internal/reward/domain"
	"github.com/jackc/pgx/v5"
)

type decisionRepository struct{ q queryer }

const decisionColumns = `
	decision_hash, resource_tenant_id, lock_id, policy_hash, task_spec_hash,
	contribution_hash, algorithm_version, currency, gross_amount_minor,
	platform_fee_minor, dispute_reserve_minor, net_amount_minor,
	agent_amount_minor, maintainer_amount_minor, reviewer_pool_amount_minor,
	unallocated_amount_minor, quality_multiplier_bps, criterion_results,
	required_criteria_passed, recipient_ref, challenge_deadline, signature,
	signature_algorithm, decided_at`

func (r *decisionRepository) Insert(ctx context.Context, decision *domain.Decision) error {
	outcomes := decision.CriterionResults
	if outcomes == nil {
		outcomes = []domain.CriterionOutcome{}
	}
	results, err := json.Marshal(outcomes)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO reward_decisions (`+decisionColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
		decision.DecisionHash, decision.ResourceTenantID, decision.LockID,
		decision.PolicyHash, decision.TaskSpecHash, decision.ContributionHash,
		decision.AlgorithmVersion, string(decision.Currency),
		decision.Allocation.GrossAmountMinor, decision.Allocation.PlatformFeeMinor,
		decision.Allocation.DisputeReserveMinor, decision.Allocation.NetAmountMinor,
		decision.Allocation.AgentAmountMinor, decision.Allocation.MaintainerAmountMinor,
		decision.Allocation.ReviewerPoolAmountMinor, decision.Allocation.UnallocatedAmountMinor,
		decision.QualityMultiplierBps, results, decision.RequiredCriteriaPassed,
		decision.RecipientRef, decision.ChallengeDeadline, decision.Signature,
		decision.SignatureAlgorithm, decision.DecidedAt,
	)
	return writeError(err)
}

func (r *decisionRepository) GetByLock(ctx context.Context, tenantID, lockID string) (*domain.Decision, error) {
	return r.scanOne(ctx, `
		SELECT`+decisionColumns+`
		FROM reward_decisions
		WHERE resource_tenant_id=$1 AND lock_id=$2`, tenantID, lockID)
}

func (r *decisionRepository) GetByHash(ctx context.Context, decisionHash string) (*domain.Decision, error) {
	return r.scanOne(ctx, `
		SELECT`+decisionColumns+`
		FROM reward_decisions
		WHERE decision_hash=$1`, decisionHash)
}

// ListReleasable 返回挑战期已过、锁仍为 releasable 的决策。争议中的锁被
// 排除在外：裁决之前不得自动释放。
func (r *decisionRepository) ListReleasable(ctx context.Context, now time.Time, limit int) ([]domain.Decision, error) {
	rows, err := r.q.Query(ctx, `
		SELECT`+prefixed(decisionColumns, "d")+`
		FROM reward_decisions AS d
		JOIN reward_locks AS l
		  ON l.resource_tenant_id = d.resource_tenant_id AND l.id = d.lock_id
		WHERE l.status='releasable' AND d.challenge_deadline <= $1
		ORDER BY d.challenge_deadline, d.decision_hash
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	return scanDecisions(rows)
}

func (r *decisionRepository) scanOne(ctx context.Context, query string, args ...any) (*domain.Decision, error) {
	rows, err := r.q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	decisions, err := scanDecisions(rows)
	if err != nil {
		return nil, err
	}
	if len(decisions) == 0 {
		return nil, domain.ErrNotFound
	}
	return &decisions[0], nil
}

func scanDecisions(rows pgx.Rows) ([]domain.Decision, error) {
	defer rows.Close()
	var decisions []domain.Decision
	for rows.Next() {
		var decision domain.Decision
		var currency string
		var results []byte
		if err := rows.Scan(
			&decision.DecisionHash, &decision.ResourceTenantID, &decision.LockID,
			&decision.PolicyHash, &decision.TaskSpecHash, &decision.ContributionHash,
			&decision.AlgorithmVersion, &currency,
			&decision.Allocation.GrossAmountMinor, &decision.Allocation.PlatformFeeMinor,
			&decision.Allocation.DisputeReserveMinor, &decision.Allocation.NetAmountMinor,
			&decision.Allocation.AgentAmountMinor, &decision.Allocation.MaintainerAmountMinor,
			&decision.Allocation.ReviewerPoolAmountMinor, &decision.Allocation.UnallocatedAmountMinor,
			&decision.QualityMultiplierBps, &results, &decision.RequiredCriteriaPassed,
			&decision.RecipientRef, &decision.ChallengeDeadline, &decision.Signature,
			&decision.SignatureAlgorithm, &decision.DecidedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(results, &decision.CriterionResults); err != nil {
			return nil, err
		}
		decision.Currency = domain.Currency(currency)
		decisions = append(decisions, decision)
	}
	return decisions, rows.Err()
}

// prefixed 给列清单加上表别名，供 JOIN 查询复用同一份列定义。
func prefixed(columns, alias string) string {
	result := make([]byte, 0, len(columns)*2)
	field := make([]byte, 0, 32)
	flush := func() {
		if len(field) == 0 {
			return
		}
		result = append(result, ' ')
		result = append(result, alias...)
		result = append(result, '.')
		result = append(result, field...)
		result = append(result, ',')
		field = field[:0]
	}
	for index := 0; index < len(columns); index++ {
		char := columns[index]
		switch char {
		case ' ', '\n', '\t', ',':
			flush()
		default:
			field = append(field, char)
		}
	}
	flush()
	if len(result) > 0 {
		result = result[:len(result)-1]
	}
	return string(result)
}
