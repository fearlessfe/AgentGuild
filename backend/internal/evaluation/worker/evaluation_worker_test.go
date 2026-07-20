package worker_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	avpostgres "agentguild.dev/agentguild/backend/internal/agentversion/postgres"
	"agentguild.dev/agentguild/backend/internal/evaluation/application"
	evdomain "agentguild.dev/agentguild/backend/internal/evaluation/domain"
	evpostgres "agentguild.dev/agentguild/backend/internal/evaluation/postgres"
	"agentguild.dev/agentguild/backend/internal/evaluation/worker"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func insertAgent(t *testing.T, db *pgxpool.Pool, tenantID, agentID, ownerID string) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO agents (id, tenant_id, owner_id, owner_email, name, status)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		agentID, tenantID, ownerID, "owner@example.com", agentID, "active",
	)
	require.NoError(t, err)
}

func insertAgentVersion(t *testing.T, db *pgxpool.Pool, tenantID, agentID, versionID, status string) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO agent_versions (
			id, tenant_id, agent_id, version_number, status,
			runtime, model, capabilities, config_fingerprint, content_hash,
			environment_digest, prompt_ref, skill_refs, memory_ref, tool_refs,
			created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		versionID, tenantID, agentID, 1, status,
		"python", "gpt-4", []string{"code"}, "fingerprint", "content-hash",
		"env-digest", "sha256:prompt", []string{"sha256:skill"}, "sha256:memory", []string{"sha256:tool"},
		"owner", time.Now(),
	)
	require.NoError(t, err)
}

func insertPlatformTask(t *testing.T, db *pgxpool.Pool, tenantID, taskID, status string) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO tasks (tenant_id, id, publisher_agent_version_id, type, title, problem, deadline, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tenantID, taskID, "publisher-version", "evaluation", "title", "problem",
		time.Now().Add(time.Hour), status,
	)
	require.NoError(t, err)
}

func insertPlatformExecution(t *testing.T, db *pgxpool.Pool, tenantID, executionID, taskID, agentVersionID, status string, startedAt, submittedAt *time.Time) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO executions (tenant_id, id, task_id, agent_version_id, status, lease_generation, started_at, submitted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tenantID, executionID, taskID, agentVersionID, status, 1, startedAt, submittedAt,
	)
	require.NoError(t, err)
}

