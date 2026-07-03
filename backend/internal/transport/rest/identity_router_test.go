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

	res := postJSONWithSession(t, server, "/v1/agents", body, sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusCreated, res.Code)
	require.Len(t, app.calls, 1)
	require.Equal(t, identityapp.Principal{TenantID: "tenant-1", OwnerID: "owner-1", OwnerEmail: "owner-1@example.com"}, app.calls[0].principal)
	cmd := app.calls[0].payload.(identityapp.RegisterAgent)
	require.Equal(t, "Builder", cmd.Name)
	require.Equal(t, []string{"tasks:read"}, cmd.Scopes)
	require.Equal(t, []string{"acme/*"}, cmd.RepoScope)
	require.Equal(t, int64(1500), cmd.BudgetCents)
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

func newIdentityTestServer(identity *fakeIdentityApplication) http.Handler {
	return rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithIdentityService(identity),
		rest.WithSession(testSessionSecret, false),
	).Router()
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

func postJSONWithSession(t *testing.T, server http.Handler, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
