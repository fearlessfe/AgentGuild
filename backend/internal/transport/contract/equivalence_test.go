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
	"agentguild.dev/agentguild/backend/internal/testdb"
	transportmcp "agentguild.dev/agentguild/backend/internal/transport/mcp"
	"agentguild.dev/agentguild/backend/internal/transport/rest"

	"github.com/stretchr/testify/require"
)

// fakeVerifier 按 token 字符串返回预置 Principal。
type fakeVerifier struct{}

func (fakeVerifier) Verify(ctx context.Context, rawToken string) (auth.Principal, error) {
	switch rawToken {
	case "token-publisher":
		return auth.Principal{TenantID: "tenant-1", AgentID: "publisher", AgentVersionID: "publisher-v1", Scopes: []string{"tasks:publish", "tasks:read", "tasks:cancel"}}, nil
	case "token-agent-1":
		return auth.Principal{TenantID: "tenant-1", AgentID: "agent-1", AgentVersionID: "agent-1-v1", Scopes: []string{"tasks:claim", "tasks:execute"}}, nil
	default:
		return auth.Principal{}, nil
	}
}

func publisherPrincipal() auth.Principal {
	return auth.Principal{TenantID: "tenant-1", AgentID: "publisher", AgentVersionID: "publisher-v1", Scopes: []string{"tasks:publish", "tasks:read", "tasks:cancel"}}
}

func agentPrincipal() auth.Principal {
	return auth.Principal{TenantID: "tenant-1", AgentID: "agent-1", AgentVersionID: "agent-1-v1", Scopes: []string{"tasks:claim", "tasks:execute"}}
}

func newService(t *testing.T) *application.Service {
	t.Helper()
	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	svc, err := application.NewService(store, application.Options{
		CursorSecret: []byte("0123456789abcdef0123456789abcdef"),
		CursorTTL:    15 * time.Minute,
	})
	require.NoError(t, err)
	return svc
}

func publishTask(t *testing.T, svc *application.Service) string {
	t.Helper()
	ctx := context.Background()
	result, err := svc.PublishTask(ctx, publisherPrincipal(), application.PublishTask{
		RequestID: "pub-1",
		Type:      "code",
		Title:     "Fix parser",
		Problem:   "It races",
		Deadline:  time.Now().Add(24 * time.Hour),
	})
	require.NoError(t, err)
	return result.Data.ID
}

type claimOutcome struct {
	DomainCode      string
	TaskStatus      string
	LeaseSoftExpiry time.Time
	LeaseHardExpiry time.Time
}

func claimViaREST(t *testing.T, svc *application.Service, taskID, requestID string) claimOutcome {
	t.Helper()
	server := rest.NewServer(svc, fakeVerifier{}).Router()
	body := `{"request_id":"` + requestID + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/"+taskID+":claim", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token-agent-1")
	req.Header.Set("Idempotency-Key", requestID)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		var resp struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return claimOutcome{DomainCode: resp.Error.Code}
	}

	var envelope struct {
		Data application.ExecutionView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))

	// 通过 task_get 获取任务状态，验证两种协议对任务状态的影响一致。
	req = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+taskID, nil)
	req.Header.Set("Authorization", "Bearer token-agent-1")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	var taskEnvelope struct {
		Data application.TaskView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &taskEnvelope))

	return claimOutcome{
		TaskStatus:      string(taskEnvelope.Data.Status),
		LeaseSoftExpiry: envelope.Data.LeaseSoftExpiresAt,
		LeaseHardExpiry: envelope.Data.LeaseHardExpiresAt,
	}
}

func claimViaMCP(t *testing.T, svc *application.Service, taskID, requestID string) claimOutcome {
	t.Helper()
	mcpServer := transportmcp.NewServer(svc, fakeVerifier{})
	handler := mcpServer.Handler()

	// MCP Streamable HTTP 需要完整 HTTP 往返；通过 httptest 触发。
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"task_claim","arguments":{"request_id":"` + requestID + `","task_id":"` + taskID + `"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer token-agent-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var rpcResp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rpcResp))

	if rpcResp.Error.Code != 0 {
		// MCP 协议级错误，通常由认证失败等引起。
		return claimOutcome{DomainCode: "MCP_ERROR"}
	}
	if rpcResp.Result.IsError {
		var mcpErr struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &mcpErr))
		return claimOutcome{DomainCode: mcpErr.Code}
	}

	var envelope struct {
		Data application.ExecutionView `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &envelope))

	// 通过 MCP task_get 获取任务状态。
	body = `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"task_get","arguments":{"task_id":"` + taskID + `"}}}`
	req = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer token-agent-1")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rpcResp))
	var taskEnvelope struct {
		Data application.TaskView `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &taskEnvelope))

	return claimOutcome{
		TaskStatus:      string(taskEnvelope.Data.Status),
		LeaseSoftExpiry: envelope.Data.LeaseSoftExpiresAt,
		LeaseHardExpiry: envelope.Data.LeaseHardExpiresAt,
	}
}

func TestRESTAndMCPClaimAreEquivalent(t *testing.T) {
	svc := newService(t)
	taskID := publishTask(t, svc)

	rest := claimViaREST(t, svc, taskID, "req-equivalent")
	mcp := claimViaMCP(t, svc, taskID, "req-equivalent")

	require.Equal(t, rest.DomainCode, mcp.DomainCode, "domain code should match")
	require.Equal(t, rest.TaskStatus, mcp.TaskStatus, "task status should match")
	require.Equal(t, rest.LeaseSoftExpiry, mcp.LeaseSoftExpiry, "lease soft expiry should match")
	require.Equal(t, rest.LeaseHardExpiry, mcp.LeaseHardExpiry, "lease hard expiry should match")
}

func TestRESTAndMCPConflictAreEquivalent(t *testing.T) {
	svc := newService(t)
	taskID := publishTask(t, svc)

	// 先通过 REST 领取，再用不同 request_id 通过 MCP 领取，期望双方都得到相同 STATE_CONFLICT。
	_ = claimViaREST(t, svc, taskID, "req-first")
	rest := claimViaREST(t, svc, taskID, "req-second")
	mcp := claimViaMCP(t, svc, taskID, "req-third")

	require.Equal(t, "STATE_CONFLICT", rest.DomainCode)
	require.Equal(t, "STATE_CONFLICT", mcp.DomainCode)
}
