package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	contributiondomain "agentguild.dev/agentguild/backend/internal/contribution/domain"
	"agentguild.dev/agentguild/backend/internal/criteria"
	publictaskdomain "agentguild.dev/agentguild/backend/internal/publictask/domain"
	"agentguild.dev/agentguild/backend/internal/reputation/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FactSource 从不可变事实读取重算所需的一切：contribution 事件账本、
// criterion 证据账本、验证步骤终态与版本化的难度分级。
//
// 它刻意不读任何可变的展示态表，也不读时钟：给定同一份数据库快照，
// 它必须返回逐字节相同（含顺序）的事实，重算才可复现。
type FactSource struct {
	pool *pgxpool.Pool
}

func NewFactSource(pool *pgxpool.Pool) *FactSource {
	return &FactSource{pool: pool}
}

// LatestEventID 返回事实层的变更水位，供增量 worker 判断是否需要重算。
//
// 三个来源都是 append-only 的自增序列（触发器拒绝 UPDATE/DELETE），因此
// 它们的和是单调递增的：任何一条新事实都会让水位变大。只看
// contribution_events 是不够的——criterion 结果往往在 PR 事件之后才产生，
// 那会让 correctness 维度长期滞后。
func (s *FactSource) LatestEventID(ctx context.Context) (int64, error) {
	var watermark int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE((SELECT MAX(id) FROM contribution_events), 0)
		     + COALESCE((SELECT MAX(id) FROM execution_criterion_results), 0)
		     + COALESCE((SELECT MAX(id) FROM validation_steps), 0)`).Scan(&watermark)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	if watermark == 0 {
		return -1, nil
	}
	return watermark, nil
}

// ListContributionFacts 返回全部归属已核验的交付事实。
//
// 未核验（pending_verification / rejected）的 Contribution 一律排除：
// 归属没有确认的贡献不得进入任何 Agent 的声望。
func (s *FactSource) ListContributionFacts(ctx context.Context, algorithmVersion string) ([]application.ContributionFact, error) {
	facts, err := s.listContributions(ctx, algorithmVersion)
	if err != nil {
		return nil, err
	}
	if len(facts) == 0 {
		return nil, nil
	}
	index := make(map[string]int, len(facts))
	for i, fact := range facts {
		index[fact.ContributionID] = i
	}
	if err := s.attachEvents(ctx, facts, index); err != nil {
		return nil, err
	}
	if err := s.attachCriteria(ctx, facts, index); err != nil {
		return nil, err
	}
	if err := s.attachValidationSteps(ctx, facts, index); err != nil {
		return nil, err
	}
	return facts, nil
}

func (s *FactSource) listContributions(ctx context.Context, algorithmVersion string) ([]application.ContributionFact, error) {
	// AttemptOrdinal 用窗口函数算出同一 Agent 在同一任务上的第几次尝试：
	// 重复尝试是 reliability 的负信号，而不是额外的成功样本。
	rows, err := s.pool.Query(ctx, `
		SELECT
			contribution.id,
			contribution.resource_tenant_id,
			contribution.agent_id,
			contribution.agent_version_id,
			contribution.task_id,
			contribution.execution_id,
			contribution.canonical_repository,
			contribution.created_at,
			task.type,
			task.deadline,
			execution.status,
			execution.submitted_at,
			COALESCE(projection.difficulty_class, 'standard'),
			COALESCE(difficulty.multiplier, 1.0),
			COALESCE(projection.acceptance_criteria, '[]'::jsonb),
			ROW_NUMBER() OVER (
				PARTITION BY contribution.agent_id, contribution.task_id
				ORDER BY contribution.created_at, contribution.id
			)
		FROM contributions AS contribution
		JOIN tasks AS task
		  ON task.tenant_id = contribution.resource_tenant_id
		 AND task.id = contribution.task_id
		JOIN executions AS execution
		  ON execution.tenant_id = contribution.resource_tenant_id
		 AND execution.id = contribution.execution_id
		LEFT JOIN public_task_projections AS projection
		  ON projection.resource_tenant_id = contribution.resource_tenant_id
		 AND projection.task_id = contribution.task_id
		LEFT JOIN task_difficulty_classes AS difficulty
		  ON difficulty.algorithm_version = $1
		 AND difficulty.class = COALESCE(projection.difficulty_class, 'standard')
		WHERE contribution.attribution_status = 'verified'
		ORDER BY contribution.id`, algorithmVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	facts := make([]application.ContributionFact, 0, 32)
	for rows.Next() {
		var (
			fact        application.ContributionFact
			tenantID    string
			deadline    *time.Time
			submittedAt *time.Time
			rawCriteria []byte
			ordinal     int64
		)
		if err := rows.Scan(
			&fact.ContributionID, &tenantID, &fact.AgentID, &fact.AgentVersionID,
			&fact.TaskID, &fact.ExecutionID, &fact.CanonicalRepository, &fact.CreatedAt,
			&fact.Capability, &deadline, &fact.ExecutionStatus, &submittedAt,
			&fact.DifficultyClass, &fact.DifficultyMultiplier, &rawCriteria, &ordinal,
		); err != nil {
			return nil, err
		}
		fact.CreatedAt = fact.CreatedAt.UTC()
		fact.AttemptOrdinal = int(ordinal)
		if deadline != nil {
			fact.TaskDeadline = deadline.UTC()
		}
		if submittedAt != nil {
			utc := submittedAt.UTC()
			fact.SubmittedAt = &utc
		}
		// 规格里的 verifier_ref 走与 criterion 写入路径同一套 fail-closed
		// 白名单，指向未知步骤的绑定不被采信。
		fact.Criteria = specVerifierRefs(rawCriteria)
		facts = append(facts, fact)
	}
	return facts, rows.Err()
}

// specVerifierRefs 先把规格里的验收标准解析成占位事实，随后由 criterion
// 账本填充结果。没有结果的占位会被丢弃——未验证绝不等于通过。
func specVerifierRefs(raw []byte) []application.CriterionFact {
	var parsed []publictaskdomain.AcceptanceCriterion
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil
		}
	}
	specs := criteria.MapCriteria(parsed)
	placeholders := make([]application.CriterionFact, 0, len(specs))
	for _, spec := range specs {
		placeholders = append(placeholders, application.CriterionFact{
			CriterionID: spec.ID,
			Critical:    spec.Critical,
			VerifierRef: spec.VerifierRef,
		})
	}
	return placeholders
}

func (s *FactSource) attachEvents(ctx context.Context, facts []application.ContributionFact, index map[string]int) error {
	rows, err := s.pool.Query(ctx, `
		SELECT contribution_id, id, outcome, occurred_at
		FROM contribution_events
		ORDER BY contribution_id, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			contributionID string
			event          application.OutcomeFact
			outcome        string
		)
		if err := rows.Scan(&contributionID, &event.EventID, &outcome, &event.OccurredAt); err != nil {
			return err
		}
		position, found := index[contributionID]
		if !found {
			continue
		}
		event.Outcome = contributiondomain.Outcome(outcome)
		event.OccurredAt = event.OccurredAt.UTC()
		facts[position].Events = append(facts[position].Events, event)
	}
	return rows.Err()
}

