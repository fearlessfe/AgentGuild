package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/postgres"
	"agentguild.dev/agentguild/backend/internal/ratelimit"
	"agentguild.dev/agentguild/backend/internal/telemetry"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"agentguild.dev/agentguild/backend/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// TestLangfuseFailureLeavesLifecycleCommitted 验证 Provider 故障不影响领域事务提交。
// 即使 Langfuse 调用失败，task lifecycle 仍应提交，execution_usage 标记为 unavailable。
func TestLangfuseFailureLeavesLifecycleCommitted(t *testing.T) {
	ctx := context.Background()
	fixture := serviceWithCostProvider(t, telemetry.FailingProvider(errors.New("timeout")))

	task := publishAndClaim(t, fixture)

	require.NoError(t, fixture.Worker.RunOnce(ctx))

	loaded := loadTask(t, fixture.DB, task.TenantID, task.ID)
	require.Equal(t, domain.TaskClaimed, loaded.Status)

	usage := loadUsage(t, fixture.DB, task.TenantID, task.ExecutionID)
	require.Equal(t, "unavailable", usage.Coverage)
}

type serviceFixture struct {
	DB      *pgxpool.Pool
	Store   *postgres.Store
	Service *application.Service
	Worker  *worker.Outbox
}

func serviceWithCostProvider(t *testing.T, provider telemetry.TraceCostProvider) *serviceFixture {
	t.Helper()
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	svc, err := application.NewService(store, application.Options{
		CursorSecret: []byte("01234567890123456789012345678901"),
		CursorTTL:    time.Hour,
		RateLimiter:  ratelimit.NewUnlimited(),
	})
	require.NoError(t, err)
	w := worker.NewOutbox(db, provider)
	return &serviceFixture{DB: db, Store: store, Service: svc, Worker: w}
}

type claimedTask struct {
	TenantID    string
	ID          string
	ExecutionID string
}

func publishAndClaim(t *testing.T, f *serviceFixture) claimedTask {
	t.Helper()
	ctx := context.Background()
	pub := principal("tenant-1", "publisher-1", "tasks:publish")
	published, err := f.Service.PublishTask(ctx, pub, application.PublishTask{
		RequestID: "pub-1",
		Type:      "code",
		Title:     "T",
		Problem:   "P",
		Deadline:  time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	claim := principal("tenant-1", "agent-1", "tasks:claim")
	claimed, err := f.Service.ClaimTask(ctx, claim, application.ClaimTask{
		RequestID: "claim-1",
		TaskID:    published.Data.ID,
	})
	require.NoError(t, err)

	return claimedTask{TenantID: "tenant-1", ID: published.Data.ID, ExecutionID: claimed.Data.ID}
}

func loadTask(t *testing.T, db *pgxpool.Pool, tenantID, taskID string) *application.TaskRecord {
	t.Helper()
	var r application.TaskRecord
	var status string
	err := db.QueryRow(context.Background(), `
		SELECT id, tenant_id, publisher_agent_version_id, status, state_version, active_execution_id
		FROM tasks WHERE tenant_id=$1 AND id=$2`, tenantID, taskID,
	).Scan(&r.ID, &r.TenantID, &r.PublisherAgentVersionID, &status, &r.StateVersion, &r.ActiveExecutionID)
	require.NoError(t, err)
	r.Status = domain.TaskStatus(status)
	return &r
}

type usageRecord struct {
	Coverage         string
	Provider         string
	SourceCursor     string
	ObservedCost     *float64
	SelfReportedCost *float64
}

func loadUsage(t *testing.T, db *pgxpool.Pool, tenantID, executionID string) *usageRecord {
	t.Helper()
	var u usageRecord
	err := db.QueryRow(context.Background(), `
		SELECT coverage, provider, source_cursor, observed_cost, self_reported_cost
		FROM execution_usage WHERE tenant_id=$1 AND execution_id=$2`, tenantID, executionID,
	).Scan(&u.Coverage, &u.Provider, &u.SourceCursor, &u.ObservedCost, &u.SelfReportedCost)
	require.NoError(t, err)
	return &u
}
