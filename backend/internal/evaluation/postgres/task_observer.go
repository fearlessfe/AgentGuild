package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/evaluation/application"
	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// evaluationTaskObserver reads harvest snapshots from the core task tables.
// It lives in the evaluation module's own postgres layer so the evaluation
// module never imports the core task service; all queries are tenant-scoped
// and read-only.
type evaluationTaskObserver struct {
	q queryer
}

// NewEvaluationTaskObserver returns an EvaluationTaskObserver backed by pool.
func NewEvaluationTaskObserver(pool *pgxpool.Pool) application.EvaluationTaskObserver {
	return &evaluationTaskObserver{q: pool}
}

// Observe builds the harvest snapshot for one platform task:
//
//   - task status: tasks.status.
//   - execution: the task's latest execution (executions.created_at DESC); at
//     most one execution per task is active at any moment, so the latest row
//     is the one that determines the outcome.
//   - submission: the latest submission of that execution
//     (submissions.created_at DESC); submissions.status='validated' is the
//     objective automated-validation signal the worker scores as passed.
//   - latency endpoints: executions.started_at and executions.submitted_at are
//     returned verbatim; the worker derives the latency from them.
//   - cost: latest non-null execution_usage.observed_cost for the execution,
//     following the review reputation projection precedent; NULL means no
//     observation and the worker counts 0 with a details annotation.
func (o *evaluationTaskObserver) Observe(ctx context.Context, tenantID, taskID string) (*application.EvaluationTaskSnapshot, error) {
	row := o.q.QueryRow(ctx, `
		SELECT t.status,
		       e.id, e.agent_version_id, e.status, e.started_at, e.submitted_at,
		       s.status,
		       (SELECT eu.observed_cost::bigint
		        FROM execution_usage eu
		        WHERE eu.tenant_id = e.tenant_id
		          AND eu.execution_id = e.id
		          AND eu.observed_cost IS NOT NULL
		        ORDER BY eu.observed_at DESC
		        LIMIT 1) AS observed_cost_cents
		FROM tasks t
		LEFT JOIN LATERAL (
			SELECT e.tenant_id, e.id, e.agent_version_id, e.status, e.started_at, e.submitted_at
			FROM executions e
			WHERE e.tenant_id = t.tenant_id AND e.task_id = t.id
			ORDER BY e.created_at DESC
			LIMIT 1
		) e ON true
		LEFT JOIN LATERAL (
			SELECT s.status
			FROM submissions s
			WHERE s.tenant_id = t.tenant_id AND s.execution_id = e.id
			ORDER BY s.created_at DESC
			LIMIT 1
		) s ON true
		WHERE t.tenant_id=$1 AND t.id=$2`,
		tenantID, taskID,
	)

	snapshot := &application.EvaluationTaskSnapshot{TaskID: taskID}
	var executionID, executionVersionID, executionStatus, submissionStatus *string
	err := row.Scan(
		&snapshot.TaskStatus,
		&executionID, &executionVersionID, &executionStatus,
		&snapshot.ExecutionStartedAt, &snapshot.ExecutionSubmittedAt,
		&submissionStatus,
		&snapshot.ObservedCostCents,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if executionID != nil {
		snapshot.ExecutionID = *executionID
	}
	if executionVersionID != nil {
		snapshot.ExecutionVersionID = *executionVersionID
	}
	if executionStatus != nil {
		snapshot.ExecutionStatus = *executionStatus
	}
	if submissionStatus != nil {
		snapshot.SubmissionStatus = *submissionStatus
	}
	return snapshot, nil
}

var _ application.EvaluationTaskObserver = (*evaluationTaskObserver)(nil)
