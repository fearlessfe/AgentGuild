package testdb

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func StartPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("AGENTGUILD_TEST_DATABASE_URL")
	if dsn == "" {
		const localDSN = "postgres://agentguild:agentguild@127.0.0.1:55432/agentguild?sslmode=disable"
		if databaseAvailable(localDSN) {
			dsn = localDSN
		} else {
			dsn = startContainer(t)
		}
	}
	return openAndMigrate(t, dsn)
}

func databaseAvailable(dsn string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return false
	}
	defer db.Close()
	return db.Ping(ctx) == nil
}

func startContainer(t *testing.T) string {
	t.Helper()
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Fatalf("find docker for PostgreSQL 18.4 integration tests: %v", err)
	}
	name := fmt.Sprintf("agentguild-test-%d", time.Now().UnixNano())
	output, err := exec.Command(
		docker, "run", "--detach", "--rm", "--name", name,
		"--env", "POSTGRES_DB=agentguild",
		"--env", "POSTGRES_USER=agentguild",
		"--env", "POSTGRES_PASSWORD=agentguild",
		"--publish", "127.0.0.1::5432",
		"postgres:18.4",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("start PostgreSQL 18.4: %v: %s", err, output)
	}
	t.Cleanup(func() { _ = exec.Command(docker, "rm", "--force", name).Run() })

	output, err = exec.Command(docker, "port", name, "5432/tcp").CombinedOutput()
	if err != nil {
		t.Fatalf("inspect PostgreSQL port: %v: %s", err, output)
	}
	_, port, err := net.SplitHostPort(strings.TrimSpace(string(output)))
	if err != nil {
		t.Fatalf("parse PostgreSQL port %q: %v", output, err)
	}
	return fmt.Sprintf(
		"postgres://agentguild:agentguild@127.0.0.1:%s/agentguild?sslmode=disable",
		port,
	)
}

func openAndMigrate(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	admin, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(admin.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		if err := admin.Ping(ctx); err == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("wait for PostgreSQL at AGENTGUILD_TEST_DATABASE_URL: %v", ctx.Err())
		}
		time.Sleep(100 * time.Millisecond)
	}

	var version string
	if err := admin.QueryRow(context.Background(), `SHOW server_version`).Scan(&version); err != nil {
		t.Fatalf("read PostgreSQL version: %v", err)
	}
	if !strings.HasPrefix(version, "18.4") {
		t.Fatalf("PostgreSQL version=%q, want 18.4", version)
	}

	schema := fmt.Sprintf("agentguild_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(context.Background(), `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create isolated test schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse PostgreSQL DSN: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("open isolated PostgreSQL schema: %v", err)
	}
	t.Cleanup(db.Close)

	applyMigration(t, db, "000001_task_lifecycle.up.sql")
	applyMigration(t, db, "000002_agent_identity.up.sql")
	applyMigration(t, db, "000003_git_credentials.up.sql")
	return db
}

func ApplyDownMigration(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	applyMigration(t, db, "000003_git_credentials.down.sql")
	applyMigration(t, db, "000002_agent_identity.down.sql")
	applyMigration(t, db, "000001_task_lifecycle.down.sql")
}

func ApplyUpMigration(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	applyMigration(t, db, "000001_task_lifecycle.up.sql")
	applyMigration(t, db, "000002_agent_identity.up.sql")
	applyMigration(t, db, "000003_git_credentials.up.sql")
}

func applyMigration(t *testing.T, db *pgxpool.Pool, name string) {
	t.Helper()
	_, filename, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(filename), "..", "..", "migrations", name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	if _, err := db.Exec(context.Background(), string(body)); err != nil {
		t.Fatalf("apply migration %s: %v", name, err)
	}
}