// attachCriteria 用 criterion 账本的最新态填充规格占位。规格里存在但账本
// 里没有结果的标准被丢弃，不产生任何观测。
func (s *FactSource) attachCriteria(ctx context.Context, facts []application.ContributionFact, index map[string]int) error {
	type resultKey struct{ tenantID, executionID, criterionID string }
	results := make(map[resultKey]application.CriterionFact)

	rows, err := s.pool.Query(ctx, `
		SELECT resource_tenant_id, execution_id, criterion_id, critical, passed, observed_at
		FROM execution_criterion_latest
		ORDER BY resource_tenant_id, execution_id, criterion_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			key    resultKey
			result application.CriterionFact
		)
		if err := rows.Scan(&key.tenantID, &key.executionID, &result.CriterionID,
			&result.Critical, &result.Passed, &result.ObservedAt); err != nil {
			return err
		}
		result.ObservedAt = result.ObservedAt.UTC()
		results[key] = result
	}
	if err := rows.Err(); err != nil {
		return err
	}

	tenants, err := s.contributionTenants(ctx)
	if err != nil {
		return err
	}
	for contributionID, position := range index {
		fact := facts[position]
		verified := make([]application.CriterionFact, 0, len(fact.Criteria))
		for _, placeholder := range fact.Criteria {
			result, found := results[resultKey{
				tenantID:    tenants[contributionID],
				executionID: fact.ExecutionID,
				criterionID: placeholder.CriterionID,
			}]
			if !found {
				continue
			}
			result.VerifierRef = placeholder.VerifierRef
			// critical 以规格为准：账本记录的是写入当时的判断。
			result.Critical = placeholder.Critical
			verified = append(verified, result)
		}
		facts[position].Criteria = verified
	}
	return nil
}

func (s *FactSource) attachValidationSteps(ctx context.Context, facts []application.ContributionFact, index map[string]int) error {
	type stepKey struct{ tenantID, executionID string }
	steps := make(map[stepKey][]application.ValidationStepFact)

	// 只采信真正跑完的步骤：pending / running / skipped 不产生结论。
	rows, err := s.pool.Query(ctx, `
		SELECT job.tenant_id, job.execution_id, job.id, step.step, step.status,
		       COALESCE(step.finished_at, step.created_at)
		FROM validation_steps AS step
		JOIN validation_jobs AS job
		  ON job.tenant_id = step.tenant_id AND job.id = step.job_id
		WHERE step.status IN ('succeeded', 'failed')
		ORDER BY job.tenant_id, job.execution_id, job.id, step.step`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			key    stepKey
			step   application.ValidationStepFact
			status string
		)
		if err := rows.Scan(&key.tenantID, &key.executionID, &step.JobID,
			&step.Step, &status, &step.ObservedAt); err != nil {
			return err
		}
		step.Passed = status == "succeeded"
		step.ObservedAt = step.ObservedAt.UTC()
		steps[key] = append(steps[key], step)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	tenants, err := s.contributionTenants(ctx)
	if err != nil {
		return err
	}
	for contributionID, position := range index {
		facts[position].ValidationSteps = steps[stepKey{
			tenantID:    tenants[contributionID],
			executionID: facts[position].ExecutionID,
		}]
	}
	return nil
}

// contributionTenants 返回 contribution → sponsor 租户的映射。租户只用于
// 定位事实，绝不进入任何对外视图。
func (s *FactSource) contributionTenants(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, resource_tenant_id FROM contributions WHERE attribution_status='verified'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tenants := make(map[string]string, 32)
	for rows.Next() {
		var id, tenantID string
		if err := rows.Scan(&id, &tenantID); err != nil {
			return nil, err
		}
		tenants[id] = tenantID
	}
	return tenants, rows.Err()
}
