package application_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/agentversion/application"
	"agentguild.dev/agentguild/backend/internal/agentversion/domain"
	avpostgres "agentguild.dev/agentguild/backend/internal/agentversion/postgres"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
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

func newVersion(t *testing.T, tenantID, agentID string, number int, parentID string, status domain.VersionStatus) *domain.AgentVersion {
	t.Helper()
	v, err := domain.NewAgentVersion(
		randomID(), tenantID, agentID, number, parentID,
		"python", "gpt-4", []string{"code"},
		"sha256:prompt", []string{"sha256:skill"},
		"sha256:memory", []string{"sha256:tool"},
		"env-digest", "owner", time.Now(),
	)
	require.NoError(t, err)
	v.Status = status
	if status == domain.StatusActive {
		now := time.Now()
		v.PromotedAt = &now
	}
	if status == domain.StatusRetired {
		now := time.Now()
		v.PromotedAt = &now
		retired := now.Add(time.Minute)
		v.RetiredAt = &retired
	}
	return v
}

func createVersion(t *testing.T, store application.Store, repo application.VersionRepository, v *domain.AgentVersion) {
	t.Helper()
	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		return repo.Create(context.Background(), tx, v)
	})
	require.NoError(t, err)
}

func updateCurrentVersion(t *testing.T, store application.Store, repo application.VersionRepository, tenantID, agentID, versionID string) {
	t.Helper()
	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		return repo.UpdateAgentCurrentVersion(context.Background(), tx, tenantID, agentID, versionID)
	})
	require.NoError(t, err)
}

func ownerPrincipal(tenantID, ownerID string) identityapp.Principal {
	return identityapp.Principal{
		TenantID: tenantID,
		OwnerID:  ownerID,
	}
}

type fakeEvalProvider struct{}

func (fakeEvalProvider) GetLatestPassed(context.Context, application.Tx, string, string) (*application.EvaluationRunInfo, error) {
	return &application.EvaluationRunInfo{ID: randomID(), Status: "passed"}, nil
}

func newService(t *testing.T, db *pgxpool.Pool) (*application.VersionService, application.VersionRepository, application.Store) {
	t.Helper()
	repo := avpostgres.NewVersionRepository(db)
	store := avpostgres.NewStore(db)
	policy := application.NewPolicy(repo)
	svc, err := application.NewVersionService(store, repo, fakeEvalProvider{}, nil, policy, application.VersionOptions{})
	require.NoError(t, err)
	return svc, repo, store
}

func TestCreateDraftUsesLatestVersionNumber(t *testing.T) {
	db := testdb.StartPostgres(t)
	svc, repo, store := newService(t, db)

	tenantID := "tenant-latest"
	agentID := "agent-latest"
	ownerID := "owner"
	insertAgent(t, db, tenantID, agentID, ownerID)

	initial := newVersion(t, tenantID, agentID, 1, "", domain.StatusActive)
	createVersion(t, store, repo, initial)
	updateCurrentVersion(t, store, repo, tenantID, agentID, initial.ID)

	first, err := svc.CreateDraft(context.Background(), application.CreateDraft{
		TenantID:  tenantID,
		AgentID:   agentID,
		CreatedBy: ownerID,
		IsAdmin:   false,
		Runtime:   "python",
		Model:     "gpt-4",
		Capabilities: []string{"code"},
		PromptRef: "sha256:prompt-v2",
		SkillRefs: []string{"sha256:skill"},
		MemoryRef: "sha256:memory",
		ToolRefs:  []string{"sha256:tool"},
	})
	require.NoError(t, err)
	require.Equal(t, 2, first.Version.VersionNumber)

	second, err := svc.CreateDraft(context.Background(), application.CreateDraft{
		TenantID:  tenantID,
		AgentID:   agentID,
		CreatedBy: ownerID,
		IsAdmin:   false,
		Runtime:   "python",
		Model:     "gpt-4",
		Capabilities: []string{"code"},
		PromptRef: "sha256:prompt-v3",
		SkillRefs: []string{"sha256:skill"},
		MemoryRef: "sha256:memory",
		ToolRefs:  []string{"sha256:tool"},
	})
	require.NoError(t, err)
	require.Equal(t, 3, second.Version.VersionNumber)
	require.Equal(t, first.Version.ID, second.Version.ParentVersionID)
}

