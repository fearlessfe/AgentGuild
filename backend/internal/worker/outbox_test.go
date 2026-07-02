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

func TestOutboxProcessesExecutionEventAndRecordsUsage(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)

	provider := &staticProvider{obs: telemetry.CostObservation{
		ObservedCost: decimal.RequireFromString("0.001"),
		Coverage:     telemetry.CoverageFull,
		Provider:     "langfuse",
		Cursor:       "cursor-1",
	}}
	w := worker.NewOutbox(db, provider)
	processed, err := w.RunBatch(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	usage := loadUsage(t, db, "tenant-1", "exe-1")
	require.Equal(t, "full", usage.Coverage)
	require.Equal(t, "langfuse", usage.Provider)
	require.Equal(t, "cursor-1", usage.SourceCursor)
	require.NotNil(t, usage.ObservedCost)
	require.Equal(t, "0.001", usage.ObservedCost.String())

	remain := countOutbox(t, db)
	require.Equal(t, 0, remain)
}

func TestOutboxIdempotentForSameCursor(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedTaskAndExecution(t, db, "tenant-1", "task-1", "exe-1", "agent-1")
	insertOutbox(t, db, "tenant-1", "evt-1", "execution.started", "execution", "exe-1", `{"task_id":"task-1","execution_id":"exe-1"}`)

	provider := &staticProvider{obs: telemetry.CostObservation{
		ObservedCost: decimal.RequireFromString("0.001"),
		Coverage:     telemetry.CoverageFull,
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

type staticProvider struct {
	obs telemetry.CostObservation
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
