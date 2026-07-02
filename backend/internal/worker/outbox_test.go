package worker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/telemetry"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"agentguild.dev/agentguild/backend/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestDisabledProviderDrainsEventWithoutRetry(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)

	w := worker.NewOutbox(db, disabledProvider{})
	processed, err := w.RunBatch(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	var published bool
	require.NoError(t, db.QueryRow(ctx, `SELECT published_at IS NOT NULL FROM outbox_events WHERE tenant_id='tenant-1' AND id='evt-1'`).Scan(&published))
	require.True(t, published)

	usage := loadUsage(t, db, "tenant-1", "exe-1")
	require.Equal(t, "unavailable", usage.Coverage)
	require.Equal(t, "disabled", usage.Provider)
}

type disabledProvider struct{}

func (disabledProvider) Observe(context.Context, telemetry.ExecutionRef) (telemetry.CostObservation, error) {
	return telemetry.CostObservation{Coverage: telemetry.CoverageUnavailable, Provider: "disabled", Disabled: true}, nil
}

func TestOutboxProcessesExecutionEventAndRecordsUsage(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)

	provider := &staticProvider{obs: telemetry.CostObservation{
		ObservedCost: decimal.RequireFromString("0.001"),
		Coverage:     telemetry.CoverageComplete,
		Provider:     "langfuse",
		Cursor:       "cursor-1",
	}}
	w := worker.NewOutbox(db, provider)
	processed, err := w.RunBatch(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	usage := loadUsage(t, db, "tenant-1", "exe-1")
	require.Equal(t, "complete", usage.Coverage)
	require.Equal(t, "langfuse", usage.Provider)
	require.Equal(t, "execution-snapshot:exe-1", usage.SourceCursor)
	require.NotNil(t, usage.ObservedCost)
	require.Equal(t, "0.001", usage.ObservedCost.String())

	var published bool
	require.NoError(t, db.QueryRow(ctx, `SELECT published_at IS NOT NULL FROM outbox_events WHERE tenant_id='tenant-1' AND id='evt-1'`).Scan(&published))
	require.True(t, published)
}

func TestOutboxIdempotentForSameCursor(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)

	provider := &staticProvider{obs: telemetry.CostObservation{
		ObservedCost: decimal.RequireFromString("0.001"),
		Coverage:     telemetry.CoverageComplete,
		Provider:     "langfuse",
		Cursor:       "cursor-1",
	}}
	w := worker.NewOutbox(db, provider)
	_, err := w.RunBatch(ctx, 10)
	require.NoError(t, err)
	// 直接再次插入相同事件模拟重复投递
	insertOutbox(t, db, "tenant-1", "evt-2", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)
	_, err = w.RunBatch(ctx, 10)
	require.NoError(t, err)

	var count int
	err = db.QueryRow(ctx, `SELECT count(*) FROM execution_usage WHERE tenant_id='tenant-1' AND execution_id='exe-1'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "同一 cursor 不应产生重复 usage 行")
}

func TestOutboxEmptyCursorUnavailableRecoversToComplete(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)

	calls := 0
	provider := providerFunc(func(context.Context, telemetry.ExecutionRef) (telemetry.CostObservation, error) {
		calls++
		if calls == 1 {
			return telemetry.CostObservation{Coverage: telemetry.CoverageUnavailable, Provider: "langfuse"}, errors.New("temporary")
		}
		return telemetry.CostObservation{ObservedCost: decimal.RequireFromString("0.5"), Coverage: telemetry.CoverageComplete, Provider: "langfuse"}, nil
	})
	w := worker.NewOutbox(db, provider)
	_, err := w.RunBatch(ctx, 1)
	require.NoError(t, err)
	_, err = db.Exec(ctx, `UPDATE outbox_events SET claimed_until=clock_timestamp()-interval '1 second' WHERE tenant_id='tenant-1' AND id='evt-1'`)
	require.NoError(t, err)
	_, err = w.RunBatch(ctx, 1)
	require.NoError(t, err)

	var count int
	var coverage string
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*), max(coverage) FROM execution_usage WHERE tenant_id='tenant-1' AND execution_id='exe-1'`).Scan(&count, &coverage))
	require.Equal(t, 1, count)
	require.Equal(t, "complete", coverage)
}