func insertPlatformSubmission(t *testing.T, db *pgxpool.Pool, tenantID, submissionID, taskID, executionID, status string) {
	t.Helper()
	now := time.Now()
	_, err := db.Exec(context.Background(), `
		INSERT INTO submissions (id, tenant_id, task_id, execution_id, branch, commit_sha, base_commit_sha, summary, diff_fingerprint, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		submissionID, tenantID, taskID, executionID, "agent/task", "deadbeef", "base0000", "summary", "fingerprint", status, now, now,
	)
	require.NoError(t, err)
}

func insertExecutionUsage(t *testing.T, db *pgxpool.Pool, tenantID, taskID, executionID, agentVersionID string, observedCost int64) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO execution_usage (tenant_id, task_id, execution_id, agent_version_id, observed_cost, coverage, provider, source_cursor, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		tenantID, taskID, executionID, agentVersionID, observedCost, "complete", "langfuse", randomID(), time.Now(),
	)
	require.NoError(t, err)
}

func versionStatus(t *testing.T, db *pgxpool.Pool, tenantID, versionID string) string {
	t.Helper()
	var status string
	require.NoError(t, db.QueryRow(context.Background(),
		`SELECT status FROM agent_versions WHERE tenant_id=$1 AND id=$2`, tenantID, versionID,
	).Scan(&status))
	return status
}

// fixture bundles the repositories the worker is assembled from.
type fixture struct {
	db       *pgxpool.Pool
	store    *evpostgres.Store
	bsRepo   application.BenchmarkSetRepository
	runRepo  application.EvaluationRunRepository
	runTasks application.RunTaskRepository
}

func newFixture(t *testing.T, runTimeout time.Duration) (*fixture, *worker.Worker) {
	t.Helper()
	db := testdb.StartPostgres(t)
	f := &fixture{
		db:       db,
		store:    evpostgres.NewStore(db),
		bsRepo:   evpostgres.NewBenchmarkSetRepository(db),
		runRepo:  evpostgres.NewEvaluationRunRepository(db),
		runTasks: evpostgres.NewEvaluationRunTaskRepository(db),
	}
	w := worker.NewWorker(
		f.store, f.runRepo, f.runTasks, f.bsRepo,
		avpostgres.NewVersionLifecycleAdapter(db),
		evpostgres.NewEvaluationTaskObserver(db),
		time.Second, runTimeout, 10, nil,
	)
	return f, w
}

// setupRun creates an agent version in "evaluating" status, a benchmark set
// with the given task definitions, a running evaluation run started at
// startedAt, and one run-task plan row per benchmark task. taskIDs maps
// task_ref to the published platform task id; refs missing from the map get an
// empty task id and a publish_error marker.
func (f *fixture) setupRun(
	t *testing.T,
	tenantID, agentID, versionID string,
	tasks []evdomain.BenchmarkTask,
	startedAt time.Time,
	taskIDs map[string]string,
) string {
	t.Helper()
	insertAgent(t, f.db, tenantID, agentID, "owner")
	insertAgentVersion(t, f.db, tenantID, agentID, versionID, "evaluating")

	bs, err := evdomain.NewBenchmarkSetWithTasks(randomID(), tenantID, "owner", 1, "Set", "", tasks, time.Now())
	require.NoError(t, err)
	require.NoError(t, f.store.WithTx(context.Background(), func(tx application.Tx) error {
		return f.bsRepo.Create(context.Background(), tx, bs)
	}))

	run, err := evdomain.NewEvaluationRun(randomID(), tenantID, versionID, bs.ID(), "env", evdomain.ScoringRuleVersionV1, startedAt)
	require.NoError(t, err)
	plan := make([]evdomain.EvaluationRunTask, 0, len(tasks))
	for _, task := range tasks {
		row := evdomain.EvaluationRunTask{
			TenantID: tenantID,
			RunID:    run.ID(),
			TaskRef:  task.TaskRef,
			Ordering: task.Ordering,
			Details:  map[string]any{},
		}
		if taskID, ok := taskIDs[task.TaskRef]; ok {
			row.TaskID = taskID
		} else {
			row.Details["publish_error"] = "publish boom"
		}
		plan = append(plan, row)
	}
	require.NoError(t, f.store.WithTx(context.Background(), func(tx application.Tx) error {
		if err := f.runRepo.Create(context.Background(), tx, run); err != nil {
			return err
		}
		return f.runTasks.Insert(context.Background(), tx, plan)
	}))
	return run.ID()
}

func runTasksByRef(t *testing.T, f *fixture, tenantID, runID string) map[string]evdomain.EvaluationRunTask {
	t.Helper()
	rows, err := f.runTasks.ListByRun(context.Background(), tenantID, runID)
	require.NoError(t, err)
	byRef := make(map[string]evdomain.EvaluationRunTask, len(rows))
	for _, row := range rows {
		byRef[row.TaskRef] = row
	}
	return byRef
}

func TestHarvestValidatedPassAndPendingOutcomes(t *testing.T) {
	f, w := newFixture(t, 24*time.Hour)
	tenantID, agentID, versionID := "tenant-harvest", "agent-harvest", randomID()
	base := time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	started := base
	submitted := base.Add(2 * time.Second)

	runID := f.setupRun(t, tenantID, agentID, versionID, []evdomain.BenchmarkTask{
		{TaskRef: "task-1", Ordering: 0},
		{TaskRef: "task-2", Ordering: 1},
		{TaskRef: "task-3", Ordering: 2},
	}, time.Now(), map[string]string{"task-1": "pt-1", "task-2": "pt-2", "task-3": "pt-3"})

	// task-1: validated submission by the evaluated version with observed cost.
	insertPlatformTask(t, f.db, tenantID, "pt-1", "in_progress")
	insertPlatformExecution(t, f.db, tenantID, "ex-1", "pt-1", versionID, "reviewing", &started, &submitted)
	insertPlatformSubmission(t, f.db, tenantID, "sub-1", "pt-1", "ex-1", "validated")
	insertExecutionUsage(t, f.db, tenantID, "pt-1", "ex-1", versionID, 42)
	// task-2: claimed but still running, no submission yet.
	insertPlatformTask(t, f.db, tenantID, "pt-2", "in_progress")
	insertPlatformExecution(t, f.db, tenantID, "ex-2", "pt-2", versionID, "running", &started, nil)
	// task-3: submitted, validation still pending.
	insertPlatformTask(t, f.db, tenantID, "pt-3", "in_progress")
	insertPlatformExecution(t, f.db, tenantID, "ex-3", "pt-3", versionID, "validating", &started, &submitted)
	insertPlatformSubmission(t, f.db, tenantID, "sub-3", "pt-3", "ex-3", "pending_verification")

	require.NoError(t, w.RunOnce(context.Background()))

	rows := runTasksByRef(t, f, tenantID, runID)
	passed := rows["task-1"]
	require.True(t, passed.Resolved)
	require.NotNil(t, passed.Passed)
	require.True(t, *passed.Passed)
	require.NotNil(t, passed.LatencyMs)
	require.Equal(t, 2000.0, *passed.LatencyMs)
	require.NotNil(t, passed.CostCents)
	require.Equal(t, int64(42), *passed.CostCents)
	require.Equal(t, "validated", passed.Details["resolution"])
	require.Equal(t, "ex-1", passed.Details["execution_id"])
	require.NotNil(t, passed.ResolvedAt)

	require.False(t, rows["task-2"].Resolved, "running execution must stay unresolved")
	require.False(t, rows["task-3"].Resolved, "pending validation must stay unresolved")

	// Not everything resolved: the run stays running and produces no results.
	run, err := f.runRepo.GetByID(context.Background(), tenantID, runID)
	require.NoError(t, err)
	require.Equal(t, evdomain.StatusRunning, run.Status())
	results, err := f.runRepo.ListResults(context.Background(), tenantID, runID)
	require.NoError(t, err)
	require.Empty(t, results)
	require.Equal(t, "evaluating", versionStatus(t, f.db, tenantID, versionID))
}

func TestHarvestFailureOutcomesCompleteRejected(t *testing.T) {
	f, w := newFixture(t, 24*time.Hour)
	tenantID, agentID, versionID := "tenant-fail", "agent-fail", randomID()
	otherVersionID := randomID()
	base := time.Now().Add(-time.Minute).Truncate(time.Millisecond)

	runID := f.setupRun(t, tenantID, agentID, versionID, []evdomain.BenchmarkTask{
		{TaskRef: "t-validation-failed", Ordering: 0},
		{TaskRef: "t-expired", Ordering: 1},
		{TaskRef: "t-cancelled", Ordering: 2},
		{TaskRef: "t-wrong-version", Ordering: 3},
		{TaskRef: "t-publish-error", Ordering: 4},
	}, time.Now(), map[string]string{
		"t-validation-failed": "pt-1",
		"t-expired":           "pt-2",
		"t-cancelled":         "pt-3",
		"t-wrong-version":     "pt-4",
		// t-publish-error was never published: no task id, publish_error marker.
	})

	// Validation failed terminally.
	insertPlatformTask(t, f.db, tenantID, "pt-1", "in_progress")
	insertPlatformExecution(t, f.db, tenantID, "ex-1", "pt-1", versionID, "reviewing", &base, &base)
	insertPlatformSubmission(t, f.db, tenantID, "sub-1", "pt-1", "ex-1", "validation_failed")
	// The platform task expired.
	insertPlatformTask(t, f.db, tenantID, "pt-2", "expired")
	insertPlatformExecution(t, f.db, tenantID, "ex-2", "pt-2", versionID, "expired", &base, nil)
	// The platform task was cancelled.
	insertPlatformTask(t, f.db, tenantID, "pt-3", "cancelled")
	// Claimed by a different agent version (contaminated evidence).
	insertPlatformTask(t, f.db, tenantID, "pt-4", "in_progress")
	insertPlatformExecution(t, f.db, tenantID, "ex-4", "pt-4", otherVersionID, "running", &base, nil)

	require.NoError(t, w.RunOnce(context.Background()))

	rows := runTasksByRef(t, f, tenantID, runID)
	for ref, row := range rows {
		require.True(t, row.Resolved, "%s must be resolved", ref)
		require.NotNil(t, row.Passed)
		require.False(t, *row.Passed, "%s must be failed", ref)
	}
	require.Equal(t, "validation_failed", rows["t-validation-failed"].Details["resolution"])
	require.Equal(t, "validation_failed", rows["t-validation-failed"].Details["submission_state"])
	require.Equal(t, "task_expired", rows["t-expired"].Details["resolution"])
	require.Equal(t, "task_cancelled", rows["t-cancelled"].Details["resolution"])
	require.Equal(t, "version_mismatch", rows["t-wrong-version"].Details["resolution"])
	require.Equal(t, versionID, rows["t-wrong-version"].Details["expected_agent_version_id"])
	require.Equal(t, otherVersionID, rows["t-wrong-version"].Details["actual_agent_version_id"])
	require.Equal(t, "publish_failed", rows["t-publish-error"].Details["resolution"])
	require.Equal(t, "publish boom", rows["t-publish-error"].Details["publish_error"], "publish error marker must be preserved")

	// All rows resolved: the run completed as failed with per-task results and
	// the version was rejected.
	run, err := f.runRepo.GetByID(context.Background(), tenantID, runID)
	require.NoError(t, err)
	require.Equal(t, evdomain.StatusFailed, run.Status())
	require.False(t, run.IsPassed())
	require.NotNil(t, run.CompletedAt())
	require.Equal(t, application.PlatformBenchmarkExecutorID, run.Summary().Executor)
	require.Equal(t, 0.0, run.Summary().PassRate)

	results, err := f.runRepo.ListResults(context.Background(), tenantID, runID)
	require.NoError(t, err)
	require.Len(t, results, 5)
	for _, result := range results {
		require.False(t, result.Passed)
		require.Equal(t, 0.0, result.Score)
	}
	require.Equal(t, "rejected", versionStatus(t, f.db, tenantID, versionID))
}

func TestWorkerCompletesEligibleRun(t *testing.T) {
	f, w := newFixture(t, 24*time.Hour)
	tenantID, agentID, versionID := "tenant-pass", "agent-pass", randomID()
	base := time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	started1, submitted1 := base, base.Add(100*time.Millisecond)
	started2, submitted2 := base, base.Add(300*time.Millisecond)

	runID := f.setupRun(t, tenantID, agentID, versionID, []evdomain.BenchmarkTask{
		{TaskRef: "task-1", Ordering: 0, IsSecurity: true},
		{TaskRef: "task-2", Ordering: 1},
	}, time.Now(), map[string]string{"task-1": "pt-1", "task-2": "pt-2"})

	insertPlatformTask(t, f.db, tenantID, "pt-1", "in_progress")
	insertPlatformExecution(t, f.db, tenantID, "ex-1", "pt-1", versionID, "reviewing", &started1, &submitted1)
	insertPlatformSubmission(t, f.db, tenantID, "sub-1", "pt-1", "ex-1", "validated")
	insertExecutionUsage(t, f.db, tenantID, "pt-1", "ex-1", versionID, 10)
	// task-2 has no cost observation: the worker counts 0 and annotates it.
	insertPlatformTask(t, f.db, tenantID, "pt-2", "in_progress")
	insertPlatformExecution(t, f.db, tenantID, "ex-2", "pt-2", versionID, "reviewing", &started2, &submitted2)
	insertPlatformSubmission(t, f.db, tenantID, "sub-2", "pt-2", "ex-2", "validated")

	require.NoError(t, w.RunOnce(context.Background()))

	rows := runTasksByRef(t, f, tenantID, runID)
	require.True(t, rows["task-1"].Resolved)
	require.Equal(t, 100.0, *rows["task-1"].LatencyMs)
	require.Equal(t, int64(10), *rows["task-1"].CostCents)
	require.True(t, rows["task-2"].Resolved)
	require.Equal(t, 300.0, *rows["task-2"].LatencyMs)
	require.Equal(t, int64(0), *rows["task-2"].CostCents)
	require.Equal(t, false, rows["task-2"].Details["cost_observed"])

	run, err := f.runRepo.GetByID(context.Background(), tenantID, runID)
	require.NoError(t, err)
	require.Equal(t, evdomain.StatusPassed, run.Status())
	require.True(t, run.IsPassed())
	require.NotNil(t, run.CompletedAt())

	summary := run.Summary()
	require.Equal(t, application.PlatformBenchmarkExecutorID, summary.Executor)
	require.Equal(t, 1.0, summary.PassRate)
	require.Equal(t, 200.0, summary.AvgLatencyMs)
	require.Equal(t, int64(10), summary.CostCents)
	require.True(t, summary.SecurityPassed)

	results, err := f.runRepo.ListResults(context.Background(), tenantID, runID)
	require.NoError(t, err)
	require.Len(t, results, 2)
	for _, result := range results {
		require.True(t, result.Passed)
		require.Equal(t, 1.0, result.Score)
	}
	require.Equal(t, "eligible", versionStatus(t, f.db, tenantID, versionID))
}

func TestWorkerRunTimeoutFailsUnresolved(t *testing.T) {
	f, w := newFixture(t, 24*time.Hour)
	tenantID, agentID, versionID := "tenant-timeout", "agent-timeout", randomID()
	base := time.Now().Add(-time.Minute).Truncate(time.Millisecond)

	// The run started 25h ago, beyond the 24h run timeout.
	runID := f.setupRun(t, tenantID, agentID, versionID, []evdomain.BenchmarkTask{
		{TaskRef: "task-1", Ordering: 0},
	}, time.Now().Add(-25*time.Hour), map[string]string{"task-1": "pt-1"})

	// The execution is still running: no terminal outcome ever arrives.
	insertPlatformTask(t, f.db, tenantID, "pt-1", "in_progress")
	insertPlatformExecution(t, f.db, tenantID, "ex-1", "pt-1", versionID, "running", &base, nil)

	require.NoError(t, w.RunOnce(context.Background()))

	rows := runTasksByRef(t, f, tenantID, runID)
	require.True(t, rows["task-1"].Resolved)
	require.NotNil(t, rows["task-1"].Passed)
	require.False(t, *rows["task-1"].Passed)
	require.Equal(t, "run_timeout", rows["task-1"].Details["resolution"])

	run, err := f.runRepo.GetByID(context.Background(), tenantID, runID)
	require.NoError(t, err)
	require.Equal(t, evdomain.StatusFailed, run.Status())
	require.Equal(t, application.PlatformBenchmarkExecutorID, run.Summary().Executor)
	require.Equal(t, "rejected", versionStatus(t, f.db, tenantID, versionID))
}

func TestWorkerConcurrentTicksCompleteOnce(t *testing.T) {
	f, w := newFixture(t, 24*time.Hour)
	tenantID, agentID, versionID := "tenant-once", "agent-once", randomID()
	base := time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	submitted := base.Add(100 * time.Millisecond)

	runID := f.setupRun(t, tenantID, agentID, versionID, []evdomain.BenchmarkTask{
		{TaskRef: "task-1", Ordering: 0},
		{TaskRef: "task-2", Ordering: 1},
	}, time.Now(), map[string]string{"task-1": "pt-1", "task-2": "pt-2"})
	for _, ref := range []string{"pt-1", "pt-2"} {
		execID := "ex-" + ref
		insertPlatformTask(t, f.db, tenantID, ref, "in_progress")
		insertPlatformExecution(t, f.db, tenantID, execID, ref, versionID, "reviewing", &base, &submitted)
		insertPlatformSubmission(t, f.db, tenantID, "sub-"+ref, ref, execID, "validated")
	}

	// Two ticks race for the same run; the row lock serializes them so exactly
	// one completes it.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = w.RunOnce(context.Background())
		}(i)
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	// A later tick is a no-op: still exactly one completion.
	require.NoError(t, w.RunOnce(context.Background()))

	run, err := f.runRepo.GetByID(context.Background(), tenantID, runID)
	require.NoError(t, err)
	require.Equal(t, evdomain.StatusPassed, run.Status())
	results, err := f.runRepo.ListResults(context.Background(), tenantID, runID)
	require.NoError(t, err)
	require.Len(t, results, 2, "results must be written exactly once")
	require.Equal(t, "eligible", versionStatus(t, f.db, tenantID, versionID))
}
