package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type submissionRepository struct {
	q queryer
}

// NewSubmissionRepository returns a submission repository that operates
// directly against pool (outside a transaction).
func NewSubmissionRepository(pool *pgxpool.Pool) application.SubmissionRepository {
	return &submissionRepository{q: pool}
}

func (r *submissionRepository) Save(ctx context.Context, submission *gitdomain.Submission) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO submissions (
			id, tenant_id, task_id, execution_id, branch, commit_sha, base_commit_sha,
			summary, test_declaration, evidence, diff_fingerprint, status,
			validation_job_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
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
			updated_at = EXCLUDED.updated_at`,
		submission.ID, submission.TenantID, submission.TaskID, submission.ExecutionID, submission.Branch,
		submission.CommitSHA, submission.BaseCommitSHA, submission.Summary, submission.TestDeclaration,
		string(submission.Evidence), submission.DiffFingerprint, string(submission.Status), submission.ValidationJobID,
		submission.CreatedAt, submission.UpdatedAt,
	)
	if err != nil {
		return &domain.Error{Code: "internal", Message: "failed to save submission: " + err.Error()}
	}
	return nil
}

func (r *submissionRepository) GetByID(ctx context.Context, tenantID, id string) (*gitdomain.Submission, error) {
	submission, err := r.scanRow(r.q.QueryRow(ctx, `
		SELECT id, tenant_id, task_id, execution_id, branch, commit_sha, base_commit_sha,
		       summary, test_declaration, evidence, diff_fingerprint, status,
		       validation_job_id, created_at, updated_at
		FROM submissions
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, git.ErrSubmissionNotFound
	}
	if err != nil {
		return nil, &domain.Error{Code: "internal", Message: "failed to get submission: " + err.Error()}
	}
	return submission, nil
}

func (r *submissionRepository) GetByExecutionID(ctx context.Context, tenantID, executionID string) ([]*gitdomain.Submission, error) {
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
		return nil, &domain.Error{Code: "internal", Message: "failed to list submissions: " + err.Error()}
	}
	defer rows.Close()

	var out []*gitdomain.Submission
	for rows.Next() {
		submission, err := r.scanRow(rows)
		if err != nil {
			return nil, &domain.Error{Code: "internal", Message: "failed to scan submission: " + err.Error()}
		}
		out = append(out, submission)
	}
	if err := rows.Err(); err != nil {
		return nil, &domain.Error{Code: "internal", Message: "failed to read submissions: " + err.Error()}
	}
	return out, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func (r *submissionRepository) scanRow(row scanner) (*gitdomain.Submission, error) {
	var submission gitdomain.Submission
	var status string
	var evidence *string
	err := row.Scan(
		&submission.ID, &submission.TenantID, &submission.TaskID, &submission.ExecutionID,
		&submission.Branch, &submission.CommitSHA, &submission.BaseCommitSHA,
		&submission.Summary, &submission.TestDeclaration, &evidence,
		&submission.DiffFingerprint, &status,
		&submission.ValidationJobID, &submission.CreatedAt, &submission.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	submission.Status = gitdomain.SubmissionStatus(status)
	if evidence != nil {
		submission.Evidence = []byte(*evidence)
	}
	return &submission, nil
}

var _ application.SubmissionRepository = (*submissionRepository)(nil)
