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
