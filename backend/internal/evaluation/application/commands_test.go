package application_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/evaluation/application"
	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	evpostgres "agentguild.dev/agentguild/backend/internal/evaluation/postgres"
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

type fakeVersionLifecycle struct {
	versions map[string]*application.VersionInfo
	calls    []string
}

func newFakeVersionLifecycle() *fakeVersionLifecycle {
	return &fakeVersionLifecycle{versions: make(map[string]*application.VersionInfo)}
}

func (f *fakeVersionLifecycle) AddVersion(info *application.VersionInfo) {
	f.versions[info.ID] = info
}

func (f *fakeVersionLifecycle) GetByID(ctx context.Context, tenantID, versionID string) (*application.VersionInfo, error) {
	v, ok := f.versions[versionID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return v, nil
}

func (f *fakeVersionLifecycle) MarkEvaluating(ctx context.Context, tx application.Tx, tenantID, agentID, versionID string) error {
	f.calls = append(f.calls, "evaluating:"+versionID)
	if v, ok := f.versions[versionID]; ok {
		v.Status = "evaluating"
	}
	return nil
}

func (f *fakeVersionLifecycle) MarkEligible(ctx context.Context, tx application.Tx, tenantID, agentID, versionID string) error {
	f.calls = append(f.calls, "eligible:"+versionID)
	if v, ok := f.versions[versionID]; ok {
		v.Status = "eligible"
	}
	return nil
}

func (f *fakeVersionLifecycle) MarkRejected(ctx context.Context, tx application.Tx, tenantID, agentID, versionID, reason string) error {
	f.calls = append(f.calls, "rejected:"+versionID)
	if v, ok := f.versions[versionID]; ok {
		v.Status = "rejected"
	}
	return nil
}

type fakeExecutor struct {
	results []domain.TaskResult
	err     error
}

func (f *fakeExecutor) Execute(ctx context.Context, benchmarkSet *domain.BenchmarkSet, environmentDigest string) ([]domain.TaskResult, error) {
	return f.results, f.err
}

type fakeOwnerProvider struct{}

func (fakeOwnerProvider) GetAgentOwner(ctx context.Context, tenantID, agentID string) (string, error) {
	return "owner", nil
}

func newService(t *testing.T, db *pgxpool.Pool, executor application.BenchmarkExecutor, versions application.VersionLifecyclePort) *application.EvaluationService {
	t.Helper()
	store := evpostgres.NewStore(db)
	bsRepo := evpostgres.NewBenchmarkSetRepository(db)
	runRepo := evpostgres.NewEvaluationRunRepository(db)
	policy := application.NewPolicy(fakeOwnerProvider{})
	svc, err := application.NewEvaluationService(store, bsRepo, runRepo, versions, executor, policy, application.EvaluationOptions{})
	require.NoError(t, err)
	return svc
}

func TestCreateBenchmarkSetAndActivate(t *testing.T) {
	db := testdb.StartPostgres(t)
	svc := newService(t, db, &fakeExecutor{}, newFakeVersionLifecycle())

	tenantID := "tenant-app-bs"
	resp, err := svc.CreateBenchmarkSet(context.Background(), application.CreateBenchmarkSet{
		TenantID:    tenantID,
		Name:        "Benchmark Set 1",
		Description: "desc",
		Tasks: []domain.BenchmarkTask{
			{TaskRef: "task-1", Ordering: 0},
		},
		IsActive:  true,
		CreatedBy: "owner",
		IsAdmin:   true,
	})
	require.NoError(t, err)
	require.Equal(t, 1, resp.BenchmarkSet.VersionNumber())
	require.True(t, resp.BenchmarkSet.IsActive())

	resp2, err := svc.CreateBenchmarkSet(context.Background(), application.CreateBenchmarkSet{
		TenantID:  tenantID,
		Name:      "Benchmark Set 2",
		IsActive:  false,
		CreatedBy: "owner",
		IsAdmin:   true,
	})
	require.NoError(t, err)
	require.Equal(t, 2, resp2.BenchmarkSet.VersionNumber())

	active, err := svc.GetActiveBenchmarkSet(context.Background(), tenantID)
	require.NoError(t, err)
	require.Equal(t, resp.BenchmarkSet.ID(), active.ID())
}

func TestStartEvaluationRunPassedMarksEligible(t *testing.T) {
	db := testdb.StartPostgres(t)

	tenantID := "tenant-pass"
	agentID := "agent-pass"
	ownerID := "owner"
	versionID := randomID()
	insertAgent(t, db, tenantID, agentID, ownerID)
	insertAgentVersion(t, db, tenantID, agentID, versionID, 1, "draft")

	versions := newFakeVersionLifecycle()
	versions.AddVersion(&application.VersionInfo{ID: versionID, TenantID: tenantID, AgentID: agentID, Status: "draft"})

	executor := &fakeExecutor{results: []domain.TaskResult{
		{TaskRef: "task-1", Passed: true, LatencyMs: 100, Score: 1.0, IsSecurity: true},
		{TaskRef: "task-2", Passed: true, LatencyMs: 200, Score: 1.0},
		{TaskRef: "task-3", Passed: true, LatencyMs: 300, Score: 1.0},
		{TaskRef: "task-4", Passed: true, LatencyMs: 400, Score: 1.0},
		{TaskRef: "task-5", Passed: true, LatencyMs: 500, Score: 1.0},
	}}

	svc := newService(t, db, executor, versions)

	bsResp, err := svc.CreateBenchmarkSet(context.Background(), application.CreateBenchmarkSet{
		TenantID:  tenantID,
		Name:      "Set",
		Tasks:     []domain.BenchmarkTask{{TaskRef: "task-1"}, {TaskRef: "task-2"}, {TaskRef: "task-3"}, {TaskRef: "task-4"}, {TaskRef: "task-5"}},
		CreatedBy: ownerID,
		IsAdmin:   true,
	})
	require.NoError(t, err)

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
	require.True(t, runResp.EvaluationRun.IsPassed())
	require.Equal(t, "eligible", versions.versions[versionID].Status)

	runs, err := svc.ListEvaluationRuns(context.Background(), tenantID, versionID)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, domain.StatusPassed, runs[0].Status())
}

