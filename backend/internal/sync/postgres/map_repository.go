package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/sync/application"
	"agentguild.dev/agentguild/backend/internal/sync/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MapRepository struct {
	q queryer
}

func NewMapRepository(pool *pgxpool.Pool) *MapRepository {
	return &MapRepository{q: pool}
}

func (r *MapRepository) Get(ctx context.Context, tenantID, repo string, issueNumber int) (*application.Mapping, error) {
	var mapping application.Mapping
	err := r.q.QueryRow(ctx, `
		SELECT tenant_id, repo, issue_number, task_id, issue_state, issue_closed,
		       COALESCE(issue_url, ''), last_synced_at
		FROM issue_task_map
		WHERE tenant_id=$1 AND repo=$2 AND issue_number=$3`,
		tenantID, repo, issueNumber,
	).Scan(
		&mapping.TenantID,
		&mapping.Repo,
		&mapping.IssueNumber,
		&mapping.TaskID,
		&mapping.IssueState,
		&mapping.IssueClosed,
		&mapping.IssueURL,
		&mapping.LastSyncedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &mapping, nil
}

func (r *MapRepository) Upsert(ctx context.Context, mapping *application.Mapping) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO issue_task_map (
			tenant_id, repo, issue_number, task_id, issue_state, issue_closed,
			issue_url, last_synced_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (tenant_id, repo, issue_number)
		DO UPDATE SET
			task_id=EXCLUDED.task_id,
			issue_state=EXCLUDED.issue_state,
			issue_closed=EXCLUDED.issue_closed,
			issue_url=EXCLUDED.issue_url,
			last_synced_at=EXCLUDED.last_synced_at`,
		mapping.TenantID, mapping.Repo, mapping.IssueNumber, mapping.TaskID,
		mapping.IssueState, mapping.IssueClosed, nullableString(mapping.IssueURL),
		mapping.LastSyncedAt,
	)
	return err
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

var _ application.MapRepository = (*MapRepository)(nil)
