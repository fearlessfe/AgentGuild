package rest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func TestRepositoryOnboardingSummaryReturnsAppAndOnboardedRepositories(t *testing.T) {
	svc := newRepositoryOnboardingService(t)
	svc.apps.store["tenant-1"] = &gitapp.GitHubAppRecord{
		TenantID:       "tenant-1",
		Provider:       "github",
		AppID:          123,
		InstallationID: 456,
		BaseURL:        "https://api.github.com",
		PrivateKey:     "secret-key",
		CreatedAt:      fixedRepositoryTime(),
		UpdatedAt:      fixedRepositoryTime(),
	}
	svc.apps.issueSource = &fakeSyncIssueSource{repos: []git.Repository{{
		FullName:      "agentguild/agentguild",
		DefaultBranch: "main",
		Visibility:    "private",
	}}}
	require.NoError(t, svc.store.UpsertOnboardedRepository(context.Background(), &gitapp.OnboardedRepositoryRecord{
		ID:            "repo-1",
		TenantID:      "tenant-1",
		SourceType:    gitapp.RepositorySourcePublicGitHub,
		FullName:      "octo/hello-world",
		DefaultBranch: "main",
		Visibility:    "public",
		CreatedAt:     fixedRepositoryTime(),
		UpdatedAt:     fixedRepositoryTime(),
	}))
	server := newTestServer(&fakeApplication{}, rest.WithRepositoryOnboardingService(svc.service))

	res := getWithSession(t, server, "/v1/repository-onboarding", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	require.NotContains(t, res.Body.String(), "secret-key")
	var body struct {
		Data struct {
			GitHubApp struct {
				Configured     bool  `json:"configured"`
				AppID          int64 `json:"app_id"`
				InstallationID int64 `json:"installation_id"`
			} `json:"github_app"`
			AppRepositories struct {
				Items []repositoryItemBody `json:"items"`
			} `json:"app_repositories"`
			OnboardedRepositories struct {
				Items []repositoryItemBody `json:"items"`
			} `json:"onboarded_repositories"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.True(t, body.Data.GitHubApp.Configured)
	require.Equal(t, int64(123), body.Data.GitHubApp.AppID)
	require.Equal(t, int64(456), body.Data.GitHubApp.InstallationID)
	require.Equal(t, []repositoryItemBody{{
		FullName:      "agentguild/agentguild",
		DefaultBranch: "main",
		Visibility:    "private",
	}}, body.Data.AppRepositories.Items)
	require.Equal(t, []repositoryItemBody{{
		ID:            "repo-1",
		SourceType:    gitapp.RepositorySourcePublicGitHub,
		FullName:      "octo/hello-world",
		DefaultBranch: "main",
		Visibility:    "public",
	}}, body.Data.OnboardedRepositories.Items)
}

func TestRepositoryOnboardingPublicAddRequiresAdminSession(t *testing.T) {
	svc := newRepositoryOnboardingService(t)
	svc.public.repos["octo/hello-world"] = git.Repository{
		FullName:      "octo/hello-world",
		DefaultBranch: "main",
		Visibility:    "public",
	}
	server := newTestServer(&fakeApplication{}, rest.WithRepositoryOnboardingService(svc.service))

	unauthorized := postJSON(t, server, "/v1/repositories/public", `{"repo":"octo/hello-world"}`, "token-publisher")
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	created := postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/hello-world"}`, sessionCookie(t, "admin-1", true))

	require.Equal(t, http.StatusCreated, created.Code)
	var body struct {
		Data repositoryItemBody `json:"data"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &body))
	require.Equal(t, "repo-1", body.Data.ID)
	require.Equal(t, gitapp.RepositorySourcePublicGitHub, body.Data.SourceType)
	require.Equal(t, "octo/hello-world", body.Data.FullName)
	require.Equal(t, "tenant-1", svc.store.records["tenant-1"]["repo-1"].TenantID)
}

func TestRepositoryOnboardingGitHubAppAddRequiresAdminSession(t *testing.T) {
	svc := newRepositoryOnboardingService(t)
	svc.apps.store["tenant-1"] = &gitapp.GitHubAppRecord{
		TenantID:       "tenant-1",
		Provider:       "github",
		AppID:          123,
		InstallationID: 456,
		PrivateKey:     "secret-key",
		BaseURL:        "https://api.github.com",
	}
	svc.apps.issueSource = &fakeSyncIssueSource{repos: []git.Repository{{
		FullName:      "agentguild/agentguild",
		DefaultBranch: "main",
		Visibility:    "private",
	}}}
	server := newTestServer(&fakeApplication{}, rest.WithRepositoryOnboardingService(svc.service))

	unauthorized := postJSON(t, server, "/v1/repositories/github-app", `{"repo":"agentguild/agentguild"}`, "token-publisher")
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	created := postJSONWithSession(t, server, "/v1/repositories/github-app", `{"repo":"agentguild/agentguild"}`, sessionCookie(t, "admin-1", true))

	require.Equal(t, http.StatusCreated, created.Code)
	var body struct {
		Data repositoryItemBody `json:"data"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &body))
	require.Equal(t, "repo-1", body.Data.ID)
	require.Equal(t, gitapp.RepositorySourceGitHubApp, body.Data.SourceType)
	require.Equal(t, "agentguild/agentguild", body.Data.FullName)
	require.NotContains(t, created.Body.String(), "secret-key")
}

func TestRepositoryOnboardingDeleteRequiresAdminSession(t *testing.T) {
	svc := newRepositoryOnboardingService(t)
	require.NoError(t, svc.store.UpsertOnboardedRepository(context.Background(), &gitapp.OnboardedRepositoryRecord{
		ID:            "repo-1",
		TenantID:      "tenant-1",
		SourceType:    gitapp.RepositorySourcePublicGitHub,
		FullName:      "octo/hello-world",
		DefaultBranch: "main",
		Visibility:    "public",
		CreatedAt:     fixedRepositoryTime(),
		UpdatedAt:     fixedRepositoryTime(),
	}))
	server := newTestServer(&fakeApplication{}, rest.WithRepositoryOnboardingService(svc.service))

	unauthorized := deleteWithBearer(t, server, "/v1/repositories/repo-1", "token-publisher")
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	deleted := deleteWithSession(t, server, "/v1/repositories/repo-1", sessionCookie(t, "admin-1", true))

	require.Equal(t, http.StatusOK, deleted.Code)
	var body struct {
		Data struct {
			Deleted bool `json:"deleted"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(deleted.Body.Bytes(), &body))
	require.True(t, body.Data.Deleted)
	require.Empty(t, svc.store.records["tenant-1"])
}

func TestRepositoryOnboardingMutationsRejectNonAdminSession(t *testing.T) {
	svc := newRepositoryOnboardingService(t)
	svc.public.repos["octo/hello-world"] = git.Repository{FullName: "octo/hello-world", DefaultBranch: "main", Visibility: "public"}
	svc.apps.store["tenant-1"] = &gitapp.GitHubAppRecord{TenantID: "tenant-1", Provider: "github", AppID: 1, InstallationID: 2, PrivateKey: "secret-key", BaseURL: "https://api.github.com"}
	svc.apps.issueSource = &fakeSyncIssueSource{repos: []git.Repository{{FullName: "agentguild/agentguild", DefaultBranch: "main", Visibility: "private"}}}
	require.NoError(t, svc.store.UpsertOnboardedRepository(context.Background(), &gitapp.OnboardedRepositoryRecord{ID: "repo-1", TenantID: "tenant-1", SourceType: gitapp.RepositorySourcePublicGitHub, FullName: "octo/hello-world", DefaultBranch: "main", Visibility: "public"}))
	server := newTestServer(&fakeApplication{}, rest.WithRepositoryOnboardingService(svc.service))
	cookie := sessionCookie(t, "owner-1", false)

	cases := []struct {
		name string
		do   func() *httptest.ResponseRecorder
	}{
		{name: "public add", do: func() *httptest.ResponseRecorder {
			return postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/hello-world"}`, cookie)
		}},
		{name: "app add", do: func() *httptest.ResponseRecorder {
			return postJSONWithSession(t, server, "/v1/repositories/github-app", `{"repo":"agentguild/agentguild"}`, cookie)
		}},
		{name: "delete", do: func() *httptest.ResponseRecorder {
			return deleteWithSession(t, server, "/v1/repositories/repo-1", cookie)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := tc.do()
			require.Equal(t, http.StatusForbidden, res.Code)
			require.JSONEq(t, `{"error":{"code":"FORBIDDEN","message":"admin session is required"}}`, res.Body.String())
		})
	}
}

