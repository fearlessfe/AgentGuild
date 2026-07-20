package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/evaluation/application"
	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	evpostgres "agentguild.dev/agentguild/backend/internal/evaluation/postgres"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type fakePublisher struct {
	cmds    []application.PublishEvaluationTaskCommand
	nextID  int
	failRef string
	failErr error
}

func (f *fakePublisher) PublishEvaluationTask(ctx context.Context, cmd application.PublishEvaluationTaskCommand) (string, error) {
	f.cmds = append(f.cmds, cmd)
	if f.failRef != "" && strings.HasSuffix(cmd.RequestID, ":"+f.failRef) {
		return "", f.failErr
	}
	f.nextID++
	return "task-id-" + cmd.RequestID, nil
}

func newPlatformService(
	t *testing.T,
	db *pgxpool.Pool,
	versions application.VersionLifecyclePort,
	publisher application.EvaluationTaskPublisher,
) *application.EvaluationService {
	t.Helper()
	store := evpostgres.NewStore(db)
	bsRepo := evpostgres.NewBenchmarkSetRepository(db)
	runRepo := evpostgres.NewEvaluationRunRepository(db)
	policy := application.NewPolicy(fakeOwnerProvider{})
	svc, err := application.NewEvaluationService(store, bsRepo, runRepo, versions,
		application.NewPlatformBenchmarkExecutor(), policy, application.EvaluationOptions{
			TaskPublisher: publisher,
			RunTasks:      evpostgres.NewEvaluationRunTaskRepository(db),
			TaskDeadline:  time.Hour,
		})
	require.NoError(t, err)
	return svc
}

func TestNewEvaluationServiceRequiresPublisherAndRunTasksTogether(t *testing.T) {
	db := testdb.StartPostgres(t)
	store := evpostgres.NewStore(db)
	bsRepo := evpostgres.NewBenchmarkSetRepository(db)
	runRepo := evpostgres.NewEvaluationRunRepository(db)
	policy := application.NewPolicy(fakeOwnerProvider{})

	_, err := application.NewEvaluationService(store, bsRepo, runRepo, newFakeVersionLifecycle(),
		application.NewPlatformBenchmarkExecutor(), policy, application.EvaluationOptions{
			TaskPublisher: &fakePublisher{},
		})
	require.Error(t, err)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))

	_, err = application.NewEvaluationService(store, bsRepo, runRepo, newFakeVersionLifecycle(),
		application.NewPlatformBenchmarkExecutor(), policy, application.EvaluationOptions{
			RunTasks: evpostgres.NewEvaluationRunTaskRepository(db),
		})
	require.Error(t, err)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))
}

