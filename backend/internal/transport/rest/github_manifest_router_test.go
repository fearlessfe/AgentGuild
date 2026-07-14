package rest_test

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	"agentguild.dev/agentguild/backend/internal/git/gittest"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func newManifestService(manager gitapp.GitHubAppManager) *gitapp.ManifestService {
	service, err := gitapp.NewManifestService(manager, gitapp.ManifestOptions{
		PublicBaseURL: "https://agentguild.example",
		StateSecret:   []byte("test-state-secret"),
	})
	if err != nil {
		panic(err)
	}
	return service
}

func TestGitHubManifest_BuildForm(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{}}
	svc := newManifestService(manager)
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager), rest.WithGitHubManifest(svc))

	res := getWithSession(t, server, "/oauth/github/app/manifest", sessionCookie(t, "admin-1", true))

	body := res.Body.String()
	switch res.Code {
	case http.StatusOK:
		require.Contains(t, body, "github.com/settings/apps/new")
		require.Contains(t, body, "manifest")
		require.Contains(t, body, "state")
	case http.StatusFound:
		loc := res.Header().Get("Location")
		require.Contains(t, loc, "github.com")
		require.Contains(t, loc, "state")
	default:
		t.Fatalf("unexpected status %d: %s", res.Code, body)
	}
	// state must be present in the response somewhere.
	require.Contains(t, body+res.Header().Get("Location"), "state")
}

func TestGitHubManifest_BuildFormIgnoresConflictingForwardedHeaders(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{}}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager), rest.WithGitHubManifest(newManifestService(manager)))
	req := httptest.NewRequest(http.MethodGet, "/oauth/github/app/manifest", nil)
	req.Host = "attacker.example"
	req.Header.Set("X-Forwarded-Host", "forwarded-attacker.example")
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set("X-Forwarded-Port", "8080")
	req.AddCookie(sessionCookie(t, "admin-1", true))
	res := httptest.NewRecorder()

	server.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	match := regexp.MustCompile(`name="manifest" value="([^"]+)"`).FindStringSubmatch(res.Body.String())
	require.Len(t, match, 2)
	var manifest struct {
		URL         string `json:"url"`
		RedirectURL string `json:"redirect_url"`
		SetupURL    string `json:"setup_url"`
	}
	require.NoError(t, json.Unmarshal([]byte(html.UnescapeString(match[1])), &manifest))
	require.Equal(t, "https://agentguild.example", manifest.URL)
	require.Equal(t, "https://agentguild.example/oauth/github/app/callback", manifest.RedirectURL)
	require.Equal(t, "https://agentguild.example/oauth/github/app/installed", manifest.SetupURL)
}

func TestGitHubManifest_CallbackBadState(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{}}
	svc := newManifestService(manager)
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager), rest.WithGitHubManifest(svc))

	res := getWithSession(t, server, "/oauth/github/app/callback?state=bad&code=x", sessionCookie(t, "admin-1", true))

	require.Equal(t, http.StatusBadRequest, res.Code)
	for _, call := range manager.calls {
		require.NotEqual(t, "Upsert", call.method, "must not persist on bad state")
	}
}

func TestGitHubAppInstallRedirectUsesScopedApp(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{
		"tenant-1": {
			ID:             "gha-2",
			TenantID:       "tenant-1",
			Provider:       "github",
			AppID:          123,
			PrivateKey:     "PRIVATE KEY",
			BaseURL:        "https://api.github.com",
			AppSlug:        "agentguild-test",
			InstallationID: 0,
		},
	}}
	svc := newManifestService(manager)
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager), rest.WithGitHubManifest(svc))

	res := getWithSession(t, server, "/oauth/github/app/install?github_app_id=gha-2", sessionCookie(t, "admin-1", true))

	require.Equal(t, http.StatusFound, res.Code)
	location := res.Header().Get("Location")
	require.Contains(t, location, "https://github.com/apps/agentguild-test/installations/new?state=")
	state := strings.TrimPrefix(location, "https://github.com/apps/agentguild-test/installations/new?state=")
	decoded, err := svc.VerifyState(state, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, "gha-2", decoded.GitHubAppID)
}

func TestGitHubAppInstallLegacyRedirectUsesDeterministicDefault(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{
		"tenant-1": {
			ID: "gha-default", TenantID: "tenant-1", AppID: 123, PrivateKey: "PRIVATE KEY",
			BaseURL: "https://api.github.com", AppSlug: "agentguild-default", IsDefault: true,
		},
	}}
	svc := newManifestService(manager)
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager), rest.WithGitHubManifest(svc))

	res := getWithSession(t, server, "/oauth/github/app/install", sessionCookie(t, "admin-1", true))

	require.Equal(t, http.StatusFound, res.Code)
	state := strings.TrimPrefix(res.Header().Get("Location"), "https://github.com/apps/agentguild-default/installations/new?state=")
	decoded, err := svc.VerifyState(state, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, "gha-default", decoded.GitHubAppID)
}

