package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
	"github.com/stretchr/testify/require"
)

func TestRepositoryOnboardingService_AddPublicRepositoryNormalizesGitHubURL(t *testing.T) {
	store := newMemoryOnboardedRepositoryStore()
	resolver := fakePublicRepositoryResolver{
		repo: git.Repository{FullName: "vercel/next.js", DefaultBranch: "canary", Visibility: "public"},
	}
	svc, err := application.NewRepositoryOnboardingService(store, fakeGitHubApps{}, resolver, func() string { return "repo-1" })
	require.NoError(t, err)

	view, err := svc.AddPublicRepository(context.Background(), adminPrincipal("tenant-1"), "https://github.com/vercel/next.js")

	require.NoError(t, err)
	require.Equal(t, "repo-1", view.ID)
	require.Equal(t, "public_github", view.SourceType)
	require.Equal(t, "vercel/next.js", view.FullName)
	require.Equal(t, "canary", view.DefaultBranch)
	require.Equal(t, "public", view.Visibility)
}

func TestRepositoryOnboardingService_AddPublicRepositoryRejectsDuplicate(t *testing.T) {
	store := newMemoryOnboardedRepositoryStore()
	svc, err := application.NewRepositoryOnboardingService(store, fakeGitHubApps{}, fakePublicRepositoryResolver{}, func() string {
		return "repo-1"
	})
	require.NoError(t, err)

	_, err = svc.AddPublicRepository(context.Background(), adminPrincipal("tenant-1"), "acme/docs")
	require.NoError(t, err)
	_, err = svc.AddPublicRepository(context.Background(), adminPrincipal("tenant-1"), "acme/docs")

	require.ErrorIs(t, err, git.ErrRepositoryBindingConflict)
	require.Len(t, store.records, 1)
}

func TestRepositoryOnboardingServiceConstructorRejectsNilDependencies(t *testing.T) {
	cases := []struct {
		name  string
		store application.OnboardedRepositoryStore
		apps  application.GitHubAppManager
		pub   application.PublicRepositoryResolver
		field string
	}{
		{
			name:  "store",
			apps:  fakeGitHubApps{},
			pub:   fakePublicRepositoryResolver{},
			field: "repository_store",
		},
		{
			name:  "github apps",
			store: newMemoryOnboardedRepositoryStore(),
			pub:   fakePublicRepositoryResolver{},
			field: "github_app_manager",
		},
		{
			name:  "public resolver",
			store: newMemoryOnboardedRepositoryStore(),
			apps:  fakeGitHubApps{},
			field: "public_repository_resolver",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := application.NewRepositoryOnboardingService(tc.store, tc.apps, tc.pub, nil)

			require.Nil(t, svc)
			require.Error(t, err)
			require.Equal(t, "invalid_argument", domain.CodeOf(err))
			require.Equal(t, tc.field, domain.FieldOf(err))
		})
	}
}

func TestRepositoryOnboardingService_AddPublicRepositoryRejectsInvalidRepoShapes(t *testing.T) {
	svc, err := application.NewRepositoryOnboardingService(newMemoryOnboardedRepositoryStore(), fakeGitHubApps{}, fakePublicRepositoryResolver{}, func() string { return "repo-1" })
	require.NoError(t, err)

	for _, input := range []string{"owner", "/owner/repo", "https://example.com/owner/repo"} {
		t.Run(input, func(t *testing.T) {
			_, err := svc.AddPublicRepository(context.Background(), adminPrincipal("tenant-1"), input)

			require.Error(t, err)
			require.Equal(t, "invalid_argument", domain.CodeOf(err))
			require.Equal(t, "repo", domain.FieldOf(err))
		})
	}
}

func TestRepositoryOnboardingService_AddGitHubAppRepositoryRequiresVisibleRepo(t *testing.T) {
	svc, err := application.NewRepositoryOnboardingService(
		newMemoryOnboardedRepositoryStore(),
		fakeGitHubApps{repos: []git.Repository{{FullName: "acme/api", DefaultBranch: "main", Visibility: "private"}}},
		fakePublicRepositoryResolver{},
		func() string { return "repo-1" },
	)
	require.NoError(t, err)

	_, err = svc.AddGitHubAppRepository(context.Background(), adminPrincipal("tenant-1"), "gha-1", "acme/missing")

	require.Error(t, err)
	require.Equal(t, "not_found", domain.CodeOf(err))
}

