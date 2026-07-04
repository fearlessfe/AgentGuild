package postgres

import (
	"context"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type submissionRepository struct {
	q   queryer
	now func(context.Context) (time.Time, error)
}

// NewSubmissionRepository returns a submission repository that operates
// directly against pool (outside a transaction, using the local clock).
func NewSubmissionRepository(pool *pgxpool.Pool) application.SubmissionRepository {
	return &submissionRepository{
		q:   pool,
		now: func(context.Context) (time.Time, error) { return time.Now(), nil },
	}
}

func (r *submissionRepository) Save(ctx context.Context, record *application.SubmissionRecord) error {
	now, err := r.now(ctx)
	if err != nil {
		return err
	}

	_, err = r.q.Exec(ctx, `
		INSERT INTO submissions (
			id, tenant_id, task_id, execution_id, branch, commit_sha, base_commit_sha,
			summary, test_declaration, evidence, diff_fingerprint, status,
			validation_job_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, ($10)::jsonb, $11, $12, $13, $14, $15)
		ON CONFLICT (tenant_id, id) DO UPDATE SET
			branch = EXCLUDED.branch,
			commit_sha = EXCLUDED.commit_sha,
			base_commit_sha = EXCLUDED.base_commit_sha,
			summary = EXCLUDED.summary,
			test_declaration = EXCLUDED.test_declaration,
			evidence = EXCLUDED.evidence,
			diff_fingerprint = EXCLUDED.diff_fingerprint,
			status = EXCLUDED.status,
			validation_job_id = EXCLUDED.validation_job_id,
			updated_at = $15`,
		record.ID, record.TenantID, record.TaskID, record.ExecutionID, record.Branch,
		record.CommitSHA, record.BaseCommitSHA, record.Summary, record.TestDeclaration,
		record.Evidence, record.DiffFingerprint, string(record.Status), record.ValidationJobID,
		record.CreatedAt, now,
	)
	return err
}

func (r *submissionRepository) GetByID(ctx context.Context, tenantID, id string) (*application.SubmissionRecord, error) {
	return r.scanOne(ctx, `
		SELECT id, tenant_id, task_id, execution_id, branch, commit_sha, base_commit_sha,
		       summary, test_declaration, evidence, diff_fingerprint, status,
		       validation_job_id, created_at, updated_at
		FROM submissions
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, id,
	)
}

func (r *submissionRepository) GetByExecutionID(ctx context.Context, tenantID, executionID string) ([]*application.SubmissionRecord, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, tenant_id, task_id, execution_id, branch, commit_sha, base_commit_sha,
		       summary, test_declaration, evidence, diff_fingerprint, status,
		       validation_job_id, created_at, updated_at
		FROM submissions
		WHERE tenant_id=$1 AND execution_id=$2
		ORDER BY created_at ASC`,
		tenantID, executionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*application.SubmissionRecord
	for rows.Next() {
		record, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *submissionRepository) scanOne(ctx context.Context, query string, args ...any) (*application.SubmissionRecord, error) {
	row := r.q.QueryRow(ctx, query, args...)
	record, err := r.scanRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, git.ErrSubmissionNotFound
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func (r *submissionRepository) scanRow(row scanner) (*application.SubmissionRecord, error) {
	var record application.SubmissionRecord
	var status string
	err := row.Scan(
		&record.ID, &record.TenantID, &record.TaskID, &record.ExecutionID,
		&record.Branch, &record.CommitSHA, &record.BaseCommitSHA,
		&record.Summary, &record.TestDeclaration, &record.Evidence,
		&record.DiffFingerprint, &status,
		&record.ValidationJobID, &record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	record.Status = gitdomain.SubmissionStatus(status)
	return &record, nil
}

var _ application.SubmissionRepository = (*submissionRepository)(nil)