func TestStartEvaluationRunPlatformPublishesTasksAndStaysRunning(t *testing.T) {
	db := testdb.StartPostgres(t)

	tenantID := "tenant-platform"
	agentID := "agent-platform"
	ownerID := "owner"
	versionID := randomID()
	insertAgent(t, db, tenantID, agentID, ownerID)
	insertAgentVersion(t, db, tenantID, agentID, versionID, 1, "draft")

	versions := newFakeVersionLifecycle()
	versions.AddVersion(&application.VersionInfo{ID: versionID, TenantID: tenantID, AgentID: agentID, Status: "draft"})

	publisher := &fakePublisher{}
	svc := newPlatformService(t, db, versions, publisher)

	bsResp, err := svc.CreateBenchmarkSet(context.Background(), application.CreateBenchmarkSet{
		TenantID: tenantID,
		Name:     "Set",
		Tasks: []domain.BenchmarkTask{
			{
				TaskRef:      "task-1",
				Ordering:     0,
				Title:        "Fix the bug",
				Problem:      "The thing is broken",
				Constraints:  []string{"repo:org/repo", "base_commit:abc123"},
				Requirements: []string{"all tests pass"},
				IsSecurity:   true,
			},
			{TaskRef: "task-2", Ordering: 1, Title: "Second"},
		},
		CreatedBy: ownerID,
		IsAdmin:   true,
	})
	require.NoError(t, err)

	before := time.Now()
	runResp, err := svc.StartEvaluationRun(context.Background(), application.StartEvaluationRun{
		TenantID:           tenantID,
		AgentID:            agentID,
		VersionID:          versionID,
		BenchmarkSetID:     bsResp.BenchmarkSet.ID(),
		EnvironmentDigest:  "env",
		ScoringRuleVersion: domain.ScoringRuleVersionV1,
		ActorID:            ownerID,
		IsAdmin:            false,
	})
	require.NoError(t, err)

	// The run stays running; the version transitions to evaluating only.
	run := runResp.EvaluationRun
	require.Equal(t, domain.StatusRunning, run.Status())
	require.False(t, run.IsPassed())
	require.Equal(t, "evaluating", versions.versions[versionID].Status)

	// The publisher received one command per benchmark task with the stable
	// idempotency key, the evaluation type, the definition content, the
	// eval_run constraint and the configured deadline.
	require.Len(t, publisher.cmds, 2)
	first := publisher.cmds[0]
	require.Equal(t, tenantID, first.TenantID)
	require.Equal(t, "eval:"+run.ID()+":task-1", first.RequestID)
	require.Equal(t, "evaluation", first.Type)
	require.Equal(t, "Fix the bug", first.Title)
	require.Equal(t, "The thing is broken", first.Problem)
	require.Equal(t, []string{"all tests pass"}, first.Requirements)
	require.Contains(t, first.Constraints, "repo:org/repo")
	require.Contains(t, first.Constraints, "base_commit:abc123")
	require.Contains(t, first.Constraints, "eval_run:"+run.ID())
	require.Equal(t, "eval:"+run.ID()+":task-2", publisher.cmds[1].RequestID)
	for _, cmd := range publisher.cmds {
		require.False(t, cmd.Deadline.Before(before.Add(time.Hour)))
		require.True(t, cmd.Deadline.Before(time.Now().Add(time.Hour+time.Minute)))
	}

	// The run-task mappings are persisted with the published task identifiers
	// backfilled, unresolved, in benchmark order.
	runTasks, err := evpostgres.NewEvaluationRunTaskRepository(db).ListByRun(context.Background(), tenantID, run.ID())
	require.NoError(t, err)
	require.Len(t, runTasks, 2)
	require.Equal(t, "task-1", runTasks[0].TaskRef)
	require.Equal(t, 0, runTasks[0].Ordering)
	require.Equal(t, "task-id-eval:"+run.ID()+":task-1", runTasks[0].TaskID)
	require.False(t, runTasks[0].Resolved)
	require.Nil(t, runTasks[0].Passed)
	require.Equal(t, "task-2", runTasks[1].TaskRef)
	require.Equal(t, 1, runTasks[1].Ordering)

	// The platform executor identity is visible and the run remains listed as
	// running.
	require.Equal(t, application.PlatformBenchmarkExecutorID, application.NewPlatformBenchmarkExecutor().ExecutorID())
	runs, err := svc.ListEvaluationRuns(context.Background(), identityapp.Principal{TenantID: tenantID, OwnerID: ownerID}, tenantID, versionID)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, domain.StatusRunning, runs[0].Status())
}

func TestStartEvaluationRunPlatformRecordsPublishFailureAndContinues(t *testing.T) {
	db := testdb.StartPostgres(t)

	tenantID := "tenant-platform-fail"
	agentID := "agent-platform-fail"
	ownerID := "owner"
	versionID := randomID()
	insertAgent(t, db, tenantID, agentID, ownerID)
	insertAgentVersion(t, db, tenantID, agentID, versionID, 1, "draft")

	versions := newFakeVersionLifecycle()
	versions.AddVersion(&application.VersionInfo{ID: versionID, TenantID: tenantID, AgentID: agentID, Status: "draft"})

	publisher := &fakePublisher{failRef: "task-1", failErr: errors.New("publish boom")}
	svc := newPlatformService(t, db, versions, publisher)

	bsResp, err := svc.CreateBenchmarkSet(context.Background(), application.CreateBenchmarkSet{
		TenantID: tenantID,
		Name:     "Set",
		Tasks: []domain.BenchmarkTask{
			{TaskRef: "task-1", Ordering: 0},
			{TaskRef: "task-2", Ordering: 1},
		},
		CreatedBy: ownerID,
		IsAdmin:   true,
	})
	require.NoError(t, err)

	runResp, err := svc.StartEvaluationRun(context.Background(), application.StartEvaluationRun{
		TenantID:          tenantID,
		AgentID:           agentID,
		VersionID:         versionID,
		BenchmarkSetID:    bsResp.BenchmarkSet.ID(),
		EnvironmentDigest: "env",
		ActorID:           ownerID,
		IsAdmin:           false,
	})
	require.NoError(t, err)
	require.Equal(t, domain.StatusRunning, runResp.EvaluationRun.Status())
	require.Equal(t, "evaluating", versions.versions[versionID].Status)

	// The failed row keeps an empty task id and records the publish error; the
	// other task is still published and backfilled.
	runTasks, err := evpostgres.NewEvaluationRunTaskRepository(db).ListByRun(context.Background(), tenantID, runResp.EvaluationRun.ID())
	require.NoError(t, err)
	require.Len(t, runTasks, 2)
	require.Equal(t, "task-1", runTasks[0].TaskRef)
	require.Equal(t, "", runTasks[0].TaskID)
	require.Contains(t, runTasks[0].Details["publish_error"], "publish boom")
	require.Equal(t, "task-2", runTasks[1].TaskRef)
	require.NotEqual(t, "", runTasks[1].TaskID)
	require.Len(t, publisher.cmds, 2)
}
