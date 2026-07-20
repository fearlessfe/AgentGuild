package postgres

import (
	"context"
	"encoding/json"

	"agentguild.dev/agentguild/backend/internal/evaluation/application"
	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// evaluationRunTaskRepository persists the mapping between an evaluation run's
// benchmark tasks and the real platform tasks published for them.
type evaluationRunTaskRepository struct {
	q queryer
}

// NewEvaluationRunTaskRepository returns a RunTaskRepository backed by pool.
func NewEvaluationRunTaskRepository(pool *pgxpool.Pool) application.RunTaskRepository {
	return &evaluationRunTaskRepository{q: pool}
}

// Insert batch-inserts the run-task plan rows for a run.
func (r *evaluationRunTaskRepository) Insert(ctx context.Context, tx application.Tx, tasks []domain.EvaluationRunTask) error {
	for _, task := range tasks {
		detailsJSON, err := json.Marshal(task.Details)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO evaluation_run_tasks (
				tenant_id, run_id, task_ref, task_id, ordering,
				resolved, passed, latency_ms, cost_cents, details, resolved_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			task.TenantID, task.RunID, task.TaskRef, task.TaskID, task.Ordering,
			task.Resolved, task.Passed, task.LatencyMs, task.CostCents, detailsJSON, task.ResolvedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// ListByRun returns the run-task mappings for a run in benchmark order.
func (r *evaluationRunTaskRepository) ListByRun(ctx context.Context, tenantID, runID string) ([]domain.EvaluationRunTask, error) {
	rows, err := r.q.Query(ctx, `
		SELECT tenant_id, run_id, task_ref, task_id, ordering,
		       resolved, passed, latency_ms, cost_cents, details, created_at, resolved_at
		FROM evaluation_run_tasks
		WHERE tenant_id=$1 AND run_id=$2
		ORDER BY ordering ASC, task_ref ASC`,
		tenantID, runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRunTasks(rows)
}

// ListByRunTx is the transaction-scoped variant of ListByRun: the harvest
// worker completes a run in the same transaction that resolved its rows, so it
// must read them through tx to see its own uncommitted writes.
func (r *evaluationRunTaskRepository) ListByRunTx(ctx context.Context, tx application.Tx, tenantID, runID string) ([]domain.EvaluationRunTask, error) {
	rows, err := tx.Query(ctx, `
		SELECT tenant_id, run_id, task_ref, task_id, ordering,
		       resolved, passed, latency_ms, cost_cents, details, created_at, resolved_at
		FROM evaluation_run_tasks
		WHERE tenant_id=$1 AND run_id=$2
		ORDER BY ordering ASC, task_ref ASC`,
		tenantID, runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRunTasks(rows)
}

// ListUnresolved returns the run's unresolved mappings in benchmark order. It
// runs inside the worker transaction so the harvest loop and the subsequent
// resolution writes see a consistent view.
func (r *evaluationRunTaskRepository) ListUnresolved(ctx context.Context, tx application.Tx, tenantID, runID string) ([]domain.EvaluationRunTask, error) {
	rows, err := tx.Query(ctx, `
		SELECT tenant_id, run_id, task_ref, task_id, ordering,
		       resolved, passed, latency_ms, cost_cents, details, created_at, resolved_at
		FROM evaluation_run_tasks
		WHERE tenant_id=$1 AND run_id=$2 AND resolved=false
		ORDER BY ordering ASC, task_ref ASC`,
		tenantID, runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRunTasks(rows)
}

// Resolve marks a run-task row resolved with its harvested outcome. The
// resolution details are merged into the existing details document so earlier
// markers (for example publish_error) stay visible. Resolving an already
// resolved row fails with a state conflict, keeping concurrent ticks
// idempotent.
func (r *evaluationRunTaskRepository) Resolve(ctx context.Context, tx application.Tx, tenantID, runID, taskRef string, resolution application.RunTaskResolution) error {
	details := resolution.Details
	if details == nil {
		details = map[string]any{}
	}
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE evaluation_run_tasks
		SET resolved=true,
		    passed=$4,
		    latency_ms=$5,
		    cost_cents=$6,
		    details = details || $7::jsonb,
		    resolved_at=$8
		WHERE tenant_id=$1 AND run_id=$2 AND task_ref=$3 AND resolved=false`,
		tenantID, runID, taskRef,
		resolution.Passed, resolution.LatencyMs, resolution.CostCents, detailsJSON, resolution.ResolvedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrStateConflict
	}
	return nil
}

// scanRunTasks reads run-task rows produced by the queries above.
func scanRunTasks(rows pgx.Rows) ([]domain.EvaluationRunTask, error) {
	var tasks []domain.EvaluationRunTask
	for rows.Next() {
		var task domain.EvaluationRunTask
		var detailsJSON []byte
		if err := rows.Scan(
			&task.TenantID, &task.RunID, &task.TaskRef, &task.TaskID, &task.Ordering,
			&task.Resolved, &task.Passed, &task.LatencyMs, &task.CostCents, &detailsJSON,
			&task.CreatedAt, &task.ResolvedAt,
		); err != nil {
			return nil, err
		}
		if len(detailsJSON) > 0 {
			_ = json.Unmarshal(detailsJSON, &task.Details)
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// SetTaskID backfills the platform task identifier after the task has been
// published.
func (r *evaluationRunTaskRepository) SetTaskID(ctx context.Context, tx application.Tx, tenantID, runID, taskRef, taskID string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE evaluation_run_tasks
		SET task_id=$4
		WHERE tenant_id=$1 AND run_id=$2 AND task_ref=$3`,
		tenantID, runID, taskRef, taskID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// RecordPublishFailure records a task publication error in the row's details
// so the phase-2 harvest worker can score it as a failed result.
func (r *evaluationRunTaskRepository) RecordPublishFailure(ctx context.Context, tx application.Tx, tenantID, runID, taskRef, publishError string) error {
	detailsJSON, err := json.Marshal(map[string]any{"publish_error": publishError})
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE evaluation_run_tasks
		SET details = details || $4::jsonb
		WHERE tenant_id=$1 AND run_id=$2 AND task_ref=$3`,
		tenantID, runID, taskRef, detailsJSON,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

var _ application.RunTaskRepository = (*evaluationRunTaskRepository)(nil)
