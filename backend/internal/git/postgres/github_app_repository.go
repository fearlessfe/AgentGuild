package postgres

import (
	"context"
	"database/sql"
	"errors"

	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
	"github.com/jackc/pgx/v5"
)

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type githubAppRepository struct {
	q queryer
}

// NewGitHubAppRepository returns a GitHubAppRepository backed by pool.
func NewGitHubAppRepository(pool queryer) application.GitHubAppRepository {
	return &githubAppRepository{q: pool}
}

func (r *githubAppRepository) Upsert(ctx context.Context, record *application.GitHubAppRecord) error {
	if record == nil {
		return errors.New("github app record is nil")
	}
	_, err := r.q.Exec(ctx, `
		INSERT INTO github_apps (
			tenant_id, provider, app_id, installation_id, private_key, base_url,
			webhook_secret, client_id, client_secret, app_slug, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, clock_timestamp(), clock_timestamp())
		ON CONFLICT (tenant_id) DO UPDATE SET
			provider = EXCLUDED.provider,
			app_id = EXCLUDED.app_id,
			installation_id = EXCLUDED.installation_id,
			private_key = EXCLUDED.private_key,
			base_url = EXCLUDED.base_url,
			webhook_secret = EXCLUDED.webhook_secret,
			client_id = EXCLUDED.client_id,
			client_secret = EXCLUDED.client_secret,
			app_slug = EXCLUDED.app_slug,
			updated_at = clock_timestamp()`,
		record.TenantID, record.Provider, record.AppID, record.InstallationID,
		record.PrivateKey, record.BaseURL,
		nullString(record.WebhookSecret), nullString(record.ClientID),
		nullString(record.ClientSecret), nullString(record.AppSlug),
	)
	return err
}

func (r *githubAppRepository) GetByTenant(ctx context.Context, tenantID string) (*application.GitHubAppRecord, error) {
	var record application.GitHubAppRecord
	var webhookSecret, clientID, clientSecret, appSlug sql.NullString
	err := r.q.QueryRow(ctx, `
		SELECT tenant_id, provider, app_id, installation_id, private_key, base_url,
			webhook_secret, client_id, client_secret, app_slug, created_at, updated_at
		FROM github_apps
		WHERE tenant_id = $1`, tenantID).Scan(
		&record.TenantID, &record.Provider, &record.AppID, &record.InstallationID,
		&record.PrivateKey, &record.BaseURL,
		&webhookSecret, &clientID, &clientSecret, &appSlug,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, git.ErrGitHubAppNotConfigured
	}
	if err != nil {
		return nil, err
	}
	record.WebhookSecret = webhookSecret.String
	record.ClientID = clientID.String
	record.ClientSecret = clientSecret.String
	record.AppSlug = appSlug.String
	return &record, nil
}

func (r *githubAppRepository) Delete(ctx context.Context, tenantID string) error {
	tag, err := r.q.Exec(ctx, `DELETE FROM github_apps WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return git.ErrGitHubAppNotConfigured
	}
	return nil
}

var _ application.GitHubAppRepository = (*githubAppRepository)(nil)
