package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/publictask/application"
	"agentguild.dev/agentguild/backend/internal/publictask/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Insert(ctx context.Context, p *domain.Projection) error {
	if p == nil || p.Status != domain.StatusPublished {
		return domain.ErrInvalidArgument
	}
	steps, constraints, nonGoals, risks, criteria, evidence, err := encodeFields(p)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO public_task_projections (
			id, resource_tenant_id, task_id, task_specification_version_id,
			canonical_repository, source_issue_url, issue_revision, base_commit,
			title, summary, problem_diagnosis, impact, proposed_solution,
			implementation_steps, constraints, non_goals, risks,
			acceptance_criteria, evidence_refs, quality_level, status, published_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22
		)`, p.ID, p.ResourceTenantID, p.TaskID, p.TaskSpecificationVersionID,
		p.CanonicalRepository, p.SourceIssueURL, p.IssueRevision, p.BaseCommit,
		p.Title, p.Summary, p.ProblemDiagnosis, p.Impact, p.ProposedSolution,
		steps, constraints, nonGoals, risks, criteria, evidence,
		p.QualityLevel, p.Status, p.PublishedAt)
	return writeError(err)
}

func (r *Repository) GetByID(ctx context.Context, id string) (*domain.Projection, error) {
	p, err := scanProjection(r.pool.QueryRow(ctx, selectProjection+` WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return p, err
}

func (r *Repository) ListPublished(ctx context.Context, limit int) ([]domain.Projection, error) {
	return r.ListPublishedPage(ctx, application.PublishedPageQuery{Limit: limit})
}

func (r *Repository) ListPublishedPage(ctx context.Context, query application.PublishedPageQuery) ([]domain.Projection, error) {
	limit := query.Limit
	if limit < 1 || limit > 101 || (query.AfterPublishedAt.IsZero() != (query.AfterID == "")) {
		return nil, domain.ErrInvalidArgument
	}
	rows, err := r.pool.Query(ctx, selectProjection+`
		WHERE status='published'
		  AND ($2::timestamptz IS NULL OR published_at < $2 OR (published_at = $2 AND id > $3))
		ORDER BY published_at DESC, id
		LIMIT $1`, limit, nullableTime(query.AfterPublishedAt), query.AfterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Projection, 0)
	for rows.Next() {
		item, err := scanProjection(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func (r *Repository) Update(ctx context.Context, p *domain.Projection) error {
	if p == nil || p.Status != domain.StatusRevoked || p.RevokedAt == nil || p.RevocationActor == "" || p.RevocationReason == "" {
		return domain.ErrInvalidArgument
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE public_task_projections
		SET status='revoked', revoked_at=$2, revocation_actor=$3, revocation_reason=$4
		WHERE id=$1 AND status='published'`, p.ID, p.RevokedAt, p.RevocationActor, p.RevocationReason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrStateConflict
	}
	return nil
}

const selectProjection = `
	SELECT id, resource_tenant_id, task_id, task_specification_version_id,
	       canonical_repository, source_issue_url, issue_revision, base_commit,
	       title, summary, problem_diagnosis, impact, proposed_solution,
	       implementation_steps, constraints, non_goals, risks,
	       acceptance_criteria, evidence_refs, quality_level, status, published_at,
	       revoked_at, COALESCE(revocation_actor, ''), COALESCE(revocation_reason, '')
	FROM public_task_projections`

type scanner interface{ Scan(...any) error }

func scanProjection(row scanner) (*domain.Projection, error) {
	var p domain.Projection
	var steps, constraints, nonGoals, risks, criteria, evidence []byte
	err := row.Scan(
		&p.ID, &p.ResourceTenantID, &p.TaskID, &p.TaskSpecificationVersionID,
		&p.CanonicalRepository, &p.SourceIssueURL, &p.IssueRevision, &p.BaseCommit,
		&p.Title, &p.Summary, &p.ProblemDiagnosis, &p.Impact, &p.ProposedSolution,
		&steps, &constraints, &nonGoals, &risks, &criteria, &evidence,
		&p.QualityLevel, &p.Status, &p.PublishedAt, &p.RevokedAt,
		&p.RevocationActor, &p.RevocationReason,
	)
	if err != nil {
		return nil, err
	}
	for _, target := range []struct {
		raw   []byte
		value any
	}{
		{steps, &p.ImplementationSteps}, {constraints, &p.Constraints},
		{nonGoals, &p.NonGoals}, {risks, &p.Risks},
		{criteria, &p.AcceptanceCriteria}, {evidence, &p.EvidenceRefs},
	} {
		if err := json.Unmarshal(target.raw, target.value); err != nil {
			return nil, err
		}
	}
	return &p, nil
}

func encodeFields(p *domain.Projection) ([]byte, []byte, []byte, []byte, []byte, []byte, error) {
	values := []any{p.ImplementationSteps, p.Constraints, p.NonGoals, p.Risks, p.AcceptanceCriteria, p.EvidenceRefs}
	encoded := make([][]byte, len(values))
	for index, value := range values {
		body, err := json.Marshal(value)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		encoded[index] = body
	}
	return encoded[0], encoded[1], encoded[2], encoded[3], encoded[4], encoded[5], nil
}

func writeError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrStateConflict
	}
	return err
}

var _ application.Repository = (*Repository)(nil)