type repositoryItemBody struct {
	ID            string `json:"id,omitempty"`
	SourceType    string `json:"source_type,omitempty"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Visibility    string `json:"visibility"`
}

type repositoryOnboardingTestService struct {
	service *gitapp.RepositoryOnboardingService
	store   *memoryOnboardedRepositoryStore
	apps    *fakeGitHubAppManager
	public  *fakePublicRepositoryResolver
}

func newRepositoryOnboardingService(t *testing.T) repositoryOnboardingTestService {
	t.Helper()
	var next int
	store := &memoryOnboardedRepositoryStore{records: map[string]map[string]gitapp.OnboardedRepositoryRecord{}}
	apps := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{}}
	public := &fakePublicRepositoryResolver{repos: map[string]git.Repository{}}
	service, err := gitapp.NewRepositoryOnboardingService(store, apps, public, func() string {
		next++
		return "repo-" + string(rune('0'+next))
	})
	require.NoError(t, err)
	return repositoryOnboardingTestService{service: service, store: store, apps: apps, public: public}
}

type memoryOnboardedRepositoryStore struct {
	records map[string]map[string]gitapp.OnboardedRepositoryRecord
}

func (s *memoryOnboardedRepositoryStore) ListOnboardedRepositories(ctx context.Context, tenantID string) ([]gitapp.OnboardedRepositoryRecord, error) {
	records := s.records[tenantID]
	items := make([]gitapp.OnboardedRepositoryRecord, 0, len(records))
	for _, record := range records {
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (s *memoryOnboardedRepositoryStore) UpsertOnboardedRepository(ctx context.Context, record *gitapp.OnboardedRepositoryRecord) error {
	if s.records[record.TenantID] == nil {
		s.records[record.TenantID] = map[string]gitapp.OnboardedRepositoryRecord{}
	}
	copy := *record
	if copy.CreatedAt.IsZero() {
		copy.CreatedAt = fixedRepositoryTime()
	}
	copy.UpdatedAt = fixedRepositoryTime()
	*record = copy
	s.records[copy.TenantID][copy.ID] = copy
	return nil
}

func (s *memoryOnboardedRepositoryStore) DeleteOnboardedRepository(ctx context.Context, tenantID, id string) error {
	delete(s.records[tenantID], id)
	return nil
}

type fakePublicRepositoryResolver struct {
	repos map[string]git.Repository
}

func (r *fakePublicRepositoryResolver) ResolvePublicRepository(ctx context.Context, fullName string) (git.Repository, error) {
	repo, ok := r.repos[fullName]
	if !ok {
		return git.Repository{}, git.ErrGitHubAppNotConfigured
	}
	return repo, nil
}

func fixedRepositoryTime() time.Time {
	return time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
}
