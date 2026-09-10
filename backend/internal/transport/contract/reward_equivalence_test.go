package contract_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/postgres"
	rewardapp "agentguild.dev/agentguild/backend/internal/reward/application"
	rewarddomain "agentguild.dev/agentguild/backend/internal/reward/domain"
	"agentguild.dev/agentguild/backend/internal/testdb"
	transportmcp "agentguild.dev/agentguild/backend/internal/transport/mcp"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

// stubRewardService 返回固定视图，让两个 transport 的差异只来自序列化与
// 错误映射，而不是数据。它同时满足 rest.RewardService 与 mcp.RewardService。
type stubRewardService struct {
	policy      rewardapp.PolicyView
	lock        rewardapp.LockView
	challenge   rewardapp.ChallengeView
	destination rewardapp.DestinationView
	err         error
	calls       []string
}

func (s *stubRewardService) PublicTaskReward(_ context.Context, publicTaskID string) (rewardapp.Envelope[rewardapp.PolicyView], error) {
	s.calls = append(s.calls, "PublicTaskReward:"+publicTaskID)
	if s.err != nil {
		return rewardapp.Envelope[rewardapp.PolicyView]{}, s.err
	}
	return rewardapp.Envelope[rewardapp.PolicyView]{Data: s.policy}, nil
}

func (s *stubRewardService) Decision(_ context.Context, decisionHash string) (rewardapp.Envelope[rewardapp.DecisionView], error) {
	s.calls = append(s.calls, "Decision:"+decisionHash)
	return rewardapp.Envelope[rewardapp.DecisionView]{}, s.err
}

func (s *stubRewardService) ExecutionReward(_ context.Context, _ auth.Principal, executionID string) (rewardapp.Envelope[rewardapp.LockView], error) {
	s.calls = append(s.calls, "ExecutionReward:"+executionID)
	if s.err != nil {
		return rewardapp.Envelope[rewardapp.LockView]{}, s.err
	}
	return rewardapp.Envelope[rewardapp.LockView]{Data: s.lock}, nil
}

func (s *stubRewardService) GetEscrow(_ context.Context, _ auth.Principal, _ string) (rewardapp.Envelope[rewardapp.EscrowView], error) {
	return rewardapp.Envelope[rewardapp.EscrowView]{}, s.err
}

func (s *stubRewardService) TopUpEscrow(_ context.Context, _ auth.Principal, _ rewardapp.TopUpEscrow) (rewardapp.Envelope[rewardapp.EscrowView], error) {
	return rewardapp.Envelope[rewardapp.EscrowView]{}, s.err
}

func (s *stubRewardService) CreateRewardPolicy(_ context.Context, _ auth.Principal, _ rewardapp.CreateRewardPolicy) (rewardapp.Envelope[rewardapp.PolicyView], error) {
	return rewardapp.Envelope[rewardapp.PolicyView]{Data: s.policy}, s.err
}

func (s *stubRewardService) FundRewardPolicy(_ context.Context, _ auth.Principal, _ rewardapp.FundRewardPolicy) (rewardapp.Envelope[rewardapp.PolicyView], error) {
	return rewardapp.Envelope[rewardapp.PolicyView]{Data: s.policy}, s.err
}

func (s *stubRewardService) OpenDispute(_ context.Context, _ auth.Principal, lockID, _ string) (rewardapp.Envelope[rewardapp.DisputeView], error) {
	s.calls = append(s.calls, "OpenDispute:"+lockID)
	return rewardapp.Envelope[rewardapp.DisputeView]{}, s.err
}

func (s *stubRewardService) ResolveDispute(_ context.Context, _ auth.Principal, disputeID, _ string, _ int64) (rewardapp.Envelope[rewardapp.DisputeView], error) {
	s.calls = append(s.calls, "ResolveDispute:"+disputeID)
	return rewardapp.Envelope[rewardapp.DisputeView]{}, s.err
}

func (s *stubRewardService) ChallengePayoutDestination(_ context.Context, _ auth.Principal, command rewardapp.ChallengePayoutDestination) (rewardapp.Envelope[rewardapp.ChallengeView], error) {
	s.calls = append(s.calls, "Challenge:"+command.Chain+"|"+command.Address)
	if s.err != nil {
		return rewardapp.Envelope[rewardapp.ChallengeView]{}, s.err
	}
	return rewardapp.Envelope[rewardapp.ChallengeView]{Data: s.challenge}, nil
}

