package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	avapplication "agentguild.dev/agentguild/backend/internal/agentversion/application"
	"agentguild.dev/agentguild/backend/internal/evaluation/application"
	evdomain "agentguild.dev/agentguild/backend/internal/evaluation/domain"
	evpostgres "agentguild.dev/agentguild/backend/internal/evaluation/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func insertAgent(t *testing.T, db *pgxpool.Pool, tenantID, agentID, ownerID string) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO agents (id, tenant_id, owner_id, owner_email, name, status)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		agentID, tenantID, ownerID, "owner@example.com", agentID, "active",
	)
	require.NoError(t, err)
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func insertAgentVersion(t *testing.T, db *pgxpool.Pool, tenantID, agentID, versionID string, versionNumber int, status string) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO agent_versions (
			id, tenant_id, agent_id, version_number, status,
			runtime, model, capabilities, config_fingerprint, content_hash,
			environment_digest, prompt_ref, skill_refs, memory_ref, tool_refs,
			created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		versionID, tenantID, agentID, versionNumber, status,
		"python", "gpt-4", []string{"code"}, "fingerprint", "content-hash",
		"env-digest", "sha256:prompt", []string{"sha256:skill"}, "sha256:memory", []string{"sha256:tool"},
		"owner", time.Now(),
	)
	require.NoError(t, err)
}

func TestRepositoryCreateAndGetBenchmarkSet(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := evpostgres.NewStore(db)
	repo := evpostgres.NewBenchmarkSetRepository(db)

	tenantID := "tenant-bs"
	bs, err := evdomain.NewBenchmarkSetWithTasks(
		randomID(), tenantID, "owner", 1,
		"Set 1", "first set",
		[]evdomain.BenchmarkTask{
			{TaskRef: "task-1", Ordering: 0},
			{TaskRef: "task-2", Ordering: 1},
		},
		time.Now(),
	)
	require.NoError(t, err)

	err = store.WithTx(context.Background(), func(tx application.Tx) error {
		return repo.Create(context.Background(), tx, bs)
	})
	require.NoError(t, err)

	loaded, err := repo.GetByID(context.Background(), tenantID, bs.ID())
	require.NoError(t, err)
	require.Equal(t, bs.ID(), loaded.ID())
	require.Equal(t, 1, loaded.VersionNumber())
	require.Len(t, loaded.Tasks(), 2)
	require.Equal(t, "task-1", loaded.Tasks()[0].TaskRef)
}

func TestRepositoryBenchmarkSetVersionNumberMonotonic(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := evpostgres.NewStore(db)
	repo := evpostgres.NewBenchmarkSetRepository(db)

	tenantID := "tenant-bs-vn"
	var numbers []int
	for i := 0; i < 3; i++ {
		err := store.WithTx(context.Background(), func(tx application.Tx) error {
			vn, err := repo.NextVersionNumber(context.Background(), tx, tenantID)
			if err != nil {
				return err
			}
			bs, err := evdomain.NewBenchmarkSetWithTasks(
				randomID(), tenantID, "owner", vn,
				"Set", "desc", nil, time.Now(),
			)
			if err != nil {
				return err
			}
			numbers = append(numbers, bs.VersionNumber())
			return repo.Create(context.Background(), tx, bs)
		})
		require.NoError(t, err)
	}
	require.Equal(t, []int{1, 2, 3}, numbers)
}

func TestRepositorySetActiveBenchmarkSet(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := evpostgres.NewStore(db)
	repo := evpostgres.NewBenchmarkSetRepository(db)

	tenantID := "tenant-active"
	bs1, _ := evdomain.NewBenchmarkSetWithTasks(randomID(), tenantID, "owner", 1, "A", "", nil, time.Now())
	bs2, _ := evdomain.NewBenchmarkSetWithTasks(randomID(), tenantID, "owner", 2, "B", "", nil, time.Now())

	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		if err := repo.Create(context.Background(), tx, bs1); err != nil {
			return err
		}
		return repo.Create(context.Background(), tx, bs2)
	}))

	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		if err := repo.SetInactiveAll(context.Background(), tx, tenantID); err != nil {
			return err
		}
		return repo.SetActive(context.Background(), tx, tenantID, bs2.ID())
	}))

	active, err := repo.GetActiveByTenant(context.Background(), tenantID)
	require.NoError(t, err)
	require.Equal(t, bs2.ID(), active.ID())

	loaded, err := repo.GetByID(context.Background(), tenantID, bs1.ID())
	require.NoError(t, err)
	require.False(t, loaded.IsActive())
}