func TestOutboxPartialRemainsPendingAndUpgradesWithoutDowngrade(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)

	observations := []telemetry.CostObservation{
		{Coverage: telemetry.CoveragePartial, Provider: "langfuse"},
		{ObservedCost: decimal.RequireFromString("0.75"), Coverage: telemetry.CoverageComplete, Provider: "langfuse"},
	}
	provider := providerFunc(func(context.Context, telemetry.ExecutionRef) (telemetry.CostObservation, error) {
		obs := observations[0]
		if len(observations) > 1 {
			observations = observations[1:]
		}
		return obs, nil
	})
	w := worker.NewOutbox(db, provider)
	_, err := w.RunBatch(ctx, 1)
	require.NoError(t, err)
	var published bool
	require.NoError(t, db.QueryRow(ctx, `SELECT published_at IS NOT NULL FROM outbox_events WHERE tenant_id='tenant-1' AND id='evt-1'`).Scan(&published))
	require.False(t, published)
	_, err = db.Exec(ctx, `UPDATE outbox_events SET claimed_until=clock_timestamp()-interval '1 second' WHERE tenant_id='tenant-1' AND id='evt-1'`)
	require.NoError(t, err)
	_, err = w.RunBatch(ctx, 1)
	require.NoError(t, err)

	u := loadUsage(t, db, "tenant-1", "exe-1")
	require.Equal(t, "complete", u.Coverage)
	require.NotNil(t, u.ObservedCost)
	require.Equal(t, "0.75", u.ObservedCost.String())
}

func TestOutboxWorseCoverageCannotOverwriteCompleteSnapshot(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-complete", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)
	provider := &staticProvider{obs: telemetry.CostObservation{ObservedCost: decimal.RequireFromString("2.5"), Coverage: telemetry.CoverageComplete, Provider: "langfuse"}}
	w := worker.NewOutbox(db, provider)
	_, err := w.RunBatch(ctx, 1)
	require.NoError(t, err)

	insertOutbox(t, db, "tenant-1", "evt-partial", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)
	provider.obs = telemetry.CostObservation{Coverage: telemetry.CoveragePartial, Provider: "langfuse"}
	_, err = w.RunBatch(ctx, 1)
	require.NoError(t, err)
	u := loadUsage(t, db, "tenant-1", "exe-1")
	require.Equal(t, "complete", u.Coverage)
	require.NotNil(t, u.ObservedCost)
	require.Equal(t, "2.5", u.ObservedCost.String())
}

