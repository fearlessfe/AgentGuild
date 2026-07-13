package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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

func newManifestServiceWithIDs(t *testing.T, apps application.GitHubAppManager, conversionsBaseURL string, ids ...string) *application.ManifestService {
	t.Helper()
	next := 0
	svc := application.NewManifestService(apps, application.ManifestOptions{
		PublicBaseURL:      "https://guild.example.com",
		StateSecret:        []byte("test-state-secret"),
		ConversionsBaseURL: conversionsBaseURL,
		Now:                func() time.Time { return time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC) },
		NewID: func() string {
			id := ids[next]
			next++
			return id
		},
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
	_, err = svc.VerifyState(state, "tenant-1")
	require.NoError(t, err)

	// Tampered tenant → error.
	_, err = svc.VerifyState(state, "tenant-2")
	require.Error(t, err)
	var derr *domain.Error
	require.ErrorAs(t, err, &derr)
	require.Equal(t, "state", derr.Field)

	// Mangled signature → error.
	parts := strings.Split(state, ".")
	require.Len(t, parts, 2)
	mangled := parts[0] + "." + parts[1] + "x"
	_, err = svc.VerifyState(mangled, "tenant-1")
	require.Error(t, err)

	// Empty state → error.
	_, err = svc.VerifyState("", "tenant-1")
	require.Error(t, err)
}

func TestManifestStateKeepsGitHubAppIDAcrossParallelFlows(t *testing.T) {
	svc := newManifestServiceWithIDs(t, newGitHubAppManager(t), "", "gha-1", "gha-2")

	_, firstState, _, err := svc.BuildManifest("tenant-1")
	require.NoError(t, err)
	_, secondState, _, err := svc.BuildManifest("tenant-1")
	require.NoError(t, err)

	first, err := svc.VerifyState(firstState, "tenant-1")
	require.NoError(t, err)
	second, err := svc.VerifyState(secondState, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, "gha-1", first.GitHubAppID)
	require.Equal(t, "gha-2", second.GitHubAppID)
}

func TestManifestBuildInstallURLIncludesVerifiableState(t *testing.T) {
	svc := newManifestService(t, newGitHubAppManager(t), "")

	installURL, err := svc.BuildInstallURL("tenant-1", "gha-2", "agentguild-test")

	require.NoError(t, err)
	require.Contains(t, installURL, "https://github.com/apps/agentguild-test/installations/new?state=")
	state := strings.TrimPrefix(installURL, "https://github.com/apps/agentguild-test/installations/new?state=")
	require.NotEmpty(t, state)
	decoded, err := svc.VerifyState(state, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, "gha-2", decoded.GitHubAppID)
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

	svc := newManifestServiceWithIDs(t, manager, server.URL, "gha-manifest")
	_, state, _, err := svc.BuildManifest("tenant-1")
	require.NoError(t, err)
	decoded, err := svc.VerifyState(state, "tenant-1")
	require.NoError(t, err)

	view, err := svc.ExchangeCode(ctx, "tenant-1", decoded.GitHubAppID, code)
	require.NoError(t, err)

	require.Equal(t, "/app-manifests/"+code+"/conversions", gotPath)
	require.Equal(t, "application/vnd.github+json", gotAccept)

	// Persisted record carries all credentials.
	record, err := repo.GetDefault(ctx, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, int64(123), record.AppID)
	require.Equal(t, "gha-manifest", record.ID)
	require.Equal(t, int64(0), record.InstallationID)
	require.Equal(t, server.URL, record.BaseURL)
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

func TestManifestParallelStatesPersistCredentialsByGitHubAppID(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/app-manifests/code-1/conversions":
			_, _ = w.Write([]byte(`{"id":101,"slug":"first-app","pem":"first-private-key","client_id":"first-client","client_secret":"first-client-secret","webhook_secret":"first-webhook-secret"}`))
		case "/app-manifests/code-2/conversions":
			_, _ = w.Write([]byte(`{"id":202,"slug":"second-app","pem":"second-private-key","client_id":"second-client","client_secret":"second-client-secret","webhook_secret":"second-webhook-secret"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	repo := newMemoryGitHubAppRepo()
	manager, err := application.NewGitHubAppManager(repo)
	require.NoError(t, err)
	svc := newManifestServiceWithIDs(t, manager, server.URL, "gha-1", "gha-2")

	_, firstState, _, err := svc.BuildManifest("tenant-1")
	require.NoError(t, err)
	_, secondState, _, err := svc.BuildManifest("tenant-1")
	require.NoError(t, err)
	first, err := svc.VerifyState(firstState, "tenant-1")
	require.NoError(t, err)
	second, err := svc.VerifyState(secondState, "tenant-1")
	require.NoError(t, err)

	_, err = svc.ExchangeCode(ctx, "tenant-1", first.GitHubAppID, "code-1")
	require.NoError(t, err)
	_, err = svc.ExchangeCode(ctx, "tenant-1", second.GitHubAppID, "code-2")
	require.NoError(t, err)

	firstRecord, err := repo.GetByID(ctx, "tenant-1", "gha-1")
	require.NoError(t, err)
	secondRecord, err := repo.GetByID(ctx, "tenant-1", "gha-2")
	require.NoError(t, err)
	require.Equal(t, int64(101), firstRecord.AppID)
	require.Equal(t, int64(202), secondRecord.AppID)
	require.Equal(t, "first-app", firstRecord.AppSlug)
	require.Equal(t, "second-app", secondRecord.AppSlug)
	require.Equal(t, server.URL, firstRecord.BaseURL)
	require.Equal(t, server.URL, secondRecord.BaseURL)
	require.Equal(t, credentialDigest("first-private-key", "first-client", "first-client-secret", "first-webhook-secret"), credentialDigest(firstRecord.PrivateKey, firstRecord.ClientID, firstRecord.ClientSecret, firstRecord.WebhookSecret))
	require.Equal(t, credentialDigest("second-private-key", "second-client", "second-client-secret", "second-webhook-secret"), credentialDigest(secondRecord.PrivateKey, secondRecord.ClientID, secondRecord.ClientSecret, secondRecord.WebhookSecret))
}

func credentialDigest(privateKey, clientID, clientSecret, webhookSecret string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(privateKey+"\x00"+clientID+"\x00"+clientSecret+"\x00"+webhookSecret)))
}

func TestManifestInstallFetchesAccountWithoutExposingCredentials(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/app/installations/456", r.URL.Path)
		require.NotEmpty(t, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"account":{"login":"acme-corp"}}`))
	}))
	defer server.Close()

	repo := newMemoryGitHubAppRepo()
	manager, err := application.NewGitHubAppManager(repo)
	require.NoError(t, err)
	require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{
		ID: "gha-2", TenantID: "tenant-1", AppID: 42,
		PrivateKey: generateRSAPrivateKeyPEM(t), BaseURL: server.URL, AppSlug: "agentguild-test",
	}))
	svc := newManifestService(t, manager, "")

	view, err := svc.Install(ctx, "tenant-1", "gha-2", 456)
	require.NoError(t, err)
	require.Equal(t, "gha-2", view.ID)
	require.Equal(t, int64(456), view.InstallationID)
	require.Equal(t, "acme-corp", view.InstallationAccountLogin)

	encoded, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "PRIVATE KEY")
	require.NotContains(t, string(encoded), "private_key")
}
