package postgres

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/application"
)

func (tx *Tx) AppendTaskEvent(ctx context.Context, event application.TaskEvent) error {
	_, err := tx.tx.Exec(ctx, `
		INSERT INTO task_events (
			tenant_id, task_id, execution_id, actor_type, actor_id, intent,
			from_state, to_state, reason, payload, created_at
		) VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, NULLIF($9, ''), $10::jsonb, $11)`,
		event.TenantID, event.TaskID, event.ExecutionID, event.ActorType, event.ActorID,
		event.Intent, event.FromState, event.ToState, event.Reason, event.Payload, event.CreatedAt,
	)
	return err
}

func (tx *Tx) AppendOutboxEvent(ctx context.Context, event application.OutboxEvent) error {
	_, err := tx.tx.Exec(ctx, `
		INSERT INTO outbox_events (
			tenant_id, id, event_type, aggregate_type, aggregate_id, payload, available_at
		) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)`,
		event.TenantID, event.ID, event.EventType, event.AggregateType,
		event.AggregateID, event.Payload, event.AvailableAt,
	)
	return err
}
