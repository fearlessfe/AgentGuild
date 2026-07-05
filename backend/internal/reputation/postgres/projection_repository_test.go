package postgres_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	appdomain "agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/postgres"
	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
	reputationpostgres "agentguild.dev/agentguild/backend/internal/reputation/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestProjectionRepositorySaveAndGetByKey(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	repo := reputationpostgres.NewProjectionRepository(db)
	projection := newProjection("agent-v1", "go", "code")

	require.NoError(t, repo.Save(ctx, reputationapp.ProjectionRecord{TenantID: "tenant-1", Projection: *projection}))

	got, err := repo.GetByKey(ctx, "tenant-1", projection.Key)
	require.NoError(t, err)
	requireEqualProjection(t, projection, got)
}

func TestProjectionRepositoryIsTenantScoped(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	repo := reputationpostgres.NewProjectionRepository(db)
	p1 := newProjection("agent-v1", "go", "code")
	p2 := newProjection("agent-v1", "go", "code")

	require.NoError(t, saveProjectionForTenant(ctx, db, "tenant-1", *p1))
	require.NoError(t, saveProjectionForTenant(ctx, db, "tenant-2", *p2))

	got, err := repo.GetByKey(ctx, "tenant-1", p1.Key)
	require.NoError(t, err)
	requireEqualProjection(t, p1, got)

	_, err = repo.GetByKey(ctx, "tenant-3", p1.Key)
	require.ErrorIs(t, err, appdomain.ErrNotFound)
}

func TestProjectionRepositorySaveConflictsForSameTenantKey(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	repo := reputationpostgres.NewProjectionRepository(db)
	p := newProjection("agent-v1", "go", "code")
	require.NoError(t, repo.Save(ctx, reputationapp.ProjectionRecord{TenantID: "tenant-1", Projection: *p}))

	err := repo.Save(ctx, reputationapp.ProjectionRecord{TenantID: "tenant-1", Projection: *p})
	require.Error(t, err)
}

func TestTxUpsertReputationProjectionCreatesAndUpdates(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	store := postgres.NewStore(db)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.UpsertReputationProjection(ctx, reputationapp.ProjectionRecord{
			TenantID:   "tenant-1",
			Projection: *newProjection("agent-v1", "go", "code"),
		})
	}))

	var firstUpdatedAt time.Time
	require.NoError(t, db.QueryRow(ctx, `
		SELECT updated_at FROM reputation_projections
		WHERE tenant_id='tenant-1' AND agent_version_id='agent-v1' AND capability='go' AND task_type='code'`).Scan(&firstUpdatedAt))

	time.Sleep(10 * time.Millisecond)

	updated := newProjection("agent-v1", "go", "code")
	updated.TotalReviews = 5
	updated.AcceptedCount = 5
	updated.PassRate = 1.0
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.UpsertReputationProjection(ctx, reputationapp.ProjectionRecord{
			TenantID:   "tenant-1",
			Projection: *updated,
		})
	}))

	got, err := loadProjectionForTenant(ctx, db, "tenant-1", updated.Key)
	require.NoError(t, err)
	require.Equal(t, 5, got.TotalReviews)

	var secondUpdatedAt time.Time
	require.NoError(t, db.QueryRow(ctx, `
		SELECT updated_at FROM reputation_projections
		WHERE tenant_id='tenant-1' AND agent_version_id='agent-v1' AND capability='go' AND task_type='code'`).Scan(&secondUpdatedAt))
	require.True(t, secondUpdatedAt.After(firstUpdatedAt) || secondUpdatedAt.Equal(firstUpdatedAt))
}

func TestListReputationProjectionsByAgentVersion(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	store := postgres.NewStore(db)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		for _, p := range []reputationapp.ProjectionRecord{
			{TenantID: "tenant-1", Projection: *newProjection("agent-v1", "go", "code")},
			{TenantID: "tenant-1", Projection: *newProjection("agent-v1", "python", "code")},
			{TenantID: "tenant-1", Projection: *newProjection("agent-v2", "go", "code")},
			{TenantID: "tenant-2", Projection: *newProjection("agent-v1", "go", "code")},
		} {
			if err := tx.UpsertReputationProjection(ctx, p); err != nil {
				return err
			}
		}
		return nil
	}))

	var got []reputationapp.ProjectionRecord
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		var err error
		got, err = tx.ListReputationProjectionsByAgentVersion(ctx, "tenant-1", "agent-v1")
		return err
	}))
	require.Len(t, got, 2)
	keys := make(map[string]struct{})
	for _, r := range got {
		require.Equal(t, "tenant-1", r.TenantID)
		require.Equal(t, "agent-v1", r.Projection.Key.AgentVersionID)
		keys[r.Projection.Key.Capability] = struct{}{}
	}
	require.Contains(t, keys, "go")
	require.Contains(t, keys, "python")
}