func TestRepositoryOnboardingService_ListGitHubAppRepositoriesUsesSelectedApp(t *testing.T) {
	svc, err := application.NewRepositoryOnboardingService(
		newMemoryOnboardedRepositoryStore(),
		fakeGitHubApps{
			allowedAppID: "gha-2",
			repos:        []git.Repository{{FullName: "acme/api", DefaultBranch: "main", Visibility: "private"}},
		},
		fakePublicRepositoryResolver{},
		func() string { return "repo-1" },
	)
	require.NoError(t, err)

	items, err := svc.ListGitHubAppRepositories(context.Background(), adminPrincipal("tenant-1"), "gha-2")

	require.NoError(t, err)
	require.Equal(t, []application.RepositoryCandidateView{{FullName: "acme/api", DefaultBranch: "main", Visibility: "private"}}, items)
}

func TestRepositoryOnboardingService_AddGitHubAppRepositoryBindsSelectedApp(t *testing.T) {
	store := newMemoryOnboardedRepositoryStore()
	svc, err := application.NewRepositoryOnboardingService(
		store,
		fakeGitHubApps{
			allowedAppID: "gha-2",
			repos:        []git.Repository{{FullName: "acme/api", DefaultBranch: "main", Visibility: "private"}},
		},
		fakePublicRepositoryResolver{},
		func() string { return "repo-1" },
	)
	require.NoError(t, err)

	view, err := svc.AddGitHubAppRepository(context.Background(), adminPrincipal("tenant-1"), "gha-2", "acme/api")

	require.NoError(t, err)
	require.Equal(t, "gha-2", view.GitHubAppID)
	require.Equal(t, "gha-2", store.records[0].GitHubAppID)
}

func TestRepositoryOnboardingService_AddRepositoryRejectsDuplicateAcrossApps(t *testing.T) {
	store := newMemoryOnboardedRepositoryStore()
	svc, err := application.NewRepositoryOnboardingService(
		store,
		fakeGitHubApps{repos: []git.Repository{{FullName: "acme/api", DefaultBranch: "main", Visibility: "private"}}},
		fakePublicRepositoryResolver{},
		func() string { return "repo-1" },
	)
	require.NoError(t, err)

	_, err = svc.AddGitHubAppRepository(context.Background(), adminPrincipal("tenant-1"), "gha-1", "acme/api")
	require.NoError(t, err)
	_, err = svc.AddGitHubAppRepository(context.Background(), adminPrincipal("tenant-1"), "gha-2", "acme/api")

	require.Error(t, err)
	require.Equal(t, "state_conflict", domain.CodeOf(err))
}

func TestRepositoryOnboardingService_ForeignAppIsNotFound(t *testing.T) {
	svc, err := application.NewRepositoryOnboardingService(
		newMemoryOnboardedRepositoryStore(),
		fakeGitHubApps{allowedAppID: "gha-owned"},
		fakePublicRepositoryResolver{},
		func() string { return "repo-1" },
	)
	require.NoError(t, err)

	_, err = svc.ListGitHubAppRepositories(context.Background(), adminPrincipal("tenant-1"), "gha-foreign")

	require.Error(t, err)
	require.Equal(t, "not_found", domain.CodeOf(err))
}

func TestRepositoryOnboardingService_MutationsRequireAdmin(t *testing.T) {
	svc, err := application.NewRepositoryOnboardingService(
		newMemoryOnboardedRepositoryStore(),
		fakeGitHubApps{repos: []git.Repository{{FullName: "acme/api", DefaultBranch: "main", Visibility: "private"}}},
		fakePublicRepositoryResolver{},
		func() string { return "repo-1" },
	)
	require.NoError(t, err)
	principal := application.Principal{TenantID: "tenant-1", OwnerID: "owner-1"}

	_, err = svc.AddPublicRepository(context.Background(), principal, "acme/docs")
	require.Error(t, err)
	require.Equal(t, "forbidden", domain.CodeOf(err))

	_, err = svc.AddGitHubAppRepository(context.Background(), principal, "gha-1", "acme/api")
	require.Error(t, err)
	require.Equal(t, "forbidden", domain.CodeOf(err))

	err = svc.Remove(context.Background(), principal, "repo-1")
	require.Error(t, err)
	require.Equal(t, "forbidden", domain.CodeOf(err))
}

