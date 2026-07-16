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

type credentialRepository struct {
	q   queryer
	now func(context.Context) (time.Time, error)
}

// NewCredentialRepository returns a credential repository that operates
// directly against pool (outside a transaction, using the local clock).
func NewCredentialRepository(pool *pgxpool.Pool) application.CredentialRepository {
	return &credentialRepository{
		q:   pool,
		now: func(context.Context) (time.Time, error) { return time.Now(), nil },
	}
}

func (r *credentialRepository) Insert(ctx context.Context, record *application.CredentialRecord) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO git_credentials (
			id, tenant_id, execution_id, provider, repo, repo_url, branch,
			base_commit_sha, expires_at, revoked_at, status, request_hash, token_hash, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULL, $10, $11, $12, $13)`,
		record.ID, record.TenantID, record.ExecutionID, record.Provider,
		record.Repo, record.RepoURL, record.Branch, record.BaseCommit, record.ExpiresAt,
		string(record.Status), record.RequestHash, record.TokenHash, record.CreatedAt,
	)
	return err
}

func (r *credentialRepository) GetByID(ctx context.Context, tenantID, id string) (*application.CredentialRecord, error) {
	return r.scanOne(ctx, `
		SELECT id, tenant_id, execution_id, provider, repo, repo_url, branch,
		       base_commit_sha, expires_at, revoked_at, status, request_hash, token_hash, created_at
		FROM git_credentials
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, id,
	)
}

func (r *credentialRepository) GetByExecutionID(ctx context.Context, tenantID, executionID string) (*application.CredentialRecord, error) {
	return r.scanOne(ctx, `
		SELECT id, tenant_id, execution_id, provider, repo, repo_url, branch,
		       base_commit_sha, expires_at, revoked_at, status, request_hash, token_hash, created_at
		FROM git_credentials
		WHERE tenant_id=$1 AND execution_id=$2`,
		tenantID, executionID,
	)
}

func (r *credentialRepository) GetByExecutionIDForUpdate(ctx context.Context, tenantID, executionID string) (*application.CredentialRecord, error) {
	return r.scanOne(ctx, `
		SELECT id, tenant_id, execution_id, provider, repo, repo_url, branch,
		       base_commit_sha, expires_at, revoked_at, status, request_hash, token_hash, created_at
		FROM git_credentials
		WHERE tenant_id=$1 AND execution_id=$2
		FOR UPDATE`,
		tenantID, executionID,
	)
}

func (r *credentialRepository) Update(ctx context.Context, record *application.CredentialRecord) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE git_credentials
		SET provider=$3, repo=$4, repo_url=$5, branch=$6, base_commit_sha=$7,
		    expires_at=$8, status=$9, request_hash=$10, token_hash=$11
		WHERE tenant_id=$1 AND id=$2 AND revoked_at IS NULL`,
		record.TenantID, record.ID, record.Provider, record.Repo, record.RepoURL,
		record.Branch, record.BaseCommit, record.ExpiresAt, string(record.Status), record.RequestHash, record.TokenHash,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return r.classifyMissing(ctx, record.TenantID, record.ID)
	}
	return nil
}

func (r *credentialRepository) Reactivate(ctx context.Context, record *application.CredentialRecord) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE git_credentials
		SET provider=$3, repo=$4, repo_url=$5, branch=$6, base_commit_sha=$7,
		    expires_at=$8, revoked_at=NULL, status=$9, request_hash=$10, token_hash=$11
		WHERE tenant_id=$1 AND id=$2 AND revoked_at IS NOT NULL`,
		record.TenantID, record.ID, record.Provider, record.Repo, record.RepoURL,
		record.Branch, record.BaseCommit, record.ExpiresAt, string(record.Status), record.RequestHash, record.TokenHash,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return git.ErrAlreadyIssued
	}
	return nil
}

func (r *credentialRepository) Revoke(ctx context.Context, tenantID, executionID string) error {
	now, err := r.now(ctx)
	if err != nil {
		return err
	}
	tag, err := r.q.Exec(ctx, `
		UPDATE git_credentials
		SET revoked_at=$3, status='revoked'
		WHERE tenant_id=$1 AND execution_id=$2 AND revoked_at IS NULL`,
		tenantID, executionID, now,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return git.ErrCredentialNotFound
	}
	return nil
}

func (r *credentialRepository) scanOne(ctx context.Context, query string, args ...any) (*application.CredentialRecord, error) {
	var record application.CredentialRecord
	var revokedAt *time.Time
	var status string
	err := r.q.QueryRow(ctx, query, args...).Scan(
		&record.ID, &record.TenantID, &record.ExecutionID,
		&record.Provider, &record.Repo, &record.RepoURL, &record.Branch,
		&record.BaseCommit, &record.ExpiresAt, &revokedAt, &status,
		&record.RequestHash, &record.TokenHash, &record.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, git.ErrCredentialNotFound
	}
	if err != nil {
		return nil, err
	}
	record.RevokedAt = revokedAt
	record.Status = gitdomain.CredentialStatus(status)
	return &record, nil
}

func (r *credentialRepository) classifyMissing(ctx context.Context, tenantID, id string) error {
	var revokedAt *time.Time
	err := r.q.QueryRow(ctx, `
		SELECT revoked_at FROM git_credentials
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, id,
	).Scan(&revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return git.ErrCredentialNotFound
	}
	if err != nil {
		return err
	}
	if revokedAt != nil {
		return git.ErrCredentialRevoked
	}
	return git.ErrCredentialNotFound
}

var _ application.CredentialRepository = (*credentialRepository)(nil)
