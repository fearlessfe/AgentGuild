package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

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
