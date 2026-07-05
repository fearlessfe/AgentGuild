package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	appdomain "agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/review/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type reviewRepository struct {
	q   queryer
	now func(context.Context) (time.Time, error)
}

func NewReviewRepository(pool *pgxpool.Pool) application.ReviewRepository {
	return &reviewRepository{q: pool, now: func(context.Context) (time.Time, error) { return time.Now(), nil }}
}

func (r *reviewRepository) Insert(ctx context.Context, review *domain.Review) error {
	now, err := r.now(ctx)
	if err != nil {
		return err
	}
	scores, err := json.Marshal(review.RubricScores)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO reviews (
			tenant_id, id, submission_id, reviewer_id, rubric_version_id,
			rubric_scores, summary, status, final_decision, submitted_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)`,
		review.TenantID, review.ID, review.SubmissionID, review.ReviewerID, review.RubricVersionID,
		scores, nullString(review.Summary), review.Status, nullString(string(review.FinalDecision)),
		nullTime(review.SubmittedAt), now,
	)
	return err
}

func (r *reviewRepository) Update(ctx context.Context, review *domain.Review) error {
	now, err := r.now(ctx)
	if err != nil {
		return err
	}
	scores, err := json.Marshal(review.RubricScores)
	if err != nil {
		return err
	}
	tag, err := r.q.Exec(ctx, `
		UPDATE reviews
		SET rubric_scores=$3, summary=$4, status=$5, final_decision=$6, submitted_at=$7, updated_at=$8
		WHERE tenant_id=$1 AND id=$2`,
		review.TenantID, review.ID,
		scores, nullString(review.Summary), review.Status, nullString(string(review.FinalDecision)),
		nullTime(review.SubmittedAt), now,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return appdomain.ErrNotFound
	}
	return nil
}

func (r *reviewRepository) GetByID(ctx context.Context, tenantID, reviewID string) (*domain.Review, error) {
	var review domain.Review
	var summary, decision sql.NullString
	var submittedAt *time.Time
	var scores []byte
	err := r.q.QueryRow(ctx, `
		SELECT tenant_id, id, submission_id, reviewer_id, rubric_version_id,
		       rubric_scores, summary, status, final_decision, submitted_at, created_at
		FROM reviews
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, reviewID,
	).Scan(
		&review.TenantID, &review.ID, &review.SubmissionID, &review.ReviewerID, &review.RubricVersionID,
		&scores, &summary, &review.Status, &decision, &submittedAt,
		&review.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, appdomain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	review.Summary = summary.String
	review.FinalDecision = domain.Decision(decision.String)
	review.SubmittedAt = derefTime(submittedAt)
	if len(scores) > 0 {
		if err := json.Unmarshal(scores, &review.RubricScores); err != nil {
			return nil, err
		}
	}
	return &review, nil
}

func (r *reviewRepository) ListBySubmission(ctx context.Context, tenantID, submissionID string) ([]domain.Review, error) {
	rows, err := r.q.Query(ctx, `
		SELECT tenant_id, id, submission_id, reviewer_id, rubric_version_id,
		       rubric_scores, summary, status, final_decision, submitted_at, created_at
		FROM reviews
		WHERE tenant_id=$1 AND submission_id=$2
		ORDER BY created_at DESC, id ASC`,
		tenantID, submissionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []domain.Review
	for rows.Next() {
		review, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, *review)
	}
	return reviews, rows.Err()
}

func (r *reviewRepository) ListUnprojected(ctx context.Context, batchSize int) ([]application.ReviewSignalRecord, error) {
	if batchSize <= 0 {
		return nil, &appdomain.Error{Code: "invalid_argument", Message: "batch_size is invalid", Field: "batch_size"}
	}
	rows, err := r.q.Query(ctx, `
		SELECT
			r.tenant_id,
			r.id,
			e.agent_version_id,
			COALESCE(rp.capabilities[1], t.type) AS capability,
			t.type AS task_type,
			r.final_decision,
			0 AS cost_cents,
			COALESCE(EXTRACT(EPOCH FROM (r.submitted_at - e.started_at)) * 1000, 0)::bigint AS latency_ms
		FROM reviews r
		JOIN executions e ON e.tenant_id = r.tenant_id AND e.id = r.submission_id
		JOIN tasks t ON t.tenant_id = e.tenant_id AND t.id = e.task_id
		JOIN reviewer_profiles rp ON rp.tenant_id = r.tenant_id AND rp.id = r.reviewer_id
		WHERE r.status = 'submitted' AND r.projected_at IS NULL
		ORDER BY r.tenant_id, r.id
		LIMIT $1
		FOR UPDATE OF r SKIP LOCKED`, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []application.ReviewSignalRecord
	for rows.Next() {
		var rec application.ReviewSignalRecord
		var decision string
		if err := rows.Scan(
			&rec.TenantID, &rec.ReviewID, &rec.AgentVersionID, &rec.Capability, &rec.TaskType,
			&decision, &rec.CostCents, &rec.LatencyMs,
		); err != nil {
			return nil, err
		}
		rec.Decision = domain.Decision(decision)
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (r *reviewRepository) MarkProjected(ctx context.Context, tenantID, reviewID string) error {
	now, err := r.now(ctx)
	if err != nil {
		return err
	}
	tag, err := r.q.Exec(ctx, `
		UPDATE reviews
		SET projected_at=$3, updated_at=$3
		WHERE tenant_id=$1 AND id=$2 AND projected_at IS NULL`,
		tenantID, reviewID, now,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return appdomain.ErrNotFound
	}
	return nil
}

var _ application.ReviewRepository = (*reviewRepository)(nil)

// NewReviewRepositoryFromTx returns a review repository bound to an existing pgx transaction.
func NewReviewRepositoryFromTx(tx pgx.Tx, now func(context.Context) (time.Time, error)) application.ReviewRepository {
	return &reviewRepository{q: tx, now: now}
}

type reviewScanner interface {
	Scan(...any) error
}

func scanReview(row reviewScanner) (*domain.Review, error) {
	var review domain.Review
	var summary, decision sql.NullString
	var submittedAt *time.Time
	var scores []byte
	if err := row.Scan(
		&review.TenantID, &review.ID, &review.SubmissionID, &review.ReviewerID, &review.RubricVersionID,
		&scores, &summary, &review.Status, &decision, &submittedAt,
		&review.CreatedAt,
	); err != nil {
		return nil, err
	}
	review.Summary = summary.String
	review.FinalDecision = domain.Decision(decision.String)
	review.SubmittedAt = derefTime(submittedAt)
	if len(scores) > 0 {
		if err := json.Unmarshal(scores, &review.RubricScores); err != nil {
			return nil, err
		}
	}
	return &review, nil
}
