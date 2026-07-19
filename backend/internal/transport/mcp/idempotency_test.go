package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	agentexperienceapp "agentguild.dev/agentguild/backend/internal/agentexperience/application"
	agentversionapp "agentguild.dev/agentguild/backend/internal/agentversion/application"
	agentversiondomain "agentguild.dev/agentguild/backend/internal/agentversion/domain"
	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	evaluationapp "agentguild.dev/agentguild/backend/internal/evaluation/application"
	evaluationdomain "agentguild.dev/agentguild/backend/internal/evaluation/domain"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// fakeMutationIdempotencyStore 是 mutationIdempotencyStore 的内存实现，
// 语义对齐 postgres.Store：摘要不同报 idempotency_mismatch，完成后可重放。
type fakeMutationIdempotencyStore struct {
	mu      sync.Mutex
	records map[application.IdempotencyKey]*fakeMutationRecord
}

type fakeMutationRecord struct {
	hash         [32]byte
	ownerToken   string
	responseCode *int
	responseBody []byte
}

func newFakeMutationIdempotencyStore() *fakeMutationIdempotencyStore {
	return &fakeMutationIdempotencyStore{records: make(map[application.IdempotencyKey]*fakeMutationRecord)}
}

func (s *fakeMutationIdempotencyStore) AcquireMutationIdempotency(_ context.Context, key application.IdempotencyKey, hash [32]byte, _ time.Duration) (*application.IdempotencyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.records[key]; ok {
		if rec.hash != hash {
			return nil, &domain.Error{Code: "idempotency_mismatch", Message: "idempotency key was already used with a different request"}
		}
		record := &application.IdempotencyRecord{Key: key, RequestHash: rec.hash, ResponseBody: append([]byte(nil), rec.responseBody...)}
		if rec.responseCode != nil {
			code := *rec.responseCode
			record.ResponseCode = &code
			record.Completed = true
		}
		return record, nil
	}
	token := fmt.Sprintf("owner-%d", len(s.records)+1)
	s.records[key] = &fakeMutationRecord{hash: hash, ownerToken: token}
	return &application.IdempotencyRecord{Key: key, RequestHash: hash, OwnerToken: token, Acquired: true}, nil
}

func (s *fakeMutationIdempotencyStore) CompleteIdempotency(_ context.Context, key application.IdempotencyKey, owner string, code int, body []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[key]
	if !ok || rec.ownerToken != owner || rec.responseCode != nil {
		return &domain.Error{Code: "idempotency_not_owner", Message: "idempotency record is not pending for this owner"}
	}
	rec.responseCode = &code
	rec.responseBody = append([]byte(nil), body...)
	return nil
}

// fakeVersionService 记录版本变更调用次数并按预置值返回。
type fakeVersionService struct {
	createCalls   int
	create        *agentversionapp.CreateDraftResponse
	createErr     error
	promoteCalls  int
	promoteErr    error
	rollbackCalls int
	rollbackErr   error
}

func (f *fakeVersionService) ListVersions(context.Context, string, string) ([]agentversionapp.VersionSummary, error) {
	return nil, nil
}

func (f *fakeVersionService) GetVersion(context.Context, string, string, string) (*agentversionapp.VersionDetail, error) {
	return nil, nil
}

func (f *fakeVersionService) CreateDraft(_ context.Context, _ agentversionapp.CreateDraft) (*agentversionapp.CreateDraftResponse, error) {
	f.createCalls++
	return f.create, f.createErr
}

func (f *fakeVersionService) Promote(_ context.Context, _ agentversionapp.Promote) error {
	f.promoteCalls++
	return f.promoteErr
}

func (f *fakeVersionService) Rollback(_ context.Context, _ agentversionapp.Rollback) error {
	f.rollbackCalls++
	return f.rollbackErr
}

// fakeEvaluationService 记录评测启动调用次数并按预置值返回。
type fakeEvaluationService struct {
	startCalls int
	start      *evaluationapp.StartEvaluationRunResponse
	startErr   error
}

func (f *fakeEvaluationService) StartEvaluationRun(_ context.Context, _ evaluationapp.StartEvaluationRun) (*evaluationapp.StartEvaluationRunResponse, error) {
	f.startCalls++
	return f.start, f.startErr
}

