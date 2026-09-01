package postgres

import (
	"context"
	"database/sql"
	"errors"

	"agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/identity/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type publicAgentRepository struct {
	pool *pgxpool.Pool
}

func NewPublicAgentRepository(pool *pgxpool.Pool) application.PublicAgentRepository {
	return &publicAgentRepository{pool: pool}
}

func (r *publicAgentRepository) ListPublic(ctx context.Context, organizationID, status string, limit int) ([]application.PublicAgentView, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT a.id, COALESCE(a.current_version_id, ''), a.handle, a.display_name, a.description,
		       a.status, a.last_seen_at, a.created_at,
		       COALESCE(v.runtime, ''), COALESCE(v.model, ''), COALESCE(v.capabilities, '{}'::text[])
		FROM agent_identities a
		JOIN agent_organization_memberships m
		  ON m.agent_id=a.id AND m.organization_id=$1 AND m.status='active'
		LEFT JOIN agent_identity_versions v
		  ON v.id=a.current_version_id AND v.agent_id=a.id AND v.status='active'
		WHERE ($2='' AND a.status <> 'revoked') OR ($2<>'' AND a.status=$2)
		ORDER BY a.last_seen_at DESC NULLS LAST, a.created_at DESC, a.id ASC
		LIMIT $3`, organizationID, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]application.PublicAgentView, 0)
	for rows.Next() {
		var item application.PublicAgentView
		var description, versionID, runtime, model sql.NullString
		if err := rows.Scan(
			&item.AgentID, &versionID, &item.Handle, &item.DisplayName, &description,
			&item.Status, &item.LastSeenAt, &item.CreatedAt,
			&runtime, &model, &item.Capabilities,
		); err != nil {
			return nil, err
		}
		item.AgentVersionID = versionID.String
		item.Description = description.String
		item.Runtime = runtime.String
		item.Model = model.String
		item.OrganizationID = organizationID
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *publicAgentRepository) GetPublic(ctx context.Context, organizationID, agentID string) (application.PublicAgentView, error) {
	var item application.PublicAgentView
	var description, versionID, runtime, model sql.NullString
	err := r.pool.QueryRow(ctx, `
		SELECT a.id, COALESCE(a.current_version_id, ''), a.handle, a.display_name, a.description,
		       a.status, a.last_seen_at, a.created_at,
		       COALESCE(v.runtime, ''), COALESCE(v.model, ''), COALESCE(v.capabilities, '{}'::text[])
		FROM agent_identities a
		JOIN agent_organization_memberships m
		  ON m.agent_id=a.id AND m.organization_id=$1 AND m.status='active'
		LEFT JOIN agent_identity_versions v
		  ON v.id=a.current_version_id AND v.agent_id=a.id AND v.status='active'
		WHERE a.id=$2 AND a.status <> 'revoked'`, organizationID, agentID).Scan(
		&item.AgentID, &versionID, &item.Handle, &item.DisplayName, &description,
		&item.Status, &item.LastSeenAt, &item.CreatedAt,
		&runtime, &model, &item.Capabilities,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.PublicAgentView{}, domain.ErrNotFound
	}
	if err != nil {
		return application.PublicAgentView{}, err
	}
	item.AgentVersionID = versionID.String
	item.Description = description.String
	item.Runtime = runtime.String
	item.Model = model.String
	item.OrganizationID = organizationID
	return item, nil
}

var _ application.PublicAgentRepository = (*publicAgentRepository)(nil)
