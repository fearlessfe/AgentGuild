package postgres_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git/application"
	"agentguild.dev/agentguild/backend/internal/git/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestOnboardedRepositoryRepositoryPersistsTenantScopedRepositories(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	repo := postgres.NewOnboardedRepositoryRepository(db)

	record := onboardedRepositoryRecord("tenant-1", "repo-1", "public_github", "owner/repo")
	recordTenant2 := onboardedRepositoryRecord("tenant-2", "repo-2", "public_github", "owner/repo")
	recordTenant1Other := onboardedRepositoryRecord("tenant-1", "repo-3", "public_github", "public/repo")

	require.NoError(t, repo.UpsertOnboardedRepository(ctx, record))
	require.NoError(t, repo.UpsertOnboardedRepository(ctx, recordTenant2))
	require.NoError(t, repo.UpsertOnboardedRepository(ctx, recordTenant1Other))

	got, err := repo.ListOnboardedRepositories(ctx, "tenant-1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.ElementsMatch(t, []string{"repo-1", "repo-3"}, []string{got[0].ID, got[1].ID})
	require.Equal(t, "tenant-1", got[0].TenantID)
	require.Equal(t, "tenant-1", got[1].TenantID)
	require.NotZero(t, got[0].CreatedAt)
	require.NotZero(t, got[0].UpdatedAt)
	require.NotZero(t, got[1].CreatedAt)
	require.NotZero(t, got[1].UpdatedAt)
}

func TestOnboardedRepositoryRepositoryUpsertUpdatesExistingRepository(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	repo := postgres.NewOnboardedRepositoryRepository(db)

	record := onboardedRepositoryRecord("tenant-1", "repo-1", "public_github", "owner/repo")
	require.NoError(t, repo.UpsertOnboardedRepository(ctx, record))

	before, err := repo.ListOnboardedRepositories(ctx, "tenant-1")
	require.NoError(t, err)
	require.Len(t, before, 1)

	time.Sleep(time.Millisecond)
	updated := onboardedRepositoryRecord("tenant-1", "repo-2", "public_github", "owner/repo")
	updated.DefaultBranch = "trunk"
	updated.Visibility = "private"
	require.NoError(t, repo.UpsertOnboardedRepository(ctx, updated))
	require.Equal(t, "repo-1", updated.ID)

	got, err := repo.ListOnboardedRepositories(ctx, "tenant-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "repo-1", got[0].ID)
	require.Equal(t, "owner/repo", got[0].FullName)
	require.Equal(t, "trunk", got[0].DefaultBranch)
	require.Equal(t, "private", got[0].Visibility)
	require.True(t, got[0].CreatedAt.Equal(before[0].CreatedAt))
	require.True(t, got[0].UpdatedAt.After(before[0].UpdatedAt))
}

func TestOnboardedRepositoryRepositoryDeleteUsesTenantAndID(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	repo := postgres.NewOnboardedRepositoryRepository(db)

	recordTenant1 := onboardedRepositoryRecord("tenant-1", "repo-1", "public_github", "owner/repo")
	recordTenant2 := onboardedRepositoryRecord("tenant-2", "repo-1", "public_github", "owner/repo")
	require.NoError(t, repo.UpsertOnboardedRepository(ctx, recordTenant1))
	require.NoError(t, repo.UpsertOnboardedRepository(ctx, recordTenant2))

	require.NoError(t, repo.DeleteOnboardedRepository(ctx, "tenant-1", "repo-1"))

	gotTenant1, err := repo.ListOnboardedRepositories(ctx, "tenant-1")
	require.NoError(t, err)
	require.Empty(t, gotTenant1)

	gotTenant2, err := repo.ListOnboardedRepositories(ctx, "tenant-2")
	require.NoError(t, err)
	require.Len(t, gotTenant2, 1)
	require.Equal(t, "repo-1", gotTenant2[0].ID)
}

func TestOnboardedRepositoryPersistsGitHubAppBinding(t *testing.T) {
	db := testdb.StartPostgres(t)
	apps := postgres.NewGitHubAppRepository(db)
	require.NoError(t, apps.Upsert(context.Background(), &application.GitHubAppRecord{
		ID: "gha-1", TenantID: "tenant-1", AppID: 11, PrivateKey: "key-1", IsDefault: true,
	}))
	repos := postgres.NewOnboardedRepositoryRepository(db)
	require.NoError(t, repos.UpsertOnboardedRepository(context.Background(), &application.OnboardedRepositoryRecord{
		ID: "repo-1", TenantID: "tenant-1", SourceType: application.RepositorySourceGitHubApp,
		FullName: "acme/api", GitHubAppID: "gha-1", DefaultBranch: "main", Visibility: "private",
	}))
	got, err := repos.GetOnboardedRepositoryByFullName(context.Background(), "tenant-1", "acme/api")
	require.NoError(t, err)
	require.Equal(t, "gha-1", got.GitHubAppID)
}

func onboardedRepositoryRecord(tenantID, id, sourceType, fullName string) *application.OnboardedRepositoryRecord {
	return &application.OnboardedRepositoryRecord{
		ID:            id,
		TenantID:      tenantID,
		SourceType:    sourceType,
		FullName:      fullName,
		DefaultBranch: "main",
		Visibility:    "public",
	}
}