func (f *fakeEvaluationService) GetEvaluationRunSummary(context.Context, identityapp.Principal, string, string) (*evaluationapp.EvaluationRunSummary, error) {
	return nil, nil
}

func (f *fakeEvaluationService) GetEvaluationRunDetail(context.Context, identityapp.Principal, string, string) (*evaluationapp.EvaluationRunDetail, error) {
	return nil, nil
}

// fakeExperienceService 记录经验审批调用次数并按预置值返回。
type fakeExperienceService struct {
	reviewCalls int
	reviewErr   error
}

func (f *fakeExperienceService) ListCandidates(context.Context, string, string, string) ([]agentexperienceapp.CandidateSummary, error) {
	return nil, nil
}

func (f *fakeExperienceService) ReviewCandidate(_ context.Context, _ agentexperienceapp.ReviewCandidate) error {
	f.reviewCalls++
	return f.reviewErr
}

func newGovernanceMCPServer(t *testing.T, store mutationIdempotencyStore, versions versionService, evaluations evaluationService, experiences experienceService, creds credentialService) *mcp.Server {
	t.Helper()
	return newGovernanceMCPServerForPrincipal(t, testPrincipal("tasks:claim", "tasks:execute", "tasks:read", "tasks:publish", "tasks:cancel"), store, versions, evaluations, experiences, creds)
}

func newGovernanceMCPServerForPrincipal(t *testing.T, principal auth.Principal, store mutationIdempotencyStore, versions versionService, evaluations evaluationService, experiences experienceService, creds credentialService) *mcp.Server {
	t.Helper()
	app := &fakeApplication{}
	verifier := &fakeVerifier{principal: principal}
	opts := []Option{}
	if store != nil {
		opts = append(opts, WithIdempotencyStore(store))
	}
	if versions != nil {
		opts = append(opts, WithVersionService(versions))
	}
	if evaluations != nil {
		opts = append(opts, WithEvaluationService(evaluations))
	}
	if experiences != nil {
		opts = append(opts, WithExperienceService(experiences))
	}
	if creds != nil {
		opts = append(opts, WithCredentialService(creds))
	}
	s := NewServer(app, verifier, opts...)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), verifier.principal))
	return s.mcpServer(req)
}

func connectMCPSession(t *testing.T, server *mcp.Server) (*mcp.ClientSession, context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	return session, ctx, cancel
}

func toolResultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	return text.Text
}

// 后加的 6 个变更工具都必须在 schema 中要求 request_id。
func TestGovernanceMutationToolsRequireRequestID(t *testing.T) {
	server := newGovernanceMCPServer(t, newFakeMutationIdempotencyStore(), &fakeVersionService{}, &fakeEvaluationService{}, &fakeExperienceService{}, &fakeCredentialService{})
	for _, name := range []string{
		"credential_revoke",
		"agent_version_create",
		"agent_version_promote",
		"agent_version_rollback",
		"evaluation_run_start",
		"experience_candidate_review",
	} {
		t.Run(name, func(t *testing.T) {
			schema := toolSchema(t, server, name)
			required, ok := schema["required"].([]any)
			require.True(t, ok, "schema required should be an array")
			var found bool
			for _, r := range required {
				if r == "request_id" {
					found = true
					break
				}
			}
			require.True(t, found, "%s schema should require request_id", name)
		})
	}
}

func TestExperienceCandidateReviewIdempotentReplay(t *testing.T) {
	store := newFakeMutationIdempotencyStore()
	experiences := &fakeExperienceService{}
	server := newGovernanceMCPServer(t, store, nil, nil, experiences, nil)
	session, ctx, cancel := connectMCPSession(t, server)
	defer cancel()
	defer session.Close()

	args := map[string]any{
		"request_id":   "req-xp-1",
		"agent_id":     "agent-1",
		"candidate_id": "cand-1",
		"action":       "approve",
	}
	first, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "experience_candidate_review", Arguments: args})
	require.NoError(t, err)
	require.False(t, first.IsError)
	require.Contains(t, toolResultText(t, first), `"reviewed":true`)

	second, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "experience_candidate_review", Arguments: args})
	require.NoError(t, err)
	require.False(t, second.IsError)
	require.Equal(t, toolResultText(t, first), toolResultText(t, second), "重放必须返回首次结果")
	require.Equal(t, 1, experiences.reviewCalls, "重放不得重复执行变更")
}

