package application_test

import (
	"context"
	"testing"

	avapplication "agentguild.dev/agentguild/backend/internal/agentversion/application"
	avdomain "agentguild.dev/agentguild/backend/internal/agentversion/domain"
	avpostgres "agentguild.dev/agentguild/backend/internal/agentversion/postgres"
	"agentguild.dev/agentguild/backend/internal/evaluation/application"
	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	evpostgres "agentguild.dev/agentguild/backend/internal/evaluation/postgres"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// fakeAgentVersionEvalProvider satisfies the agentversion module's
// EvaluationRunProvider port; promotion is not exercised in these tests.
type fakeAgentVersionEvalProvider struct{}

func (fakeAgentVersionEvalProvider) GetLatestPassed(context.Context, avapplication.Tx, string, string) (*avapplication.EvaluationRunInfo, error) {
	return nil, nil
}

// newVersionServiceWithHook builds a real agentversion service (real postgres
// repositories) whose only stub is the draft-created hook under test.
func newVersionServiceWithHook(t *testing.T, db *pgxpool.Pool, hook avapplication.DraftCreatedHook) *avapplication.VersionService {
	t.Helper()
	repo := avpostgres.NewVersionRepository(db)
	svc, err := avapplication.NewVersionService(
		avpostgres.NewStore(db), repo, fakeAgentVersionEvalProvider{}, nil,
		avapplication.NewPolicy(repo), avapplication.VersionOptions{DraftCreatedHook: hook},
	)
	require.NoError(t, err)
	return svc
}

func autoDraftCommand(tenantID, agentID, ownerID string) avapplication.CreateDraft {
	return avapplication.CreateDraft{
		TenantID:          tenantID,
		AgentID:           agentID,
		CreatedBy:         ownerID,
		Runtime:           "python",
		Model:             "gpt-4",
		Capabilities:      []string{"code"},
		PromptRef:         "sha256:prompt",
		SkillRefs:         []string{"sha256:skill"},
		MemoryRef:         "sha256:memory",
		ToolRefs:          []string{"sha256:tool"},
		EnvironmentDigest: "env-auto",
	}
}

// newAutoEvaluator wires the hook exactly like main.go does for
// EVALUATION_EXECUTOR=platform: a real evaluation service publishing through a
// fake task publisher, real benchmark set and environment repositories.
func newAutoEvaluator(t *testing.T, db *pgxpool.Pool, enabled bool) (*application.AutoEvaluator, *application.EvaluationService) {
	t.Helper()
	evalSvc := newPlatformService(t, db, avpostgres.NewVersionLifecycleAdapter(db), &fakePublisher{})
	hook := application.NewAutoEvaluator(
		enabled, evalSvc,
		evpostgres.NewBenchmarkSetRepository(db),
		evpostgres.NewVersionEnvironmentProvider(db),
		nil,
	)
	return hook, evalSvc
}

func createActiveBenchmarkSet(t *testing.T, svc *application.EvaluationService, tenantID, ownerID string, isActive bool) {
	t.Helper()
	_, err := svc.CreateBenchmarkSet(context.Background(), application.CreateBenchmarkSet{
		TenantID: tenantID,
		Name:     "Set",
		Tasks: []domain.BenchmarkTask{
			{TaskRef: "task-1", Ordering: 0, Title: "Fix the bug", Problem: "broken"},
		},
		IsActive:  isActive,
		CreatedBy: ownerID,
		IsAdmin:   true,
	})
	require.NoError(t, err)
}

func listRuns(t *testing.T, svc *application.EvaluationService, tenantID, ownerID, versionID string) []domain.EvaluationRun {
	t.Helper()
	runs, err := svc.ListEvaluationRuns(context.Background(), identityapp.Principal{TenantID: tenantID, OwnerID: ownerID}, tenantID, versionID)
	require.NoError(t, err)
	return runs
}

func TestAutoEvaluatorStartsRunWhenDraftCreated(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	tenantID := "tenant-auto"
	agentID := "agent-auto"
	ownerID := "owner"
	insertAgent(t, db, tenantID, agentID, ownerID)

	hook, evalSvc := newAutoEvaluator(t, db, true)
	createActiveBenchmarkSet(t, evalSvc, tenantID, ownerID, true)
	versionSvc := newVersionServiceWithHook(t, db, hook)

	draft, err := versionSvc.CreateDraft(ctx, autoDraftCommand(tenantID, agentID, ownerID))
	require.NoError(t, err)

	// The draft creation produced one running evaluation run bound to the
	// active benchmark set and inheriting the version's environment digest.
	runs := listRuns(t, evalSvc, tenantID, ownerID, draft.Version.ID)
	require.Len(t, runs, 1)
	require.Equal(t, domain.StatusRunning, runs[0].Status())
	require.Equal(t, "env-auto", runs[0].EnvironmentDigest())

	version, err := avpostgres.NewVersionRepository(db).GetByID(ctx, tenantID, agentID, draft.Version.ID)
	require.NoError(t, err)
	require.Equal(t, avdomain.StatusEvaluating, version.Status)

	// Duplicate hook delivery is idempotent: the draft->evaluating state
	// machine rejects the second start and the hook skips it quietly.
	require.NoError(t, hook.OnDraftCreated(ctx, tenantID, agentID, draft.Version.ID, ownerID))
	require.Len(t, listRuns(t, evalSvc, tenantID, ownerID, draft.Version.ID), 1)
}

func TestAutoEvaluatorSkipsWithoutActiveBenchmarkSet(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	tenantID := "tenant-auto-inactive"
	agentID := "agent-auto-inactive"
	ownerID := "owner"
	insertAgent(t, db, tenantID, agentID, ownerID)

	hook, evalSvc := newAutoEvaluator(t, db, true)
	// Only an inactive benchmark set exists: nothing to evaluate against.
	createActiveBenchmarkSet(t, evalSvc, tenantID, ownerID, false)
	versionSvc := newVersionServiceWithHook(t, db, hook)

	draft, err := versionSvc.CreateDraft(ctx, autoDraftCommand(tenantID, agentID, ownerID))
	require.NoError(t, err)
	require.Empty(t, listRuns(t, evalSvc, tenantID, ownerID, draft.Version.ID))

	version, err := avpostgres.NewVersionRepository(db).GetByID(ctx, tenantID, agentID, draft.Version.ID)
	require.NoError(t, err)
	require.Equal(t, avdomain.StatusDraft, version.Status)
}

func TestAutoEvaluatorDisabledStartsNothing(t *testing.T) {
	db := testdb.StartPostgres(t)

	tenantID := "tenant-auto-off"
	agentID := "agent-auto-off"
	ownerID := "owner"
	insertAgent(t, db, tenantID, agentID, ownerID)

	hook, evalSvc := newAutoEvaluator(t, db, false)
	createActiveBenchmarkSet(t, evalSvc, tenantID, ownerID, true)
	versionSvc := newVersionServiceWithHook(t, db, hook)

	draft, err := versionSvc.CreateDraft(context.Background(), autoDraftCommand(tenantID, agentID, ownerID))
	require.NoError(t, err)
	require.Empty(t, listRuns(t, evalSvc, tenantID, ownerID, draft.Version.ID))
}
