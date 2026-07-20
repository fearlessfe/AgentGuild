package postgres_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/evaluation/application"
	evdomain "agentguild.dev/agentguild/backend/internal/evaluation/domain"
	evpostgres "agentguild.dev/agentguild/backend/internal/evaluation/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestBenchmarkSetTaskDefinitionRoundTrip(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := evpostgres.NewStore(db)
	repo := evpostgres.NewBenchmarkSetRepository(db)

	tenantID := "tenant-task-def"
	bs, err := evdomain.NewBenchmarkSetWithTasks(
		randomID(), tenantID, "owner", 1,
		"Set", "",
		[]evdomain.BenchmarkTask{
			{
				TaskRef:      "task-1",
				Ordering:     0,
				Title:        "Fix the bug",
				Problem:      "The thing is broken",
				Constraints:  []string{"repo:org/repo", "base_commit:abc123"},
				Requirements: []string{"all tests pass"},
				IsSecurity:   true,
			},
			{TaskRef: "task-2", Ordering: 1},
		},
		time.Now(),
	)
	require.NoError(t, err)
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return repo.Create(context.Background(), tx, bs)
	}))

	loaded, err := repo.GetByID(context.Background(), tenantID, bs.ID())
	require.NoError(t, err)
	require.Len(t, loaded.Tasks(), 2)

	first := loaded.Tasks()[0]
	require.Equal(t, "task-1", first.TaskRef)
	require.Equal(t, "Fix the bug", first.Title)
	require.Equal(t, "The thing is broken", first.Problem)
	require.Equal(t, []string{"repo:org/repo", "base_commit:abc123"}, first.Constraints)
	require.Equal(t, []string{"all tests pass"}, first.Requirements)
	require.True(t, first.IsSecurity)

	// Legacy rows without a definition read back with zero values.
	second := loaded.Tasks()[1]
	require.Equal(t, "", second.Title)
	require.Empty(t, second.Constraints)
	require.Empty(t, second.Requirements)
	require.False(t, second.IsSecurity)
}

func TestEvaluationRunTaskRepositoryRoundTrip(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := evpostgres.NewStore(db)
	bsRepo := evpostgres.NewBenchmarkSetRepository(db)
	runRepo := evpostgres.NewEvaluationRunRepository(db)
	runTasks := evpostgres.NewEvaluationRunTaskRepository(db)

	tenantID := "tenant-run-tasks"
	agentID := "agent-run-tasks"
	ownerID := "owner"
	versionID := randomID()
	insertAgent(t, db, tenantID, agentID, ownerID)
	insertAgentVersion(t, db, tenantID, agentID, versionID, 1, "draft")

	bs, _ := evdomain.NewBenchmarkSetWithTasks(randomID(), tenantID, ownerID, 1, "Set", "", nil, time.Now())
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return bsRepo.Create(context.Background(), tx, bs)
	}))
	run, err := evdomain.NewEvaluationRun(randomID(), tenantID, versionID, bs.ID(), "env", evdomain.ScoringRuleVersionV1, time.Now())
	require.NoError(t, err)
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return runRepo.Create(context.Background(), tx, run)
	}))

	plan := []evdomain.EvaluationRunTask{
		{TenantID: tenantID, RunID: run.ID(), TaskRef: "task-1", Ordering: 0, Details: map[string]any{}},
		{TenantID: tenantID, RunID: run.ID(), TaskRef: "task-2", Ordering: 1, Details: map[string]any{}},
	}
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return runTasks.Insert(context.Background(), tx, plan)
	}))

	// Backfill the published platform task id and record a publish failure.
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		if err := runTasks.SetTaskID(context.Background(), tx, tenantID, run.ID(), "task-1", "platform-task-1"); err != nil {
			return err
		}
		return runTasks.RecordPublishFailure(context.Background(), tx, tenantID, run.ID(), "task-2", "publish boom")
	}))

	loaded, err := runTasks.ListByRun(context.Background(), tenantID, run.ID())
	require.NoError(t, err)
	require.Len(t, loaded, 2)

	require.Equal(t, "task-1", loaded[0].TaskRef)
	require.Equal(t, "platform-task-1", loaded[0].TaskID)
	require.Equal(t, 0, loaded[0].Ordering)
	require.False(t, loaded[0].Resolved)
	require.Nil(t, loaded[0].Passed)
	require.Nil(t, loaded[0].LatencyMs)
	require.Nil(t, loaded[0].CostCents)
	require.False(t, loaded[0].CreatedAt.IsZero())
	require.Nil(t, loaded[0].ResolvedAt)

	require.Equal(t, "task-2", loaded[1].TaskRef)
	require.Equal(t, "", loaded[1].TaskID)
	require.Equal(t, "publish boom", loaded[1].Details["publish_error"])

	// Updating an unknown mapping fails with not_found.
	err = store.WithTx(context.Background(), func(tx application.Tx) error {
		return runTasks.SetTaskID(context.Background(), tx, tenantID, run.ID(), "task-unknown", "x")
	})
	require.ErrorIs(t, err, evdomain.ErrNotFound)
}

