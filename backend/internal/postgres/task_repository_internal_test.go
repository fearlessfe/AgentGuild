package postgres

import (
	"testing"

	"agentguild.dev/agentguild/backend/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapExecutionInsertErrorMapsActivePerTaskUniqueViolation(t *testing.T) {
	err := mapExecutionInsertError(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "executions_one_active_per_task",
	})
	if domain.CodeOf(err) != "state_conflict" {
		t.Fatalf("error=%v, want state_conflict", err)
	}
}
