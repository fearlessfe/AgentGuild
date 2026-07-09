package rest_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	"agentguild.dev/agentguild/backend/internal/git/gittest"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func newManifestService(manager gitapp.GitHubAppManager) *gitapp.ManifestService {
	return gitapp.NewManifestService(manager, gitapp.ManifestOptions{
		PublicBaseURL: "https://agentguild.example",
		StateSecret:   []byte("test-state-secret"),
	})
}

func TestGitHubManifest_BuildForm(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{}}
	svc := newManifestService(manager)
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager), rest.WithGitHubManifest(svc))

	res := getWithSession(t, server, "/oauth/github/app/manifest", sessionCookie(t, "owner-1", false))

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

func TestGitHubManifest_CallbackBadState(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{}}
	svc := newManifestService(manager)
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager), rest.WithGitHubManifest(svc))

	res := getWithSession(t, server, "/oauth/github/app/callback?state=bad&code=x", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusBadRequest, res.Code)
	for _, call := range manager.calls {
		require.NotEqual(t, "Upsert", call.method, "must not persist on bad state")
	}
}

func TestGitHubManifest_InstallRedirectUsesAppSlug(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{
		"tenant-1": {
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

	res := getWithSession(t, server, "/oauth/github/app/install", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusFound, res.Code)
	location := res.Header().Get("Location")
	require.Contains(t, location, "https://github.com/apps/agentguild-test/installations/new?state=")
	state := strings.TrimPrefix(location, "https://github.com/apps/agentguild-test/installations/new?state=")
	require.NoError(t, svc.VerifyState(state, "tenant-1"))
}

func TestGitHubManifest_InstalledPreservesExistingAppCredentials(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{
		"tenant-1": {
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
	_, state, _, err := svc.BuildManifest("tenant-1")
	require.NoError(t, err)
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager), rest.WithGitHubManifest(svc))

	res := getWithSession(t, server, "/oauth/github/app/installed?installation_id=456&state="+state, sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusFound, res.Code)
	require.Equal(t, "/git-integration?installed=1", res.Header().Get("Location"))
	record := manager.store["tenant-1"]
	require.Equal(t, int64(456), record.InstallationID)
	require.Equal(t, "PRIVATE KEY", record.PrivateKey)
	require.Equal(t, "agentguild-test", record.AppSlug)
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
