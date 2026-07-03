package postgres

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/identity/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type auditRepository struct {
	q   queryer
	now func(context.Context) (time.Time, error)
}

func NewAuditRepository(pool *pgxpool.Pool) application.AuditRepository {
	return &auditRepository{q: pool, now: func(context.Context) (time.Time, error) { return time.Now(), nil }}
}

func (r *auditRepository) Append(ctx context.Context, event domain.IdentityEvent) error {
	if event.CreatedAt.IsZero() {
		now, err := r.now(ctx)
		if err != nil {
			return err
		}
		event.CreatedAt = now
	}

	_, err := r.q.Exec(ctx, `
		INSERT INTO identity_events (
			tenant_id, agent_id, actor_type, actor_id, intent, from_state, to_state,
			reason, payload, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		event.TenantID, event.AgentID, string(event.ActorType), event.ActorID,
		event.Intent, event.FromState, event.ToState, event.Reason, event.Payload,
		event.CreatedAt,
	)
	return err
}

func (r *auditRepository) ListByAgent(ctx context.Context, tenantID, agentID string, limit int) ([]domain.IdentityEvent, error) {
	rows, err := r.q.Query(ctx, `
		SELECT tenant_id, agent_id, actor_type, actor_id, intent, from_state, to_state,
		       reason, payload, created_at
		FROM identity_events
		WHERE tenant_id=$1 AND agent_id=$2
		ORDER BY created_at DESC, id DESC
		LIMIT $3`,
		tenantID, agentID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.IdentityEvent
	for rows.Next() {
		var event domain.IdentityEvent
		var actorType string
		if err := rows.Scan(
			&event.TenantID, &event.AgentID, &actorType, &event.ActorID,
			&event.Intent, &event.FromState, &event.ToState, &event.Reason,
			&event.Payload, &event.CreatedAt,
		); err != nil {
			return nil, err
		}
		event.ActorType = domain.ActorType(actorType)
		events = append(events, event)
	}
	return events, rows.Err()
}

var _ application.AuditRepository = (*auditRepository)(nil)
