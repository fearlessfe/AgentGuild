package github_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git/github"
	"github.com/stretchr/testify/require"
)

func TestInstallationAccountReturnsOwnerLogin(t *testing.T) {
	key, privateKey := newRSAKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/app/installations/42", r.URL.Path)
		require.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		authorization := r.Header.Get("Authorization")
		require.True(t, strings.HasPrefix(authorization, "Bearer "))
		verifyGitHubAppJWT(t, strings.TrimPrefix(authorization, "Bearer "), key, time.Now())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"account":{"login":"acme-corp"}}`))
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	client, err := github.NewInstallationClient(github.InstallationClientConfig{
		BaseURL: server.URL, AppID: 42, PrivateKey: privateKey, HTTPClient: server.Client(),
		AllowedHosts: []string{serverURL.Hostname()}, AllowInsecureHTTP: true,
	})
	require.NoError(t, err)
	login, err := client.InstallationAccount(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "acme-corp", login)
}