func TestProjectionRepositoryGetByKeyNotFound(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	repo := reputationpostgres.NewProjectionRepository(db)
	_, err := repo.GetByKey(ctx, "tenant-1", newProjection("agent-v1", "go", "code").Key)
	require.ErrorIs(t, err, appdomain.ErrNotFound)
	require.Equal(t, "not_found", appdomain.CodeOf(err))
}

func newProjection(agentVersionID, capability, taskType string) *reputationdomain.Projection {
	p := reputationdomain.NewProjection(agentVersionID, capability, taskType)
	p.TotalReviews = 1
	p.AcceptedCount = 1
	p.PassRate = 1.0
	p.AvgReviewCostCents = 100
	p.AvgReviewLatencyMs = 1000
	return p
}

func requireEqualProjection(t *testing.T, want, got *reputationdomain.Projection) {
	t.Helper()
	require.Equal(t, want.Key, got.Key)
	require.Equal(t, want.TotalReviews, got.TotalReviews)
	require.Equal(t, want.AcceptedCount, got.AcceptedCount)
	require.Equal(t, want.RejectedCount, got.RejectedCount)
	require.Equal(t, want.RevisionRequestedCount, got.RevisionRequestedCount)
	require.InDelta(t, want.PassRate, got.PassRate, 1e-9)
	require.InDelta(t, want.ReworkRate, got.ReworkRate, 1e-9)
	require.InDelta(t, want.AvgReviewCostCents, got.AvgReviewCostCents, 1e-9)
	require.InDelta(t, want.AvgReviewLatencyMs, got.AvgReviewLatencyMs, 1e-9)
	require.Equal(t, want.SampleSizeHint, got.SampleSizeHint)
	require.Equal(t, want.AlgorithmVersion, got.AlgorithmVersion)
}

func saveProjectionForTenant(ctx context.Context, db *pgxpool.Pool, tenantID string, p reputationdomain.Projection) error {
	_, err := db.Exec(ctx, `
		INSERT INTO reputation_projections (
			tenant_id, id, agent_version_id, capability, task_type,
			total_reviews, accepted_count, rejected_count, revision_requested_count,
			pass_rate, rework_rate, avg_review_cost_cents, avg_review_latency_ms,
			sample_size_hint, algorithm_version, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, clock_timestamp())`,
		tenantID, tenantID+"-"+p.Key.AgentVersionID+"-"+p.Key.Capability+"-"+p.Key.TaskType,
		p.Key.AgentVersionID, p.Key.Capability, p.Key.TaskType,
		p.TotalReviews, p.AcceptedCount, p.RejectedCount, p.RevisionRequestedCount,
		p.PassRate, p.ReworkRate, int64(p.AvgReviewCostCents), int64(p.AvgReviewLatencyMs),
		p.SampleSizeHint, p.AlgorithmVersion,
	)
	return err
}

func loadProjectionForTenant(ctx context.Context, db *pgxpool.Pool, tenantID string, key reputationdomain.ProjectionKey) (*reputationdomain.Projection, error) {
	var p reputationdomain.Projection
	p.Key = key
	err := db.QueryRow(ctx, `
		SELECT total_reviews, accepted_count, rejected_count, revision_requested_count,
		       pass_rate, rework_rate, avg_review_cost_cents, avg_review_latency_ms,
		       sample_size_hint, algorithm_version
		FROM reputation_projections
		WHERE tenant_id=$1 AND agent_version_id=$2 AND capability=$3 AND task_type=$4`,
		tenantID, key.AgentVersionID, key.Capability, key.TaskType,
	).Scan(
		&p.TotalReviews, &p.AcceptedCount, &p.RejectedCount, &p.RevisionRequestedCount,
		&p.PassRate, &p.ReworkRate, &p.AvgReviewCostCents, &p.AvgReviewLatencyMs,
		&p.SampleSizeHint, &p.AlgorithmVersion,
	)
	if err == pgx.ErrNoRows {
		return nil, appdomain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
