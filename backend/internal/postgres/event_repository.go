package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/application"
	"github.com/jackc/pgx/v5"
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

func (tx *Tx) GetLatestExecutionEvent(ctx context.Context, tenantID, executionID string) (application.TaskEventSummary, error) {
	var e application.TaskEventSummary
	var executionIDPtr *string
	err := tx.tx.QueryRow(ctx, `
		SELECT id, tenant_id, task_id, execution_id, actor_type, actor_id, intent,
		       from_state, to_state, created_at
		FROM task_events
		WHERE tenant_id=$1 AND execution_id=$2
		ORDER BY id DESC
		LIMIT 1`,
		tenantID, executionID,
	).Scan(
		&e.ID, &e.TenantID, &e.TaskID, &executionIDPtr,
		&e.ActorType, &e.ActorID, &e.Intent,
		&e.FromState, &e.ToState, &e.CreatedAt,
	)
	if executionIDPtr != nil {
		e.ExecutionID = *executionIDPtr
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return e, nil
	}
	return e, err
}
func (tx *Tx) ListTaskEvents(ctx context.Context, tenantID, taskID string, afterID int64, limit int) ([]application.TaskEventSummary, error) {
	rows, err := tx.tx.Query(ctx, `
		SELECT id, tenant_id, task_id, execution_id, actor_type, actor_id, intent,
		       from_state, to_state, created_at
		FROM task_events
		WHERE tenant_id=$1 AND task_id=$2 AND id>$3
		ORDER BY id
		LIMIT $4`,
		tenantID, taskID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []application.TaskEventSummary
	for rows.Next() {
		var e application.TaskEventSummary
		var executionID *string
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.TaskID, &executionID,
			&e.ActorType, &e.ActorID, &e.Intent,
			&e.FromState, &e.ToState, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		if executionID != nil {
			e.ExecutionID = *executionID
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
