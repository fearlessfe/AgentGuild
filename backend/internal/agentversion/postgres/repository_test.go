package postgres_test

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

func newDraftVersion(t *testing.T, tenantID, agentID string, versionNumber int, parentID string) *domain.AgentVersion {
	t.Helper()
	v, err := domain.NewAgentVersion(
		randomID(), tenantID, agentID, versionNumber, parentID,
		"python", "gpt-4", []string{"code"},
		"sha256:prompt", []string{"sha256:skill"},
		"sha256:memory", []string{"sha256:tool"},
		"env-digest", "owner-id", time.Now(),
	)
	require.NoError(t, err)
	return v
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func createVersion(t *testing.T, store application.Store, repo application.VersionRepository, v *domain.AgentVersion) {
	t.Helper()
	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		return repo.Create(context.Background(), tx, v)
	})
	require.NoError(t, err)
}

func TestRepositoryCreateAndGet(t *testing.T) {
	db := testdb.StartPostgres(t)
	repo := avpostgres.NewVersionRepository(db)
	store := avpostgres.NewStore(db)

	tenantID := "tenant-create"
	agentID := "agent-create"
	insertAgent(t, db, tenantID, agentID, "owner")

	v := newDraftVersion(t, tenantID, agentID, 1, "")
	createVersion(t, store, repo, v)
	require.True(t, v.Persisted)

	loaded, err := repo.GetByID(context.Background(), tenantID, agentID, v.ID)
	require.NoError(t, err)
	require.Equal(t, v.ID, loaded.ID)
	require.Equal(t, domain.StatusDraft, loaded.Status)
	require.Equal(t, v.ContentHash, loaded.ContentHash)
	require.Equal(t, v.ConfigFingerprint, loaded.ConfigFingerprint)
}

func TestRepositoryUpdateStatusDoesNotMutateContent(t *testing.T) {
	db := testdb.StartPostgres(t)
	repo := avpostgres.NewVersionRepository(db)
	store := avpostgres.NewStore(db)

	tenantID := "tenant-update"
	agentID := "agent-update"
	insertAgent(t, db, tenantID, agentID, "owner")

	v := newDraftVersion(t, tenantID, agentID, 1, "")
	createVersion(t, store, repo, v)

	v.Status = domain.StatusEvaluating
	err := store.WithTx(context.Background(), func(tx application.Tx) error {
		return repo.UpdateStatus(context.Background(), tx, v)
	})
	require.NoError(t, err)

	loaded, err := repo.GetByID(context.Background(), tenantID, agentID, v.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusEvaluating, loaded.Status)
	require.Equal(t, v.ContentHash, loaded.ContentHash)
	require.Equal(t, "sha256:prompt", loaded.PromptRef)
	require.Equal(t, []string{"sha256:skill"}, loaded.SkillRefs)
}

func TestRepositoryListByAgentOrdered(t *testing.T) {
	db := testdb.StartPostgres(t)
	repo := avpostgres.NewVersionRepository(db)
	store := avpostgres.NewStore(db)

	tenantID := "tenant-list"
	agentID := "agent-list"
	insertAgent(t, db, tenantID, agentID, "owner")

	v1 := newDraftVersion(t, tenantID, agentID, 1, "")
	v2 := newDraftVersion(t, tenantID, agentID, 2, v1.ID)
	v3 := newDraftVersion(t, tenantID, agentID, 3, v2.ID)
	createVersion(t, store, repo, v1)
	createVersion(t, store, repo, v2)
	createVersion(t, store, repo, v3)

	versions, err := repo.ListByAgent(context.Background(), tenantID, agentID)
	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, 3, versions[0].VersionNumber)
	require.Equal(t, 2, versions[1].VersionNumber)
	require.Equal(t, 1, versions[2].VersionNumber)
}

func TestRepositoryConcurrentCreateDoesNotDuplicateVersionNumber(t *testing.T) {
	db := testdb.StartPostgres(t)
	repo := avpostgres.NewVersionRepository(db)
	store := avpostgres.NewStore(db)

	tenantID := "tenant-concurrent"
	agentID := "agent-concurrent"
	insertAgent(t, db, tenantID, agentID, "owner")

	parent := newDraftVersion(t, tenantID, agentID, 1, "")
	createVersion(t, store, repo, parent)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := newDraftVersion(t, tenantID, agentID, 2, parent.ID)
			errs <- store.WithTx(context.Background(), func(tx application.Tx) error {
				return repo.Create(context.Background(), tx, v)
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
	require.Equal(t, 1, success, "only one concurrent draft creation with the same version_number may succeed")

	versions, err := repo.ListByAgent(context.Background(), tenantID, agentID)
	require.NoError(t, err)
	require.Len(t, versions, 2)
}