func TestCreateDraftAndFullLifecycle(t *testing.T) {
	db := testdb.StartPostgres(t)
	svc, repo, store := newService(t, db)

	tenantID := "tenant-lifecycle"
	agentID := "agent-lifecycle"
	ownerID := "owner"
	insertAgent(t, db, tenantID, agentID, ownerID)

	// Seed an active initial version.
	initial := newVersion(t, tenantID, agentID, 1, "", domain.StatusActive)
	createVersion(t, store, repo, initial)
	updateCurrentVersion(t, store, repo, tenantID, agentID, initial.ID)

	// Create draft with changed prompt.
	resp, err := svc.CreateDraft(context.Background(), application.CreateDraft{
		TenantID:  tenantID,
		AgentID:   agentID,
		CreatedBy: ownerID,
		IsAdmin:   false,
		Runtime:   "python",
		Model:     "gpt-4",
		Capabilities: []string{"code"},
		PromptRef: "sha256:new-prompt",
		SkillRefs: []string{"sha256:skill"},
		MemoryRef: "sha256:memory",
		ToolRefs:  []string{"sha256:tool"},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Version)
	require.Equal(t, 2, resp.Version.VersionNumber)
	require.Equal(t, initial.ID, resp.Version.ParentVersionID)
	require.Equal(t, domain.StatusDraft, resp.Version.Status)

	// Start evaluation.
	err = svc.StartEvaluation(context.Background(), ownerPrincipal(tenantID, ownerID), application.StartEvaluation{
		TenantID:  tenantID,
		AgentID:   agentID,
		VersionID: resp.Version.ID,
	})
	require.NoError(t, err)

	// Mark eligible directly via repository (evaluation module would do this).
	eligible, err := repo.GetByID(context.Background(), tenantID, agentID, resp.Version.ID)
	require.NoError(t, err)
	require.NoError(t, eligible.MarkEligible())
	err = store.WithTx(context.Background(), func(tx application.Tx) error {
		return repo.UpdateStatus(context.Background(), tx, eligible)
	})
	require.NoError(t, err)

	// Promote.
	err = svc.Promote(context.Background(), application.Promote{
		TenantID:  tenantID,
		AgentID:   agentID,
		VersionID: resp.Version.ID,
		ActorID:   ownerID,
		IsAdmin:   false,
	})
	require.NoError(t, err)

	active, err := repo.GetActiveByAgent(context.Background(), tenantID, agentID)
	require.NoError(t, err)
	require.Equal(t, resp.Version.ID, active.ID)
	require.Equal(t, domain.StatusActive, active.Status)

	initialReload, err := repo.GetByID(context.Background(), tenantID, agentID, initial.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusRetired, initialReload.Status)
}

// promoteRaceRepo deterministically interleaves two concurrent Promote calls:
// the transaction that reaches the final agents update waits until both callers
// have read the expected current_version_id, so the loser always observes a
// stale expectation regardless of goroutine scheduling.
type promoteRaceRepo struct {
	application.VersionRepository
	reads    chan struct{}
	waitOnce sync.Once
}

func (r *promoteRaceRepo) GetAgentCurrentVersionID(ctx context.Context, tenantID, agentID string) (string, error) {
	id, err := r.VersionRepository.GetAgentCurrentVersionID(ctx, tenantID, agentID)
	if err == nil {
		r.reads <- struct{}{}
	}
	return id, err
}

func (r *promoteRaceRepo) PromoteAgentCurrentVersion(ctx context.Context, tx application.Tx, tenantID, agentID, versionID, expectedVersionID string) error {
	r.waitOnce.Do(func() {
		<-r.reads
		<-r.reads
	})
	return r.VersionRepository.PromoteAgentCurrentVersion(ctx, tx, tenantID, agentID, versionID, expectedVersionID)
}

// TestConcurrentPromoteDifferentVersionsOnlyOneSucceeds pins the spec scenario
// "并发晋级被阻止": two concurrent promotes of different eligible versions for
// the same agent must not both succeed; the late one gets a state conflict
// instead of retiring the version the winner just activated. Sequential
// promotion (a later Promote call started after the previous one committed)
// remains the normal lifecycle and is covered by TestCreateDraftAndFullLifecycle.
func TestConcurrentPromoteDifferentVersionsOnlyOneSucceeds(t *testing.T) {
	db := testdb.StartPostgres(t)
	baseRepo := avpostgres.NewVersionRepository(db)
	store := avpostgres.NewStore(db)

	tenantID := "tenant-concurrent-promote-diff"
	agentID := "agent-concurrent-promote-diff"
	ownerID := "owner"
	insertAgent(t, db, tenantID, agentID, ownerID)

	initial := newVersion(t, tenantID, agentID, 1, "", domain.StatusActive)
	createVersion(t, store, baseRepo, initial)
	updateCurrentVersion(t, store, baseRepo, tenantID, agentID, initial.ID)

	versionA := newVersion(t, tenantID, agentID, 2, initial.ID, domain.StatusEligible)
	createVersion(t, store, baseRepo, versionA)
	versionB := newVersion(t, tenantID, agentID, 3, initial.ID, domain.StatusEligible)
	createVersion(t, store, baseRepo, versionB)

	race := &promoteRaceRepo{VersionRepository: baseRepo, reads: make(chan struct{}, 2)}
	svc, err := application.NewVersionService(store, race, fakeEvalProvider{}, nil, application.NewPolicy(race), application.VersionOptions{})
	require.NoError(t, err)

	results := make(map[string]error, 2)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, target := range []*domain.AgentVersion{versionA, versionB} {
		wg.Add(1)
		go func(versionID string) {
			defer wg.Done()
			err := svc.Promote(context.Background(), application.Promote{
				TenantID:  tenantID,
				AgentID:   agentID,
				VersionID: versionID,
				ActorID:   ownerID,
				IsAdmin:   false,
			})
			mu.Lock()
			results[versionID] = err
			mu.Unlock()
		}(target.ID)
	}
	wg.Wait()

	winner := ""
	for versionID, err := range results {
		if err == nil {
			require.Empty(t, winner, "only one concurrent promote may succeed")
			winner = versionID
		} else {
			require.ErrorIs(t, err, domain.ErrStateConflict)
		}
	}
	require.NotEmpty(t, winner, "exactly one concurrent promote must succeed")

	active, err := baseRepo.GetActiveByAgent(context.Background(), tenantID, agentID)
	require.NoError(t, err)
	require.Equal(t, winner, active.ID)

	loserID := versionA.ID
	if winner == versionA.ID {
		loserID = versionB.ID
	}
	loser, err := baseRepo.GetByID(context.Background(), tenantID, agentID, loserID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusEligible, loser.Status, "losing promote must roll back and leave the version eligible")

	var currentVersionID string
	require.NoError(t, db.QueryRow(context.Background(),
		`SELECT current_version_id FROM agents WHERE tenant_id=$1 AND id=$2`,
		tenantID, agentID,
	).Scan(&currentVersionID))
	require.Equal(t, winner, currentVersionID)
}

func TestPromoteSameVersionTwiceReturnsStateConflict(t *testing.T) {
	db := testdb.StartPostgres(t)
	svc, repo, store := newService(t, db)

	tenantID := "tenant-double-promote"
	agentID := "agent-double-promote"
	ownerID := "owner"
	insertAgent(t, db, tenantID, agentID, ownerID)

	initial := newVersion(t, tenantID, agentID, 1, "", domain.StatusActive)
	createVersion(t, store, repo, initial)
	updateCurrentVersion(t, store, repo, tenantID, agentID, initial.ID)

	target := newVersion(t, tenantID, agentID, 2, initial.ID, domain.StatusEligible)
	createVersion(t, store, repo, target)

	cmd := application.Promote{
		TenantID:  tenantID,
		AgentID:   agentID,
		VersionID: target.ID,
		ActorID:   ownerID,
		IsAdmin:   false,
	}
	require.NoError(t, svc.Promote(context.Background(), cmd))
	require.ErrorIs(t, svc.Promote(context.Background(), cmd), domain.ErrStateConflict)
}

func TestPromoteRecordsApprover(t *testing.T) {
	db := testdb.StartPostgres(t)
	svc, repo, store := newService(t, db)

	tenantID := "tenant-promote-audit"
	agentID := "agent-promote-audit"
	ownerID := "owner"
	insertAgent(t, db, tenantID, agentID, ownerID)

	initial := newVersion(t, tenantID, agentID, 1, "", domain.StatusActive)
	createVersion(t, store, repo, initial)
	updateCurrentVersion(t, store, repo, tenantID, agentID, initial.ID)

	target := newVersion(t, tenantID, agentID, 2, initial.ID, domain.StatusEligible)
	createVersion(t, store, repo, target)

	require.NoError(t, svc.Promote(context.Background(), application.Promote{
		TenantID:  tenantID,
		AgentID:   agentID,
		VersionID: target.ID,
		ActorID:   ownerID,
		IsAdmin:   false,
	}))

	detail, err := svc.GetVersion(context.Background(), tenantID, agentID, target.ID)
	require.NoError(t, err)
	require.Equal(t, ownerID, detail.PromotedBy, "promote must persist the approver")
	require.NotNil(t, detail.PromotedAt, "promote must persist the promotion time")

	// Versions seeded without a promote transition expose no approver.
	initialDetail, err := svc.GetVersion(context.Background(), tenantID, agentID, initial.ID)
	require.NoError(t, err)
	require.Empty(t, initialDetail.PromotedBy)
}

func TestCreateDraftSameFingerprintReturnsNoChange(t *testing.T) {
	db := testdb.StartPostgres(t)
	svc, repo, store := newService(t, db)

	tenantID := "tenant-nochange"
	agentID := "agent-nochange"
	ownerID := "owner"
	insertAgent(t, db, tenantID, agentID, ownerID)

	cfg := application.CreateDraft{
		TenantID:  tenantID,
		AgentID:   agentID,
		CreatedBy: ownerID,
		IsAdmin:   false,
		Runtime:   "python",
		Model:     "gpt-4",
		Capabilities: []string{"code"},
		PromptRef: "sha256:prompt",
		SkillRefs: []string{"sha256:skill"},
		MemoryRef: "sha256:memory",
		ToolRefs:  []string{"sha256:tool"},
	}

	initial := newVersion(t, tenantID, agentID, 1, "", domain.StatusActive)
	createVersion(t, store, repo, initial)
	updateCurrentVersion(t, store, repo, tenantID, agentID, initial.ID)

	_, err := svc.CreateDraft(context.Background(), cfg)
	require.ErrorIs(t, err, domain.ErrNoChange)
}

func TestConcurrentPromoteOnlyOneSucceeds(t *testing.T) {
	db := testdb.StartPostgres(t)
	svc, repo, store := newService(t, db)

	tenantID := "tenant-concurrent-promote"
	agentID := "agent-concurrent-promote"
	ownerID := "owner"
	insertAgent(t, db, tenantID, agentID, ownerID)

	initial := newVersion(t, tenantID, agentID, 1, "", domain.StatusActive)
	createVersion(t, store, repo, initial)
	updateCurrentVersion(t, store, repo, tenantID, agentID, initial.ID)

	target := newVersion(t, tenantID, agentID, 2, initial.ID, domain.StatusEligible)
	createVersion(t, store, repo, target)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- svc.Promote(context.Background(), application.Promote{
				TenantID:  tenantID,
				AgentID:   agentID,
				VersionID: target.ID,
				ActorID:   ownerID,
				IsAdmin:   false,
			})
		}()
	}
	wg.Wait()
	close(errs)

	success := 0
	for err := range errs {
		if err == nil {
			success++
		}
	}
	require.Equal(t, 1, success)
}