func TestOutboxStaleClaimantCannotWriteUsageOrPublish(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)

	provider := providerFunc(func(context.Context, telemetry.ExecutionRef) (telemetry.CostObservation, error) {
		_, err := db.Exec(ctx, `UPDATE outbox_events SET claimed_until=clock_timestamp()+interval '2 minutes' WHERE tenant_id='tenant-1' AND id='evt-1'`)
		require.NoError(t, err)
		return telemetry.CostObservation{ObservedCost: decimal.RequireFromString("1"), Coverage: telemetry.CoverageComplete, Provider: "langfuse"}, nil
	})
	_, err := worker.NewOutbox(db, provider).RunBatch(ctx, 1)
	require.Error(t, err)
	var usageCount int
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*) FROM execution_usage WHERE tenant_id='tenant-1' AND execution_id='exe-1'`).Scan(&usageCount))
	require.Zero(t, usageCount)
	var published bool
	require.NoError(t, db.QueryRow(ctx, `SELECT published_at IS NOT NULL FROM outbox_events WHERE tenant_id='tenant-1' AND id='evt-1'`).Scan(&published))
	require.False(t, published)
}

func TestOutboxStaleClaimantCannotScheduleFailure(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)
	var replacementClaim time.Time
	provider := providerFunc(func(context.Context, telemetry.ExecutionRef) (telemetry.CostObservation, error) {
		require.NoError(t, db.QueryRow(ctx, `SELECT clock_timestamp()+interval '2 minutes'`).Scan(&replacementClaim))
		_, err := db.Exec(ctx, `UPDATE outbox_events SET claimed_until=$1 WHERE tenant_id='tenant-1' AND id='evt-1'`, replacementClaim)
		require.NoError(t, err)
		return telemetry.CostObservation{Coverage: telemetry.CoverageUnavailable, Provider: "langfuse"}, errors.New("temporary")
	})
	_, err := worker.NewOutbox(db, provider).RunBatch(ctx, 1)
	require.Error(t, err)
	var attempts int
	var claimedUntil time.Time
	require.NoError(t, db.QueryRow(ctx, `SELECT attempts, claimed_until FROM outbox_events WHERE tenant_id='tenant-1' AND id='evt-1'`).Scan(&attempts, &claimedUntil))
	require.Zero(t, attempts)
	require.Equal(t, replacementClaim, claimedUntil)
}

func TestOutboxPoisonEventIsRetried(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	insertOutbox(t, db, "tenant-1", "evt-poison", "execution.started", "execution", "exe", `{}`)
	_, err := worker.NewOutbox(db, &staticProvider{}).RunBatch(ctx, 1)
	require.NoError(t, err)
	var attempts int
	require.NoError(t, db.QueryRow(ctx, `SELECT attempts FROM outbox_events WHERE tenant_id='tenant-1' AND id='evt-poison'`).Scan(&attempts))
	require.Equal(t, 1, attempts)
}

func TestOutboxCompleteZeroCostIsStoredAsZero(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)
	provider := &staticProvider{obs: telemetry.CostObservation{Coverage: telemetry.CoverageComplete, Provider: "langfuse"}}
	_, err := worker.NewOutbox(db, provider).RunBatch(ctx, 1)
	require.NoError(t, err)
	u := loadUsage(t, db, "tenant-1", "exe-1")
	require.NotNil(t, u.ObservedCost)
	require.True(t, u.ObservedCost.IsZero())
}

func TestOutboxSuccessUsesFreshDatabaseTimeAfterProviderCall(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)
	var providerFinished time.Time
	provider := providerFunc(func(context.Context, telemetry.ExecutionRef) (telemetry.CostObservation, error) {
		require.NoError(t, db.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&providerFinished))
		return telemetry.CostObservation{Coverage: telemetry.CoverageComplete, Provider: "langfuse"}, nil
	})
	_, err := worker.NewOutbox(db, provider).RunBatch(ctx, 1)
	require.NoError(t, err)
	var observedAt, publishedAt time.Time
	require.NoError(t, db.QueryRow(ctx, `
		SELECT u.observed_at, o.published_at
		FROM execution_usage u JOIN outbox_events o ON o.tenant_id=u.tenant_id
		WHERE u.tenant_id='tenant-1' AND u.execution_id='exe-1' AND o.id='evt-1'`).Scan(&observedAt, &publishedAt))
	require.False(t, observedAt.Before(providerFinished))
	require.False(t, publishedAt.Before(providerFinished))
}

func TestOutboxFailureBackoffUsesFreshDatabaseTimeAfterProviderCall(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)
	var providerFinished time.Time
	provider := providerFunc(func(context.Context, telemetry.ExecutionRef) (telemetry.CostObservation, error) {
		time.Sleep(1300 * time.Millisecond)
		require.NoError(t, db.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&providerFinished))
		return telemetry.CostObservation{Coverage: telemetry.CoverageUnavailable, Provider: "langfuse"}, errors.New("temporary")
	})
	_, err := worker.NewOutbox(db, provider).RunBatch(ctx, 1)
	require.NoError(t, err)
	var retryAt time.Time
	require.NoError(t, db.QueryRow(ctx, `SELECT claimed_until FROM outbox_events WHERE tenant_id='tenant-1' AND id='evt-1'`).Scan(&retryAt))
	require.True(t, retryAt.After(providerFinished.Add(1500*time.Millisecond)), "backoff must start from fresh post-provider database time")
}

func TestOutboxCommitsClaimBeforeCallingProvider(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)

	provider := providerFunc(func(context.Context, telemetry.ExecutionRef) (telemetry.CostObservation, error) {
		var claimed bool
		err := db.QueryRow(ctx, `SELECT claimed_until IS NOT NULL FROM outbox_events WHERE tenant_id='tenant-1' AND id='evt-1'`).Scan(&claimed)
		require.NoError(t, err)
		require.True(t, claimed, "调用外部 Provider 前必须提交领取状态")
		return telemetry.CostObservation{Coverage: telemetry.CoveragePartial, Provider: "langfuse", Cursor: "cursor"}, nil
	})

	processed, err := worker.NewOutbox(db, provider).RunBatch(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, processed)
}

func TestOutboxMarksUnavailableWhenProviderFails(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)

	w := worker.NewOutbox(db, telemetry.FailingProvider(errors.New("timeout")))
	processed, err := w.RunBatch(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	usage := loadUsage(t, db, "tenant-1", "exe-1")
	require.Equal(t, "unavailable", usage.Coverage)
	var attempts int
	var pending bool
	require.NoError(t, db.QueryRow(ctx, `
		SELECT attempts, published_at IS NULL AND claimed_until > clock_timestamp()
		FROM outbox_events WHERE tenant_id='tenant-1' AND id='evt-1'`).Scan(&attempts, &pending))
	require.Equal(t, 1, attempts)
	require.True(t, pending)
}

func TestOutboxSkipLockedPreventsDuplicateProcessing(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	for i := 0; i < 5; i++ {
		insertOutbox(t, db, "tenant-1", "evt-"+string(rune('a'+i)), "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)
	}

	provider := &staticProvider{obs: telemetry.CostObservation{Coverage: telemetry.CoveragePartial, Provider: "langfuse", Cursor: "cursor-1"}}
	w := worker.NewOutbox(db, provider)

	var wg sync.WaitGroup
	var total int
	var mu sync.Mutex
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := w.RunBatch(ctx, 10)
			require.NoError(t, err)
			mu.Lock()
			total += n
			mu.Unlock()
		}()
	}
	wg.Wait()
	require.Equal(t, 5, total, "多个 worker 实例应恰好处理全部事件")
}

func TestOutboxBackoffOnDatabaseFailureIsNotExercised(t *testing.T) {
	// 该测试验证 RunBatch 对无效 limit 返回错误；指数退避逻辑由其他测试覆盖。
	w := worker.NewOutbox(nil, nil)
	_, err := w.RunBatch(context.Background(), 0)
	require.Error(t, err)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))
}

func TestOutboxBackoffCapsWithoutOverflow(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	insertOutbox(t, db, "tenant-1", "evt-overflow", "execution.started", "execution", "exe", `{"task_id":1}`)
	_, err := db.Exec(ctx, `UPDATE outbox_events SET attempts=100 WHERE tenant_id='tenant-1' AND id='evt-overflow'`)
	require.NoError(t, err)

	processed, err := worker.NewOutbox(db, &staticProvider{}).RunBatch(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	var retryAt time.Time
	require.NoError(t, db.QueryRow(ctx, `SELECT claimed_until FROM outbox_events WHERE tenant_id='tenant-1' AND id='evt-overflow'`).Scan(&retryAt))
	require.True(t, retryAt.After(time.Now()), "高 attempts 的退避不能溢出到过去")
	require.True(t, retryAt.Before(time.Now().Add(6*time.Minute)), "退避必须受五分钟上限约束")
}

type staticProvider struct {
	obs telemetry.CostObservation
}

type providerFunc func(context.Context, telemetry.ExecutionRef) (telemetry.CostObservation, error)

func (f providerFunc) Observe(ctx context.Context, ref telemetry.ExecutionRef) (telemetry.CostObservation, error) {
	return f(ctx, ref)
}

func (p *staticProvider) Observe(_ context.Context, _ telemetry.ExecutionRef) (telemetry.CostObservation, error) {
	return p.obs, nil
}

type usageRecord struct {
	Coverage         string
	Provider         string
	SourceCursor     string
	ObservedCost     *decimal.Decimal
	SelfReportedCost *decimal.Decimal
}

func seedTaskAndExecution(t *testing.T, db *pgxpool.Pool, tenantID, taskID, executionID, agentVersionID string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO tasks (tenant_id, id, publisher_agent_version_id, type, title, problem, constraints, requirements, deadline, status)
		VALUES ($1, $2, 'publisher-1', 'code', 'title', 'problem', '{}'::jsonb, '{}'::jsonb, $3, 'open')`,
		tenantID, taskID, time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = db.Exec(ctx, `
		INSERT INTO executions (tenant_id, id, task_id, agent_version_id, status, lease_generation)
		VALUES ($1, $2, $3, $4, 'running', 1)`,
		tenantID, executionID, taskID, agentVersionID)
	require.NoError(t, err)
}

func insertOutbox(t *testing.T, db *pgxpool.Pool, tenantID, id, eventType, aggregateType, aggregateID, payload string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO outbox_events (tenant_id, id, event_type, aggregate_type, aggregate_id, payload, available_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, clock_timestamp())`,
		tenantID, id, eventType, aggregateType, aggregateID, payload)
	require.NoError(t, err)
}

func loadUsage(t *testing.T, db *pgxpool.Pool, tenantID, executionID string) *usageRecord {
	t.Helper()
	var u usageRecord
	err := db.QueryRow(context.Background(), `
		SELECT coverage, provider, source_cursor, observed_cost, self_reported_cost
		FROM execution_usage WHERE tenant_id=$1 AND execution_id=$2`, tenantID, executionID).Scan(
		&u.Coverage, &u.Provider, &u.SourceCursor, &u.ObservedCost, &u.SelfReportedCost)
	require.NoError(t, err)
	return &u
}

func countOutbox(t *testing.T, db *pgxpool.Pool) int {
	t.Helper()
	var n int
	err := db.QueryRow(context.Background(), `SELECT count(*) FROM outbox_events`).Scan(&n)
	require.NoError(t, err)
	return n
}
