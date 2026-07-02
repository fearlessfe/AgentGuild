// worker 实现 outbox 异步消费。
package worker

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/telemetry"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Outbox 消费 outbox_events 中的事件并异步观测执行成本。
type Outbox struct {
	pool     *pgxpool.Pool
	provider telemetry.TraceCostProvider
}

// NewOutbox 创建 outbox worker。
func NewOutbox(pool *pgxpool.Pool, provider telemetry.TraceCostProvider) *Outbox {
	return &Outbox{pool: pool, provider: provider}
}

// RunOnce 处理一批事件并返回处理数量。
func (o *Outbox) RunOnce(ctx context.Context) error {
	_, err := o.RunBatch(ctx, 10)
	return err
}

// event 是 outbox_events 的一条待处理记录。
type event struct {
	tenantID, id, eventType, aggregateType, aggregateID string
	payload                                             []byte
	attempts                                            int
}

// RunBatch 批量消费 outbox 事件。
// 使用 FOR UPDATE SKIP LOCKED 领取事件；对 execution.* 事件调用 TraceCostProvider，
// 幂等写入 execution_usage；成功消费后删除事件。
func (o *Outbox) RunBatch(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		return 0, &domain.Error{Code: "invalid_argument", Message: "limit 无效", Field: "limit"}
	}
	events, now, err := o.claimBatch(ctx, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, e := range events {
		if err := o.handle(ctx, e, now); err != nil {
			if dbErr := o.markFailure(ctx, e, now); dbErr != nil {
				return processed, dbErr
			}
			processed++
			continue
		}
		processed++
	}
	return processed, nil
}

func (o *Outbox) claimBatch(ctx context.Context, limit int) ([]event, time.Time, error) {
	pgxTx, err := o.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, time.Time{}, err
	}
	defer func() { _ = pgxTx.Rollback(ctx) }()

	var now time.Time
	if err := pgxTx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return nil, time.Time{}, err
	}

	rows, err := pgxTx.Query(ctx, `
		SELECT tenant_id, id, event_type, aggregate_type, aggregate_id, payload, attempts
		FROM outbox_events
		WHERE available_at <= $1
		  AND published_at IS NULL
		  AND (claimed_until IS NULL OR claimed_until <= $1)
		ORDER BY available_at, created_at
		FOR UPDATE SKIP LOCKED
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, time.Time{}, err
	}

	var events []event
	for rows.Next() {
		var e event
		if err := rows.Scan(&e.tenantID, &e.id, &e.eventType, &e.aggregateType, &e.aggregateID, &e.payload, &e.attempts); err != nil {
			rows.Close()
			return nil, time.Time{}, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, time.Time{}, err
	}
	rows.Close()

	for _, e := range events {
		if _, err := pgxTx.Exec(ctx, `
			UPDATE outbox_events SET claimed_until=$1 WHERE tenant_id=$2 AND id=$3`,
			now.Add(claimDuration(e.attempts)), e.tenantID, e.id); err != nil {
			return nil, time.Time{}, err
		}
	}

	if err := pgxTx.Commit(ctx); err != nil {
		return nil, time.Time{}, err
	}
	return events, now, nil
}

func (o *Outbox) handle(ctx context.Context, e event, now time.Time) error {
	if !strings.HasPrefix(e.eventType, "execution.") {
		return o.markPublished(ctx, e, now)
	}
	var payload struct {
		TaskID      string `json:"task_id"`
		ExecutionID string `json:"execution_id"`
	}
	if err := json.Unmarshal(e.payload, &payload); err != nil {
		return err
	}
	if payload.TaskID == "" || payload.ExecutionID == "" {
		return nil
	}

	var agentVersionID string
	err := o.pool.QueryRow(ctx, `
		SELECT agent_version_id FROM executions
		WHERE tenant_id=$1 AND id=$2 AND task_id=$3`,
		e.tenantID, payload.ExecutionID, payload.TaskID).Scan(&agentVersionID)
	if err != nil {
		// 执行记录不存在则忽略该事件。
		if err == pgx.ErrNoRows {
			return o.markPublished(ctx, e, now)
		}
		return err
	}

	ref := telemetry.ExecutionRef{
		TenantID:       e.tenantID,
		TaskID:         payload.TaskID,
		ExecutionID:    payload.ExecutionID,
		AgentVersionID: agentVersionID,
	}
	obs, err := o.provider.Observe(ctx, ref)
	providerErr := err
	if providerErr != nil && obs.Coverage == "" {
		// Provider 故障先记录 unavailable，再由调用方安排退避重试。
		obs = telemetry.CostObservation{Coverage: telemetry.CoverageUnavailable, Provider: "langfuse", Cursor: ""}
	}

	pgxTx, err := o.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = pgxTx.Rollback(ctx) }()
	if _, err = pgxTx.Exec(ctx, `
		INSERT INTO execution_usage (
			tenant_id, task_id, execution_id, agent_version_id,
			observed_cost, self_reported_cost, coverage, provider, source_cursor, observed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (tenant_id, execution_id, provider, source_cursor) DO NOTHING`,
		e.tenantID, payload.TaskID, payload.ExecutionID, agentVersionID,
		toNumeric(obs.ObservedCost), toNumeric(obs.SelfReportedCost), string(obs.Coverage), obs.Provider, obs.Cursor, now,
	); err != nil {
		return err
	}
	if providerErr == nil {
		if _, err = pgxTx.Exec(ctx, `
			UPDATE outbox_events SET published_at=$1, claimed_until=NULL
			WHERE tenant_id=$2 AND id=$3 AND published_at IS NULL`, now, e.tenantID, e.id); err != nil {
			return err
		}
	}
	if err := pgxTx.Commit(ctx); err != nil {
		return err
	}
	return providerErr
}

func (o *Outbox) markPublished(ctx context.Context, e event, now time.Time) error {
	_, err := o.pool.Exec(ctx, `
		UPDATE outbox_events SET published_at=$1, claimed_until=NULL
		WHERE tenant_id=$2 AND id=$3 AND published_at IS NULL`, now, e.tenantID, e.id)
	return err
}

func (o *Outbox) markFailure(ctx context.Context, e event, now time.Time) error {
	_, err := o.pool.Exec(ctx, `
		UPDATE outbox_events
		SET attempts=attempts+1, claimed_until=$1, published_at=NULL
		WHERE tenant_id=$2 AND id=$3 AND published_at IS NULL`,
		now.Add(backoff(e.attempts+1)), e.tenantID, e.id)
	return err
}

func toNumeric(d decimalOrZero) interface{} {
	if d.IsZero() {
		return nil
	}
	return d.String()
}

type decimalOrZero interface {
	IsZero() bool
	String() string
}

func backoff(attempts int) time.Duration {
	return cappedExponential(time.Second, attempts, 5*time.Minute)
}

func claimDuration(attempts int) time.Duration {
	// 单次领取持有时间，防止 worker 崩溃导致事件长期被锁。
	return cappedExponential(30*time.Second, attempts, 5*time.Minute)
}

func cappedExponential(base time.Duration, exponent int, maximum time.Duration) time.Duration {
	if exponent < 0 {
		exponent = 0
	}
	d := base
	for range exponent {
		if d >= maximum/2 {
			return maximum
		}
		d *= 2
	}
	return d
}
