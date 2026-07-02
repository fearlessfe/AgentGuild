// worker 实现 outbox 异步消费。
package worker

import (
	"context"
	"encoding/json"
	"math"
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
	pgxTx, err := o.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = pgxTx.Rollback(ctx) }()

	var now time.Time
	if err := pgxTx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return 0, err
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
		return 0, err
	}

	var events []event
	for rows.Next() {
		var e event
		if err := rows.Scan(&e.tenantID, &e.id, &e.eventType, &e.aggregateType, &e.aggregateID, &e.payload, &e.attempts,
		); err != nil {
			rows.Close()
			return 0, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	processed := 0
	for _, e := range events {
		// 立即更新 claimed_until，防止同一事务内超时前被其他实例抢锁。
		if _, err := pgxTx.Exec(ctx, `
			UPDATE outbox_events SET claimed_until=$1 WHERE tenant_id=$2 AND id=$3`,
			now.Add(claimDuration(e.attempts)), e.tenantID, e.id); err != nil {
			return 0, err
		}

		if err := o.handle(ctx, pgxTx, e, now); err != nil {
			// 处理失败时增加 attempts 并设置退避时间，保留事件供重试。
			if _, dbErr := pgxTx.Exec(ctx, `
				UPDATE outbox_events
				SET attempts=attempts+1, claimed_until=$1, published_at=NULL
				WHERE tenant_id=$2 AND id=$3`,
				now.Add(backoff(e.attempts+1)), e.tenantID, e.id); dbErr != nil {
				return 0, dbErr
			}
			continue
		}
		// 成功消费后删除事件。
		if _, err := pgxTx.Exec(ctx, `
			DELETE FROM outbox_events WHERE tenant_id=$1 AND id=$2`,
			e.tenantID, e.id); err != nil {
			return 0, err
		}
		processed++
	}

	if err := pgxTx.Commit(ctx); err != nil {
		return 0, err
	}
	return processed, nil
}

func (o *Outbox) handle(ctx context.Context, pgxTx pgx.Tx, e event, now time.Time) error {
	if !strings.HasPrefix(e.eventType, "execution.") {
		return nil
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
	err := pgxTx.QueryRow(ctx, `
		SELECT agent_version_id FROM executions
		WHERE tenant_id=$1 AND id=$2 AND task_id=$3`,
		e.tenantID, payload.ExecutionID, payload.TaskID).Scan(&agentVersionID)
	if err != nil {
		// 执行记录不存在则忽略该事件。
		return nil
	}

	ref := telemetry.ExecutionRef{
		TenantID:       e.tenantID,
		TaskID:         payload.TaskID,
		ExecutionID:    payload.ExecutionID,
		AgentVersionID: agentVersionID,
	}
	obs, err := o.provider.Observe(ctx, ref)
	if err != nil {
		// Provider 故障映射为 unavailable 覆盖，仍视为已处理。
		obs = telemetry.CostObservation{Coverage: telemetry.CoverageUnavailable, Provider: "langfuse", Cursor: ""}
	}

	_, err = pgxTx.Exec(ctx, `
		INSERT INTO execution_usage (
			tenant_id, task_id, execution_id, agent_version_id,
			observed_cost, self_reported_cost, coverage, provider, source_cursor, observed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (tenant_id, execution_id, provider, source_cursor) DO NOTHING`,
		e.tenantID, payload.TaskID, payload.ExecutionID, agentVersionID,
		toNumeric(obs.ObservedCost), toNumeric(obs.SelfReportedCost), string(obs.Coverage), obs.Provider, obs.Cursor, now,
	)
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
	base := time.Second
	max := 5 * time.Minute
	// 指数退避：2^attempts 秒。
	d := base * time.Duration(math.Pow(2, float64(attempts)))
	if d > max {
		return max
	}
	return d
}

func claimDuration(attempts int) time.Duration {
	// 单次领取持有时间，防止 worker 崩溃导致事件长期被锁。
	base := 30 * time.Second
	d := base * time.Duration(math.Pow(2, float64(attempts)))
	if d > 5*time.Minute {
		return 5 * time.Minute
	}
	return d
}
