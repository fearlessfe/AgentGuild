package rest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

const testSessionSecret = "rest-session-secret"

type fakeIdentityApplication struct {
	calls []identityCall

	register     identityapp.Envelope[identityapp.RegisterAgentResponse]
	registerErr  error
	list         identityapp.Envelope[identityapp.AgentPage]
	listErr      error
	get          identityapp.Envelope[identityapp.AgentView]
	getErr       error
	suspend      identityapp.Envelope[identityapp.AgentView]
	suspendErr   error
	resume       identityapp.Envelope[identityapp.AgentView]
	resumeErr    error
	revoke       identityapp.Envelope[identityapp.AgentView]
	revokeErr    error
	status       identityapp.Envelope[identityapp.ActivationStatusView]
	statusErr    error
	activate     identityapp.Envelope[identityapp.AccessTokenView]
	activateErr  error
	refresh      identityapp.Envelope[identityapp.AccessTokenView]
	refreshErr   error
	heartbeat    identityapp.Envelope[identityapp.AgentView]
	heartbeatErr error
}

type identityCall struct {
	method    string
	principal identityapp.Principal
	payload   any
}

func (f *fakeIdentityApplication) RegisterAgent(ctx context.Context, p identityapp.Principal, cmd identityapp.RegisterAgent) (identityapp.Envelope[identityapp.RegisterAgentResponse], error) {
	f.calls = append(f.calls, identityCall{method: "RegisterAgent", principal: p, payload: cmd})
	return f.register, f.registerErr
}

func (f *fakeIdentityApplication) ListAgents(ctx context.Context, p identityapp.Principal, q identityapp.ListAgents) (identityapp.Envelope[identityapp.AgentPage], error) {
	f.calls = append(f.calls, identityCall{method: "ListAgents", principal: p, payload: q})
	return f.list, f.listErr
}

func (f *fakeIdentityApplication) GetAgent(ctx context.Context, p identityapp.Principal, q identityapp.GetAgent) (identityapp.Envelope[identityapp.AgentView], error) {
	f.calls = append(f.calls, identityCall{method: "GetAgent", principal: p, payload: q})
	return f.get, f.getErr
}

func (f *fakeIdentityApplication) SuspendAgent(ctx context.Context, p identityapp.Principal, cmd identityapp.SuspendAgent) (identityapp.Envelope[identityapp.AgentView], error) {
	f.calls = append(f.calls, identityCall{method: "SuspendAgent", principal: p, payload: cmd})
	return f.suspend, f.suspendErr
}

func (f *fakeIdentityApplication) ResumeAgent(ctx context.Context, p identityapp.Principal, cmd identityapp.ResumeAgent) (identityapp.Envelope[identityapp.AgentView], error) {
	f.calls = append(f.calls, identityCall{method: "ResumeAgent", principal: p, payload: cmd})
	return f.resume, f.resumeErr
}

func (f *fakeIdentityApplication) RevokeAgent(ctx context.Context, p identityapp.Principal, cmd identityapp.RevokeAgent) (identityapp.Envelope[identityapp.AgentView], error) {
	f.calls = append(f.calls, identityCall{method: "RevokeAgent", principal: p, payload: cmd})
	return f.revoke, f.revokeErr
}

func (f *fakeIdentityApplication) GetActivationStatus(ctx context.Context, p identityapp.Principal, q identityapp.GetActivationStatus) (identityapp.Envelope[identityapp.ActivationStatusView], error) {
	f.calls = append(f.calls, identityCall{method: "GetActivationStatus", principal: p, payload: q})
	return f.status, f.statusErr
}

func (f *fakeIdentityApplication) ActivateAgent(ctx context.Context, cmd identityapp.ActivateAgent) (identityapp.Envelope[identityapp.AccessTokenView], error) {
	f.calls = append(f.calls, identityCall{method: "ActivateAgent", payload: cmd})
	return f.activate, f.activateErr
}

func (f *fakeIdentityApplication) IssueAccessToken(ctx context.Context, p identityapp.Principal, cmd identityapp.IssueAccessToken) (identityapp.Envelope[identityapp.AccessTokenView], error) {
	f.calls = append(f.calls, identityCall{method: "IssueAccessToken", principal: p, payload: cmd})
	return f.refresh, f.refreshErr
}

