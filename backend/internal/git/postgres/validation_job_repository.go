package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type validationJobRepository struct {
	q   queryer
	now func(context.Context) (time.Time, error)
}

// NewValidationJobRepository returns a validation job repository that operates
// directly against pool (outside a transaction).
func NewValidationJobRepository(pool *pgxpool.Pool) application.ValidationJobRepository {
	return &validationJobRepository{
		q:   pool,
		now: func(context.Context) (time.Time, error) { return time.Now(), nil },
	}
}

func (r *validationJobRepository) Insert(ctx context.Context, job *gitdomain.ValidationJob) error {
	stepsJSON, err := marshalSteps(job.Steps)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO validation_jobs (
			id, tenant_id, submission_id, repo, branch, commit_sha, status, attempt,
			claimed_until, claimed_by, config_version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		job.ID, job.TenantID, job.SubmissionID, job.Repo, job.Branch, job.CommitSHA, string(job.Status), job.Attempt,
		job.ClaimedUntil, job.ClaimedBy, job.ConfigVersion, job.CreatedAt, job.UpdatedAt,
	)
	if err != nil {
		return &domain.Error{Code: "internal", Message: "failed to insert validation job: " + err.Error()}
	}
	for _, step := range job.Steps {
		if err := r.insertStep(ctx, job.TenantID, job.ID, step); err != nil {
			return err
		}
	}
	_ = stepsJSON
	return nil
}

func (r *validationJobRepository) GetByID(ctx context.Context, tenantID, id string) (*gitdomain.ValidationJob, error) {
	job, err := r.scanJob(r.q.QueryRow(ctx, `
		SELECT id, tenant_id, submission_id, repo, branch, commit_sha, status, attempt,
		       claimed_until, claimed_by, config_version, created_at, updated_at
		FROM validation_jobs
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, git.ErrValidationJobNotFound
	}
	if err != nil {
		return nil, err
	}
	steps, err := r.loadSteps(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	job.Steps = steps
	return job, nil
}

func (r *validationJobRepository) GetBySubmissionID(ctx context.Context, tenantID, submissionID string) (*gitdomain.ValidationJob, error) {
	job, err := r.scanJob(r.q.QueryRow(ctx, `
		SELECT id, tenant_id, submission_id, repo, branch, commit_sha, status, attempt,
		       claimed_until, claimed_by, config_version, created_at, updated_at
		FROM validation_jobs
		WHERE tenant_id=$1 AND submission_id=$2`,
		tenantID, submissionID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, git.ErrValidationJobNotFound
	}
	if err != nil {
		return nil, err
	}
	steps, err := r.loadSteps(ctx, tenantID, job.ID)
	if err != nil {
		return nil, err
	}
	job.Steps = steps
	return job, nil
}

func (r *validationJobRepository) ClaimNextPending(ctx context.Context, tenantID, workerID string, now, until time.Time) (*gitdomain.ValidationJob, error) {
	pgxTx, ok := r.q.(pgx.Tx)
	if !ok {
		return nil, &domain.Error{Code: "internal", Message: "ClaimNextPending must run inside a transaction"}
	}
	job, err := r.scanJob(pgxTx.QueryRow(ctx, `
		UPDATE validation_jobs
		SET status='running', attempt=attempt+1, claimed_until=$3, claimed_by=$4, updated_at=$3
		WHERE tenant_id=$1 AND id=(
			SELECT id FROM validation_jobs
			WHERE tenant_id=$1
			  AND status IN ('pending', 'running')
			  AND (claimed_until IS NULL OR claimed_until <= $2)
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, tenant_id, submission_id, repo, branch, commit_sha, status, attempt,
		          claimed_until, claimed_by, config_version, created_at, updated_at`,
		tenantID, now, until, workerID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, &domain.Error{Code: "internal", Message: "failed to claim validation job: " + err.Error()}
	}
	steps, err := r.loadSteps(ctx, tenantID, job.ID)
	if err != nil {
		return nil, err
	}
	job.Steps = steps
	return job, nil
}

func (r *validationJobRepository) Update(ctx context.Context, job *gitdomain.ValidationJob) error {
	_, err := r.q.Exec(ctx, `
		UPDATE validation_jobs
		SET status=$3, attempt=$4, claimed_until=$5, claimed_by=$6, updated_at=$7
		WHERE tenant_id=$1 AND id=$2`,
		job.TenantID, job.ID, string(job.Status), job.Attempt,
		job.ClaimedUntil, job.ClaimedBy, job.UpdatedAt,
	)
	if err != nil {
		return &domain.Error{Code: "internal", Message: "failed to update validation job: " + err.Error()}
	}
	return nil
}

func (r *validationJobRepository) UpdateStep(ctx context.Context, tenantID, jobID string, step gitdomain.Step) error {
	_, err := r.q.Exec(ctx, `
		UPDATE validation_steps
		SET status=$4, log_summary=$5, resource_usage=$6, started_at=$7, finished_at=$8
		WHERE tenant_id=$1 AND job_id=$2 AND step=$3`,
		tenantID, jobID, string(step.Step), string(step.Status), step.LogSummary, step.ResourceUsage, step.StartedAt, step.FinishedAt,
	)
	if err != nil {
		return &domain.Error{Code: "internal", Message: "failed to update validation step: " + err.Error()}
	}
	return nil
}

func (r *validationJobRepository) insertStep(ctx context.Context, tenantID, jobID string, step gitdomain.Step) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO validation_steps (
			tenant_id, job_id, step, status, log_summary, resource_usage,
			started_at, finished_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		tenantID, jobID, string(step.Step), string(step.Status), step.LogSummary,
		step.ResourceUsage, step.StartedAt, step.FinishedAt, step.CreatedAt,
	)
	if err != nil {
		return &domain.Error{Code: "internal", Message: "failed to insert validation step: " + err.Error()}
	}
	return nil
}

func (r *validationJobRepository) loadSteps(ctx context.Context, tenantID, jobID string) ([]gitdomain.Step, error) {
	rows, err := r.q.Query(ctx, `
		SELECT step, status, log_summary, resource_usage, started_at, finished_at, created_at
		FROM validation_steps
		WHERE tenant_id=$1 AND job_id=$2
		ORDER BY id ASC`,
		tenantID, jobID,
	)
	if err != nil {
		return nil, &domain.Error{Code: "internal", Message: "failed to list validation steps: " + err.Error()}
	}
	defer rows.Close()

	var out []gitdomain.Step
	for rows.Next() {
		var step gitdomain.Step
		var status, name string
		if err := rows.Scan(&name, &status, &step.LogSummary, &step.ResourceUsage, &step.StartedAt, &step.FinishedAt, &step.CreatedAt); err != nil {
			return nil, err
		}
		step.Step = gitdomain.ValidationStep(name)
		step.Status = gitdomain.ValidationStepStatus(status)
		out = append(out, step)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *validationJobRepository) scanJob(row scanner) (*gitdomain.ValidationJob, error) {
	var job gitdomain.ValidationJob
	var status string
	err := row.Scan(
		&job.ID, &job.TenantID, &job.SubmissionID, &job.Repo, &job.Branch, &job.CommitSHA, &status, &job.Attempt,
		&job.ClaimedUntil, &job.ClaimedBy, &job.ConfigVersion, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	job.Status = gitdomain.ValidationStatus(status)
	return &job, nil
}

func marshalSteps(steps []gitdomain.Step) ([]byte, error) {
	return json.Marshal(steps)
}

var _ application.ValidationJobRepository = (*validationJobRepository)(nil)
