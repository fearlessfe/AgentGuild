package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/config"
	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestRepeatContinuesAfterPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	calls := 0

	go func() {
		repeat(ctx, 5*time.Millisecond, "test", func(context.Context) error {
			mu.Lock()
			calls++
			call := calls
			mu.Unlock()
			if call == 1 {
				panic("intentional test panic")
			}
			if call >= 3 {
				cancel()
			}
			return nil
		})
	}()

	// Wait for repeat to process several iterations after the panic.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	got := calls
	mu.Unlock()
	if got < 3 {
		t.Fatalf("repeat did not continue after panic: calls=%d", got)
	}
}

func TestGitHubManifestOptionsUsesConfiguredAPIBaseURL(t *testing.T) {
	cfg := config.Config{
		GitHubAppPublicBaseURL:       "https://agentguild.example",
		GitHubAppManifestStateSecret: "state-secret",
	}
	cfg.GitHub.BaseURL = "https://ghe.example/api/v3/"

	opts := githubManifestOptions(cfg)

	require.Equal(t, cfg.GitHubAppPublicBaseURL, opts.PublicBaseURL)
	require.Equal(t, cfg.GitHubAppManifestStateSecret, string(opts.StateSecret))
	require.Equal(t, cfg.GitHub.BaseURL, opts.ConversionsBaseURL)
}

func TestListValidationWorkerTenantsIncludesTerminalUnsyncedJobs(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	cases := []struct {
		tenant string
		status string
		synced bool
	}{
		{tenant: "tenant-failed", status: "failed", synced: false},
		{tenant: "tenant-pending", status: "pending", synced: false},
		{tenant: "tenant-succeeded", status: "succeeded", synced: false},
		{tenant: "tenant-synced", status: "succeeded", synced: true},
		{tenant: "tenant-cancelled", status: "cancelled", synced: false},
	}
	for _, tc := range cases {
		submissionID := "submission-" + tc.tenant
		_, err := db.Exec(ctx, `
			INSERT INTO submissions (
				tenant_id, id, task_id, execution_id, repo, branch, commit_sha,
				base_commit_sha, summary, diff_fingerprint, status, created_at, updated_at
			) VALUES ($1, $2, 'task-1', 'execution-1', 'owner/repo', 'agentguild/execution-1',
			          'head-sha', 'base-sha', 'summary', 'fingerprint', 'pending_verification',
			          clock_timestamp(), clock_timestamp())`, tc.tenant, submissionID)
		require.NoError(t, err)
		_, err = db.Exec(ctx, `
			INSERT INTO validation_jobs (
				tenant_id, id, submission_id, execution_id, repo, branch, commit_sha,
				status, attempt, config_version, execution_state_synced, created_at, updated_at
			) VALUES ($1, $2, $3, 'execution-1', 'owner/repo', 'agentguild/execution-1',
			          'head-sha', $4, 0, 'v1', $5, clock_timestamp(), clock_timestamp())`,
			tc.tenant, "job-"+tc.tenant, submissionID, tc.status, tc.synced)
		require.NoError(t, err)
	}

	tenants, err := listValidationWorkerTenants(ctx, db)

	require.NoError(t, err)
	require.Equal(t, []string{"tenant-failed", "tenant-pending", "tenant-succeeded"}, tenants)
}

func TestWaitWorkersReturnsTrueWhenWorkersStop(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		wg.Done()
	}()

	if !waitWorkers(&wg, time.Second) {
		t.Fatal("waitWorkers returned false for workers that stopped")
	}
}

func TestWaitWorkersReturnsFalseOnTimeout(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Done()

	if waitWorkers(&wg, 20*time.Millisecond) {
		t.Fatal("waitWorkers returned true despite timeout")
	}
}

func TestLoadAgentRSAPrivateKeyFromPEM(t *testing.T) {
	privateKey := newRSAPrivateKeyForTest(t)
	cfg := config.Config{AgentRSAPrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))}

	got, err := loadAgentRSAPrivateKey(cfg)

	require.NoError(t, err)
	require.Equal(t, privateKey.N, got.N)
}

func TestLoadAgentRSAPrivateKeyFromPath(t *testing.T) {
	privateKey := newRSAPrivateKeyForTest(t)
	path := filepath.Join(t.TempDir(), "agent-private-key.pem")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}), 0o600))
	cfg := config.Config{AgentRSAPrivateKeyPath: path}

	got, err := loadAgentRSAPrivateKey(cfg)

	require.NoError(t, err)
	require.Equal(t, privateKey.N, got.N)
}

func TestLoadAgentRSAPrivateKeyRejectsInvalidPEM(t *testing.T) {
	_, err := loadAgentRSAPrivateKey(config.Config{AgentRSAPrivateKeyPEM: "not a pem"})

	require.Error(t, err)
}

func TestBuildTokenVerifierLocalOnly(t *testing.T) {
	testKey := newRSAPrivateKeyForTest(t)

	verifier := buildTokenVerifier(config.Config{OAuthIssuer: "http://agentguild.local", OAuthAudience: "agentguild"}, &testKey.PublicKey)

	require.NotNil(t, verifier)
}

func TestBuildTokenVerifierIncludesExternalJWKSWhenConfigured(t *testing.T) {
	testKey := newRSAPrivateKeyForTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	verifier := buildTokenVerifier(config.Config{OAuthIssuer: "issuer", OAuthAudience: "audience", OAuthJWKSURL: server.URL}, &testKey.PublicKey)

	require.NotNil(t, verifier)
}

func TestPublicRepositoryResolverMapsGitHubRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/rust-lang/rust", r.URL.Path)
		w.Header().Set("Retry-After", "42")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer server.Close()
	resolver := newPublicRepositoryResolver(server.URL)

	_, err := resolver.ResolvePublicRepository(context.Background(), "rust-lang/rust")

	require.Error(t, err)
	require.Equal(t, "rate_limited", domain.CodeOf(err))
	require.Equal(t, 42*time.Second, domain.RetryAfterOf(err))
}

func newRSAPrivateKeyForTest(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return privateKey
}