func TestEvaluationRunTaskRepositoryResolveAndListUnresolved(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := evpostgres.NewStore(db)
	bsRepo := evpostgres.NewBenchmarkSetRepository(db)
	runRepo := evpostgres.NewEvaluationRunRepository(db)
	runTasks := evpostgres.NewEvaluationRunTaskRepository(db)

	tenantID := "tenant-resolve"
	agentID := "agent-resolve"
	ownerID := "owner"
	versionID := randomID()
	insertAgent(t, db, tenantID, agentID, ownerID)
	insertAgentVersion(t, db, tenantID, agentID, versionID, 1, "draft")

	bs, _ := evdomain.NewBenchmarkSetWithTasks(randomID(), tenantID, ownerID, 1, "Set", "", nil, time.Now())
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return bsRepo.Create(context.Background(), tx, bs)
	}))
	run, err := evdomain.NewEvaluationRun(randomID(), tenantID, versionID, bs.ID(), "env", evdomain.ScoringRuleVersionV1, time.Now())
	require.NoError(t, err)
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return runRepo.Create(context.Background(), tx, run)
	}))

	plan := []evdomain.EvaluationRunTask{
		{TenantID: tenantID, RunID: run.ID(), TaskRef: "task-1", Ordering: 0, TaskID: "pt-1", Details: map[string]any{}},
		{TenantID: tenantID, RunID: run.ID(), TaskRef: "task-2", Ordering: 1, TaskID: "pt-2", Details: map[string]any{"publish_error": "boom"}},
	}
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return runTasks.Insert(context.Background(), tx, plan)
	}))

	// Both rows start unresolved.
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		unresolved, err := runTasks.ListUnresolved(context.Background(), tx, tenantID, run.ID())
		require.NoError(t, err)
		require.Len(t, unresolved, 2)
		return nil
	}))

	// Resolve one row with an outcome; the details are merged into the
	// existing document.
	latencyMs := 125.5
	costCents := int64(7)
	resolvedAt := time.Now().Truncate(time.Millisecond)
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return runTasks.Resolve(context.Background(), tx, tenantID, run.ID(), "task-1", application.RunTaskResolution{
			Passed:     true,
			LatencyMs:  &latencyMs,
			CostCents:  &costCents,
			Details:    map[string]any{"resolution": "validated"},
			ResolvedAt: resolvedAt,
		})
	}))

	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		unresolved, err := runTasks.ListUnresolved(context.Background(), tx, tenantID, run.ID())
		require.NoError(t, err)
		require.Len(t, unresolved, 1)
		require.Equal(t, "task-2", unresolved[0].TaskRef)

		all, err := runTasks.ListByRunTx(context.Background(), tx, tenantID, run.ID())
		require.NoError(t, err)
		require.Len(t, all, 2)
		require.True(t, all[0].Resolved)
		require.NotNil(t, all[0].Passed)
		require.True(t, *all[0].Passed)
		require.NotNil(t, all[0].LatencyMs)
		require.Equal(t, latencyMs, *all[0].LatencyMs)
		require.NotNil(t, all[0].CostCents)
		require.Equal(t, costCents, *all[0].CostCents)
		require.Equal(t, "validated", all[0].Details["resolution"])
		require.NotNil(t, all[0].ResolvedAt)
		require.True(t, resolvedAt.Equal(*all[0].ResolvedAt))
		require.False(t, all[1].Resolved)
		require.Equal(t, "boom", all[1].Details["publish_error"])
		return nil
	}))

	// Resolving an already resolved row conflicts, keeping ticks idempotent.
	err = store.WithTx(context.Background(), func(tx application.Tx) error {
		return runTasks.Resolve(context.Background(), tx, tenantID, run.ID(), "task-1", application.RunTaskResolution{
			Passed:     false,
			Details:    map[string]any{"resolution": "run_timeout"},
			ResolvedAt: resolvedAt,
		})
	})
	require.ErrorIs(t, err, evdomain.ErrStateConflict)

	// The pool-based ListByRun sees the committed resolution.
	loaded, err := runTasks.ListByRun(context.Background(), tenantID, run.ID())
	require.NoError(t, err)
	require.True(t, loaded[0].Resolved)
	require.False(t, loaded[1].Resolved)
}
