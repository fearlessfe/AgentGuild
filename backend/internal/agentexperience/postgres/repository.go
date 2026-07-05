package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/agentexperience/application"
	"agentguild.dev/agentguild/backend/internal/agentexperience/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type experienceCandidateRepository struct {
	q queryer
}

// NewExperienceCandidateRepository returns an ExperienceCandidateRepository
// backed by pool.
func NewExperienceCandidateRepository(pool *pgxpool.Pool) application.ExperienceCandidateRepository {
	return &experienceCandidateRepository{q: pool}
}

func (r *experienceCandidateRepository) Create(ctx context.Context, tx application.Tx, c *domain.ExperienceCandidate) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO experience_candidates (
			id, tenant_id, agent_id, source_task_id, source_submission_id, source_review_id,
			evidence_ref, content_hash, applicable_capabilities, tenant_scope,
			sensitivity_class, status, policy_reason, reviewed_by, reviewed_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		c.ID, c.TenantID, c.AgentID, c.SourceTaskID, c.SourceSubmissionID, c.SourceReviewID,
		c.EvidenceRef, c.ContentHash, stringSlice(c.ApplicableCapabilities), c.TenantScope,
		string(c.SensitivityClass), string(c.Status), nullString(c.PolicyReason),
		nullString(c.ReviewedBy), c.ReviewedAt, c.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrStateConflict
		}
		return err
	}
	return nil
}

func (r *experienceCandidateRepository) GetByID(ctx context.Context, tenantID, agentID, candidateID string) (*domain.ExperienceCandidate, error) {
	var c domain.ExperienceCandidate
	var sourceReviewID, policyReason, reviewedBy sql.NullString
	var reviewedAt *time.Time
	err := r.q.QueryRow(ctx, `
		SELECT id, tenant_id, agent_id, source_task_id, source_submission_id, source_review_id,
		       evidence_ref, content_hash, applicable_capabilities, tenant_scope,
		       sensitivity_class, status, policy_reason, reviewed_by, reviewed_at, created_at
		FROM experience_candidates
		WHERE tenant_id=$1 AND agent_id=$2 AND id=$3`,
		tenantID, agentID, candidateID,
	).Scan(
		&c.ID, &c.TenantID, &c.AgentID, &c.SourceTaskID, &c.SourceSubmissionID, &sourceReviewID,
		&c.EvidenceRef, &c.ContentHash, &c.ApplicableCapabilities, &c.TenantScope,
		&c.SensitivityClass, &c.Status, &policyReason, &reviewedBy, &reviewedAt, &c.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.SourceReviewID = sourceReviewID.String
	c.PolicyReason = policyReason.String
	c.ReviewedBy = reviewedBy.String
	c.ReviewedAt = reviewedAt
	return &c, nil
}

func (r *experienceCandidateRepository) ListByAgent(ctx context.Context, tenantID, agentID string) ([]domain.ExperienceCandidate, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, tenant_id, agent_id, source_task_id, source_submission_id, source_review_id,
		       evidence_ref, content_hash, applicable_capabilities, tenant_scope,
		       sensitivity_class, status, policy_reason, reviewed_by, reviewed_at, created_at
		FROM experience_candidates
		WHERE tenant_id=$1 AND agent_id=$2
		ORDER BY created_at DESC`,
		tenantID, agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []domain.ExperienceCandidate
	for rows.Next() {
		c, err := scanCandidateRow(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, *c)
	}
	return candidates, rows.Err()
}

func (r *experienceCandidateRepository) ListByAgentAndStatus(ctx context.Context, tenantID, agentID string, status domain.CandidateStatus) ([]domain.ExperienceCandidate, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, tenant_id, agent_id, source_task_id, source_submission_id, source_review_id,
		       evidence_ref, content_hash, applicable_capabilities, tenant_scope,
		       sensitivity_class, status, policy_reason, reviewed_by, reviewed_at, created_at
		FROM experience_candidates
		WHERE tenant_id=$1 AND agent_id=$2 AND status=$3
		ORDER BY created_at DESC`,
		tenantID, agentID, string(status),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []domain.ExperienceCandidate
	for rows.Next() {
		c, err := scanCandidateRow(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, *c)
	}
	return candidates, rows.Err()
}

func (r *experienceCandidateRepository) UpdateStatus(ctx context.Context, tx application.Tx, c *domain.ExperienceCandidate) error {
	tag, err := tx.Exec(ctx, `
		UPDATE experience_candidates
		SET status=$4,
		    sensitivity_class=$5,
		    policy_reason=COALESCE($6, policy_reason),
		    reviewed_by=COALESCE($7, reviewed_by),
		    reviewed_at=COALESCE($8, reviewed_at)
		WHERE tenant_id=$1 AND agent_id=$2 AND id=$3`,
		c.TenantID, c.AgentID, c.ID,
		string(c.Status), string(c.SensitivityClass), nullString(c.PolicyReason),
		nullString(c.ReviewedBy), c.ReviewedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *experienceCandidateRepository) ListApprovedByAgent(ctx context.Context, tenantID, agentID string) ([]domain.ExperienceCandidate, error) {
	return r.ListByAgentAndStatus(ctx, tenantID, agentID, domain.StatusApproved)
}

var _ application.ExperienceCandidateRepository = (*experienceCandidateRepository)(nil)

type candidateScanner interface {
	Scan(...any) error
}

func scanCandidateRow(row candidateScanner) (*domain.ExperienceCandidate, error) {
	var c domain.ExperienceCandidate
	var sourceReviewID, policyReason, reviewedBy sql.NullString
	var reviewedAt *time.Time
	if err := row.Scan(
		&c.ID, &c.TenantID, &c.AgentID, &c.SourceTaskID, &c.SourceSubmissionID, &sourceReviewID,
		&c.EvidenceRef, &c.ContentHash, &c.ApplicableCapabilities, &c.TenantScope,
		&c.SensitivityClass, &c.Status, &policyReason, &reviewedBy, &reviewedAt, &c.CreatedAt,
	); err != nil {
		return nil, err
	}
	c.SourceReviewID = sourceReviewID.String
	c.PolicyReason = policyReason.String
	c.ReviewedBy = reviewedBy.String
	c.ReviewedAt = reviewedAt
	return &c, nil
}

func stringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