func (s *stubRewardService) VerifyPayoutDestination(_ context.Context, _ auth.Principal, command rewardapp.VerifyPayoutDestination) (rewardapp.Envelope[rewardapp.DestinationView], error) {
	s.calls = append(s.calls, "Verify:"+command.Nonce)
	if s.err != nil {
		return rewardapp.Envelope[rewardapp.DestinationView]{}, s.err
	}
	return rewardapp.Envelope[rewardapp.DestinationView]{Data: s.destination}, nil
}

func (s *stubRewardService) ListPayoutDestinations(_ context.Context, _ auth.Principal) (rewardapp.Envelope[rewardapp.DestinationPage], error) {
	return rewardapp.Envelope[rewardapp.DestinationPage]{}, s.err
}

func sampleRewardStub() *stubRewardService {
	issued := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	return &stubRewardService{
		policy: rewardapp.PolicyView{
			ID: "policy-1", TaskID: "task-1",
			PolicyHash: "aa11bb22cc33dd44ee55ff6607182930aa11bb22cc33dd44ee55ff6607182930",
			Currency:   "USDC", SettlementProvider: "fake", GrossAmountMinor: 100_000,
			FundedAmountMinor: 100_000,
			CriterionWeightsBps: []rewarddomain.CriterionWeight{
				{CriterionID: "c1", WeightBps: 10_000},
			},
			PlatformFeeBps: 500, DisputeReserveBps: 500,
			ChallengePeriodSeconds: 3600, ExpiresAt: issued, Status: "funded",
		},
		lock: rewardapp.LockView{
			ID: "lock-1", PolicyID: "policy-1", TaskID: "task-1",
			ExecutionID: "execution-1", AgentID: "agent-1", AgentVersionID: "agent-1-v1",
			PolicyHash: "aa11bb22cc33dd44ee55ff6607182930aa11bb22cc33dd44ee55ff6607182930",
			Currency:   "USDC", LockedAmountMinor: 100_000, Status: "locked",
			LockedAt: issued, ExpiresAt: issued.Add(time.Hour),
		},
		challenge: rewardapp.ChallengeView{
			Nonce: "nonce-1", Chain: "base", Address: "0xabc", ExpiresAt: issued.Add(10 * time.Minute),
		},
		destination: rewardapp.DestinationView{
			ID: "destination-1", Chain: "base", RecipientRef: "ref-1",
			Status: "verified", CreatedAt: issued,
		},
	}
}

// newContractStore 提供 mutation 幂等中间件所需的持久化存储。
func newContractStore(t *testing.T) *postgres.Store {
	t.Helper()
	return postgres.NewStore(testdb.StartPostgres(t))
}