func TestRepositoryCreateAndCompleteEvaluationRun(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := evpostgres.NewStore(db)
	bsRepo := evpostgres.NewBenchmarkSetRepository(db)
	runRepo := evpostgres.NewEvaluationRunRepository(db)

	tenantID := "tenant-run"
	agentID := "agent-run"
	ownerID := "owner"
	versionID := randomID()
	insertAgent(t, db, tenantID, agentID, ownerID)
	insertAgentVersion(t, db, tenantID, agentID, versionID, 1, "draft")

	bs, _ := evdomain.NewBenchmarkSetWithTasks(
		randomID(), tenantID, ownerID, 1, "Set", "", nil, time.Now(),
	)
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return bsRepo.Create(context.Background(), tx, bs)
	}))

	run, err := evdomain.NewEvaluationRun(
		randomID(), tenantID, versionID, bs.ID(),
		"env-digest", evdomain.ScoringRuleVersionV1, time.Now(),
	)
	require.NoError(t, err)

	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return runRepo.Create(context.Background(), tx, run)
	}))

	loaded, err := runRepo.GetByID(context.Background(), tenantID, run.ID())
	require.NoError(t, err)
	require.Equal(t, evdomain.StatusRunning, loaded.Status())

	thresholds := []evdomain.ThresholdResult{
		{Name: "security_regression", Passed: true},
		{Name: "pass_rate", Passed: true},
	}
	summary := evdomain.EvaluationSummary{PassRate: 1.0}
	require.NoError(t, run.CompleteAt(thresholds, summary, time.Now()))

	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return runRepo.Complete(context.Background(), tx, run)
	}))

	completed, err := runRepo.GetByID(context.Background(), tenantID, run.ID())
	require.NoError(t, err)
	require.Equal(t, evdomain.StatusPassed, completed.Status())
	require.True(t, completed.IsPassed())
}

func TestRepositoryListEvaluationRunResults(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := evpostgres.NewStore(db)
	bsRepo := evpostgres.NewBenchmarkSetRepository(db)
	runRepo := evpostgres.NewEvaluationRunRepository(db)

	tenantID := "tenant-results"
	agentID := "agent-results"
	ownerID := "owner"
	versionID := randomID()
	insertAgent(t, db, tenantID, agentID, ownerID)
	insertAgentVersion(t, db, tenantID, agentID, versionID, 1, "draft")

	bs, _ := evdomain.NewBenchmarkSetWithTasks(
		randomID(), tenantID, ownerID, 1, "Set", "", nil, time.Now(),
	)
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return bsRepo.Create(context.Background(), tx, bs)
	}))

	run, _ := evdomain.NewEvaluationRun(
		randomID(), tenantID, versionID, bs.ID(),
		"env", evdomain.ScoringRuleVersionV1, time.Now(),
	)
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		if err := runRepo.Create(context.Background(), tx, run); err != nil {
			return err
		}
		return runRepo.CreateResult(context.Background(), tx, &evdomain.EvaluationRunResult{
			EvaluationRunID: run.ID(),
			TenantID:        tenantID,
			TaskRef:         "task-1",
			Score:           1.0,
			Passed:          true,
			Details:         map[string]any{"latency_ms": 100},
		})
	}))

	results, err := runRepo.ListResults(context.Background(), tenantID, run.ID())
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "task-1", results[0].TaskRef)
	require.Equal(t, 100.0, results[0].Details["latency_ms"])
}

