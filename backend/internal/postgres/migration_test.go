package postgres_test

import (
	"context"
	"strings"
	"testing"

	"agentguild.dev/agentguild/backend/internal/testdb"
)

func TestEachTestDatabaseUsesAnIsolatedSchema(t *testing.T) {
	db := testdb.StartPostgres(t)

	var schema string
	if err := db.QueryRow(context.Background(), `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if schema == "public" || !strings.HasPrefix(schema, "agentguild_test_") {
		t.Fatalf("current schema=%q, want an isolated agentguild_test_ schema", schema)
	}
}

func TestMigrationRoundTrip(t *testing.T) {
	db := testdb.StartPostgres(t)
	testdb.ApplyDownMigration(t, db)

	tables := []string{
		"tasks", "executions", "idempotency_records",
		"task_events", "outbox_events", "execution_usage",
	}
	for _, table := range tables {
		var name *string
		err := db.QueryRow(context.Background(), `SELECT to_regclass($1)::text`, table).Scan(&name)
		if err != nil || name != nil {
			t.Fatalf("%s still exists after down migration: name=%v err=%v", table, name, err)
		}
	}

	testdb.ApplyUpMigration(t, db)
	for _, table := range tables {
		var name *string
		if err := db.QueryRow(context.Background(), `SELECT to_regclass($1)::text`, table).Scan(&name); err != nil {
			t.Fatal(err)
		}
		if name == nil || *name != table {
			t.Fatalf("%s missing after up migration: %v", table, name)
		}
	}

	requiredRelations := []string{
		"executions_one_active_per_task",
		"idempotency_scope_key",
		"task_events_tenant_task_created",
		"outbox_events_ready",
	}
	for _, relation := range requiredRelations {
		var name *string
		if err := db.QueryRow(context.Background(), `SELECT to_regclass($1)::text`, relation).Scan(&name); err != nil {
			t.Fatal(err)
		}
		if name == nil {
			t.Fatalf("required index %s missing after up migration", relation)
		}
	}

	requiredConstraints := []string{
		"executions_task_fk",
		"executions_tenant_id_task_id_key",
		"tasks_active_execution_fk",
		"task_events_execution_fk",
		"execution_usage_execution_fk",
	}
	for _, constraint := range requiredConstraints {
		var exists bool
		if err := db.QueryRow(context.Background(), `
			SELECT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname=$1 AND connamespace=current_schema()::regnamespace
			)`, constraint).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("required constraint %s missing after up migration", constraint)
		}
	}
}

func TestExecutionMigrationAcceptsCancelledAsTerminal(t *testing.T) {
	db := testdb.StartPostgres(t)
	_, err := db.Exec(context.Background(), `
		INSERT INTO tasks
			(tenant_id, id, publisher_agent_version_id, type, title, problem, deadline, status)
		VALUES ('tenant-1', 'task-1', 'publisher-1', 'code', 'title', 'problem',
		        clock_timestamp() + interval '1 hour', 'cancelled')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(context.Background(), `
		INSERT INTO executions
			(tenant_id, id, task_id, agent_version_id, status, lease_generation)
		VALUES ('tenant-1', 'execution-1', 'task-1', 'agent-1', 'cancelled', 1)`)
	if err != nil {
		t.Fatalf("cancelled execution rejected by migration: %v", err)
	}
}

func TestTaskJSONDefaultsAreArrays(t *testing.T) {
	db := testdb.StartPostgres(t)
	var constraints, requirements string
	err := db.QueryRow(context.Background(), `
		INSERT INTO tasks (tenant_id,id,publisher_agent_version_id,type,title,problem,deadline,status)
		VALUES ('tenant','task','publisher','type','title','problem',clock_timestamp()+interval '1 hour','open')
		RETURNING constraints::text, requirements::text`).Scan(&constraints, &requirements)
	if err != nil {
		t.Fatal(err)
	}
	if constraints != "[]" || requirements != "[]" {
		t.Fatalf("defaults=%s/%s, want []/[]", constraints, requirements)
	}
}
