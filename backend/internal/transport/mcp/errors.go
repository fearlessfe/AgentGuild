package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPError 是工具错误响应中暴露的稳定结构；仅包含 code、message 与可选的 retry_after_seconds。
type MCPError struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
}

// isAdministrator 判断主体是否可查看真实权限/存在性差异。
func isAdministrator(p auth.Principal) bool {
	for _, s := range p.Scopes {
		if s == "admin:tasks" {
			return true
		}
	}
	return false
}

// mapDomainError 把领域错误映射为 MCP tool error content。
// 对非管理员，forbidden 与 not_found 返回一致的安全响应，避免资源探测。
func mapDomainError(err error, principal auth.Principal) *mcp.CallToolResult {
	code := domain.CodeOf(err)
	if code == "" {
		code = identitydomain.CodeOf(err)
	}
	errContent := MCPError{Code: "INTERNAL_ERROR", Message: "internal server error"}
	switch code {
	case "invalid_argument":
		errContent = MCPError{Code: "INVALID_ARGUMENT", Message: err.Error()}
	case "forbidden":
		if isAdministrator(principal) {
			errContent = MCPError{Code: "FORBIDDEN", Message: err.Error()}
		} else {
			errContent = MCPError{Code: "NOT_FOUND", Message: "resource not found"}
		}
	case "not_found":
		if isAdministrator(principal) {
			errContent = MCPError{Code: "NOT_FOUND", Message: err.Error()}
		} else {
			errContent = MCPError{Code: "NOT_FOUND", Message: "resource not found"}
		}
	case "state_conflict":
		errContent = MCPError{Code: "STATE_CONFLICT", Message: err.Error()}
	case "hard_gates_failed":
		errContent = MCPError{Code: "HARD_GATES_FAILED", Message: err.Error()}
	case "lease_expired":
		errContent = MCPError{Code: "LEASE_EXPIRED", Message: err.Error()}
	case "idempotency_mismatch":
		errContent = MCPError{Code: "IDEMPOTENCY_MISMATCH", Message: err.Error()}
	case "deadline_exceeded":
		errContent = MCPError{Code: "DEADLINE_EXCEEDED", Message: err.Error()}
	case "token_revoked":
		errContent = MCPError{Code: "TOKEN_REVOKED", Message: err.Error()}
	case "rate_limited":
		errContent = MCPError{Code: "RATE_LIMITED", Message: err.Error(), RetryAfterSeconds: retryAfterSeconds(domain.RetryAfterOf(err))}
	default:
		if errors.Is(err, context.DeadlineExceeded) {
			errContent = MCPError{Code: "TEMPORARILY_UNAVAILABLE", Message: "request timed out"}
		}
	}
	return errorResult(errContent)
}

func retryAfterSeconds(duration time.Duration) int {
	if duration <= 0 {
		return 1
	}
	return int((duration + time.Second - 1) / time.Second)
}

// errorResult 构造带 JSON 文本内容的工具错误结果。
func errorResult(err MCPError) *mcp.CallToolResult {
	body, _ := json.Marshal(err)
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(body)},
		},
	}
}

// successResult 把领域结果包装为 JSON 文本内容返回。
func successResult(data any) *mcp.CallToolResult {
	body, _ := json.Marshal(data)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(body)},
		},
	}
}

// writeError 写入 HTTP 层（非 tool 层）稳定错误响应；用于认证失败等协议级错误。
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeErrorWithRetry(w, status, code, message, 0)
}

func writeErrorWithRetry(w http.ResponseWriter, status int, code, message string, retryAfterSeconds int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	errorBody := MCPError{Code: code, Message: message}
	if retryAfterSeconds > 0 {
		errorBody.RetryAfterSeconds = retryAfterSeconds
	}
	resp := map[string]any{
		"error": errorBody,
	}
	_ = json.NewEncoder(w).Encode(resp)
}