func TestGitHubAppInstalledUsesSignedAppAndPreservesExistingCredentials(t *testing.T) {
	manager := &fakeGitHubAppManager{installationAccount: "acme-corp", store: map[string]*gitapp.GitHubAppRecord{
		"tenant-1": {
			ID:             "gha-2",
			TenantID:       "tenant-1",
			Provider:       "github",
			AppID:          123,
			PrivateKey:     "PRIVATE KEY",
			BaseURL:        "https://api.github.com",
			WebhookSecret:  "webhook-secret",
			ClientID:       "client-id",
			ClientSecret:   "client-secret",
			AppSlug:        "agentguild-test",
			InstallationID: 0,
		},
	}}
	svc := newManifestService(manager)
	installURL, err := svc.BuildInstallURL("tenant-1", "gha-2", "agentguild-test")
	require.NoError(t, err)
	state := strings.TrimPrefix(installURL, "https://github.com/apps/agentguild-test/installations/new?state=")
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager), rest.WithGitHubManifest(svc))

	res := getWithSession(t, server, "/oauth/github/app/installed?installation_id=456&github_app_id=untrusted&state="+state, sessionCookie(t, "admin-1", true))

	require.Equal(t, http.StatusFound, res.Code)
	require.Equal(t, "/git-integration?installed=1", res.Header().Get("Location"))
	record := manager.store["tenant-1"]
	require.Equal(t, int64(456), record.InstallationID)
	require.Equal(t, "PRIVATE KEY", record.PrivateKey)
	require.Equal(t, "agentguild-test", record.AppSlug)
	require.Equal(t, "acme-corp", record.InstallationAccountLogin)
	require.Contains(t, manager.calls, githubAppCall{method: "InstallationAccount", tenantID: "tenant-1", payload: "gha-2"})
	require.Contains(t, manager.calls, githubAppCall{method: "InstallByID", tenantID: "tenant-1", payload: "gha-2"})
}

func TestGitHubManifest_TestConnectionOK(t *testing.T) {
	manager := &fakeGitHubAppManager{
		store: map[string]*gitapp.GitHubAppRecord{},
		issueSource: &gittest.StubIssueSource{
			Repos: []git.Repository{{FullName: "acme/widgets", DefaultBranch: "main", Visibility: "private"}},
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := postJSONWithSession(t, server, "/v1/github-app:test", `{}`, sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Data struct {
			OK        bool   `json:"ok"`
			RepoCount int    `json:"repo_count"`
			Error     string `json:"error"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.True(t, body.Data.OK)
	require.Equal(t, 1, body.Data.RepoCount)
	require.Empty(t, body.Data.Error)
}

func TestGitHubManifest_TestConnectionFailure(t *testing.T) {
	manager := &fakeGitHubAppManager{
		store:          map[string]*gitapp.GitHubAppRecord{},
		issueSourceErr: git.ErrGitHubAppNotConfigured,
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := postJSONWithSession(t, server, "/v1/github-app:test", `{}`, sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Data struct {
			OK        bool   `json:"ok"`
			RepoCount int    `json:"repo_count"`
			Error     string `json:"error"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.False(t, body.Data.OK)
	require.NotEmpty(t, body.Data.Error)
	require.NotContains(t, res.Body.String(), "PRIVATE KEY")
	require.NotContains(t, strings.ToLower(res.Body.String()), "private_key")
}

func TestGitHubManifest_TestSourceError(t *testing.T) {
	manager := &fakeGitHubAppManager{
		store: map[string]*gitapp.GitHubAppRecord{},
		issueSource: &gittest.StubIssueSource{
			Err: git.ErrGitHubAppNotConfigured,
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := postJSONWithSession(t, server, "/v1/github-app:test", `{}`, sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Data struct {
			OK        bool   `json:"ok"`
			RepoCount int    `json:"repo_count"`
			Error     string `json:"error"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.False(t, body.Data.OK)
	require.NotEmpty(t, body.Data.Error)
}

func TestGitHubManifest_RequiresSession(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{}}
	svc := newManifestService(manager)
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager), rest.WithGitHubManifest(svc))

	t.Run("manifest", func(t *testing.T) {
		res := get(t, server, "/oauth/github/app/manifest", "token-publisher")
		require.Equal(t, http.StatusUnauthorized, res.Code)
	})

	t.Run("test", func(t *testing.T) {
		res := postJSON(t, server, "/v1/github-app:test", `{}`, "token-publisher")
		require.Equal(t, http.StatusUnauthorized, res.Code)
	})
}

func TestGitHubManifestConfigurationFlowsRequireAdmin(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{
		"tenant-1": {ID: "gha-default", TenantID: "tenant-1", AppSlug: "agentguild-test", IsDefault: true},
	}}
	svc := newManifestService(manager)
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager), rest.WithGitHubManifest(svc))
	cookie := sessionCookie(t, "owner-1", false)

	manifest := getWithSession(t, server, "/oauth/github/app/manifest", cookie)
	install := getWithSession(t, server, "/oauth/github/app/install", cookie)
	callback := getWithSession(t, server, "/oauth/github/app/callback?state=bad&code=x", cookie)
	installed := getWithSession(t, server, "/oauth/github/app/installed?installation_id=1&state=bad", cookie)

	for _, res := range []*httptest.ResponseRecorder{manifest, install, callback, installed} {
		require.Equal(t, http.StatusForbidden, res.Code)
		require.Contains(t, res.Body.String(), "admin session is required")
	}
}
