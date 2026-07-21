package rest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	publictaskapp "agentguild.dev/agentguild/backend/internal/publictask/application"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func TestPublicTaskRoutesSupportAnonymousAndAuthenticatedAgentWithoutTenantFields(t *testing.T) {
	now := time.Now().UTC()
	service := &fakePublicTaskService{
		list: publictaskapp.Envelope[publictaskapp.TaskPage]{
			Data: publictaskapp.TaskPage{Items: []publictaskapp.TaskSummary{{
				ID: "public-1", CanonicalRepository: "acme/widgets", Title: "Fix widget",
				Summary: "Public summary", PublishedAt: now,
			}}},
			Meta: publictaskapp.Meta{ServerTime: now},
		},
		detail: publictaskapp.Envelope[publictaskapp.TaskDetail]{
			Data: publictaskapp.TaskDetail{TaskSummary: publictaskapp.TaskSummary{
				ID: "public-1", CanonicalRepository: "acme/widgets", Title: "Fix widget",
			}},
			Meta: publictaskapp.Meta{ServerTime: now},
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithPublicTaskService(service))

	req := httptest.NewRequest(http.MethodGet, "/v1/public/tasks?limit=10", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.False(t, service.lastList.AuthenticatedAgent)
	require.Equal(t, 10, service.lastList.Limit)
	require.NotContains(t, rec.Body.String(), "tenant_id")

	req = httptest.NewRequest(http.MethodGet, "/v1/public/tasks?cursor=opaque", nil)
	req.Header.Set("Authorization", "Bearer token-agent-1")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, service.lastList.AuthenticatedAgent)
	require.Equal(t, "opaque", service.lastList.Cursor)

	req = httptest.NewRequest(http.MethodGet, "/v1/public/tasks/public-1", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "public-1", service.lastGet.ID)
	require.False(t, service.lastGet.AuthenticatedAgent)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotContains(t, rec.Body.String(), "resource_tenant")
}

func TestPublicTaskRouteRejectsInvalidOptionalBearer(t *testing.T) {
	server := newTestServer(&fakeApplication{}, rest.WithPublicTaskService(&fakePublicTaskService{}))
	req := httptest.NewRequest(http.MethodGet, "/v1/public/tasks", nil)
	req.Header.Set("Authorization", "Bearer invalid")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPublicTaskClaimRequiresBearerAndForwardsIdempotencyKey(t *testing.T) {
	now := time.Now().UTC()
	service := &fakePublicTaskService{claim: publictaskapp.Envelope[publictaskapp.PublicClaimView]{
		Data: publictaskapp.PublicClaimView{
			PublicTaskID: "public-1", ExecutionID: "execution-1",
			AgentID: "global-agent", AgentVersionID: "global-version",
			TaskSpecificationVersionID: "spec-1", Status: "leased",
		},
		Meta: publictaskapp.Meta{ServerTime: now},
	}}
	principal := auth.Principal{
		SubjectID: auth.AgentSubject("global-agent"), IdentityScope: auth.IdentityScopeGlobal,
		Type: auth.PrincipalTypeAgent, AgentID: "global-agent",
		AgentVersionID: "global-version", Scopes: []string{"tasks:claim"},
	}
	server := rest.NewServer(
		&fakeApplication{}, &fakeVerifier{principal: principal},
		rest.WithPublicTaskService(service),
	).Router()

	req := httptest.NewRequest(http.MethodPost, "/v1/public/tasks/public-1:claim", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/v1/public/tasks/public-1:claim", nil)
	req.Header.Set("Authorization", "Bearer global-token")
	req.Header.Set("Idempotency-Key", "claim-request-1")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "public-1", service.lastClaim.PublicTaskID)
	require.Equal(t, "claim-request-1", service.lastClaim.RequestID)
	require.Equal(t, "global-agent", service.lastPrincipal.AgentID)
	require.NotContains(t, rec.Body.String(), "tenant")
}

type fakePublicTaskService struct {
	list          publictaskapp.Envelope[publictaskapp.TaskPage]
	detail        publictaskapp.Envelope[publictaskapp.TaskDetail]
	claim         publictaskapp.Envelope[publictaskapp.PublicClaimView]
	lastList      publictaskapp.ListPublicTasks
	lastGet       publictaskapp.GetPublicTask
	lastClaim     publictaskapp.ClaimPublicTask
	lastPrincipal auth.Principal
}

func (s *fakePublicTaskService) List(_ context.Context, query publictaskapp.ListPublicTasks) (publictaskapp.Envelope[publictaskapp.TaskPage], error) {
	s.lastList = query
	return s.list, nil
}

func (s *fakePublicTaskService) Get(_ context.Context, query publictaskapp.GetPublicTask) (publictaskapp.Envelope[publictaskapp.TaskDetail], error) {
	s.lastGet = query
	return s.detail, nil
}

func (s *fakePublicTaskService) Claim(_ context.Context, principal auth.Principal, command publictaskapp.ClaimPublicTask) (publictaskapp.Envelope[publictaskapp.PublicClaimView], error) {
	s.lastPrincipal = principal
	s.lastClaim = command
	return s.claim, nil
}