func TestRepositoryOnboardingService_SummaryReturnsConfiguredAppCandidatesAndOnboardedInventory(t *testing.T) {
	store := newMemoryOnboardedRepositoryStore()
	svc, err := application.NewRepositoryOnboardingService(
		store,
		fakeGitHubApps{
			view:  application.GitHubAppView{TenantID: "tenant-1", Provider: "github", AppID: 123, InstallationID: 456, Configured: true},
			repos: []git.Repository{{FullName: "acme/api", DefaultBranch: "main", Visibility: "private"}},
		},
		fakePublicRepositoryResolver{},
		func() string { return "repo-1" },
	)
	require.NoError(t, err)
	_, err = svc.AddPublicRepository(context.Background(), adminPrincipal("tenant-1"), "acme/docs")
	require.NoError(t, err)

	summary, err := svc.Summary(context.Background(), adminPrincipal("tenant-1"))

	require.NoError(t, err)
	require.True(t, summary.GitHubApp.Configured)
	require.Equal(t, int64(123), summary.GitHubApp.AppID)
	require.Equal(t, []application.RepositoryCandidateView{{FullName: "acme/api", DefaultBranch: "main", Visibility: "private"}}, summary.AppRepositories)
	require.Len(t, summary.OnboardedRepositories, 1)
	require.Equal(t, "acme/docs", summary.OnboardedRepositories[0].FullName)
	require.Empty(t, summary.AppRepositoriesError)
}

func TestRepositoryOnboardingService_SummaryRedactsOperationalGitHubError(t *testing.T) {
	secret := "forged-upstream-secret-token"
	svc, err := application.NewRepositoryOnboardingService(
		newMemoryOnboardedRepositoryStore(),
		fakeGitHubApps{view: application.GitHubAppView{Configured: true}, listErr: errors.New("github response contained " + secret)},
		fakePublicRepositoryResolver{},
		nil,
	)
	require.NoError(t, err)

	summary, err := svc.Summary(context.Background(), adminPrincipal("tenant-1"))

	require.NoError(t, err)
	require.NotEmpty(t, summary.AppRepositoriesError)
	require.NotContains(t, summary.AppRepositoriesError, secret)
}

func TestRepositoryOnboardingService_SummaryReturnsUnconfiguredAppWithoutCandidates(t *testing.T) {
	svc, err := application.NewRepositoryOnboardingService(
		newMemoryOnboardedRepositoryStore(),
		fakeGitHubApps{getErr: git.ErrGitHubAppNotConfigured},
		fakePublicRepositoryResolver{},
		func() string { return "repo-1" },
	)
	require.NoError(t, err)

	summary, err := svc.Summary(context.Background(), adminPrincipal("tenant-1"))

	require.NoError(t, err)
	require.False(t, summary.GitHubApp.Configured)
	require.Empty(t, summary.AppRepositories)
	require.Empty(t, summary.AppRepositoriesError)
}

func TestRepositoryOnboardingService_SummaryRedactsAppRepositoryListingError(t *testing.T) {
	svc, err := application.NewRepositoryOnboardingService(
		newMemoryOnboardedRepositoryStore(),
		fakeGitHubApps{
			view:    application.GitHubAppView{TenantID: "tenant-1", Provider: "github", AppID: 123, InstallationID: 456, Configured: true},
			listErr: errors.New("github unavailable"),
		},
		fakePublicRepositoryResolver{},
		func() string { return "repo-1" },
	)
	require.NoError(t, err)

	summary, err := svc.Summary(context.Background(), adminPrincipal("tenant-1"))

	require.NoError(t, err)
	require.True(t, summary.GitHubApp.Configured)
	require.Empty(t, summary.AppRepositories)
	require.Equal(t, "GitHub App repository inventory is temporarily unavailable", summary.AppRepositoriesError)
}

func TestRepositoryOnboardingService_RemoveDelegatesTenantScopedDelete(t *testing.T) {
	store := newMemoryOnboardedRepositoryStore()
	svc, err := application.NewRepositoryOnboardingService(store, fakeGitHubApps{}, fakePublicRepositoryResolver{}, func() string { return "repo-1" })
	require.NoError(t, err)

	err = svc.Remove(context.Background(), adminPrincipal("tenant-1"), "repo-1")

	require.NoError(t, err)
	require.Equal(t, "tenant-1", store.deletedTenantID)
	require.Equal(t, "repo-1", store.deletedID)
}

func adminPrincipal(tenantID string) application.Principal {
	return application.Principal{
		TenantID:   tenantID,
		OwnerID:    "owner-1",
		OwnerEmail: "owner@example.com",
		IsAdmin:    true,
	}
}

func newMemoryOnboardedRepositoryStore() *memoryOnboardedRepositoryStore {
	return &memoryOnboardedRepositoryStore{
		now: time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC),
	}
}

type memoryOnboardedRepositoryStore struct {
	now             time.Time
	records         []application.OnboardedRepositoryRecord
	deletedTenantID string
	deletedID       string
}

func (s *memoryOnboardedRepositoryStore) ListOnboardedRepositories(_ context.Context, tenantID string) ([]application.OnboardedRepositoryRecord, error) {
	var records []application.OnboardedRepositoryRecord
	for _, record := range s.records {
		if record.TenantID == tenantID {
			records = append(records, record)
		}
	}
	return records, nil
}

