package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	agentexperiencedomain "agentguild.dev/agentguild/backend/internal/agentexperience/domain"
	agentversiondomain "agentguild.dev/agentguild/backend/internal/agentversion/domain"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	evaluationdomain "agentguild.dev/agentguild/backend/internal/evaluation/domain"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	participationdomain "agentguild.dev/agentguild/backend/internal/participation/domain"
	rewarddomain "agentguild.dev/agentguild/backend/internal/reward/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ViolationView 是路径违规错误的结构化违规项；仅在领域错误携带违规明细时出现。
type ViolationView struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// MCPError 是工具错误响应中暴露的稳定结构；包含 code、message 与可选的
// retry_after_seconds、violations。
type MCPError struct {
	Code              string          `json:"code"`
	Message           string          `json:"message"`
	RetryAfterSeconds int             `json:"retry_after_seconds,omitempty"`
	Violations        []ViolationView `json:"violations,omitempty"`
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
	code := errorCodeOf(err)
	errContent := MCPError{Code: "INTERNAL_ERROR", Message: "internal server error"}
	switch code {
	case "invalid_argument":
		errContent = MCPError{Code: "INVALID_ARGUMENT", Message: err.Error(), Violations: pathViolationsOf(err)}
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
	// 奖励账本的两个专用冲突码，与 REST 保持一致。
	case "insufficient_escrow":
		errContent = MCPError{Code: "INSUFFICIENT_ESCROW", Message: err.Error()}
	case "policy_immutable":
		errContent = MCPError{Code: "POLICY_IMMUTABLE", Message: err.Error()}
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
	case "grant_expired":
		errContent = MCPError{Code: "GRANT_EXPIRED", Message: err.Error()}
	case "grant_revoked":
		errContent = MCPError{Code: "GRANT_REVOKED", Message: err.Error()}
	case "rate_limited":
		errContent = MCPError{Code: "RATE_LIMITED", Message: err.Error(), RetryAfterSeconds: retryAfterSeconds(errorRetryAfterOf(err))}
	default:
		if errors.Is(err, context.DeadlineExceeded) {
			errContent = MCPError{Code: "TEMPORARILY_UNAVAILABLE", Message: "request timed out"}
		}
	}
	return errorResult(errContent)
}

// errorCodeOf 尝试从所有领域包中提取稳定错误码。
func errorCodeOf(err error) string {
	if code := domain.CodeOf(err); code != "" {
		return code
	}
	if code := identitydomain.CodeOf(err); code != "" {
		return code
	}
	if code := agentversiondomain.CodeOf(err); code != "" {
		return code
	}
	if code := evaluationdomain.CodeOf(err); code != "" {
		return code
	}
	if code := agentexperiencedomain.CodeOf(err); code != "" {
		return code
	}
	if code := participationdomain.CodeOf(err); code != "" {
		return code
	}
	if code := rewarddomain.CodeOf(err); code != "" {
		return code
	}
	return ""
}

// errorRetryAfterOf 尝试从所有领域包中提取重试等待时间。
func errorRetryAfterOf(err error) time.Duration {
	if d := domain.RetryAfterOf(err); d > 0 {
		return d
	}
	if d := identitydomain.RetryAfterOf(err); d > 0 {
		return d
	}
	if d := agentversiondomain.RetryAfterOf(err); d > 0 {
		return d
	}
	if d := evaluationdomain.RetryAfterOf(err); d > 0 {
		return d
	}
	if d := agentexperiencedomain.RetryAfterOf(err); d > 0 {
		return d
	}
	return 0
}

// pathViolationsOf 提取领域错误携带的结构化路径违规项；无违规明细时返回 nil。
func pathViolationsOf(err error) []ViolationView {
	var pathErr *gitapp.PathViolationError
	if !errors.As(err, &pathErr) || len(pathErr.Violations) == 0 {
		return nil
	}
	violations := make([]ViolationView, 0, len(pathErr.Violations))
	for _, v := range pathErr.Violations {
		violations = append(violations, ViolationView{Path: v.Path, Reason: v.Reason})
	}
	return violations
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
