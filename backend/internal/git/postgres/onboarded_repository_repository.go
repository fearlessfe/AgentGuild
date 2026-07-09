package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/git/application"
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
		SELECT id, tenant_id, source_type, full_name, default_branch, visibility, created_at, updated_at
		FROM onboarded_repositories
		WHERE tenant_id = $1
		ORDER BY full_name, source_type, id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]application.OnboardedRepositoryRecord, 0)
	for rows.Next() {
		var record application.OnboardedRepositoryRecord
		if err := rows.Scan(
			&record.ID, &record.TenantID, &record.SourceType, &record.FullName,
			&record.DefaultBranch, &record.Visibility, &record.CreatedAt, &record.UpdatedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *onboardedRepositoryRepository) UpsertOnboardedRepository(ctx context.Context, record *application.OnboardedRepositoryRecord) error {
	if record == nil {
		return errors.New("onboarded repository record is nil")
	}
	return r.q.QueryRow(ctx, `
		INSERT INTO onboarded_repositories (
			tenant_id, id, source_type, full_name, default_branch, visibility, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, clock_timestamp(), clock_timestamp())
		ON CONFLICT (tenant_id, source_type, full_name) DO UPDATE SET
			default_branch = EXCLUDED.default_branch,
			visibility = EXCLUDED.visibility,
			updated_at = clock_timestamp()
		RETURNING id, created_at, updated_at`,
		record.TenantID, record.ID, record.SourceType, record.FullName, record.DefaultBranch, record.Visibility,
	).Scan(&record.ID, &record.CreatedAt, &record.UpdatedAt)
}

func (r *onboardedRepositoryRepository) DeleteOnboardedRepository(ctx context.Context, tenantID, id string) error {
	_, err := r.q.Exec(ctx, `
		DELETE FROM onboarded_repositories
		WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	return err
}

var _ application.OnboardedRepositoryStore = (*onboardedRepositoryRepository)(nil)
