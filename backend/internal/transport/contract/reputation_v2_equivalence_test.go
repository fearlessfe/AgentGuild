package contract_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
	transportmcp "agentguild.dev/agentguild/backend/internal/transport/mcp"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

// stubAgentReputationService 返回固定的声望卡片，使两个 transport 的差异
// 只可能来自序列化与错误映射。
type stubAgentReputationService struct {
	view  reputationapp.AgentReputationView
	calls []string
}

func (s *stubAgentReputationService) GetAgentReputation(_ context.Context, agentID string) (reputationapp.Envelope[reputationapp.AgentReputationView], error) {
	s.calls = append(s.calls, agentID)
	return reputationapp.Envelope[reputationapp.AgentReputationView]{Data: s.view}, nil
}

func (s *stubAgentReputationService) GetSelfReputation(_ context.Context, principal auth.Principal) (reputationapp.Envelope[reputationapp.AgentReputationView], error) {
	s.calls = append(s.calls, "self:"+principal.AgentID)
	return reputationapp.Envelope[reputationapp.AgentReputationView]{Data: s.view}, nil
}

func sampleReputation() reputationapp.AgentReputationView {
	score := 0.62
	dimensions := make([]reputationapp.DimensionView, 0, len(reputationdomain.Dimensions))
	for _, dimension := range reputationdomain.Dimensions {
		view := reputationapp.DimensionView{
			Dimension:      string(dimension),
			SampleSizeHint: "unverified",
		}
		if dimension == reputationdomain.DimensionCorrectness {
			view = reputationapp.DimensionView{
				Dimension: string(dimension), SampleSize: 24, PassedCount: 22,
				EffectiveSample: 19.5, RawRate: 0.9, LifetimeConfidence: 0.75,
				RecentConfidence: 0.78, Score: 0.77, SampleSizeHint: "high", Observed: true,
			}
		}
		dimensions = append(dimensions, view)
	}
	return reputationapp.AgentReputationView{
		AgentID:          "agent-1",
		AlgorithmVersion: reputationdomain.DefaultAlgorithmVersionV2,
		Lifetime: reputationapp.ScoreCardView{
			Scope: "agent_lifetime", AgentID: "agent-1",
			AlgorithmVersion: reputationdomain.DefaultAlgorithmVersionV2,
			OverallScore:     &score, SampleSize: 24, SampleSizeHint: "high",
			Dimensions: dimensions,
		},
		Versions:     []reputationapp.ScoreCardView{},
		Capabilities: []reputationapp.ScoreCardView{},
	}
}

func reputationViaREST(t *testing.T, svc *application.Service, stub rest.AgentReputationService) (int, reputationapp.AgentReputationView) {
	t.Helper()
	server := rest.NewServer(svc, fakeVerifier{}, rest.WithAgentReputationService(stub)).Router()
	req := httptest.NewRequest(http.MethodGet, "/v1/public/agents/agent-1/reputation", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	var envelope struct {
		Data reputationapp.AgentReputationView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	return rec.Code, envelope.Data
}

func reputationViaMCP(t *testing.T, svc *application.Service, stub transportmcp.AgentReputationService) reputationapp.AgentReputationView {
	t.Helper()
	handler := transportmcp.NewServer(svc, fakeVerifier{}, transportmcp.WithAgentReputationService(stub)).Handler()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agent_reputation_get","arguments":{"agent_id":"agent-1"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-agent-1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var response struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.NotEmpty(t, response.Result.Content)

	var envelope struct {
		Data reputationapp.AgentReputationView `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(response.Result.Content[0].Text), &envelope))
	return envelope.Data
}

func TestRESTAndMCPAgentReputationAreEquivalent(t *testing.T) {
	svc := newService(t)
	restStub := &stubAgentReputationService{view: sampleReputation()}
	mcpStub := &stubAgentReputationService{view: sampleReputation()}

	status, restView := reputationViaREST(t, svc, restStub)
	mcpView := reputationViaMCP(t, svc, mcpStub)

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, restView, mcpView, "both transports must project identical reputation cards")
	require.Equal(t, []string{"agent-1"}, restStub.calls)
	require.Equal(t, []string{"agent-1"}, mcpStub.calls)

	require.Len(t, restView.Lifetime.Dimensions, 7, "七个维度必须全部出现，包括零观测的")
	for _, dimension := range restView.Lifetime.Dimensions {
		if dimension.Dimension == string(reputationdomain.DimensionSecurity) {
			require.False(t, dimension.Observed,
				"没有安全证据时必须是零观测，绝不能被序列化成一条通过")
			require.Equal(t, "unverified", dimension.SampleSizeHint)
		}
	}
}

// 匿名读取声望时，响应里不得出现任何 sponsor 租户标识。
func TestPublicAgentReputationDoesNotLeakTenant(t *testing.T) {
	svc := newService(t)
	server := rest.NewServer(svc, fakeVerifier{},
		rest.WithAgentReputationService(&stubAgentReputationService{view: sampleReputation()})).Router()
	req := httptest.NewRequest(http.MethodGet, "/v1/public/agents/agent-1/reputation", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "tenant")
}

func TestAgentReputationReturnsNotImplementedWithoutService(t *testing.T) {
	svc := newService(t)
	server := rest.NewServer(svc, fakeVerifier{}).Router()
	req := httptest.NewRequest(http.MethodGet, "/v1/public/agents/agent-1/reputation", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	// 未挂载 v2 服务时路由不注册，返回 404 而不是空的声望卡片。
	require.Equal(t, http.StatusNotFound, rec.Code)
}
