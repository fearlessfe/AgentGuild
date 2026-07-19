package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/agentexperience/application"
	"agentguild.dev/agentguild/backend/internal/agentexperience/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type executionStore struct {
	q queryer
}

// NewExecutionStore returns an ExecutionStore backed by pool. The executing
// agent and its capabilities are resolved through the execution's agent
// version.
func NewExecutionStore(pool *pgxpool.Pool) application.ExecutionStore {
	return &executionStore{q: pool}
}

func (s *executionStore) GetExecution(ctx context.Context, tenantID, executionID string) (*application.Execution, error) {
	var execution application.Execution
	err := s.q.QueryRow(ctx, `
		SELECT e.id, e.tenant_id, av.agent_id, e.agent_version_id, e.task_id, av.capabilities
		FROM executions e
		JOIN agent_versions av
		  ON av.tenant_id = e.tenant_id AND av.id = e.agent_version_id
		WHERE e.tenant_id = $1 AND e.id = $2`,
		tenantID, executionID,
	).Scan(
		&execution.ID, &execution.TenantID, &execution.AgentID,
		&execution.AgentVersionID, &execution.TaskID, &execution.Capabilities,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &execution, nil
}

var _ application.ExecutionStore = (*executionStore)(nil)
