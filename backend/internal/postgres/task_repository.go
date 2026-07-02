package postgres

import (
	"context"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

func (tx *Tx) InsertTask(ctx context.Context, task application.TaskRecord) error {
	now, err := tx.Now(ctx)
	if err != nil {
		return err
	}
	_, err = tx.tx.Exec(ctx, `
		INSERT INTO tasks (
			tenant_id, id, publisher_agent_version_id, type, title, problem,
			constraints, requirements, deadline, status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9, $10, $11, $11)`,
		task.TenantID, task.ID, task.PublisherAgentVersionID, task.Type, task.Title,
		task.Problem, task.Constraints, task.Requirements, task.Deadline, task.Status, now,
	)
	return err
}

func (tx *Tx) GetTask(
	ctx context.Context,
	tenantID, taskID string,
) (*application.TaskRecord, error) {
	var task application.TaskRecord
	var status string
	err := tx.tx.QueryRow(ctx, `
		SELECT t.id, t.tenant_id, t.publisher_agent_version_id, t.type, t.title,
		       t.problem, t.constraints, t.requirements, t.deadline, t.status,
		       COALESCE(e.agent_version_id, ''), t.state_version,
		       COALESCE(t.active_execution_id, ''), t.created_at, t.updated_at
		FROM tasks t
		LEFT JOIN executions e
		  ON e.tenant_id = t.tenant_id AND e.id = t.active_execution_id AND e.task_id = t.id
		WHERE t.tenant_id = $1 AND t.id = $2`,
		tenantID, taskID,
	).Scan(
		&task.ID, &task.TenantID, &task.PublisherAgentVersionID, &task.Type, &task.Title,
		&task.Problem, &task.Constraints, &task.Requirements, &task.Deadline, &status,
		&task.ClaimedBy, &task.StateVersion, &task.ActiveExecutionID,
		&task.CreatedAt, &task.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, notFound("task")
	}
	if err != nil {
		return nil, err
	}
	task.Status = domain.TaskStatus(status)
	return &task, nil
}

func (tx *Tx) ListTaskRecords(
	ctx context.Context,
	query application.TaskListQuery,
) ([]application.TaskRecord, error) {
	statuses := make([]string, len(query.Statuses))
	for i := range query.Statuses {
		statuses[i] = string(query.Statuses[i])
	}
	rows, err := tx.tx.Query(ctx, `
		SELECT t.id, t.tenant_id, t.publisher_agent_version_id, t.type, t.title,
		       t.problem, t.constraints, t.requirements, t.deadline, t.status,
		       COALESCE(e.agent_version_id, ''), t.state_version,
		       COALESCE(t.active_execution_id, ''), t.created_at, t.updated_at
		FROM tasks t
		LEFT JOIN executions e
		  ON e.tenant_id=t.tenant_id AND e.id=t.active_execution_id AND e.task_id=t.id
		WHERE t.tenant_id=$1
		  AND (cardinality($2::text[])=0 OR t.status=ANY($2::text[]))
		  AND ($3='' OR t.type=$3)
		  AND ($4='' OR t.publisher_agent_version_id=$4)
		  AND ($5 OR (t.created_at, t.id) < ($6, $7))
		ORDER BY t.created_at DESC, t.id DESC
		LIMIT $8`,
		query.TenantID, statuses, query.Type, query.PublisherAgentVersionID,
		query.AfterCreatedAt.IsZero(), query.AfterCreatedAt, query.AfterID, query.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []application.TaskRecord
	for rows.Next() {
		var task application.TaskRecord
		var status string
		if err := rows.Scan(
			&task.ID, &task.TenantID, &task.PublisherAgentVersionID, &task.Type,
			&task.Title, &task.Problem, &task.Constraints, &task.Requirements,
			&task.Deadline, &status, &task.ClaimedBy, &task.StateVersion,
			&task.ActiveExecutionID, &task.CreatedAt, &task.UpdatedAt,
		); err != nil {
			return nil, err
		}
		task.Status = domain.TaskStatus(status)
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (tx *Tx) UpdateTask(
	ctx context.Context,
	task application.TaskRecord,
	expectedVersion int64,
	activeExecutionID string,
) (bool, error) {
	now, err := tx.Now(ctx)
	if err != nil {
		return false, err
	}
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
SET status='claimed', state_version=state_version+1, active_execution_id=$4, updated_at=$5
WHERE tenant_id=$1 AND id=$2 AND status='open' AND state_version=$3 AND deadline>$5`

func (tx *Tx) ClaimTask(
	ctx context.Context,
	tenantID, taskID string,
	expectedVersion int64,
	executionID string,
) (bool, error) {
	now, err := tx.Now(ctx)
	if err != nil {
		return false, err
	}
	tag, err := tx.tx.Exec(ctx, claimSQL, tenantID, taskID, expectedVersion, executionID, now)
	return tag.RowsAffected() == 1, err
}

func (tx *Tx) InsertExecution(
	ctx context.Context,
	execution *domain.Execution,
	leaseSecretHash []byte,
) error {
	now, err := tx.Now(ctx)
	if err != nil {
		return err
	}
	_, err = tx.tx.Exec(ctx, `
		INSERT INTO executions (
			tenant_id, id, task_id, agent_version_id, status, lease_secret_hash,
			lease_generation, lease_soft_expires_at, lease_hard_expires_at, claimed_at,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10, $10)`,
		execution.TenantID, execution.ID, execution.TaskID, execution.AgentID,
		execution.Status, leaseSecretHash, execution.Lease.Generation,
		execution.Lease.SoftExpiry, execution.Lease.HardExpiry, now,
	)
	return mapExecutionInsertError(err)
}

func (tx *Tx) GetExecution(
	ctx context.Context,
	tenantID, executionID string,
) (*domain.Execution, int64, error) {
	return tx.getExecution(ctx, tenantID, executionID, false)
}

func (tx *Tx) GetExecutionForUpdate(
	ctx context.Context,
	tenantID, executionID string,
) (*domain.Execution, int64, error) {
	return tx.getExecution(ctx, tenantID, executionID, true)
}

func (tx *Tx) getExecution(
	ctx context.Context,
	tenantID, executionID string,
	forUpdate bool,
) (*domain.Execution, int64, error) {
	var execution domain.Execution
	var version int64
	var softExpiry, hardExpiry *time.Time
	var stage *string
	var progress *float64
	var claimedAt, startedAt, submittedAt, expiredAt, lastHeartbeatAt *time.Time
	query := `
		SELECT id, tenant_id, task_id, agent_version_id, status, state_version,
		       stage, progress, lease_generation, lease_soft_expires_at, lease_hard_expires_at,
		       claimed_at, started_at, submitted_at, expired_at, last_heartbeat_at
		FROM executions
		WHERE tenant_id=$1 AND id=$2`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	err := tx.tx.QueryRow(ctx, query,
		tenantID, executionID,
	).Scan(
		&execution.ID, &execution.TenantID, &execution.TaskID, &execution.AgentID,
		&execution.Status, &version, &stage, &progress, &execution.Lease.Generation,
		&softExpiry, &hardExpiry, &claimedAt, &startedAt, &submittedAt, &expiredAt, &lastHeartbeatAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, notFound("execution")
	}
	if err != nil {
		return nil, 0, err
	}
	if stage != nil {
		execution.Stage = *stage
	}
	if progress != nil {
		execution.Progress = *progress
	}
	if softExpiry != nil {
		execution.Lease.SoftExpiry = *softExpiry
	}
	if hardExpiry != nil {
		execution.Lease.HardExpiry = *hardExpiry
	}
	if claimedAt != nil {
		execution.ClaimedAt = *claimedAt
	}
	if startedAt != nil {
		execution.StartedAt = *startedAt
	}
	if submittedAt != nil {
		execution.SubmittedAt = *submittedAt
	}
	if expiredAt != nil {
		execution.ExpiredAt = *expiredAt
	}
	if lastHeartbeatAt != nil {
		execution.LastHeartbeatAt = *lastHeartbeatAt
	}
	return &execution, version, nil
}

func mapExecutionInsertError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "executions_one_active_per_task" {
		return domain.ErrStateConflict
	}
	return err
}

func (tx *Tx) ListActiveExecutions(ctx context.Context, tenantID, taskID string) ([]application.ExecutionRecord, error) {
	rows, err := tx.tx.Query(ctx, `
		SELECT id, tenant_id, task_id, agent_version_id, status, state_version,
		       stage, progress, lease_generation, lease_soft_expires_at, lease_hard_expires_at,
		       claimed_at, started_at, submitted_at, expired_at, last_heartbeat_at
		FROM executions
		WHERE tenant_id=$1 AND task_id=$2
		  AND status IN ('leased','running','submitted','validating','reviewing','revision_requested')
		ORDER BY id
		FOR UPDATE`, tenantID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []application.ExecutionRecord
	for rows.Next() {
		var execution domain.Execution
		var version int64
		var softExpiry, hardExpiry *time.Time
		var stage *string
		var progress *float64
		var claimedAt, startedAt, submittedAt, expiredAt, lastHeartbeatAt *time.Time
		if err := rows.Scan(
			&execution.ID, &execution.TenantID, &execution.TaskID, &execution.AgentID,
			&execution.Status, &version, &stage, &progress, &execution.Lease.Generation,
			&softExpiry, &hardExpiry, &claimedAt, &startedAt, &submittedAt, &expiredAt, &lastHeartbeatAt,
		); err != nil {
			return nil, err
		}
		if stage != nil {
			execution.Stage = *stage
		}
		if progress != nil {
			execution.Progress = *progress
		}
		if softExpiry != nil {
			execution.Lease.SoftExpiry = *softExpiry
		}
		if hardExpiry != nil {
			execution.Lease.HardExpiry = *hardExpiry
		}
		if claimedAt != nil {
			execution.ClaimedAt = *claimedAt
		}
		if startedAt != nil {
			execution.StartedAt = *startedAt
		}
		if submittedAt != nil {
			execution.SubmittedAt = *submittedAt
		}
		if expiredAt != nil {
			execution.ExpiredAt = *expiredAt
		}
		if lastHeartbeatAt != nil {
			execution.LastHeartbeatAt = *lastHeartbeatAt
		}
		records = append(records, application.ExecutionRecord{Execution: &execution, StateVersion: version})
	}
	return records, rows.Err()
}

func (tx *Tx) UpdateExecution(
	ctx context.Context,
	execution *domain.Execution,
	expectedVersion int64,
) (bool, error) {
	now, err := tx.Now(ctx)
	if err != nil {
		return false, err
	}
	tag, err := tx.tx.Exec(ctx, `
		UPDATE executions
		SET status=$4, state_version=state_version+1, stage=$5, progress=$6, lease_generation=$7,
		    lease_soft_expires_at=$8, lease_hard_expires_at=$9,
		    last_heartbeat_at=$10, started_at=CASE WHEN $4='running' AND started_at IS NULL THEN $11 ELSE started_at END,
		    expired_at=CASE WHEN $4='expired' AND expired_at IS NULL THEN $11 ELSE expired_at END,
		    updated_at=$11
		WHERE tenant_id=$1 AND id=$2 AND state_version=$3`,
		execution.TenantID, execution.ID, expectedVersion, execution.Status,
		execution.Stage, execution.Progress, execution.Lease.Generation,
		execution.Lease.SoftExpiry, execution.Lease.HardExpiry, execution.LastHeartbeatAt, now,
	)
	return tag.RowsAffected() == 1, err
}

func (tx *Tx) UpdateOwnedExecution(
	ctx context.Context,
	execution *domain.Execution,
	expectedVersion int64,
	agentVersionID string,
	expectedGeneration int64,
) (bool, error) {
	now, err := tx.Now(ctx)
	if err != nil {
		return false, err
	}
	tag, err := tx.tx.Exec(ctx, `
		UPDATE executions
		SET status=$6, state_version=state_version+1, stage=$7, progress=$8, lease_generation=$9,
		    lease_soft_expires_at=$10, lease_hard_expires_at=$11,
		    last_heartbeat_at=CASE WHEN $6 IN ('leased','running') THEN $12 ELSE last_heartbeat_at END,
		    started_at=CASE WHEN $6='running' AND started_at IS NULL THEN $12 ELSE started_at END,
		    updated_at=$12
		WHERE tenant_id=$1 AND id=$2 AND state_version=$3
		  AND agent_version_id=$4 AND lease_generation=$5
		  AND status IN ('leased','running')
		  AND lease_hard_expires_at >= $12`,
		execution.TenantID, execution.ID, expectedVersion, agentVersionID,
		expectedGeneration, execution.Status, execution.Stage, execution.Progress, execution.Lease.Generation,
		execution.Lease.SoftExpiry, execution.Lease.HardExpiry, now,
	)
	return tag.RowsAffected() == 1, err
}

func (tx *Tx) GetExecutionUsage(ctx context.Context, tenantID, executionID string) (*application.UsageView, error) {
	var view application.UsageView
	var observedCost, selfReportedCost *decimal.Decimal
	err := tx.tx.QueryRow(ctx, `
		SELECT observed_cost, self_reported_cost, coverage, provider, observed_at
		FROM execution_usage
		WHERE tenant_id=$1 AND execution_id=$2
		ORDER BY CASE coverage WHEN 'complete' THEN 3 WHEN 'partial' THEN 2 ELSE 1 END DESC, observed_at DESC
		LIMIT 1`,
		tenantID, executionID,
	).Scan(&observedCost, &selfReportedCost, &view.Coverage, &view.Provider, &view.ObservedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	view.ObservedCost = observedCost
	view.SelfReportedCost = selfReportedCost
	return &view, nil
}

func notFound(resource string) error {
	return &domain.Error{Code: "not_found", Message: resource + " was not found"}
}
