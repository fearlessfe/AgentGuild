package postgres

import (
	"context"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (tx *Tx) InsertTask(ctx context.Context, task *domain.Task) error {
	_, err := tx.tx.Exec(ctx, `
		INSERT INTO tasks (
			tenant_id, id, publisher_agent_version_id, type, title, problem,
			constraints, requirements, deadline, status
		) VALUES ($1, $2, $3, 'generic', $2, $2, '{}'::jsonb, '{}'::jsonb, $4, $5)`,
		task.TenantID, task.ID, task.PublisherID, task.Deadline, task.Status,
	)
	return err
}

func (tx *Tx) GetTask(
	ctx context.Context,
	tenantID, taskID string,
) (*domain.Task, int64, error) {
	var task domain.Task
	var status string
	var version int64
	err := tx.tx.QueryRow(ctx, `
		SELECT t.id, t.tenant_id, t.publisher_agent_version_id, t.deadline, t.status,
		       COALESCE(e.agent_version_id, ''), t.state_version
		FROM tasks t
		LEFT JOIN executions e
		  ON e.tenant_id = t.tenant_id AND e.id = t.active_execution_id
		WHERE t.tenant_id = $1 AND t.id = $2`,
		tenantID, taskID,
	).Scan(
		&task.ID, &task.TenantID, &task.PublisherID, &task.Deadline, &status,
		&task.ClaimedBy, &version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, notFound("task")
	}
	if err != nil {
		return nil, 0, err
	}
	task.Status = taskStatus(status)
	return &task, version, nil
}

func (tx *Tx) UpdateTask(
	ctx context.Context,
	task *domain.Task,
	expectedVersion int64,
	activeExecutionID string,
	now time.Time,
) (bool, error) {
	tag, err := tx.tx.Exec(ctx, `
		UPDATE tasks
		SET status=$4, state_version=state_version+1,
		    active_execution_id=NULLIF($5, ''), updated_at=$6
		WHERE tenant_id=$1 AND id=$2 AND state_version=$3`,
		task.TenantID, task.ID, expectedVersion, task.Status, activeExecutionID, now,
	)
	return tag.RowsAffected() == 1, err
}

const claimSQL = `
UPDATE tasks
SET status='active', state_version=state_version+1, active_execution_id=$4, updated_at=$5
WHERE tenant_id=$1 AND id=$2 AND status='open' AND state_version=$3 AND deadline>$5`

func (tx *Tx) ClaimTask(
	ctx context.Context,
	tenantID, taskID string,
	expectedVersion int64,
	executionID string,
	now time.Time,
) (bool, error) {
	tag, err := tx.tx.Exec(ctx, claimSQL, tenantID, taskID, expectedVersion, executionID, now)
	return tag.RowsAffected() == 1, err
}

func (tx *Tx) InsertExecution(
	ctx context.Context,
	execution *domain.Execution,
	leaseSecretHash []byte,
) error {
	_, err := tx.tx.Exec(ctx, `
		INSERT INTO executions (
			tenant_id, id, task_id, agent_version_id, status, lease_secret_hash,
			lease_generation, lease_soft_expires_at, lease_hard_expires_at, claimed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, clock_timestamp())`,
		execution.TenantID, execution.ID, execution.TaskID, execution.AgentID,
		execution.Status, leaseSecretHash, execution.Lease.Generation,
		execution.Lease.SoftExpiry, execution.Lease.HardExpiry,
	)
	return err
}

func (tx *Tx) GetExecution(
	ctx context.Context,
	tenantID, executionID string,
) (*domain.Execution, int64, error) {
	var execution domain.Execution
	var version int64
	var softExpiry, hardExpiry *time.Time
	err := tx.tx.QueryRow(ctx, `
		SELECT id, tenant_id, task_id, agent_version_id, status, state_version,
		       lease_generation, lease_soft_expires_at, lease_hard_expires_at
		FROM executions
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, executionID,
	).Scan(
		&execution.ID, &execution.TenantID, &execution.TaskID, &execution.AgentID,
		&execution.Status, &version, &execution.Lease.Generation,
		&softExpiry, &hardExpiry,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, notFound("execution")
	}
	if err != nil {
		return nil, 0, err
	}
	if softExpiry != nil {
		execution.Lease.SoftExpiry = *softExpiry
	}
	if hardExpiry != nil {
		execution.Lease.HardExpiry = *hardExpiry
	}
	return &execution, version, nil
}

func (tx *Tx) UpdateExecution(
	ctx context.Context,
	execution *domain.Execution,
	expectedVersion int64,
	now time.Time,
) (bool, error) {
	tag, err := tx.tx.Exec(ctx, `
		UPDATE executions
		SET status=$4, state_version=state_version+1, lease_generation=$5,
		    lease_soft_expires_at=$6, lease_hard_expires_at=$7,
		    last_heartbeat_at=$8, updated_at=$8
		WHERE tenant_id=$1 AND id=$2 AND state_version=$3`,
		execution.TenantID, execution.ID, expectedVersion, execution.Status,
		execution.Lease.Generation, execution.Lease.SoftExpiry,
		execution.Lease.HardExpiry, now,
	)
	return tag.RowsAffected() == 1, err
}

func taskStatus(status string) domain.TaskStatus {
	if status == "active" {
		return domain.TaskClaimed
	}
	return domain.TaskStatus(status)
}

func notFound(resource string) error {
	return &domain.Error{Code: "not_found", Message: resource + " was not found"}
}