func (f *fakeIdentityApplication) AgentHeartbeat(ctx context.Context, p identityapp.Principal, cmd identityapp.AgentHeartbeat) (identityapp.Envelope[identityapp.AgentView], error) {
	f.calls = append(f.calls, identityCall{method: "AgentHeartbeat", principal: p, payload: cmd})
	return f.heartbeat, f.heartbeatErr
}

func TestRegisterAgentRequiresSession(t *testing.T) {
	server := newIdentityTestServer(&fakeIdentityApplication{})

	res := postJSONNoAuth(t, server, "/v1/agents", `{}`)

	require.Equal(t, http.StatusUnauthorized, res.Code)
	require.JSONEq(t, `{"error":{"code":"UNAUTHORIZED","message":"missing or invalid session"}}`, res.Body.String())
}

func TestRegisterAgentMapsSessionAndBody(t *testing.T) {
	app := &fakeIdentityApplication{
		register: identityapp.Envelope[identityapp.RegisterAgentResponse]{
			Data: identityapp.RegisterAgentResponse{
				Agent: identityapp.AgentView{ID: "agent-1", TenantID: "tenant-1", OwnerID: "owner-1", Name: "Builder", Status: identitydomain.AgentPendingActivation},
			},
		},
	}
	server := newIdentityTestServer(app)
	body := `{"name":"Builder","description":"does work","team":"platform","scopes":["tasks:read"],"repo_scope":["acme/*"],"budget_cents":1500,"budget_currency":"USD"}`

	res := postJSONWithSession(t, server, "/v1/agents", body, sessionCookie(t, "owner-1", false), "Idempotency-Key", "agent-register-1")

	require.Equal(t, http.StatusCreated, res.Code)
	require.Len(t, app.calls, 1)
	require.Equal(t, identityapp.Principal{TenantID: "tenant-1", OwnerID: "owner-1", OwnerEmail: "owner-1@example.com"}, app.calls[0].principal)
	cmd := app.calls[0].payload.(identityapp.RegisterAgent)
	require.Equal(t, "Builder", cmd.Name)
	require.Equal(t, []string{"tasks:read"}, cmd.Scopes)
	require.Equal(t, []string{"acme/*"}, cmd.RepoScope)
	require.Equal(t, int64(1500), cmd.BudgetCents)
}

func TestRegisterAgentDoesNotRequirePersistedResponseIdempotency(t *testing.T) {
	app := &fakeIdentityApplication{
		register: identityapp.Envelope[identityapp.RegisterAgentResponse]{
			Data: identityapp.RegisterAgentResponse{Agent: identityapp.AgentView{ID: "agent-1"}},
		},
	}
	server := newIdentityTestServer(app)

	res := postJSONWithSession(t, server, "/v1/agents", `{"name":"Builder"}`, sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusCreated, res.Code)
	require.Len(t, app.calls, 1)
}

func TestGetAgentHidesOthersFromNonOwner(t *testing.T) {
	app := &fakeIdentityApplication{
		getErr: &identitydomain.Error{Code: "forbidden", Message: "owner-other cannot view agent-owned-by-other"},
	}
	server := newIdentityTestServer(app)

	res := getWithSession(t, server, "/v1/agents/agent-owned-by-other", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusNotFound, res.Code)
	require.NotContains(t, res.Body.String(), "owner-other")
}