func TestAgentVersionCreateIdempotentReplay(t *testing.T) {
	store := newFakeMutationIdempotencyStore()
	versions := &fakeVersionService{create: &agentversionapp.CreateDraftResponse{
		Version: &agentversiondomain.AgentVersion{ID: "ver-9", VersionNumber: 9, Status: agentversiondomain.StatusDraft},
	}}
	server := newGovernanceMCPServer(t, store, versions, nil, nil, nil)
	session, ctx, cancel := connectMCPSession(t, server)
	defer cancel()
	defer session.Close()

	args := map[string]any{
		"request_id": "req-av-1",
		"agent_id":   "agent-1",
		"runtime":    "go1.26",
		"model":      "model-x",
	}
	first, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "agent_version_create", Arguments: args})
	require.NoError(t, err)
	require.False(t, first.IsError)
	require.Contains(t, toolResultText(t, first), "ver-9")

	second, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "agent_version_create", Arguments: args})
	require.NoError(t, err)
	require.False(t, second.IsError)
	require.Equal(t, toolResultText(t, first), toolResultText(t, second))
	require.Equal(t, 1, versions.createCalls)
}

func TestAgentVersionPromoteIdempotentReplay(t *testing.T) {
	store := newFakeMutationIdempotencyStore()
	versions := &fakeVersionService{}
	server := newGovernanceMCPServer(t, store, versions, nil, nil, nil)
	session, ctx, cancel := connectMCPSession(t, server)
	defer cancel()
	defer session.Close()

	args := map[string]any{"request_id": "req-avp-1", "agent_id": "agent-1", "version_id": "ver-9"}
	first, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "agent_version_promote", Arguments: args})
	require.NoError(t, err)
	require.False(t, first.IsError)

	second, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "agent_version_promote", Arguments: args})
	require.NoError(t, err)
	require.False(t, second.IsError)
	require.Equal(t, toolResultText(t, first), toolResultText(t, second))
	require.Equal(t, 1, versions.promoteCalls)
}

func TestEvaluationRunStartIdempotentReplay(t *testing.T) {
	store := newFakeMutationIdempotencyStore()
	run, err := evaluationdomain.NewEvaluationRun("run-1", "tenant-1", "agent-1-v1", "bench-1", "sha256:abc", "v1", time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	evaluations := &fakeEvaluationService{start: &evaluationapp.StartEvaluationRunResponse{EvaluationRun: run}}
	server := newGovernanceMCPServer(t, store, nil, evaluations, nil, nil)
	session, ctx, cancel := connectMCPSession(t, server)
	defer cancel()
	defer session.Close()

	args := map[string]any{
		"request_id":         "req-ev-1",
		"agent_id":           "agent-1",
		"version_id":         "agent-1-v1",
		"benchmark_set_id":   "bench-1",
		"environment_digest": "sha256:abc",
	}
	first, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "evaluation_run_start", Arguments: args})
	require.NoError(t, err)
	require.False(t, first.IsError)
	require.Contains(t, toolResultText(t, first), "run-1")

	second, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "evaluation_run_start", Arguments: args})
	require.NoError(t, err)
	require.False(t, second.IsError)
	require.Equal(t, toolResultText(t, first), toolResultText(t, second))
	require.Equal(t, 1, evaluations.startCalls)
}

func TestCredentialRevokeIdempotentReplay(t *testing.T) {
	store := newFakeMutationIdempotencyStore()
	creds := &fakeCredentialService{revoke: gitapp.Envelope[gitapp.CredentialView]{
		Data: gitapp.CredentialView{ID: "cred-1", ExecutionID: "exe-1"},
	}}
	server := newGovernanceMCPServer(t, store, nil, nil, nil, creds)
	session, ctx, cancel := connectMCPSession(t, server)
	defer cancel()
	defer session.Close()

	args := map[string]any{"request_id": "req-cr-1", "execution_id": "exe-1"}
	first, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "credential_revoke", Arguments: args})
	require.NoError(t, err)
	require.False(t, first.IsError)

	second, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "credential_revoke", Arguments: args})
	require.NoError(t, err)
	require.False(t, second.IsError)
	require.Equal(t, toolResultText(t, first), toolResultText(t, second))
	require.Len(t, creds.calls, 1)
}