func TestRepositoryListRunningAndLockRunningForUpdate(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := evpostgres.NewStore(db)
	bsRepo := evpostgres.NewBenchmarkSetRepository(db)
	runRepo := evpostgres.NewEvaluationRunRepository(db)

	tenantID := "tenant-running"
	agentID := "agent-running"
	ownerID := "owner"
	versionID := randomID()
	insertAgent(t, db, tenantID, agentID, ownerID)
	insertAgentVersion(t, db, tenantID, agentID, versionID, 1, "draft")

	bs, _ := evdomain.NewBenchmarkSetWithTasks(randomID(), tenantID, ownerID, 1, "Set", "", nil, time.Now())
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return bsRepo.Create(context.Background(), tx, bs)
	}))

	older, _ := evdomain.NewEvaluationRun(randomID(), tenantID, versionID, bs.ID(), "env", evdomain.ScoringRuleVersionV1, time.Now().Add(-time.Hour))
	newer, _ := evdomain.NewEvaluationRun(randomID(), tenantID, versionID, bs.ID(), "env", evdomain.ScoringRuleVersionV1, time.Now())
	completed, _ := evdomain.NewEvaluationRun(randomID(), tenantID, versionID, bs.ID(), "env", evdomain.ScoringRuleVersionV1, time.Now().Add(-2*time.Hour))
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		if err := runRepo.Create(context.Background(), tx, older); err != nil {
			return err
		}
		if err := runRepo.Create(context.Background(), tx, newer); err != nil {
			return err
		}
		if err := runRepo.Create(context.Background(), tx, completed); err != nil {
			return err
		}
		if err := completed.CompleteAt([]evdomain.ThresholdResult{{Name: "pass_rate", Passed: true}}, evdomain.EvaluationSummary{}, time.Now()); err != nil {
			return err
		}
		return runRepo.Complete(context.Background(), tx, completed)
	}))

	// ListRunning returns running runs only, oldest first, bounded by batch size.
	running, err := runRepo.ListRunning(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, running, 2)
	require.Equal(t, older.ID(), running[0].ID())
	require.Equal(t, newer.ID(), running[1].ID())

	running, err = runRepo.ListRunning(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, running, 1)
	require.Equal(t, older.ID(), running[0].ID())

	_, err = runRepo.ListRunning(context.Background(), 0)
	require.Error(t, err)

	// LockRunningForUpdate locks running rows and reports completed or missing
	// runs as not_found so concurrent ticks do not complete a run twice.
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		locked, err := runRepo.LockRunningForUpdate(context.Background(), tx, tenantID, older.ID())
		require.NoError(t, err)
		require.Equal(t, older.ID(), locked.ID())
		require.Equal(t, evdomain.StatusRunning, locked.Status())
		return nil
	}))
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		_, err := runRepo.LockRunningForUpdate(context.Background(), tx, tenantID, completed.ID())
		require.ErrorIs(t, err, evdomain.ErrNotFound)
		return nil
	}))
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		_, err := runRepo.LockRunningForUpdate(context.Background(), tx, tenantID, "missing")
		require.ErrorIs(t, err, evdomain.ErrNotFound)
		return nil
	}))
}

func TestRepositoryGetLatestPassedForAgentVersion(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := evpostgres.NewStore(db)
	bsRepo := evpostgres.NewBenchmarkSetRepository(db)
	runRepo := evpostgres.NewEvaluationRunRepository(db)
	runProvider := evpostgres.NewEvaluationRunProvider(db)

	tenantID := "tenant-latest"
	agentID := "agent-latest"
	ownerID := "owner"
	versionID := randomID()
	insertAgent(t, db, tenantID, agentID, ownerID)
	insertAgentVersion(t, db, tenantID, agentID, versionID, 1, "draft")

	bs, _ := evdomain.NewBenchmarkSetWithTasks(
		randomID(), tenantID, ownerID, 1, "Set", "", nil, time.Now(),
	)
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		return bsRepo.Create(context.Background(), tx, bs)
	}))

	run1, _ := evdomain.NewEvaluationRun(randomID(), tenantID, versionID, bs.ID(), "env", evdomain.ScoringRuleVersionV1, time.Now())
	run2, _ := evdomain.NewEvaluationRun(randomID(), tenantID, versionID, bs.ID(), "env", evdomain.ScoringRuleVersionV1, time.Now().Add(time.Second))
	require.NoError(t, store.WithTx(context.Background(), func(tx application.Tx) error {
		if err := runRepo.Create(context.Background(), tx, run1); err != nil {
			return err
		}
		if err := runRepo.Create(context.Background(), tx, run2); err != nil {
			return err
		}
		if err := run1.CompleteAt([]evdomain.ThresholdResult{{Name: "pass_rate", Passed: false}}, evdomain.EvaluationSummary{}, time.Now()); err != nil {
			return err
		}
		if err := runRepo.Complete(context.Background(), tx, run1); err != nil {
			return err
		}
		if err := run2.CompleteAt([]evdomain.ThresholdResult{{Name: "pass_rate", Passed: true}}, evdomain.EvaluationSummary{}, time.Now().Add(time.Second)); err != nil {
			return err
		}
		return runRepo.Complete(context.Background(), tx, run2)
	}))

	var info *avapplication.EvaluationRunInfo
	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		var err error
		info, err = runProvider.GetLatestPassed(context.Background(), tx, tenantID, versionID)
		return err
	})
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, run2.ID(), info.ID)
	require.Equal(t, "passed", info.Status)
}
