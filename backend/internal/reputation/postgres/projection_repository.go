// Package postgres provides PostgreSQL persistence for reputation projections.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	appdomain "agentguild.dev/agentguild/backend/internal/domain"
	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// queryer is the minimal surface used by the projection repository. It is
// satisfied by both *pgxpool.Pool and pgx.Tx.
type queryer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type projectionRepository struct {
	q   queryer
	now func(context.Context) (time.Time, error)
}

// NewProjectionRepository creates a projection repository backed by a pool.
func NewProjectionRepository(pool *pgxpool.Pool) reputationapp.ProjectionRepository {
	return &projectionRepository{
		q: pool,
		now: func(ctx context.Context) (time.Time, error) {
			var t time.Time
			err := pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&t)
			return t, err
		},
	}
}

// NewProjectionRepositoryFromTx creates a projection repository bound to an
// existing pgx transaction. The supplied now function provides transaction-time.
func NewProjectionRepositoryFromTx(tx pgx.Tx, now func(context.Context) (time.Time, error)) reputationapp.ProjectionRepository {
	return &projectionRepository{q: tx, now: now}
}

// GetByKey returns the projection for a single tenant/key combination.
func (r *projectionRepository) GetByKey(ctx context.Context, tenantID string, key reputationdomain.ProjectionKey) (*reputationdomain.Projection, error) {
	record, err := scanProjection(r.q.QueryRow(ctx, `
		SELECT tenant_id, agent_version_id, capability, task_type,
		       total_reviews, accepted_count, rejected_count, revision_requested_count,
		       pass_rate, rework_rate, avg_review_cost_cents, avg_review_latency_ms,
		       sample_size_hint, algorithm_version
		FROM reputation_projections
		WHERE tenant_id=$1 AND agent_version_id=$2 AND capability=$3 AND task_type=$4`,
		tenantID, key.AgentVersionID, key.Capability, key.TaskType,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, appdomain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &record.Projection, nil
}

// Save inserts a new projection. It returns an error if a projection already
// exists for the same tenant/key.
func (r *projectionRepository) Save(ctx context.Context, record reputationapp.ProjectionRecord) error {
	t, err := r.now(ctx)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO reputation_projections (
			tenant_id, id, agent_version_id, capability, task_type,
			total_reviews, accepted_count, rejected_count, revision_requested_count,
			pass_rate, rework_rate, avg_review_cost_cents, avg_review_latency_ms,
			sample_size_hint, algorithm_version, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		record.TenantID, projectionID(record), record.Projection.Key.AgentVersionID, record.Projection.Key.Capability, record.Projection.Key.TaskType,
		record.Projection.TotalReviews, record.Projection.AcceptedCount, record.Projection.RejectedCount, record.Projection.RevisionRequestedCount,
		record.Projection.PassRate, record.Projection.ReworkRate, nullableCents(record.Projection.AvgReviewCostCents),
		nullableMs(record.Projection.AvgReviewLatencyMs), record.Projection.SampleSizeHint, record.Projection.AlgorithmVersion, t,
	)
	return err
}

// Upsert inserts or updates a projection.
func (r *projectionRepository) Upsert(ctx context.Context, record reputationapp.ProjectionRecord) error {
	return UpsertProjection(ctx, r.q, r.now, record)
}

// ListByAgentVersion returns all projections for an agent version scoped to a tenant.
func (r *projectionRepository) ListByAgentVersion(ctx context.Context, tenantID, agentVersionID string) ([]reputationapp.ProjectionRecord, error) {
	return ListProjectionsByAgentVersion(ctx, r.q, tenantID, agentVersionID)
}

// UpsertProjection inserts or updates a reputation projection within the
// supplied queryer. It is the implementation shared by the transaction method
// and can also be used directly by tests or other repositories.
func UpsertProjection(ctx context.Context, q queryer, now func(context.Context) (time.Time, error), p reputationapp.ProjectionRecord) error {
	t, err := now(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO reputation_projections (
			tenant_id, id, agent_version_id, capability, task_type,
			total_reviews, accepted_count, rejected_count, revision_requested_count,
			pass_rate, rework_rate, avg_review_cost_cents, avg_review_latency_ms,
			sample_size_hint, algorithm_version, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (tenant_id, agent_version_id, capability, task_type)
		DO UPDATE SET
			total_reviews=EXCLUDED.total_reviews,
			accepted_count=EXCLUDED.accepted_count,
			rejected_count=EXCLUDED.rejected_count,
			revision_requested_count=EXCLUDED.revision_requested_count,
			pass_rate=EXCLUDED.pass_rate,
			rework_rate=EXCLUDED.rework_rate,
			avg_review_cost_cents=EXCLUDED.avg_review_cost_cents,
			avg_review_latency_ms=EXCLUDED.avg_review_latency_ms,
			sample_size_hint=EXCLUDED.sample_size_hint,
			algorithm_version=EXCLUDED.algorithm_version,
			updated_at=EXCLUDED.updated_at`,
		p.TenantID, projectionID(p), p.Projection.Key.AgentVersionID, p.Projection.Key.Capability, p.Projection.Key.TaskType,
		p.Projection.TotalReviews, p.Projection.AcceptedCount, p.Projection.RejectedCount, p.Projection.RevisionRequestedCount,
		p.Projection.PassRate, p.Projection.ReworkRate, nullableCents(p.Projection.AvgReviewCostCents),
		nullableMs(p.Projection.AvgReviewLatencyMs), p.Projection.SampleSizeHint, p.Projection.AlgorithmVersion, t,
	)
	return err
}

// ListProjectionsByAgentVersion returns all projections for an agent version.
func ListProjectionsByAgentVersion(ctx context.Context, q queryer, tenantID, agentVersionID string) ([]reputationapp.ProjectionRecord, error) {
	rows, err := q.Query(ctx, `
		SELECT tenant_id, agent_version_id, capability, task_type,
		       total_reviews, accepted_count, rejected_count, revision_requested_count,
		       pass_rate, rework_rate, avg_review_cost_cents, avg_review_latency_ms,
		       sample_size_hint, algorithm_version
		FROM reputation_projections
		WHERE tenant_id=$1 AND agent_version_id=$2
		ORDER BY capability, task_type`,
		tenantID, agentVersionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []reputationapp.ProjectionRecord
	for rows.Next() {
		record, err := scanProjection(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func projectionID(p reputationapp.ProjectionRecord) string {
	return p.TenantID + ":" + p.Projection.Key.AgentVersionID + ":" + p.Projection.Key.Capability + ":" + p.Projection.Key.TaskType
}

func nullableCents(v float64) any {
	if v == 0 {
		return nil
	}
	return int64(v)
}

func nullableMs(v float64) any {
	if v == 0 {
		return nil
	}
	return int64(v)
}

type projectionScanner interface {
	Scan(...any) error
}

func scanProjection(row projectionScanner) (reputationapp.ProjectionRecord, error) {
	var record reputationapp.ProjectionRecord
	var passRate, reworkRate sql.NullFloat64
	var avgCost, avgLatency sql.NullInt64
	err := row.Scan(
		&record.TenantID,
		&record.Projection.Key.AgentVersionID,
		&record.Projection.Key.Capability,
		&record.Projection.Key.TaskType,
		&record.Projection.TotalReviews,
		&record.Projection.AcceptedCount,
		&record.Projection.RejectedCount,
		&record.Projection.RevisionRequestedCount,
		&passRate, &reworkRate,
		&avgCost, &avgLatency,
		&record.Projection.SampleSizeHint,
		&record.Projection.AlgorithmVersion,
	)
	if err != nil {
		return reputationapp.ProjectionRecord{}, err
	}
	if passRate.Valid {
		record.Projection.PassRate = passRate.Float64
	}
	if reworkRate.Valid {
		record.Projection.ReworkRate = reworkRate.Float64
	}
	if avgCost.Valid {
		record.Projection.AvgReviewCostCents = float64(avgCost.Int64)
	}
	if avgLatency.Valid {
		record.Projection.AvgReviewLatencyMs = float64(avgLatency.Int64)
	}
	return record, nil
}

var _ reputationapp.ProjectionRepository = (*projectionRepository)(nil)