func (s *memoryOnboardedRepositoryStore) GetOnboardedRepositoryByFullName(_ context.Context, tenantID, fullName string) (*application.OnboardedRepositoryRecord, error) {
	for _, record := range s.records {
		if record.TenantID == tenantID && record.FullName == fullName {
			copy := record
			return &copy, nil
		}
	}
	return nil, errors.New("repository not found")
}

func (s *memoryOnboardedRepositoryStore) CreateOnboardedRepository(_ context.Context, record *application.OnboardedRepositoryRecord) error {
	for _, existing := range s.records {
		if existing.TenantID == record.TenantID && existing.FullName == record.FullName {
			return git.ErrRepositoryBindingConflict
		}
	}
	record.CreatedAt = s.now
	record.UpdatedAt = s.now
	s.records = append(s.records, *record)
	return nil
}

func (s *memoryOnboardedRepositoryStore) UpsertOnboardedRepository(_ context.Context, record *application.OnboardedRepositoryRecord) error {
	record.CreatedAt = s.now
	record.UpdatedAt = s.now
	s.records = append(s.records, *record)
	return nil
}

func (s *memoryOnboardedRepositoryStore) DeleteOnboardedRepository(_ context.Context, tenantID, id string) error {
	s.deletedTenantID = tenantID
	s.deletedID = id
	return nil
}

type fakePublicRepositoryResolver struct {
	repo git.Repository
}

func (r fakePublicRepositoryResolver) ResolvePublicRepository(_ context.Context, fullName string) (git.Repository, error) {
	if r.repo.FullName == "" {
		return git.Repository{FullName: fullName, DefaultBranch: "main", Visibility: "public"}, nil
	}
	return r.repo, nil
}

type fakeGitHubApps struct {
	view         application.GitHubAppView
	getErr       error
	repos        []git.Repository
	listErr      error
	allowedAppID string
}

func (fakeGitHubApps) Driver(context.Context, string) (git.Driver, error) {
	return nil, nil
}

func (fakeGitHubApps) DriverForApp(context.Context, string, string) (git.Driver, error) {
	return nil, nil
}

func (a fakeGitHubApps) IssueSource(context.Context, string) (git.IssueSource, error) {
	return fakeIssueSource{repos: a.repos, err: a.listErr}, nil
}

func (a fakeGitHubApps) IssueSourceForApp(_ context.Context, _, appID string) (git.IssueSource, error) {
	if a.allowedAppID != "" && appID != a.allowedAppID {
		return nil, git.ErrGitHubAppNotConfigured
	}
	return fakeIssueSource{repos: a.repos, err: a.listErr}, nil
}

func (fakeGitHubApps) Upsert(context.Context, application.UpsertGitHubApp) error {
	return nil
}

func (a fakeGitHubApps) Install(context.Context, string, int64) (application.GitHubAppView, error) {
	return a.view, nil
}

func (a fakeGitHubApps) InstallByID(context.Context, string, string, int64, string) (application.GitHubAppView, error) {
	return a.view, nil
}

func (fakeGitHubApps) InstallationAccount(context.Context, string, string, int64) (string, error) {
	return "", nil
}

func (a fakeGitHubApps) Get(context.Context, string) (application.GitHubAppView, error) {
	if a.getErr != nil {
		return application.GitHubAppView{}, a.getErr
	}
	return a.view, nil
}

func (a fakeGitHubApps) GetByID(ctx context.Context, tenantID, _ string) (application.GitHubAppView, error) {
	return a.Get(ctx, tenantID)
}

func (a fakeGitHubApps) List(context.Context, string) ([]application.GitHubAppView, error) {
	if a.getErr != nil {
		return nil, a.getErr
	}
	return []application.GitHubAppView{a.view}, nil
}

func (fakeGitHubApps) Delete(context.Context, string) error {
	return nil
}

func (fakeGitHubApps) DeleteByID(context.Context, string, string) error {
	return nil
}

var _ application.OnboardedRepositoryStore = (*memoryOnboardedRepositoryStore)(nil)
var _ application.PublicRepositoryResolver = (*fakePublicRepositoryResolver)(nil)
var _ application.GitHubAppManager = fakeGitHubApps{}

type fakeIssueSource struct {
	repos []git.Repository
	err   error
}

func (s fakeIssueSource) ListInstallationRepositories(context.Context) ([]git.Repository, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.repos, nil
}

func (s fakeIssueSource) ListIssues(context.Context, string, git.IssueFilter, time.Time) ([]git.Issue, error) {
	return nil, nil
}

var _ git.IssueSource = fakeIssueSource{}