// 同 request_id 但请求摘要不同必须返回 IDEMPOTENCY_MISMATCH。
func TestMutationIdempotencyMismatch(t *testing.T) {
	store := newFakeMutationIdempotencyStore()
	experiences := &fakeExperienceService{}
	server := newGovernanceMCPServer(t, store, nil, nil, experiences, nil)
	session, ctx, cancel := connectMCPSession(t, server)
	defer cancel()
	defer session.Close()

	first, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "experience_candidate_review",
		Arguments: map[string]any{
			"request_id":   "req-mis-1",
			"agent_id":     "agent-1",
			"candidate_id": "cand-1",
			"action":       "approve",
		},
	})
	require.NoError(t, err)
	require.False(t, first.IsError)

	second, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "experience_candidate_review",
		Arguments: map[string]any{
			"request_id":   "req-mis-1",
			"agent_id":     "agent-1",
			"candidate_id": "cand-1",
			"action":       "reject",
			"reason":       "not useful",
		},
	})
	require.NoError(t, err)
	require.True(t, second.IsError)
	require.Contains(t, toolResultText(t, second), "IDEMPOTENCY_MISMATCH")
	require.Equal(t, 1, experiences.reviewCalls, "摘要不同不得再次执行变更")
}

// 幂等键按 tenant+agent 隔离：不同 Agent 使用相同 request_id 互不影响。
func TestMutationIdempotencyKeyIncludesAgent(t *testing.T) {
	store := newFakeMutationIdempotencyStore()
	experiences := &fakeExperienceService{}
	scopes := []string{"tasks:claim", "tasks:execute", "tasks:read", "tasks:publish", "tasks:cancel"}
	args := map[string]any{
		"request_id":   "req-xp-shared",
		"candidate_id": "cand-1",
		"action":       "approve",
	}

	firstServer := newGovernanceMCPServerForPrincipal(t, testPrincipal(scopes...), store, nil, nil, experiences, nil)
	firstSession, firstCtx, firstCancel := connectMCPSession(t, firstServer)
	defer firstCancel()
	defer firstSession.Close()
	firstArgs := map[string]any{"agent_id": "agent-1"}
	for k, v := range args {
		firstArgs[k] = v
	}
	res, err := firstSession.CallTool(firstCtx, &mcp.CallToolParams{Name: "experience_candidate_review", Arguments: firstArgs})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Equal(t, 1, experiences.reviewCalls)

	// 相同 request_id、相同业务参数、不同 Agent：不是重放，必须真实执行。
	otherPrincipal := testPrincipal(scopes...)
	otherPrincipal.AgentID = "agent-2"
	secondServer := newGovernanceMCPServerForPrincipal(t, otherPrincipal, store, nil, nil, experiences, nil)
	secondSession, secondCtx, secondCancel := connectMCPSession(t, secondServer)
	defer secondCancel()
	defer secondSession.Close()
	secondArgs := map[string]any{"agent_id": "agent-2"}
	for k, v := range args {
		secondArgs[k] = v
	}
	res, err = secondSession.CallTool(secondCtx, &mcp.CallToolParams{Name: "experience_candidate_review", Arguments: secondArgs})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Equal(t, 2, experiences.reviewCalls, "不同 Agent 的相同 request_id 必须各自真实执行")
}

// 未配置幂等存储时变更工具 fail closed，不得静默执行。
func TestMutationToolFailsClosedWithoutIdempotencyStore(t *testing.T) {
	experiences := &fakeExperienceService{}
	server := newGovernanceMCPServer(t, nil, nil, nil, experiences, nil)
	session, ctx, cancel := connectMCPSession(t, server)
	defer cancel()
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "experience_candidate_review",
		Arguments: map[string]any{
			"request_id":   "req-xp-1",
			"agent_id":     "agent-1",
			"candidate_id": "cand-1",
			"action":       "approve",
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.True(t, strings.Contains(toolResultText(t, res), "INTERNAL_ERROR"))
	require.Equal(t, 0, experiences.reviewCalls)
}
