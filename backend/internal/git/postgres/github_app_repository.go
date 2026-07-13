package postgres

import (
	"context"
	"crypto/md5"
	"database/sql"
	"errors"
	"fmt"

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
	if record.Provider == "" {
		record.Provider = "github"
	}
	if record.BaseURL == "" {
		record.BaseURL = "https://api.github.com"
	}
	if record.ID == "" {
		existing, err := r.GetDefault(ctx, record.TenantID)
		switch {
		case err == nil:
			record.ID = existing.ID
		case errors.Is(err, git.ErrGitHubAppNotConfigured):
			record.ID = fmt.Sprintf("gha_%x", md5.Sum([]byte(fmt.Sprintf("%s:%d", record.TenantID, record.AppID))))
		default:
			return err
		}
		record.IsDefault = true
	}

	return r.q.QueryRow(ctx, `
		INSERT INTO github_apps (
			tenant_id, id, provider, app_id, installation_id, private_key, base_url,
			webhook_secret, client_id, client_secret, app_slug, installation_account_login,
			is_default, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, clock_timestamp(), clock_timestamp())
		ON CONFLICT (tenant_id, id) DO UPDATE SET
			provider = EXCLUDED.provider,
			app_id = EXCLUDED.app_id,
			installation_id = EXCLUDED.installation_id,
			private_key = EXCLUDED.private_key,
			base_url = EXCLUDED.base_url,
			webhook_secret = EXCLUDED.webhook_secret,
			client_id = EXCLUDED.client_id,
			client_secret = EXCLUDED.client_secret,
			app_slug = EXCLUDED.app_slug,
			installation_account_login = EXCLUDED.installation_account_login,
			is_default = EXCLUDED.is_default,
			updated_at = clock_timestamp()
		RETURNING created_at, updated_at`,
		record.TenantID, record.ID, record.Provider, record.AppID, record.InstallationID,
		record.PrivateKey, record.BaseURL, nullString(record.WebhookSecret),
		nullString(record.ClientID), nullString(record.ClientSecret), nullString(record.AppSlug),
		nullString(record.InstallationAccountLogin), record.IsDefault,
	).Scan(&record.CreatedAt, &record.UpdatedAt)
}

func (r *githubAppRepository) ListByTenant(ctx context.Context, tenantID string) ([]application.GitHubAppRecord, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, tenant_id, provider, app_id, installation_id, private_key, base_url,
			webhook_secret, client_id, client_secret, app_slug, installation_account_login,
			is_default, created_at, updated_at
		FROM github_apps
		WHERE tenant_id = $1
		ORDER BY id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]application.GitHubAppRecord, 0)
	for rows.Next() {
		record, err := scanGitHubApp(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *githubAppRepository) GetByID(ctx context.Context, tenantID, id string) (*application.GitHubAppRecord, error) {
	return r.getOne(ctx, `
		SELECT id, tenant_id, provider, app_id, installation_id, private_key, base_url,
			webhook_secret, client_id, client_secret, app_slug, installation_account_login,
			is_default, created_at, updated_at
		FROM github_apps
		WHERE tenant_id = $1 AND id = $2`, tenantID, id)
}

func (r *githubAppRepository) GetDefault(ctx context.Context, tenantID string) (*application.GitHubAppRecord, error) {
	return r.getOne(ctx, `
		SELECT id, tenant_id, provider, app_id, installation_id, private_key, base_url,
			webhook_secret, client_id, client_secret, app_slug, installation_account_login,
			is_default, created_at, updated_at
		FROM github_apps
		WHERE tenant_id = $1 AND is_default`, tenantID)
}

func (r *githubAppRepository) getOne(ctx context.Context, query string, args ...any) (*application.GitHubAppRecord, error) {
	record, err := scanGitHubApp(r.q.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, git.ErrGitHubAppNotConfigured
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanGitHubApp(row rowScanner) (*application.GitHubAppRecord, error) {
	var record application.GitHubAppRecord
	var webhookSecret, clientID, clientSecret, appSlug, installationAccountLogin sql.NullString
	err := row.Scan(
		&record.ID, &record.TenantID, &record.Provider, &record.AppID, &record.InstallationID,
		&record.PrivateKey, &record.BaseURL, &webhookSecret, &clientID, &clientSecret,
		&appSlug, &installationAccountLogin, &record.IsDefault, &record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	record.WebhookSecret = webhookSecret.String
	record.ClientID = clientID.String
	record.ClientSecret = clientSecret.String
	record.AppSlug = appSlug.String
	record.InstallationAccountLogin = installationAccountLogin.String
	return &record, nil
}

func (r *githubAppRepository) Delete(ctx context.Context, tenantID, id string) error {
	tag, err := r.q.Exec(ctx, `DELETE FROM github_apps WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return git.ErrGitHubAppNotConfigured
	}
	return nil
}

func (r *githubAppRepository) DeleteAndPromoteDefault(ctx context.Context, tenantID, id string) error {
	var found, bound, deleted bool
	err := r.q.QueryRow(ctx, `
		WITH target AS MATERIALIZED (
			SELECT is_default
			FROM github_apps
			WHERE tenant_id = $1 AND id = $2
			FOR UPDATE
		), binding AS MATERIALIZED (
			SELECT 1
			FROM onboarded_repositories
			WHERE tenant_id = $1 AND github_app_id = $2
			LIMIT 1
		), deleted AS (
			DELETE FROM github_apps
			WHERE tenant_id = $1 AND id = $2
				AND EXISTS (SELECT 1 FROM target)
				AND NOT EXISTS (SELECT 1 FROM binding)
			RETURNING is_default
		), promoted AS (
			UPDATE github_apps
			SET is_default = true, updated_at = clock_timestamp()
			WHERE tenant_id = $1
				AND id = (
					SELECT id
					FROM github_apps
					WHERE tenant_id = $1 AND id <> $2
					ORDER BY created_at, id
					LIMIT 1
				)
				AND EXISTS (SELECT 1 FROM deleted WHERE is_default)
			RETURNING id
		)
		SELECT
			EXISTS (SELECT 1 FROM target),
			EXISTS (SELECT 1 FROM binding),
			EXISTS (SELECT 1 FROM deleted)
		FROM (SELECT count(*) FROM promoted) AS ensure_promoted_runs`, tenantID, id,
	).Scan(&found, &bound, &deleted)
	if err != nil {
		return err
	}
	if !found {
		return git.ErrGitHubAppNotConfigured
	}
	if bound {
		return git.ErrGitHubAppInUse
	}
	if !deleted {
		return git.ErrGitHubAppInUse
	}
	return nil
}

var _ application.GitHubAppRepository = (*githubAppRepository)(nil)