func TestGetAgentTokenStatus(t *testing.T) {
	app := &fakeIdentityApplication{
		status: identityapp.Envelope[identityapp.ActivationStatusView]{
			Data: identityapp.ActivationStatusView{AgentID: "agent-1", Status: identitydomain.AgentPendingActivation, ActivationStatus: identitydomain.ActivationCredentialPending},
		},
	}
	server := newIdentityTestServer(app)

	res := getWithSession(t, server, "/v1/agents/agent-1:token", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	require.Len(t, app.calls, 1)
	require.Equal(t, "GetActivationStatus", app.calls[0].method)
	require.Equal(t, "agent-1", app.calls[0].payload.(identityapp.GetActivationStatus).AgentID)
}

func TestSnakeCaseFieldNames(t *testing.T) {
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	app := &fakeIdentityApplication{
		get: identityapp.Envelope[identityapp.AgentView]{
			Data: identityapp.AgentView{
				ID:          "agent-1",
				TenantID:    "tenant-1",
				Name:        "Builder",
				Description: "does work",
				Status:      identitydomain.AgentPendingActivation,
				OwnerID:     "owner-1",
				OwnerEmail:  "owner@example.com",
				Scopes:      []string{"tasks:read"},
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			Meta: identityapp.Meta{ServerTime: now, ResourceVersion: 1},
		},
	}
	server := newIdentityTestServer(app)

	res := getWithSession(t, server, "/v1/agents/agent-1", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	body := res.Body.String()
	require.Contains(t, body, `"tenant_id":`)
	require.Contains(t, body, `"owner_email":`)
	require.Contains(t, body, `"server_time":`)
	require.NotContains(t, body, `"TenantID"`)
	require.NotContains(t, body, `"OwnerEmail"`)
	require.NotContains(t, body, `"ServerTime"`)
}

func TestListAgentsReturnsSnakeCaseFields(t *testing.T) {
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	app := &fakeIdentityApplication{
		list: identityapp.Envelope[identityapp.AgentPage]{
			Data: identityapp.AgentPage{
				Items: []identityapp.AgentView{
					{
						ID:          "agent-1",
						TenantID:    "tenant-1",
						Name:        "Builder",
						Description: "does work",
						Status:      identitydomain.AgentPendingActivation,
						OwnerID:     "owner-1",
						OwnerEmail:  "owner@example.com",
						Scopes:      []string{"tasks:read"},
						RepoScope:   []string{"acme/*"},
						BudgetCents: 1500,
						CreatedAt:   now,
						UpdatedAt:   now,
					},
				},
			},
			Meta: identityapp.Meta{ServerTime: now, ResourceVersion: 1},
		},
	}
	server := newIdentityTestServer(app)

	res := getWithSession(t, server, "/v1/agents", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	body := res.Body.String()
	require.Contains(t, body, `"items":`)
	require.Contains(t, body, `"tenant_id":`)
	require.Contains(t, body, `"owner_email":`)
	require.Contains(t, body, `"repo_scope":`)
	require.Contains(t, body, `"budget_cents":`)
	require.Contains(t, body, `"server_time":`)
	require.NotContains(t, body, `"TenantID"`)
	require.NotContains(t, body, `"OwnerEmail"`)
	require.NotContains(t, body, `"RepoScope"`)
	require.NotContains(t, body, `"BudgetCents"`)
	require.NotContains(t, body, `"ServerTime"`)
}

func TestOIDCLoginRedirectsWithSignedStateCookie(t *testing.T) {
	provider := &fakeOIDCProvider{}
	server := newIdentityOIDCTestServer(&fakeIdentityApplication{}, provider)

	req := httptest.NewRequest(http.MethodGet, "/oauth/oidc/login", nil)
	rec := httptestRecorder(server, req)

	require.Equal(t, http.StatusFound, rec.Code)
	require.NotEmpty(t, provider.state)
	require.Contains(t, rec.Header().Get("Location"), url.QueryEscape(provider.state))
	cookie := findCookie(rec.Result().Cookies(), "agentguild_oidc_state")
	require.NotNil(t, cookie)
	require.NotEmpty(t, cookie.Value)
	require.True(t, cookie.HttpOnly)
	require.False(t, cookie.Secure)
	require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	require.Equal(t, "/oauth/oidc/callback", cookie.Path)
}

func TestOIDCCallbackAcceptsMatchingSignedState(t *testing.T) {
	provider := &fakeOIDCProvider{
		session: &auth.Session{
			TenantID:   "tenant-1",
			OwnerID:    "owner-1",
			OwnerEmail: "owner-1@example.com",
			ExpiresAt:  time.Now().Add(time.Hour),
		},
	}
	server := newIdentityOIDCTestServer(&fakeIdentityApplication{}, provider)
	login := httptestRecorder(server, httptest.NewRequest(http.MethodGet, "/oauth/oidc/login", nil))
	stateCookie := findCookie(login.Result().Cookies(), "agentguild_oidc_state")
	require.NotNil(t, stateCookie)

	req := httptest.NewRequest(http.MethodGet, "/oauth/oidc/callback?code=code-1&state="+url.QueryEscape(provider.state), nil)
	req.AddCookie(stateCookie)
	rec := httptestRecorder(server, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "code-1", provider.exchangedCode)
	require.NotNil(t, findCookie(rec.Result().Cookies(), auth.SessionCookieName))
	cleared := findCookie(rec.Result().Cookies(), "agentguild_oidc_state")
	require.NotNil(t, cleared)
	require.Less(t, cleared.MaxAge, 0)
}

func TestOIDCCallbackRejectsMissingStateWithoutSession(t *testing.T) {
	provider := &fakeOIDCProvider{
		session: &auth.Session{TenantID: "tenant-1", OwnerID: "owner-1", OwnerEmail: "owner-1@example.com"},
	}
	server := newIdentityOIDCTestServer(&fakeIdentityApplication{}, provider)

	req := httptest.NewRequest(http.MethodGet, "/oauth/oidc/callback?code=code-1", nil)
	rec := httptestRecorder(server, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Empty(t, provider.exchangedCode)
	require.Nil(t, findCookie(rec.Result().Cookies(), auth.SessionCookieName))
}

func TestOIDCCallbackRejectsMismatchedStateWithoutSession(t *testing.T) {
	provider := &fakeOIDCProvider{
		session: &auth.Session{TenantID: "tenant-1", OwnerID: "owner-1", OwnerEmail: "owner-1@example.com"},
	}
	server := newIdentityOIDCTestServer(&fakeIdentityApplication{}, provider)
	login := httptestRecorder(server, httptest.NewRequest(http.MethodGet, "/oauth/oidc/login", nil))
	stateCookie := findCookie(login.Result().Cookies(), "agentguild_oidc_state")
	require.NotNil(t, stateCookie)

	req := httptest.NewRequest(http.MethodGet, "/oauth/oidc/callback?code=code-1&state=different-state", nil)
	req.AddCookie(stateCookie)
	rec := httptestRecorder(server, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Empty(t, provider.exchangedCode)
	require.Nil(t, findCookie(rec.Result().Cookies(), auth.SessionCookieName))
}

func TestOIDCCallbackRejectsTamperedStateCookieWithoutSession(t *testing.T) {
	provider := &fakeOIDCProvider{
		session: &auth.Session{TenantID: "tenant-1", OwnerID: "owner-1", OwnerEmail: "owner-1@example.com"},
	}
	server := newIdentityOIDCTestServer(&fakeIdentityApplication{}, provider)
	login := httptestRecorder(server, httptest.NewRequest(http.MethodGet, "/oauth/oidc/login", nil))
	stateCookie := findCookie(login.Result().Cookies(), "agentguild_oidc_state")
	require.NotNil(t, stateCookie)
	stateCookie.Value = "tampered." + stateCookie.Value

	req := httptest.NewRequest(http.MethodGet, "/oauth/oidc/callback?code=code-1&state="+url.QueryEscape(provider.state), nil)
	req.AddCookie(stateCookie)
	rec := httptestRecorder(server, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Empty(t, provider.exchangedCode)
	require.Nil(t, findCookie(rec.Result().Cookies(), auth.SessionCookieName))
}

func newIdentityTestServer(identity *fakeIdentityApplication) http.Handler {
	return rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithIdentityService(identity),
		rest.WithSession(testSessionSecret, false),
		rest.WithIdempotencyStore(newMemoryIdempotencyStore()),
	).Router()
}

func newIdentityOIDCTestServer(identity *fakeIdentityApplication, provider *fakeOIDCProvider) http.Handler {
	return rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithIdentityService(identity),
		rest.WithSession(testSessionSecret, false),
		rest.WithOIDCProvider(provider),
	).Router()
}

type fakeOIDCProvider struct {
	state         string
	exchangedCode string
	session       *auth.Session
	err           error
}

func (f *fakeOIDCProvider) BeginAuthURL(state string) string {
	f.state = state
	return "/oidc/start?state=" + url.QueryEscape(state)
}

func (f *fakeOIDCProvider) Exchange(ctx context.Context, code string) (*auth.Session, error) {
	f.exchangedCode = code
	return f.session, f.err
}

func sessionCookie(t *testing.T, ownerID string, isAdmin bool) *http.Cookie {
	t.Helper()
	cookie, err := auth.NewSessionCookie(auth.Session{
		TenantID:   "tenant-1",
		OwnerID:    ownerID,
		OwnerEmail: ownerID + "@example.com",
		IsAdmin:    isAdmin,
		ExpiresAt:  time.Now().Add(time.Hour),
	}, testSessionSecret, false)
	require.NoError(t, err)
	return cookie
}

func postJSONNoAuth(t *testing.T, server http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func postJSONWithSession(t *testing.T, server http.Handler, path, body string, cookie *http.Cookie, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}

func getWithSession(t *testing.T, server http.Handler, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}
