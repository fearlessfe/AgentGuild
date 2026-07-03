package rest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func TestAgentActivateDoesNotRequireBearer(t *testing.T) {
	expiresAt := time.Date(2026, 7, 3, 10, 15, 0, 0, time.UTC)
	app := &fakeIdentityApplication{
		activate: identityapp.Envelope[identityapp.AccessTokenView]{
			Data: identityapp.AccessTokenView{
				Token:          "access-token-1",
				TokenType:      "Bearer",
				ExpiresAt:      expiresAt,
				AgentID:        "agent-1",
				AgentVersionID: "agent-1.v1",
				Scopes:         []string{"tasks:read"},
				RepoScope:      []string{"acme/*"},
			},
		},
	}
	server := newAgentSelfTestServer(app)
	body := `{"activation_token":"activation-token-1","runtime":"codex","model":"gpt-5","capabilities":["shell"],"config_fingerprint":"fp-1"}`

	res := postJSONNoAuth(t, server, "/v1/agents/me:activate", body)

	require.Equal(t, http.StatusOK, res.Code)
	require.Contains(t, res.Body.String(), "access-token-1")
	require.Len(t, app.calls, 1)
	require.Equal(t, "ActivateAgent", app.calls[0].method)
	cmd := app.calls[0].payload.(identityapp.ActivateAgent)
	require.Equal(t, "activation-token-1", cmd.Token)
	require.Equal(t, []string{"shell"}, cmd.Capabilities)
}

func TestAgentRefreshRequiresBearerAndMapsPrincipal(t *testing.T) {
	app := &fakeIdentityApplication{
		refresh: identityapp.Envelope[identityapp.AccessTokenView]{
			Data: identityapp.AccessTokenView{Token: "new-token", TokenType: "Bearer", AgentID: "agent-1", AgentVersionID: "agent-1.v1"},
		},
	}
	server := newAgentSelfTestServer(app)

	res := postJSON(t, server, "/v1/agents/me:refresh", `{}`, "token-agent-1")

	require.Equal(t, http.StatusOK, res.Code)
	require.Len(t, app.calls, 1)
	require.Equal(t, "IssueAccessToken", app.calls[0].method)
	require.Equal(t, identityapp.Principal{
		TenantID:       "tenant-1",
		AgentID:        "agent-1",
		AgentVersionID: "agent-1",
		Scopes:         []string{"tasks:claim", "tasks:execute"},
	}, app.calls[0].principal)
}

func TestAgentSelfMeUsesBearerPrincipal(t *testing.T) {
	app := &fakeIdentityApplication{
		get: identityapp.Envelope[identityapp.AgentView]{
			Data: identityapp.AgentView{ID: "agent-1", TenantID: "tenant-1", Status: identitydomain.AgentActive},
		},
	}
	server := newAgentSelfTestServer(app)

	res := get(t, server, "/v1/agents/me", "token-agent-1")

	require.Equal(t, http.StatusOK, res.Code)
	require.Len(t, app.calls, 1)
	require.Equal(t, "GetAgent", app.calls[0].method)
	require.Equal(t, "agent-1", app.calls[0].payload.(identityapp.GetAgent).AgentID)
}

func TestAgentSelfTokenExpiredMaps401(t *testing.T) {
	server := rest.NewServer(&fakeApplication{}, expiredTokenVerifier{},
		rest.WithIdentityService(&fakeIdentityApplication{}),
		rest.WithSession(testSessionSecret, false),
	).Router()
	req := httptestNewPost(t, "/v1/agents/me:refresh", `{}`)
	req.Header.Set("Authorization", "Bearer expired-token")
	rec := httptestRecorder(server, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Contains(t, rec.Body.String(), "TOKEN_EXPIRED")
}

func TestAgentActivateRateLimitKeyIncludesAnonymousRemoteAddr(t *testing.T) {
	expiresAt := time.Date(2026, 7, 3, 10, 15, 0, 0, time.UTC)
	app := &fakeIdentityApplication{
		activate: identityapp.Envelope[identityapp.AccessTokenView]{
			Data: identityapp.AccessTokenView{
				Token:          "access-token-1",
				TokenType:      "Bearer",
				ExpiresAt:      expiresAt,
				AgentID:        "agent-1",
				AgentVersionID: "agent-1.v1",
			},
		},
	}
	limiter := &perKeyBudgetRateLimiter{budget: 1}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithIdentityService(app),
		rest.WithSession(testSessionSecret, false),
		rest.WithRateLimiter(limiter),
	).Router()
	body := `{"activation_token":"activation-token-1","runtime":"codex","model":"gpt-5"}`

	req := httptestNewPost(t, "/v1/agents/me:activate", body)
	req.RemoteAddr = "203.0.113.10:1234"
	first := httptestRecorder(server, req)

	req = httptestNewPost(t, "/v1/agents/me:activate", body)
	req.RemoteAddr = "203.0.113.11:1234"
	second := httptestRecorder(server, req)

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	require.Len(t, app.calls, 2)
	require.Len(t, limiter.keys, 2)
	require.NotEqual(t, limiter.keys[0], limiter.keys[1])
	require.Contains(t, limiter.keys[0], "203.0.113.10")
	require.Contains(t, limiter.keys[1], "203.0.113.11")
}

func newAgentSelfTestServer(identity *fakeIdentityApplication) http.Handler {
	return rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithIdentityService(identity),
		rest.WithSession(testSessionSecret, false),
	).Router()
}

type expiredTokenVerifier struct{}

func (expiredTokenVerifier) Verify(ctx context.Context, rawToken string) (auth.Principal, error) {
	return auth.Principal{}, auth.ErrTokenExpired
}

type perKeyBudgetRateLimiter struct {
	budget int
	keys   []string
	counts map[string]int
}

func (f *perKeyBudgetRateLimiter) Allow(ctx context.Context, key string) (bool, int) {
	if f.counts == nil {
		f.counts = make(map[string]int)
	}
	f.keys = append(f.keys, key)
	f.counts[key]++
	if f.counts[key] > f.budget {
		return false, 60
	}
	return true, 0
}

func httptestNewPost(t *testing.T, path, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func httptestRecorder(server http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}
