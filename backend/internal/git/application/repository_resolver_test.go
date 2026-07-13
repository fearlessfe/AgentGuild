package application_test

import (
	"context"
	"testing"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
	"agentguild.dev/agentguild/backend/internal/git/gittest"
	"github.com/stretchr/testify/require"
)

func TestRepositoryGitResolverChoosesBoundApp(t *testing.T) {
	driver1 := &fakeCredentialDriver{}
	driver2 := &fakeCredentialDriver{}
	apps := &resolverGitHubApps{drivers: map[string]git.Driver{"gha-1": driver1, "gha-2": driver2}}
	resolver, err := application.NewRepositoryGitResolver(
		&resolverRepositoryStore{records: map[string]*application.OnboardedRepositoryRecord{
			"tenant-1/acme/api": {
				TenantID: "tenant-1", SourceType: application.RepositorySourceGitHubApp,
				FullName: "acme/api", GitHubAppID: "gha-2",
			},
			"tenant-1/acme/web": {
				TenantID: "tenant-1", SourceType: application.RepositorySourceGitHubApp,
				FullName: "acme/web", GitHubAppID: "gha-1",
			},
		}},
		apps,
		&gittest.StubIssueSource{},
	)
	require.NoError(t, err)

	got, err := resolver.Driver(context.Background(), "tenant-1", "https://github.com/acme/api.git")
	require.NoError(t, err)
	require.Same(t, driver2, got)

	got, err = resolver.Driver(context.Background(), "tenant-1", "acme/web")
	require.NoError(t, err)
	require.Same(t, driver1, got)
	require.Equal(t, []string{"tenant-1/gha-2", "tenant-1/gha-1"}, apps.driverCalls)
}

func TestRepositoryGitResolverUsesBoundAppIssueSource(t *testing.T) {
	source1 := &gittest.StubIssueSource{}
	source2 := &gittest.StubIssueSource{}
	apps := &resolverGitHubApps{sources: map[string]git.IssueSource{"gha-1": source1, "gha-2": source2}}
	resolver, err := application.NewRepositoryGitResolver(
		&resolverRepositoryStore{records: map[string]*application.OnboardedRepositoryRecord{
			"tenant-1/acme/api": {
				TenantID: "tenant-1", SourceType: application.RepositorySourceGitHubApp,
				FullName: "acme/api", GitHubAppID: "gha-2",
			},
		}},
		apps,
		&gittest.StubIssueSource{},
	)
	require.NoError(t, err)

	got, err := resolver.IssueSource(context.Background(), "tenant-1", "acme/api", "app")
	require.NoError(t, err)
	require.Same(t, source2, got)
	require.Equal(t, []string{"tenant-1/gha-2"}, apps.sourceCalls)
}

func TestRepositoryGitResolverPublicRepositoryFailsClosedForDriver(t *testing.T) {
	apps := &resolverGitHubApps{}
	resolver, err := application.NewRepositoryGitResolver(
		&resolverRepositoryStore{records: map[string]*application.OnboardedRepositoryRecord{
			"tenant-1/acme/api": {
				TenantID: "tenant-1", SourceType: application.RepositorySourcePublicGitHub,
				FullName: "acme/api",
			},
		}},
		apps,
		&gittest.StubIssueSource{},
	)
	require.NoError(t, err)

	_, err = resolver.Driver(context.Background(), "tenant-1", "acme/api")
	require.Error(t, err)
	require.Equal(t, "state_conflict", domain.CodeOf(err))
	require.Empty(t, apps.driverCalls)
}

func TestRepositoryGitResolverPublicIssueSourceDoesNotChooseApp(t *testing.T) {
	public := &gittest.StubIssueSource{}
	apps := &resolverGitHubApps{}
	resolver, err := application.NewRepositoryGitResolver(
		&resolverRepositoryStore{records: map[string]*application.OnboardedRepositoryRecord{
			"tenant-1/acme/api": {
				TenantID: "tenant-1", SourceType: application.RepositorySourcePublicGitHub,
				FullName: "acme/api",
			},
		}},
		apps,
		public,
	)
	require.NoError(t, err)

	got, err := resolver.IssueSource(context.Background(), "tenant-1", "acme/api", "public")
	require.NoError(t, err)
	require.Same(t, public, got)
	require.Empty(t, apps.sourceCalls)
}

func TestRepositoryGitResolverUnknownRepositoryFailsClosed(t *testing.T) {
	apps := &resolverGitHubApps{}
	resolver, err := application.NewRepositoryGitResolver(
		&resolverRepositoryStore{},
		apps,
		&gittest.StubIssueSource{},
	)
	require.NoError(t, err)

	_, err = resolver.Driver(context.Background(), "tenant-1", "acme/missing")
	require.Error(t, err)
	require.Equal(t, "not_found", domain.CodeOf(err))
	require.Empty(t, apps.driverCalls)
}

type resolverRepositoryStore struct {
	records map[string]*application.OnboardedRepositoryRecord
}

func (s *resolverRepositoryStore) ListOnboardedRepositories(context.Context, string) ([]application.OnboardedRepositoryRecord, error) {
	return nil, nil
}

func (s *resolverRepositoryStore) GetOnboardedRepositoryByFullName(_ context.Context, tenantID, fullName string) (*application.OnboardedRepositoryRecord, error) {
	record, ok := s.records[tenantID+"/"+fullName]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := *record
	return &copy, nil
}

func (s *resolverRepositoryStore) CreateOnboardedRepository(context.Context, *application.OnboardedRepositoryRecord) error {
	return nil
}

func (s *resolverRepositoryStore) UpsertOnboardedRepository(context.Context, *application.OnboardedRepositoryRecord) error {
	return nil
}

func (s *resolverRepositoryStore) DeleteOnboardedRepository(context.Context, string, string) error {
	return nil
}

type resolverGitHubApps struct {
	drivers     map[string]git.Driver
	sources     map[string]git.IssueSource
	driverCalls []string
	sourceCalls []string
}

func (a *resolverGitHubApps) DriverForApp(_ context.Context, tenantID, appID string) (git.Driver, error) {
	a.driverCalls = append(a.driverCalls, tenantID+"/"+appID)
	return a.drivers[appID], nil
}

func (a *resolverGitHubApps) IssueSourceForApp(_ context.Context, tenantID, appID string) (git.IssueSource, error) {
	a.sourceCalls = append(a.sourceCalls, tenantID+"/"+appID)
	return a.sources[appID], nil
}
