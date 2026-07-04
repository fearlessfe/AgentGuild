package postgres

import (
	"context"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
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
			id, tenant_id, execution_id, provider, repo_url, branch,
			base_commit_sha, expires_at, revoked_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL, $9)`,
		record.ID, record.TenantID, record.ExecutionID, record.Provider,
		record.RepoURL, record.Branch, record.BaseCommit, record.ExpiresAt,
		record.CreatedAt,
	)
	return err
}

func (r *credentialRepository) GetByID(ctx context.Context, tenantID, id string) (*application.CredentialRecord, error) {
	return r.scanOne(ctx, `
		SELECT id, tenant_id, execution_id, provider, repo_url, branch,
		       base_commit_sha, expires_at, revoked_at, created_at
		FROM git_credentials
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, id,
	)
}

func (r *credentialRepository) GetByExecutionID(ctx context.Context, tenantID, executionID string) (*application.CredentialRecord, error) {
	return r.scanOne(ctx, `
		SELECT id, tenant_id, execution_id, provider, repo_url, branch,
		       base_commit_sha, expires_at, revoked_at, created_at
		FROM git_credentials
		WHERE tenant_id=$1 AND execution_id=$2`,
		tenantID, executionID,
	)
}

func (r *credentialRepository) Update(ctx context.Context, record *application.CredentialRecord) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE git_credentials
		SET provider=$3, repo_url=$4, branch=$5, base_commit_sha=$6, expires_at=$7
		WHERE tenant_id=$1 AND id=$2 AND revoked_at IS NULL`,
		record.TenantID, record.ID, record.Provider, record.RepoURL,
		record.Branch, record.BaseCommit, record.ExpiresAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return git.ErrCredentialNotFound
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
		SET revoked_at=$3
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
	err := r.q.QueryRow(ctx, query, args...).Scan(
		&record.ID, &record.TenantID, &record.ExecutionID,
		&record.Provider, &record.RepoURL, &record.Branch,
		&record.BaseCommit, &record.ExpiresAt, &revokedAt, &record.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, git.ErrCredentialNotFound
	}
	if err != nil {
		return nil, err
	}
	record.RevokedAt = revokedAt
	return &record, nil
}

var _ application.CredentialRepository = (*credentialRepository)(nil)
