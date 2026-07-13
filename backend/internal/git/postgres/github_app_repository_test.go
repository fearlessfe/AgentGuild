package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	gitdomain "agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
	"agentguild.dev/agentguild/backend/internal/git/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestGitHubAppRepositoryStoresTwoAppsForTenant(t *testing.T) {
	db := testdb.StartPostgres(t)
	repo := postgres.NewGitHubAppRepository(db)
	require.NoError(t, repo.Upsert(context.Background(), &application.GitHubAppRecord{ID: "gha-1", TenantID: "tenant-1", AppID: 11, PrivateKey: "key-1", AppSlug: "alpha", IsDefault: true}))
	require.NoError(t, repo.Upsert(context.Background(), &application.GitHubAppRecord{ID: "gha-2", TenantID: "tenant-1", AppID: 22, PrivateKey: "key-2", AppSlug: "beta"}))

	apps, err := repo.ListByTenant(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Equal(t, []string{"gha-1", "gha-2"}, []string{apps[0].ID, apps[1].ID})
}

func TestGitHubAppRepositoryConcurrentFirstAppsChooseExactlyOneDefault(t *testing.T) {
	db := testdb.StartPostgres(t)
	repo := postgres.NewGitHubAppRepository(db)
	ctx := context.Background()
	start := make(chan struct{})
	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	for _, record := range []*application.GitHubAppRecord{
		{ID: "gha-1", TenantID: "tenant-1", AppID: 11, PrivateKey: "key-1"},
		{ID: "gha-2", TenantID: "tenant-1", AppID: 22, PrivateKey: "key-2"},
	} {
		record := record
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errCh <- repo.Upsert(ctx, record)
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	apps, err := repo.ListByTenant(ctx, "tenant-1")
	require.NoError(t, err)
	require.Len(t, apps, 2)
	defaultCount := 0
	for _, app := range apps {
		if app.IsDefault {
			defaultCount++
		}
	}
	require.Equal(t, 1, defaultCount)
}

func TestGitHubAppRepositoryStaleInstallUpsertPreservesPromotedDefault(t *testing.T) {
	db := testdb.StartPostgres(t)
	repo := postgres.NewGitHubAppRepository(db)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, &application.GitHubAppRecord{ID: "gha-1", TenantID: "tenant-1", AppID: 11, PrivateKey: "key-1"}))
	require.NoError(t, repo.Upsert(ctx, &application.GitHubAppRecord{ID: "gha-2", TenantID: "tenant-1", AppID: 22, PrivateKey: "key-2"}))

	stale, err := repo.GetByID(ctx, "tenant-1", "gha-2")
	require.NoError(t, err)
	require.False(t, stale.IsDefault)
	require.NoError(t, repo.DeleteAndPromoteDefault(ctx, "tenant-1", "gha-1"))

	stale.InstallationID = 42
	stale.InstallationAccountLogin = "acme"
	require.NoError(t, repo.Upsert(ctx, stale))

	got, err := repo.GetDefault(ctx, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, "gha-2", got.ID)
	require.Equal(t, int64(42), got.InstallationID)
	require.Equal(t, "acme", got.InstallationAccountLogin)
}

func TestGitHubAppRepositoryDeleteAndPromoteDefaultIsAtomic(t *testing.T) {
	db := testdb.StartPostgres(t)
	apps := postgres.NewGitHubAppRepository(db)
	require.NoError(t, apps.Upsert(context.Background(), &application.GitHubAppRecord{ID: "gha-1", TenantID: "tenant-1", AppID: 11, PrivateKey: "key-1", IsDefault: true}))
	require.NoError(t, apps.Upsert(context.Background(), &application.GitHubAppRecord{ID: "gha-2", TenantID: "tenant-1", AppID: 22, PrivateKey: "key-2"}))

	require.NoError(t, apps.DeleteAndPromoteDefault(context.Background(), "tenant-1", "gha-1"))
	got, err := apps.GetDefault(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Equal(t, "gha-2", got.ID)
}

func TestGitHubAppRepositoryRejectsDeletingBoundAppWithDomainConflict(t *testing.T) {
	db := testdb.StartPostgres(t)
	apps := postgres.NewGitHubAppRepository(db)
	repos := postgres.NewOnboardedRepositoryRepository(db)
	require.NoError(t, apps.Upsert(context.Background(), &application.GitHubAppRecord{ID: "gha-1", TenantID: "tenant-1", AppID: 11, PrivateKey: "key-1", IsDefault: true}))
	require.NoError(t, repos.UpsertOnboardedRepository(context.Background(), &application.OnboardedRepositoryRecord{
		ID: "repo-1", TenantID: "tenant-1", SourceType: "github_app", FullName: "acme/api",
		DefaultBranch: "main", Visibility: "private", GitHubAppID: "gha-1",
	}))

	err := apps.DeleteAndPromoteDefault(context.Background(), "tenant-1", "gha-1")
	require.ErrorIs(t, err, gitdomain.ErrGitHubAppInUse)
	_, getErr := apps.GetByID(context.Background(), "tenant-1", "gha-1")
	require.NoError(t, getErr)
}

func TestMultiGitHubAppsDownMigrationRefusesDataLoss(t *testing.T) {
	db := testdb.StartPostgres(t)
	repo := postgres.NewGitHubAppRepository(db)
	require.NoError(t, repo.Upsert(context.Background(), &application.GitHubAppRecord{ID: "gha-1", TenantID: "tenant-1", AppID: 11, PrivateKey: "key-1", IsDefault: true}))
	require.NoError(t, repo.Upsert(context.Background(), &application.GitHubAppRecord{ID: "gha-2", TenantID: "tenant-1", AppID: 22, PrivateKey: "key-2"}))

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	body, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "..", "..", "migrations", "000013_multi_github_apps.down.sql"))
	require.NoError(t, err)
	_, err = db.Exec(context.Background(), string(body))
	require.ErrorContains(t, err, "cannot downgrade multi GitHub App schema: at least one tenant has more than one App")
}
