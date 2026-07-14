package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	gitdomain "agentguild.dev/agentguild/backend/internal/git"
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

func TestOnboardedRepositoryCreateIsAtomicForConcurrentSameBinding(t *testing.T) {
	db := testdb.StartPostgres(t)
	apps := postgres.NewGitHubAppRepository(db)
	require.NoError(t, apps.Upsert(context.Background(), githubAppRecord("gha-1", 11, true)))
	repos := postgres.NewOnboardedRepositoryRepository(db)

	records := []*application.OnboardedRepositoryRecord{
		{ID: "repo-1", TenantID: "tenant-1", SourceType: application.RepositorySourceGitHubApp, GitHubAppID: "gha-1", FullName: "acme/api", DefaultBranch: "main", Visibility: "private"},
		{ID: "repo-2", TenantID: "tenant-1", SourceType: application.RepositorySourceGitHubApp, GitHubAppID: "gha-1", FullName: "acme/api", DefaultBranch: "trunk", Visibility: "public"},
	}
	start := make(chan struct{})
	errs := make([]error, len(records))
	var wg sync.WaitGroup
	for i := range records {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = repos.CreateOnboardedRepository(context.Background(), records[i])
		}(i)
	}
	close(start)
	wg.Wait()

	var succeeded int
	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		require.ErrorIs(t, err, gitdomain.ErrRepositoryBindingConflict)
	}
	require.Equal(t, 1, succeeded)

	got, err := repos.GetOnboardedRepositoryByFullName(context.Background(), "tenant-1", "acme/api")
	require.NoError(t, err)
	if errs[0] == nil {
		require.Equal(t, "repo-1", got.ID)
		require.Equal(t, "main", got.DefaultBranch)
		require.Equal(t, "private", got.Visibility)
	} else {
		require.Equal(t, "repo-2", got.ID)
		require.Equal(t, "trunk", got.DefaultBranch)
		require.Equal(t, "public", got.Visibility)
	}
	require.Equal(t, "gha-1", got.GitHubAppID)
}

func TestOnboardedRepositoryRejectsCrossAppRebinding(t *testing.T) {
	db := testdb.StartPostgres(t)
	apps := postgres.NewGitHubAppRepository(db)
	require.NoError(t, apps.Upsert(context.Background(), githubAppRecord("gha-1", 11, true)))
	require.NoError(t, apps.Upsert(context.Background(), githubAppRecord("gha-2", 22, false)))
	repos := postgres.NewOnboardedRepositoryRepository(db)
	original := onboardedRepositoryRecord("tenant-1", "repo-1", application.RepositorySourceGitHubApp, "acme/api")
	original.GitHubAppID = "gha-1"
	require.NoError(t, repos.UpsertOnboardedRepository(context.Background(), original))

	rebinding := onboardedRepositoryRecord("tenant-1", "repo-2", application.RepositorySourceGitHubApp, "acme/api")
	rebinding.GitHubAppID = "gha-2"
	rebinding.DefaultBranch = "trunk"
	require.ErrorIs(t, repos.UpsertOnboardedRepository(context.Background(), rebinding), gitdomain.ErrRepositoryBindingConflict)

	got, err := repos.GetOnboardedRepositoryByFullName(context.Background(), "tenant-1", "acme/api")
	require.NoError(t, err)
	require.Equal(t, "repo-1", got.ID)
	require.Equal(t, "gha-1", got.GitHubAppID)
	require.Equal(t, "main", got.DefaultBranch)
}

func TestOnboardedRepositoryRejectsSourceConversion(t *testing.T) {
	for _, test := range []struct {
		name     string
		original *application.OnboardedRepositoryRecord
		updated  *application.OnboardedRepositoryRecord
	}{
		{
			name:     "public to app",
			original: onboardedRepositoryRecord("tenant-1", "repo-1", application.RepositorySourcePublicGitHub, "acme/api"),
			updated:  onboardedRepositoryRecord("tenant-1", "repo-2", application.RepositorySourceGitHubApp, "acme/api"),
		},
		{
			name:     "app to public",
			original: onboardedRepositoryRecord("tenant-1", "repo-1", application.RepositorySourceGitHubApp, "acme/api"),
			updated:  onboardedRepositoryRecord("tenant-1", "repo-2", application.RepositorySourcePublicGitHub, "acme/api"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := testdb.StartPostgres(t)
			apps := postgres.NewGitHubAppRepository(db)
			require.NoError(t, apps.Upsert(context.Background(), githubAppRecord("gha-1", 11, true)))
			test.original.GitHubAppID = bindingForSource(test.original.SourceType)
			test.updated.GitHubAppID = bindingForSource(test.updated.SourceType)
			repos := postgres.NewOnboardedRepositoryRepository(db)
			require.NoError(t, repos.UpsertOnboardedRepository(context.Background(), test.original))

			require.ErrorIs(t, repos.UpsertOnboardedRepository(context.Background(), test.updated), gitdomain.ErrRepositoryBindingConflict)

			got, err := repos.GetOnboardedRepositoryByFullName(context.Background(), "tenant-1", "acme/api")
			require.NoError(t, err)
			require.Equal(t, test.original.ID, got.ID)
			require.Equal(t, test.original.SourceType, got.SourceType)
			require.Equal(t, test.original.GitHubAppID, got.GitHubAppID)
		})
	}
}

func TestOnboardedRepositoryUpdatesMetadataForSameBinding(t *testing.T) {
	db := testdb.StartPostgres(t)
	apps := postgres.NewGitHubAppRepository(db)
	require.NoError(t, apps.Upsert(context.Background(), githubAppRecord("gha-1", 11, true)))
	repos := postgres.NewOnboardedRepositoryRepository(db)
	original := onboardedRepositoryRecord("tenant-1", "repo-1", application.RepositorySourceGitHubApp, "acme/api")
	original.GitHubAppID = "gha-1"
	require.NoError(t, repos.UpsertOnboardedRepository(context.Background(), original))

	updated := onboardedRepositoryRecord("tenant-1", "repo-2", application.RepositorySourceGitHubApp, "acme/api")
	updated.GitHubAppID = "gha-1"
	updated.DefaultBranch = "trunk"
	updated.Visibility = "public"
	require.NoError(t, repos.UpsertOnboardedRepository(context.Background(), updated))

	got, err := repos.GetOnboardedRepositoryByFullName(context.Background(), "tenant-1", "acme/api")
	require.NoError(t, err)
	require.Equal(t, "repo-1", got.ID)
	require.Equal(t, "gha-1", got.GitHubAppID)
	require.Equal(t, "trunk", got.DefaultBranch)
	require.Equal(t, "public", got.Visibility)
}

func githubAppRecord(id string, appID int64, isDefault bool) *application.GitHubAppRecord {
	return &application.GitHubAppRecord{
		ID: id, TenantID: "tenant-1", AppID: appID, PrivateKey: "key-" + id, IsDefault: isDefault,
	}
}

func bindingForSource(sourceType string) string {
	if sourceType == application.RepositorySourceGitHubApp {
		return "gha-1"
	}
	return ""
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