func TestStartEvaluationRunFailedMarksRejected(t *testing.T) {
	db := testdb.StartPostgres(t)

	tenantID := "tenant-fail"
	agentID := "agent-fail"
	ownerID := "owner"
	versionID := randomID()
	insertAgent(t, db, tenantID, agentID, ownerID)
	insertAgentVersion(t, db, tenantID, agentID, versionID, 1, "draft")

	versions := newFakeVersionLifecycle()
	versions.AddVersion(&application.VersionInfo{ID: versionID, TenantID: tenantID, AgentID: agentID, Status: "draft"})

	executor := &fakeExecutor{results: []domain.TaskResult{
		{TaskRef: "task-1", Passed: false, LatencyMs: 100, Score: 0.0, IsSecurity: true},
	}}

	svc := newService(t, db, executor, versions)

	bsResp, err := svc.CreateBenchmarkSet(context.Background(), application.CreateBenchmarkSet{
		TenantID:  tenantID,
		Name:      "Set",
		Tasks:     []domain.BenchmarkTask{{TaskRef: "task-1"}},
		CreatedBy: ownerID,
		IsAdmin:   true,
	})
	require.NoError(t, err)

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
	require.False(t, runResp.EvaluationRun.IsPassed())
	require.Equal(t, "rejected", versions.versions[versionID].Status)
}

func TestCompleteEvaluationRunDirectly(t *testing.T) {
	db := testdb.StartPostgres(t)

	tenantID := "tenant-complete"
	agentID := "agent-complete"
	ownerID := "owner"
	versionID := randomID()
	insertAgent(t, db, tenantID, agentID, ownerID)
	insertAgentVersion(t, db, tenantID, agentID, versionID, 1, "draft")

	versions := newFakeVersionLifecycle()
	versions.AddVersion(&application.VersionInfo{ID: versionID, TenantID: tenantID, AgentID: agentID, Status: "evaluating"})

	svc := newService(t, db, &fakeExecutor{}, versions)

	bsResp, err := svc.CreateBenchmarkSet(context.Background(), application.CreateBenchmarkSet{
		TenantID:  tenantID,
		Name:      "Set",
		CreatedBy: ownerID,
		IsAdmin:   true,
	})
	require.NoError(t, err)

	run, err := domain.NewEvaluationRun(randomID(), tenantID, versionID, bsResp.BenchmarkSet.ID(), "env", domain.ScoringRuleVersionV1, time.Now())
	require.NoError(t, err)
	require.NoError(t, evpostgres.NewStore(db).WithTx(context.Background(), func(tx application.Tx) error {
		return evpostgres.NewEvaluationRunRepository(db).Create(context.Background(), tx, run)
	}))

	err = svc.CompleteEvaluationRun(context.Background(), application.CompleteEvaluationRun{
		TenantID:        tenantID,
		AgentID:         agentID,
		VersionID:       versionID,
		EvaluationRunID: run.ID(),
		ThresholdResults: []domain.ThresholdResult{
			{Name: "pass_rate", Passed: true},
		},
		Summary: domain.EvaluationSummary{PassRate: 1.0},
		ActorID: ownerID,
		IsAdmin: false,
	})
	require.NoError(t, err)
	require.Equal(t, "eligible", versions.versions[versionID].Status)
}

func TestRunningEvaluationRunBlocksVersionContentMutation(t *testing.T) {
	db := testdb.StartPostgres(t)

	tenantID := "tenant-block"
	agentID := "agent-block"
	ownerID := "owner"
	versionID := randomID()
	insertAgent(t, db, tenantID, agentID, ownerID)
	insertAgentVersion(t, db, tenantID, agentID, versionID, 1, "draft")

	versions := newFakeVersionLifecycle()
	versions.AddVersion(&application.VersionInfo{ID: versionID, TenantID: tenantID, AgentID: agentID, Status: "draft"})

	// Start a run but never complete it in this test.
	executor := &fakeExecutor{results: []domain.TaskResult{
		{TaskRef: "task-1", Passed: true, LatencyMs: 100},
	}}
	svc := newService(t, db, executor, versions)

	bsResp, err := svc.CreateBenchmarkSet(context.Background(), application.CreateBenchmarkSet{
		TenantID:  tenantID,
		Name:      "Set",
		Tasks:     []domain.BenchmarkTask{{TaskRef: "task-1"}},
		CreatedBy: ownerID,
		IsAdmin:   true,
	})
	require.NoError(t, err)

	_, err = svc.StartEvaluationRun(context.Background(), application.StartEvaluationRun{
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

	// Attempting to start another run on the same version should fail because the
	// version is no longer draft.
	_, err = svc.StartEvaluationRun(context.Background(), application.StartEvaluationRun{
		TenantID:           tenantID,
		AgentID:            agentID,
		VersionID:          versionID,
		BenchmarkSetID:     bsResp.BenchmarkSet.ID(),
		EnvironmentDigest:  "env",
		ScoringRuleVersion: domain.ScoringRuleVersionV1,
		ActorID:            ownerID,
		IsAdmin:            false,
	})
	require.ErrorIs(t, err, domain.ErrStateConflict)
}