func TestRollbackUpdatesCurrentVersion(t *testing.T) {
	db := testdb.StartPostgres(t)
	svc, repo, store := newService(t, db)

	tenantID := "tenant-rollback"
	agentID := "agent-rollback"
	ownerID := "owner"
	insertAgent(t, db, tenantID, agentID, ownerID)

	oldActive := newVersion(t, tenantID, agentID, 1, "", domain.StatusActive)
	createVersion(t, store, repo, oldActive)
	updateCurrentVersion(t, store, repo, tenantID, agentID, oldActive.ID)

	newActive := newVersion(t, tenantID, agentID, 2, oldActive.ID, domain.StatusActive)
	createVersion(t, store, repo, newActive)
	updateCurrentVersion(t, store, repo, tenantID, agentID, newActive.ID)

	err := svc.Rollback(context.Background(), application.Rollback{
		TenantID:  tenantID,
		AgentID:   agentID,
		VersionID: oldActive.ID,
		ActorID:   ownerID,
		IsAdmin:   false,
	})
	require.NoError(t, err)

	var currentVersionID string
	require.NoError(t, db.QueryRow(context.Background(),
		`SELECT current_version_id FROM agents WHERE tenant_id=$1 AND id=$2`,
		tenantID, agentID,
	).Scan(&currentVersionID))
	require.Equal(t, oldActive.ID, currentVersionID)

	owner, err := repo.GetAgentOwner(context.Background(), tenantID, agentID)
	require.NoError(t, err)
	require.Equal(t, ownerID, owner)
}
