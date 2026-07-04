package postgres

import (
	"context"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/review/application"
	"agentguild.dev/agentguild/backend/internal/review/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type lineCommentRepository struct {
	q   queryer
	now func(context.Context) (time.Time, error)
}

func NewLineCommentRepository(pool *pgxpool.Pool) application.LineCommentRepository {
	return &lineCommentRepository{q: pool, now: func(context.Context) (time.Time, error) { return time.Now(), nil }}
}

func (r *lineCommentRepository) Insert(ctx context.Context, comment *domain.LineComment) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO line_comments (
			tenant_id, id, review_id, submission_id, file_path, side,
			line_number, hunk_hash, diff_fingerprint, text, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		comment.TenantID, comment.ID, comment.ReviewID, comment.SubmissionID, comment.FilePath,
		comment.Side, comment.LineNumber, comment.HunkHash, comment.DiffFingerprint, comment.Text,
		comment.CreatedAt,
	)
	return err
}

func (r *lineCommentRepository) ListByReview(ctx context.Context, tenantID, reviewID string) ([]domain.LineComment, error) {
	rows, err := r.q.Query(ctx, `
		SELECT tenant_id, id, review_id, submission_id, file_path, side,
		       line_number, hunk_hash, diff_fingerprint, text, created_at
		FROM line_comments
		WHERE tenant_id=$1 AND review_id=$2
		ORDER BY created_at ASC, id ASC`,
		tenantID, reviewID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []domain.LineComment
	for rows.Next() {
		var comment domain.LineComment
		if err := rows.Scan(
			&comment.TenantID, &comment.ID, &comment.ReviewID, &comment.SubmissionID,
			&comment.FilePath, &comment.Side, &comment.LineNumber,
			&comment.HunkHash, &comment.DiffFingerprint, &comment.Text,
			&comment.CreatedAt,
		); err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}

func (r *lineCommentRepository) GetByID(ctx context.Context, tenantID, commentID string) (*domain.LineComment, error) {
	var comment domain.LineComment
	err := r.q.QueryRow(ctx, `
		SELECT tenant_id, id, review_id, submission_id, file_path, side,
		       line_number, hunk_hash, diff_fingerprint, text, created_at
		FROM line_comments
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, commentID,
	).Scan(
		&comment.TenantID, &comment.ID, &comment.ReviewID, &comment.SubmissionID,
		&comment.FilePath, &comment.Side, &comment.LineNumber,
		&comment.HunkHash, &comment.DiffFingerprint, &comment.Text,
		&comment.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

var _ application.LineCommentRepository = (*lineCommentRepository)(nil)
