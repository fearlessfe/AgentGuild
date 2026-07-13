package rest_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	"agentguild.dev/agentguild/backend/internal/git/gittest"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func TestGitHubAppsListReturnsOnlyTenantAppsWithoutSecrets(t *testing.T) {
	fixed := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	manager := &fakeGitHubAppManager{
		store: map[string]*gitapp.GitHubAppRecord{
			"tenant-1": {ID: "gha-default", TenantID: "tenant-1", PrivateKey: "tenant-one-secret", IsDefault: true},
			"tenant-2": {ID: "gha-foreign", TenantID: "tenant-2", PrivateKey: "foreign-secret", IsDefault: true},
		},
		listViews: map[string][]gitapp.GitHubAppView{
			"tenant-1": {
				{ID: "gha-default", TenantID: "tenant-1", AppSlug: "alpha", InstallationAccountLogin: "acme", IsDefault: true, Configured: true, CreatedAt: fixed, UpdatedAt: fixed},
				{ID: "gha-second", TenantID: "tenant-1", AppSlug: "beta", IsDefault: false, Configured: true, CreatedAt: fixed, UpdatedAt: fixed},
			},
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := getWithSession(t, server, "/v1/github-apps", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Data struct {
			Items []gitapp.GitHubAppView `json:"items"`
		} `json:"data"`
		Meta map[string]any `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, []string{"gha-default", "gha-second"}, []string{body.Data.Items[0].ID, body.Data.Items[1].ID})
	require.True(t, body.Data.Items[0].IsDefault)
	require.False(t, body.Data.Items[1].IsDefault)
	require.Equal(t, "acme", body.Data.Items[0].InstallationAccountLogin)
	require.NotEmpty(t, body.Meta["server_time"])
	require.NotContains(t, res.Body.String(), "tenant-one-secret")
	require.NotContains(t, res.Body.String(), "foreign-secret")
}

func TestGitHubAppsGetHidesForeignTenantApp(t *testing.T) {
	manager := &fakeGitHubAppManager{
		store: map[string]*gitapp.GitHubAppRecord{},
		byID: map[string]*gitapp.GitHubAppRecord{
			"tenant-2/gha-foreign": {ID: "gha-foreign", TenantID: "tenant-2", AppSlug: "secret-app", PrivateKey: "foreign-secret"},
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := getWithSession(t, server, "/v1/github-apps/gha-foreign", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusNotFound, res.Code)
	require.NotContains(t, res.Body.String(), "gha-foreign")
	require.NotContains(t, res.Body.String(), "secret-app")
	require.NotContains(t, res.Body.String(), "foreign-secret")
}

func TestGitHubAppsGetReturnsTenantScopedEnvelope(t *testing.T) {
	manager := &fakeGitHubAppManager{
		store: map[string]*gitapp.GitHubAppRecord{},
		byID: map[string]*gitapp.GitHubAppRecord{
			"tenant-1/gha-2": {ID: "gha-2", TenantID: "tenant-1", AppSlug: "beta", InstallationAccountLogin: "acme", IsDefault: false, PrivateKey: "secret"},
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := getWithSession(t, server, "/v1/github-apps/gha-2", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Data gitapp.GitHubAppView `json:"data"`
		Meta map[string]any       `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, "gha-2", body.Data.ID)
	require.Equal(t, "acme", body.Data.InstallationAccountLogin)
	require.False(t, body.Data.IsDefault)
	require.NotEmpty(t, body.Meta["server_time"])
	require.NotContains(t, res.Body.String(), "secret")
}

func TestGitHubAppsDeleteBoundAppReturnsConflict(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{}, deleteErr: git.ErrGitHubAppInUse}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := deleteWithSession(t, server, "/v1/github-apps/gha-1", sessionCookie(t, "admin-1", true))

	require.Equal(t, http.StatusConflict, res.Code)
	require.Contains(t, res.Body.String(), "STATE_CONFLICT")
	require.Contains(t, manager.calls, githubAppCall{method: "DeleteByID", tenantID: "tenant-1", payload: "gha-1"})
}

func TestGitHubAppsDeleteRequiresAdmin(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{}}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := deleteWithSession(t, server, "/v1/github-apps/gha-1", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusForbidden, res.Code)
	require.Empty(t, manager.calls)
}

func TestGitHubAppSingularMutationsRequireAdmin(t *testing.T) {
	manager := &fakeGitHubAppManager{
		store: map[string]*gitapp.GitHubAppRecord{
			"tenant-1": {ID: "gha-default", TenantID: "tenant-1", IsDefault: true},
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))
	cookie := sessionCookie(t, "owner-1", false)

	created := postJSONWithSession(t, server, "/v1/github-app", `{"app_id":1,"private_key":"secret"}`, cookie)
	deleted := deleteWithSession(t, server, "/v1/github-app", cookie)

	require.Equal(t, http.StatusForbidden, created.Code)
	require.Equal(t, http.StatusForbidden, deleted.Code)
	require.Empty(t, manager.calls)
}

func TestGitHubAppsTestUsesSelectedApp(t *testing.T) {
	manager := &fakeGitHubAppManager{
		store: map[string]*gitapp.GitHubAppRecord{
			"tenant-1": {ID: "gha-2", TenantID: "tenant-1"},
		},
		issueSource: &gittest.StubIssueSource{Repos: []git.Repository{
			{FullName: "acme/api"},
			{FullName: "acme/web"},
		}},
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := postJSONWithSession(t, server, "/v1/github-apps/gha-2:test", `{}`, sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Data struct {
			OK        bool   `json:"ok"`
			RepoCount int    `json:"repo_count"`
			Error     string `json:"error"`
		} `json:"data"`
		Meta map[string]any `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.True(t, body.Data.OK)
	require.Equal(t, 2, body.Data.RepoCount)
	require.Empty(t, body.Data.Error)
	require.NotEmpty(t, body.Meta["server_time"])
	require.Contains(t, manager.calls, githubAppCall{method: "IssueSourceForApp", tenantID: "tenant-1", payload: "gha-2"})
	require.NotContains(t, manager.calls, githubAppCall{method: "IssueSource", tenantID: "tenant-1"})
}

func TestGitHubAppsTestHidesForeignTenantApp(t *testing.T) {
	manager := &fakeGitHubAppManager{
		store: map[string]*gitapp.GitHubAppRecord{
			"tenant-2": {ID: "gha-foreign", TenantID: "tenant-2", PrivateKey: "foreign-secret"},
		},
		issueSource: &gittest.StubIssueSource{Repos: []git.Repository{{FullName: "secret/private"}}},
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := postJSONWithSession(t, server, "/v1/github-apps/gha-foreign:test", `{}`, sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusNotFound, res.Code)
	require.NotContains(t, res.Body.String(), "secret/private")
	require.NotContains(t, res.Body.String(), "foreign-secret")
}

func TestGitHubAppSingularGetUsesOnlyDefaultApp(t *testing.T) {
	manager := &fakeGitHubAppManager{
		store: map[string]*gitapp.GitHubAppRecord{
			"tenant-1": {ID: "gha-default", TenantID: "tenant-1", IsDefault: true},
		},
		listViews: map[string][]gitapp.GitHubAppView{
			"tenant-1": {{ID: "gha-other", TenantID: "tenant-1", IsDefault: false, Configured: true}},
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := getWithSession(t, server, "/v1/github-app", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	var view gitapp.GitHubAppView
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &view))
	require.Equal(t, "gha-default", view.ID)
	require.True(t, view.IsDefault)
	require.Equal(t, []githubAppCall{{method: "Get", tenantID: "tenant-1"}}, manager.calls)
}
