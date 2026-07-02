package postgres

import (
	"context"
	"encoding/json"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Reaper struct {
	pool *pgxpool.Pool
}

func NewReaper(pool *pgxpool.Pool) *Reaper {
	return &Reaper{pool: pool}
}

func (r *Reaper) RunBatch(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		return 0, &domain.Error{Code: "invalid_argument", Message: "limit is invalid", Field: "limit"}
	}
	pgxTx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = pgxTx.Rollback(ctx) }()
	tx := &Tx{tx: pgxTx, acquiredIdempotency: make(map[application.IdempotencyKey]string)}
	now, err := tx.Now(ctx)
	if err != nil {
		return 0, err
	}

	rows, err := pgxTx.Query(ctx, `
		SELECT e.tenant_id, e.id, e.task_id, e.status, t.status, t.deadline
		FROM executions e
		JOIN tasks t ON t.tenant_id=e.tenant_id AND t.id=e.task_id
		WHERE e.status IN ('leased','running')
		  AND (e.lease_hard_expires_at < $1 OR t.deadline <= $1)
		ORDER BY LEAST(e.lease_hard_expires_at, t.deadline), e.id
		FOR UPDATE OF e SKIP LOCKED
		LIMIT $2`, now, limit)
	if err != nil {
		return 0, err
	}
	type candidate struct {
		tenantID, executionID, taskID, executionStatus, taskStatus string
		deadline                                                   time.Time
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.tenantID, &item.executionID, &item.taskID, &item.executionStatus, &item.taskStatus, &item.deadline); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	processed := 0
	for _, item := range candidates {
		targetTaskStatus := string(domain.TaskOpen)
		if !now.Before(item.deadline) {
			targetTaskStatus = string(domain.TaskExpired)
		}
		tag, err := pgxTx.Exec(ctx, `
			UPDATE executions
			SET status='expired', state_version=state_version+1,
			    expired_at=$3, updated_at=$3
			WHERE tenant_id=$1 AND id=$2 AND status IN ('leased','running')`,
			item.tenantID, item.executionID, now)
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() != 1 {
			continue
		}
		tag, err = pgxTx.Exec(ctx, `
			UPDATE tasks
			SET status=$4, state_version=state_version+1,
			    active_execution_id=NULL, updated_at=$5
			WHERE tenant_id=$1 AND id=$2 AND active_execution_id=$3
			  AND status IN ('claimed','in_progress')`,
			item.tenantID, item.taskID, item.executionID, targetTaskStatus, now)
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() != 1 {
			return 0, &domain.Error{Code: "state_conflict", Message: "task changed while reaping execution"}
		}
		payload, err := json.Marshal(map[string]string{"task_id": item.taskID, "execution_id": item.executionID})
		if err != nil {
			return 0, err
		}
		if err := tx.AppendTaskEvent(ctx, application.TaskEvent{TenantID: item.tenantID, TaskID: item.taskID, ExecutionID: item.executionID, ActorType: string(domain.ActorSystem), ActorID: "reaper", Intent: "expire", FromState: item.taskStatus, ToState: targetTaskStatus, Payload: payload, CreatedAt: now}); err != nil {
			return 0, err
		}
		eventID, err := newOwnerToken()
		if err != nil {
			return 0, err
		}
		eventType := "task.reopened"
		if targetTaskStatus == string(domain.TaskExpired) {
			eventType = "task.expired"
		}
		if err := tx.AppendOutboxEvent(ctx, application.OutboxEvent{TenantID: item.tenantID, ID: item.taskID + ":expire:" + eventID, EventType: eventType, AggregateType: "task", AggregateID: item.taskID, Payload: payload, AvailableAt: now}); err != nil {
			return 0, err
		}
		processed++
	}
	if err := pgxTx.Commit(ctx); err != nil {
		return 0, err
	}
	return processed, nil
}
