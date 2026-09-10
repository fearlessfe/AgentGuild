package testdb

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// SharedContainerName 是所有测试共用的 PostgreSQL 容器。
	//
	// 这里刻意不使用一次性容器：`postgres` 镜像声明了 VOLUME，每个容器都会
	// 产生一个匿名卷；而 StartPostgres 在 22 个包里被调用 242 次，一次全量
	// 测试就会留下几百个容器和卷（约 50MB/个）。容器复用把它降到一个。
	SharedContainerName = "agentguild-test-postgres"

	// SharedDSN 固定端口，让并发的 go test 进程能互相发现同一个实例。
	SharedDSN = "postgres://agentguild:agentguild@127.0.0.1:55432/agentguild?sslmode=disable"
)

// schemaPrefix 是每个测试独立 schema 的前缀，其后紧跟创建时的 UnixNano。
const schemaPrefix = "agentguild_test_"

// staleSchemaAge 之前创建的 schema 一定不属于仍在运行的测试。取值远大于任何
// 单个测试的时长，好让并发的 go test 进程不会互相误删。
const staleSchemaAge = 6 * time.Hour

// sharedMu 只保护同一进程内的并发；跨进程的竞争由 docker 的容器名唯一性兜底。
var sharedMu sync.Mutex

// sweepOnce 让每个测试进程只清扫一次。
var sweepOnce sync.Once

// sweepStaleSchemas 回收被中断的测试进程遗留的 schema。
//
// 共享数据库跨多次运行长期存活，而 t.Cleanup 在进程被杀（超时、panic、Ctrl-C）
// 时不会执行——这正是一次性容器时代匿名卷堆积的同一类成因，只不过换到了
// schema 这一层。因此这里需要一个不依赖进程正常退出的兜底回收。
//
// 尽力而为：任何失败都不应让测试失败。
func sweepStaleSchemas(admin *pgxpool.Pool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := admin.Query(ctx, `
		SELECT schema_name FROM information_schema.schemata
		WHERE schema_name LIKE $1`, schemaPrefix+"%")
	if err != nil {
		return
	}
	var stale []string
	cutoff := time.Now().Add(-staleSchemaAge).UnixNano()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return
		}
		createdAt, err := strconv.ParseInt(strings.TrimPrefix(name, schemaPrefix), 10, 64)
		if err != nil || createdAt >= cutoff {
			continue
		}
		stale = append(stale, name)
	}
	rows.Close()
	if rows.Err() != nil {
		return
	}
	for _, name := range stale {
		_, _ = admin.Exec(ctx, `DROP SCHEMA IF EXISTS `+name+` CASCADE`)
	}
}

func StartPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("AGENTGUILD_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = ensureSharedPostgres(t)
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

// ensureSharedPostgres 返回共享实例的 DSN，必要时启动它。
//
// 该容器**不会**被测试自动删除：它的生命周期跨越所有包和多次运行，正是复用
// 的意义所在。测试之间的隔离由 openAndMigrate 的独立 schema 保证，而不是靠
// 丢弃整个数据库。需要回收时执行 `make test-db-down`。
func ensureSharedPostgres(t *testing.T) string {
	t.Helper()
	if databaseAvailable(SharedDSN) {
		return SharedDSN
	}

	sharedMu.Lock()
	defer sharedMu.Unlock()
	// 可能已被另一个 goroutine 启动。
	if databaseAvailable(SharedDSN) {
		return SharedDSN
	}

	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Fatalf("find docker for PostgreSQL 18.4 integration tests: %v", err)
	}
	output, runErr := exec.Command(
		docker, "run", "--detach", "--name", SharedContainerName,
		"--env", "POSTGRES_DB=agentguild",
		"--env", "POSTGRES_USER=agentguild",
		"--env", "POSTGRES_PASSWORD=agentguild",
		"--publish", "127.0.0.1:55432:5432",
		"postgres:18.4",
		// 几百个测试共用一个实例，默认 100 连接不够；其余参数关掉测试不需要
		// 的持久化开销。
		"-c", "max_connections=500",
		"-c", "fsync=off",
		"-c", "full_page_writes=off",
		"-c", "synchronous_commit=off",
	).CombinedOutput()
	if runErr != nil {
		// 并发的 go test 进程可能已经创建了同名容器；它也可能是上次运行留下
		// 的停止状态容器。两种情况都尝试启动既有容器。
		if startOutput, startErr := exec.Command(docker, "start", SharedContainerName).CombinedOutput(); startErr != nil {
			t.Fatalf("start shared PostgreSQL 18.4: %v: %s (create: %v: %s)",
				startErr, startOutput, runErr, output)
		}
	}

	deadline := time.Now().Add(90 * time.Second)
	for !databaseAvailable(SharedDSN) {
		if time.Now().After(deadline) {
			t.Fatalf("wait for shared PostgreSQL at %s", SharedDSN)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return SharedDSN
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

	sweepOnce.Do(func() { sweepStaleSchemas(admin) })

	schema := fmt.Sprintf("%s%d", schemaPrefix, time.Now().UnixNano())
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
	applyMigration(t, db, "000004_submissions.up.sql")
	applyMigration(t, db, "000005_validation_jobs.up.sql")
	applyMigration(t, db, "000006_submissions_repo.up.sql")
	applyMigration(t, db, "000007_code_review_and_reputation.up.sql")
	applyMigration(t, db, "000008_agent_version_and_experience.up.sql")
	applyMigration(t, db, "000009_github_apps.up.sql")
	applyMigration(t, db, "000010_issue_task_sync.up.sql")
	applyMigration(t, db, "000011_sync_rule_source_auth.up.sql")
	applyMigration(t, db, "000012_repository_onboarding.up.sql")
	applyMigration(t, db, "000013_multi_github_apps.up.sql")
	applyMigration(t, db, "000014_validation_job_execution_and_hard_gate.up.sql")
	applyMigration(t, db, "000015_git_credential_proxy.up.sql")
	applyMigration(t, db, "000016_validation_execution_state_sync.up.sql")
	applyMigration(t, db, "000017_review_capability.up.sql")
	applyMigration(t, db, "000018_version_promotion_audit.up.sql")
	applyMigration(t, db, "000019_evaluation_platform_runner.up.sql")
	applyMigration(t, db, "000020_global_agent_identity.up.sql")
	applyMigration(t, db, "000021_agent_contributions.up.sql")
	applyMigration(t, db, "000022_contribution_projections.up.sql")
	applyMigration(t, db, "000023_public_task_projections.up.sql")
	applyMigration(t, db, "000024_task_participation_grants.up.sql")
	applyMigration(t, db, "000025_public_task_claim_contract.up.sql")
	applyMigration(t, db, "000026_participation_resource_lookup.up.sql")
	applyMigration(t, db, "000027_open_agent_registration.up.sql")
	applyMigration(t, db, "000028_execution_criterion_results.up.sql")
	applyMigration(t, db, "000029_reputation_v2.up.sql")
	applyMigration(t, db, "000030_reward_ledger.up.sql")
	return db
}

func ApplyDownMigration(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	applyMigration(t, db, "000030_reward_ledger.down.sql")
	applyMigration(t, db, "000029_reputation_v2.down.sql")
	applyMigration(t, db, "000028_execution_criterion_results.down.sql")
	applyMigration(t, db, "000026_participation_resource_lookup.down.sql")
	applyMigration(t, db, "000027_open_agent_registration.down.sql")
	applyMigration(t, db, "000025_public_task_claim_contract.down.sql")
	applyMigration(t, db, "000024_task_participation_grants.down.sql")
	applyMigration(t, db, "000023_public_task_projections.down.sql")
	applyMigration(t, db, "000022_contribution_projections.down.sql")
	applyMigration(t, db, "000021_agent_contributions.down.sql")
	applyMigration(t, db, "000020_global_agent_identity.down.sql")
	applyMigration(t, db, "000019_evaluation_platform_runner.down.sql")
	applyMigration(t, db, "000018_version_promotion_audit.down.sql")
	applyMigration(t, db, "000017_review_capability.down.sql")
	applyMigration(t, db, "000016_validation_execution_state_sync.down.sql")
	applyMigration(t, db, "000015_git_credential_proxy.down.sql")
	applyMigration(t, db, "000014_validation_job_execution_and_hard_gate.down.sql")
	applyMigration(t, db, "000013_multi_github_apps.down.sql")
	applyMigration(t, db, "000012_repository_onboarding.down.sql")
	applyMigration(t, db, "000011_sync_rule_source_auth.down.sql")
	applyMigration(t, db, "000010_issue_task_sync.down.sql")
	applyMigration(t, db, "000009_github_apps.down.sql")
	applyMigration(t, db, "000008_agent_version_and_experience.down.sql")
	applyMigration(t, db, "000007_code_review_and_reputation.down.sql")
	applyMigration(t, db, "000006_submissions_repo.down.sql")
	applyMigration(t, db, "000005_validation_jobs.down.sql")
	applyMigration(t, db, "000004_submissions.down.sql")
	applyMigration(t, db, "000003_git_credentials.down.sql")
	applyMigration(t, db, "000002_agent_identity.down.sql")
	applyMigration(t, db, "000001_task_lifecycle.down.sql")
}

func ApplyUpMigration(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	applyMigration(t, db, "000001_task_lifecycle.up.sql")
	applyMigration(t, db, "000002_agent_identity.up.sql")
	applyMigration(t, db, "000003_git_credentials.up.sql")
	applyMigration(t, db, "000004_submissions.up.sql")
	applyMigration(t, db, "000005_validation_jobs.up.sql")
	applyMigration(t, db, "000006_submissions_repo.up.sql")
	applyMigration(t, db, "000007_code_review_and_reputation.up.sql")
	applyMigration(t, db, "000008_agent_version_and_experience.up.sql")
	applyMigration(t, db, "000009_github_apps.up.sql")
	applyMigration(t, db, "000010_issue_task_sync.up.sql")
	applyMigration(t, db, "000011_sync_rule_source_auth.up.sql")
	applyMigration(t, db, "000012_repository_onboarding.up.sql")
	applyMigration(t, db, "000013_multi_github_apps.up.sql")
	applyMigration(t, db, "000014_validation_job_execution_and_hard_gate.up.sql")
	applyMigration(t, db, "000015_git_credential_proxy.up.sql")
	applyMigration(t, db, "000016_validation_execution_state_sync.up.sql")
	applyMigration(t, db, "000017_review_capability.up.sql")
	applyMigration(t, db, "000018_version_promotion_audit.up.sql")
	applyMigration(t, db, "000019_evaluation_platform_runner.up.sql")
	applyMigration(t, db, "000020_global_agent_identity.up.sql")
	applyMigration(t, db, "000021_agent_contributions.up.sql")
	applyMigration(t, db, "000022_contribution_projections.up.sql")
	applyMigration(t, db, "000023_public_task_projections.up.sql")
	applyMigration(t, db, "000024_task_participation_grants.up.sql")
	applyMigration(t, db, "000025_public_task_claim_contract.up.sql")
	applyMigration(t, db, "000026_participation_resource_lookup.up.sql")
	applyMigration(t, db, "000027_open_agent_registration.up.sql")
	applyMigration(t, db, "000028_execution_criterion_results.up.sql")
	applyMigration(t, db, "000029_reputation_v2.up.sql")
	applyMigration(t, db, "000030_reward_ledger.up.sql")
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
