package application_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git/application"
	"github.com/stretchr/testify/require"
)

func newManifestService(t *testing.T, apps application.GitHubAppManager, conversionsBaseURL string) *application.ManifestService {
	t.Helper()
	svc := application.NewManifestService(apps, application.ManifestOptions{
		PublicBaseURL:      "https://guild.example.com",
		StateSecret:        []byte("test-state-secret"),
		ConversionsBaseURL: conversionsBaseURL,
		Now:                func() time.Time { return time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC) },
	})
	require.NotNil(t, svc)
	return svc
}

func TestManifestBuildContainsPermissionsAndCallbacks(t *testing.T) {
	svc := newManifestService(t, newGitHubAppManager(t), "")

	manifestJSON, state, redirectURL, err := svc.BuildManifest("tenant-1")
	require.NoError(t, err)
	require.NotEmpty(t, state)
	require.Equal(t, "https://github.com/settings/apps/new", redirectURL)

	require.Contains(t, manifestJSON, `"contents":"read"`)
	require.Contains(t, manifestJSON, `"issues":"write"`)
	require.Contains(t, manifestJSON, `"checks":"read"`)
	require.Contains(t, manifestJSON, `"metadata":"read"`)
	require.Contains(t, manifestJSON, "https://guild.example.com/oauth/github/app/callback")
	require.Contains(t, manifestJSON, "https://guild.example.com/oauth/github/app/installed")

	// Sanity: the manifest is valid JSON with the expected structure.
	var manifest struct {
		Name               string            `json:"name"`
		URL                string            `json:"url"`
		RedirectURL        string            `json:"redirect_url"`
		SetupURL           string            `json:"setup_url"`
		Public             bool              `json:"public"`
		DefaultPermissions map[string]string `json:"default_permissions"`
	}
	require.NoError(t, json.Unmarshal([]byte(manifestJSON), &manifest))
	require.NotEmpty(t, manifest.Name)
	require.False(t, manifest.Public)
	require.Equal(t, "https://guild.example.com", manifest.URL)
	require.Equal(t, "https://guild.example.com/oauth/github/app/callback", manifest.RedirectURL)
	require.Equal(t, "https://guild.example.com/oauth/github/app/installed", manifest.SetupURL)
	require.Equal(t, "read", manifest.DefaultPermissions["contents"])
	require.Equal(t, "write", manifest.DefaultPermissions["issues"])
	require.Equal(t, "read", manifest.DefaultPermissions["checks"])
	require.Equal(t, "read", manifest.DefaultPermissions["metadata"])
}

func TestManifestVerifyStateRejectsTampered(t *testing.T) {
	svc := newManifestService(t, newGitHubAppManager(t), "")

	_, state, _, err := svc.BuildManifest("tenant-1")
	require.NoError(t, err)

	// Valid state + matching tenant + fresh → nil.
	require.NoError(t, svc.VerifyState(state, "tenant-1"))

	// Tampered tenant → error.
	err = svc.VerifyState(state, "tenant-2")
	require.Error(t, err)
	var derr *domain.Error
	require.ErrorAs(t, err, &derr)
	require.Equal(t, "state", derr.Field)

	// Mangled signature → error.
	parts := strings.Split(state, ".")
	require.Len(t, parts, 2)
	mangled := parts[0] + "." + parts[1] + "x"
	err = svc.VerifyState(mangled, "tenant-1")
	require.Error(t, err)

	// Empty state → error.
	require.Error(t, svc.VerifyState("", "tenant-1"))
}

func TestManifestBuildInstallURLIncludesVerifiableState(t *testing.T) {
	svc := newManifestService(t, newGitHubAppManager(t), "")

	installURL, err := svc.BuildInstallURL("tenant-1", "agentguild-test")

	require.NoError(t, err)
	require.Contains(t, installURL, "https://github.com/apps/agentguild-test/installations/new?state=")
	state := strings.TrimPrefix(installURL, "https://github.com/apps/agentguild-test/installations/new?state=")
	require.NotEmpty(t, state)
	require.NoError(t, svc.VerifyState(state, "tenant-1"))
}

func TestManifestExchangeCodePersistsCredentials(t *testing.T) {
	ctx := context.Background()

	const code = "abc123"
	var gotPath string
	var gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":123,"slug":"my-app","pem":"-----BEGIN RSA PRIVATE KEY-----\nfake\n-----END RSA PRIVATE KEY-----","client_id":"cid","client_secret":"cs","webhook_secret":"ws"}`))
	}))
	defer server.Close()

	repo := newMemoryGitHubAppRepo()
	manager, err := application.NewGitHubAppManager(repo)
	require.NoError(t, err)

	svc := newManifestService(t, manager, server.URL)

	view, err := svc.ExchangeCode(ctx, "tenant-1", code)
	require.NoError(t, err)

	require.Equal(t, "/app-manifests/"+code+"/conversions", gotPath)
	require.Equal(t, "application/vnd.github+json", gotAccept)

	// Persisted record carries all credentials.
	record, err := repo.GetDefault(ctx, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, int64(123), record.AppID)
	require.Equal(t, int64(0), record.InstallationID)
	require.Equal(t, "my-app", record.AppSlug)
	require.Contains(t, record.PrivateKey, "BEGIN RSA PRIVATE KEY")
	require.Equal(t, "cid", record.ClientID)
	require.Equal(t, "cs", record.ClientSecret)
	require.Equal(t, "ws", record.WebhookSecret)

	// Returned view is safe: exposes slug, no private key field.
	require.Equal(t, "my-app", view.AppSlug)
	require.Equal(t, int64(123), view.AppID)
	require.True(t, view.Configured)

	// The view type must not leak a private key: marshal and confirm absence.
	blob, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(blob), "BEGIN RSA PRIVATE KEY")
	require.NotContains(t, string(blob), "private_key")
}
