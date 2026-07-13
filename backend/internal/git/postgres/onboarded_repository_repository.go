package postgres

import (
	"context"
	"database/sql"
	"errors"

	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
	"github.com/jackc/pgx/v5"
)

type onboardedRepositoryRepository struct {
	q queryer
}

// NewOnboardedRepositoryRepository returns an OnboardedRepositoryStore backed by pool.
func NewOnboardedRepositoryRepository(pool queryer) application.OnboardedRepositoryStore {
	return &onboardedRepositoryRepository{q: pool}
}

func (r *onboardedRepositoryRepository) ListOnboardedRepositories(ctx context.Context, tenantID string) ([]application.OnboardedRepositoryRecord, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, tenant_id, source_type, full_name, default_branch, visibility, github_app_id, created_at, updated_at
		FROM onboarded_repositories
		WHERE tenant_id = $1
		ORDER BY full_name, source_type, id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]application.OnboardedRepositoryRecord, 0)
	for rows.Next() {
		record, err := scanOnboardedRepository(rows)
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

func (r *onboardedRepositoryRepository) GetOnboardedRepositoryByFullName(ctx context.Context, tenantID, fullName string) (*application.OnboardedRepositoryRecord, error) {
	return scanOnboardedRepository(r.q.QueryRow(ctx, `
		SELECT id, tenant_id, source_type, full_name, default_branch, visibility, github_app_id, created_at, updated_at
		FROM onboarded_repositories
		WHERE tenant_id = $1 AND full_name = $2`, tenantID, fullName))
}

func (r *onboardedRepositoryRepository) UpsertOnboardedRepository(ctx context.Context, record *application.OnboardedRepositoryRecord) error {
	if record == nil {
		return errors.New("onboarded repository record is nil")
	}
	var githubAppID sql.NullString
	err := r.q.QueryRow(ctx, `
		INSERT INTO onboarded_repositories (
			tenant_id, id, source_type, full_name, default_branch, visibility, github_app_id, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			CASE WHEN $3 = 'github_app'
				THEN COALESCE($7, (SELECT id FROM github_apps WHERE tenant_id = $1 AND is_default))
				ELSE NULL
			END,
			clock_timestamp(), clock_timestamp()
		)
		ON CONFLICT (tenant_id, full_name) DO UPDATE SET
			default_branch = EXCLUDED.default_branch,
			visibility = EXCLUDED.visibility,
			updated_at = clock_timestamp()
		WHERE onboarded_repositories.source_type = EXCLUDED.source_type
			AND onboarded_repositories.github_app_id IS NOT DISTINCT FROM EXCLUDED.github_app_id
		RETURNING id, github_app_id, created_at, updated_at`,
		record.TenantID, record.ID, record.SourceType, record.FullName,
		record.DefaultBranch, record.Visibility, nullString(record.GitHubAppID),
	).Scan(&record.ID, &githubAppID, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return git.ErrRepositoryBindingConflict
	}
	if err != nil {
		return err
	}
	record.GitHubAppID = githubAppID.String
	return nil
}

func scanOnboardedRepository(row rowScanner) (*application.OnboardedRepositoryRecord, error) {
	var record application.OnboardedRepositoryRecord
	var githubAppID sql.NullString
	err := row.Scan(
		&record.ID, &record.TenantID, &record.SourceType, &record.FullName,
		&record.DefaultBranch, &record.Visibility, &githubAppID, &record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	record.GitHubAppID = githubAppID.String
	return &record, nil
}

func (r *onboardedRepositoryRepository) DeleteOnboardedRepository(ctx context.Context, tenantID, id string) error {
	_, err := r.q.Exec(ctx, `
		DELETE FROM onboarded_repositories
		WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	return err
}

var _ application.OnboardedRepositoryStore = (*onboardedRepositoryRepository)(nil)