func rewardREST(t *testing.T, svc *application.Service, store *postgres.Store, stub rest.RewardService, method, path, body, token string) (int, json.RawMessage) {
	t.Helper()
	server := rest.NewServer(svc, fakeVerifier{},
		rest.WithIdempotencyStore(store), rest.WithRewardService(stub)).Router()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func rewardMCP(t *testing.T, svc *application.Service, store *postgres.Store, stub transportmcp.RewardService, tool, arguments string) (bool, string) {
	t.Helper()
	handler := transportmcp.NewServer(svc, fakeVerifier{},
		transportmcp.WithIdempotencyStore(store), transportmcp.WithRewardService(stub)).Handler()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool + `","arguments":` + arguments + `}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer token-agent-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var response struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response), rec.Body.String())
	require.NotEmpty(t, response.Result.Content)
	return response.Result.IsError, response.Result.Content[0].Text
}

func dataOf[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var envelope struct {
		Data T `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	return envelope.Data
}

func TestRESTAndMCPPublicTaskRewardAreEquivalent(t *testing.T) {
	svc := newService(t)
	store := newContractStore(t)
	restStub, mcpStub := sampleRewardStub(), sampleRewardStub()

	// REST 端刻意不带 Authorization：奖励契约必须匿名可读。
	status, raw := rewardREST(t, svc, store, restStub, http.MethodGet, "/v1/public/tasks/public-1/reward", "", "")
	require.Equal(t, http.StatusOK, status, string(raw))
	_, text := rewardMCP(t, svc, store, mcpStub, "reward_get", `{"public_task_id":"public-1"}`)

	require.Equal(t,
		dataOf[rewardapp.PolicyView](t, raw),
		dataOf[rewardapp.PolicyView](t, []byte(text)),
		"两个 transport 必须投影出同一份奖励契约")
	require.Equal(t, []string{"PublicTaskReward:public-1"}, restStub.calls)
	require.Equal(t, restStub.calls, mcpStub.calls)
}

func TestRESTAndMCPExecutionRewardAreEquivalent(t *testing.T) {
	svc := newService(t)
	store := newContractStore(t)
	restStub, mcpStub := sampleRewardStub(), sampleRewardStub()

	status, raw := rewardREST(t, svc, store, restStub, http.MethodGet, "/v1/executions/execution-1/reward", "", "token-agent-1")
	require.Equal(t, http.StatusOK, status, string(raw))
	_, text := rewardMCP(t, svc, store, mcpStub, "reward_get", `{"execution_id":"execution-1"}`)

	require.Equal(t,
		dataOf[rewardapp.LockView](t, raw),
		dataOf[rewardapp.LockView](t, []byte(text)))
	require.Equal(t, []string{"ExecutionReward:execution-1"}, restStub.calls)
	require.Equal(t, restStub.calls, mcpStub.calls)
}

func TestRESTAndMCPPayoutDestinationBindingIsEquivalent(t *testing.T) {
	svc := newService(t)
	store := newContractStore(t)
	restStub, mcpStub := sampleRewardStub(), sampleRewardStub()

	status, raw := rewardREST(t, svc, store, restStub, http.MethodPost,
		"/v1/agents/me/payout-destinations:challenge",
		`{"request_id":"req-challenge","chain":"base","address":"0xabc"}`, "token-agent-1")
	require.Equal(t, http.StatusCreated, status, string(raw))
	_, text := rewardMCP(t, svc, store, mcpStub, "reward_destination_challenge",
		`{"request_id":"req-challenge","chain":"base","address":"0xabc"}`)
	require.Equal(t,
		dataOf[rewardapp.ChallengeView](t, raw),
		dataOf[rewardapp.ChallengeView](t, []byte(text)))

	status, raw = rewardREST(t, svc, store, restStub, http.MethodPost,
		"/v1/agents/me/payout-destinations:verify",
		`{"request_id":"req-verify","nonce":"nonce-1","chain":"base","address":"0xabc"}`, "token-agent-1")
	require.Equal(t, http.StatusOK, status, string(raw))
	_, text = rewardMCP(t, svc, store, mcpStub, "reward_destination_verify",
		`{"request_id":"req-verify","nonce":"nonce-1","chain":"base","address":"0xabc"}`)
	require.Equal(t,
		dataOf[rewardapp.DestinationView](t, raw),
		dataOf[rewardapp.DestinationView](t, []byte(text)))

	require.Equal(t, []string{"Challenge:base|0xabc", "Verify:nonce-1"}, restStub.calls)
	require.Equal(t, restStub.calls, mcpStub.calls)
}

// TestRESTAndMCPRewardErrorsShareStableCodes 覆盖奖励模块两个专用冲突码：
// 它们必须在两个 transport 上同名，否则客户端无法统一处理。
func TestRESTAndMCPRewardErrorsShareStableCodes(t *testing.T) {
	svc := newService(t)
	store := newContractStore(t)

	cases := []struct {
		name     string
		err      error
		httpCode int
		code     string
	}{
		{"insufficient_escrow", rewarddomain.ErrInsufficientEscrow, http.StatusConflict, "INSUFFICIENT_ESCROW"},
		{"policy_immutable", rewarddomain.ErrPolicyImmutable, http.StatusConflict, "POLICY_IMMUTABLE"},
		{"state_conflict", rewarddomain.ErrStateConflict, http.StatusConflict, "STATE_CONFLICT"},
		// forbidden 对非管理员一律降级为 NOT_FOUND，避免资源探测。
		{"forbidden", rewarddomain.ErrForbidden, http.StatusNotFound, "NOT_FOUND"},
		{"not_found", rewarddomain.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			restStub := sampleRewardStub()
			restStub.err = testCase.err
			mcpStub := sampleRewardStub()
			mcpStub.err = testCase.err

			status, raw := rewardREST(t, svc, store, restStub, http.MethodGet,
				"/v1/executions/execution-1/reward", "", "token-agent-1")
			var restError struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(raw, &restError))
			require.Equal(t, testCase.httpCode, status)
			require.Equal(t, testCase.code, restError.Error.Code)

			isError, text := rewardMCP(t, svc, store, mcpStub, "reward_get", `{"execution_id":"execution-1"}`)
			require.True(t, isError)
			var mcpError struct {
				Code string `json:"code"`
			}
			require.NoError(t, json.Unmarshal([]byte(text), &mcpError))
			require.Equal(t, restError.Error.Code, mcpError.Code,
				"REST 与 MCP 必须对同一个领域错误给出同一个错误码")
		})
	}
}
