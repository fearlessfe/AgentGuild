package postgres_test

import (
	"context"
	"testing"

	"agentguild.dev/agentguild/backend/internal/testdb"
)

func TestMigrationRoundTrip(t *testing.T) {
	db := testdb.StartPostgres(t)
	testdb.ApplyDownMigration(t, db)

	var name *string
	err := db.QueryRow(context.Background(), `SELECT to_regclass('public.tasks')::text`).Scan(&name)
	if err != nil || name != nil {
		t.Fatalf("tasks table still exists after down migration: name=%v err=%v", name, err)
	}

	testdb.ApplyUpMigration(t, db)
	if err := db.QueryRow(context.Background(), `SELECT to_regclass('public.tasks')::text`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name == nil || *name != "tasks" {
		t.Fatalf("tasks table missing after up migration: %v", name)
	}
}
