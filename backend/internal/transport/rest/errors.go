package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	agentexperiencedomain "agentguild.dev/agentguild/backend/internal/agentexperience/domain"
	agentversiondomain "agentguild.dev/agentguild/backend/internal/agentversion/domain"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	evaluationdomain "agentguild.dev/agentguild/backend/internal/evaluation/domain"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	publictaskdomain "agentguild.dev/agentguild/backend/internal/publictask/domain"
)

// ViolationView 是路径违规错误的结构化违规项；仅在领域错误携带违规明细时出现。
type ViolationView struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// ErrorResponse 是 REST 暴露的稳定错误结构；code 为全大写领域错误码。
type ErrorResponse struct {
	Error struct {
		Code              string          `json:"code"`
		Message           string          `json:"message"`
		Field             string          `json:"field,omitempty"`
		RetryAfterSeconds int             `json:"retry_after_seconds,omitempty"`
		Violations        []ViolationView `json:"violations,omitempty"`
	} `json:"error"`
}

func writeRateLimited(w http.ResponseWriter, message string, retryAfter int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	w.WriteHeader(http.StatusTooManyRequests)
	resp := ErrorResponse{}
	resp.Error.Code = "RATE_LIMITED"
	resp.Error.Message = message
	resp.Error.RetryAfterSeconds = retryAfter
	_ = json.NewEncoder(w).Encode(resp)
}

// RateLimiter 决定请求是否被限流；若不允许，返回建议等待秒数。
type RateLimiter interface {
	Allow(ctx context.Context, key string) (allowed bool, retryAfter int)
}

type noopRateLimiter struct{}

func (noopRateLimiter) Allow(context.Context, string) (bool, int) { return true, 0 }

// isAdministrator 通过 scopes 判断主体是否可以查看真实权限/存在性差异。
// 生产环境可替换为更复杂的角色模型，但本 change 仅依赖 OAuth scope。
func isAdministrator(p auth.Principal) bool {
	for _, s := range p.Scopes {
		if s == "admin:tasks" {
			return true
		}
	}
	return false
}

// writeError 写入稳定错误响应。
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := ErrorResponse{}
	resp.Error.Code = code
	resp.Error.Message = message
	_ = json.NewEncoder(w).Encode(resp)
}

// writeFieldError 写入带字段信息的参数错误；字段信息仅通过响应体暴露，不泄露内部细节。
func writeFieldError(w http.ResponseWriter, status int, code, message, field string) {
	writeFieldErrorWithViolations(w, status, code, message, field, nil)
}

// writeFieldErrorWithViolations 在字段错误基础上附带结构化路径违规项（可为空）。
func writeFieldErrorWithViolations(w http.ResponseWriter, status int, code, message, field string, violations []ViolationView) {
	w.Header().Set("Content-Type", "application/json")
	if field != "" {
		w.Header().Set("X-Error-Field", field)
	}
	w.WriteHeader(status)
	resp := ErrorResponse{}
	resp.Error.Code = code
	resp.Error.Message = message
	resp.Error.Field = field
	resp.Error.Violations = violations
	_ = json.NewEncoder(w).Encode(resp)
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

// mapDomainError 把领域错误映射为 HTTP 状态与响应体。
// 对非管理员，forbidden 与 not_found 返回一致的安全响应，避免资源探测。
func mapDomainError(w http.ResponseWriter, err error, principal auth.Principal) {
	code := errorCodeOf(err)
	switch code {
	case "invalid_argument":
		writeFieldErrorWithViolations(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error(), errorFieldOf(err), pathViolationsOf(err))
	case "forbidden":
		if isAdministrator(principal) {
			writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
			return
		}
		writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case "not_found":
		if isAdministrator(principal) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case "not_configured":
		writeError(w, http.StatusNotFound, "NOT_CONFIGURED", err.Error())
	case "evaluation_unavailable":
		writeError(w, http.StatusServiceUnavailable, "EVALUATION_UNAVAILABLE", err.Error())
	case "state_conflict":
		writeError(w, http.StatusConflict, "STATE_CONFLICT", err.Error())
	case "hard_gates_failed":
		writeError(w, http.StatusConflict, "HARD_GATES_FAILED", err.Error())
	case "lease_expired":
		writeError(w, http.StatusConflict, "LEASE_EXPIRED", err.Error())
	case "idempotency_mismatch":
		writeError(w, http.StatusConflict, "IDEMPOTENCY_MISMATCH", err.Error())
	case "idempotency_in_progress":
		writeError(w, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", err.Error())
	case "deadline_exceeded":
		writeError(w, http.StatusConflict, "DEADLINE_EXCEEDED", err.Error())
	case "token_revoked":
		writeError(w, http.StatusUnauthorized, "TOKEN_REVOKED", err.Error())
	case "rate_limited":
		writeRateLimited(w, err.Error(), retryAfterSeconds(errorRetryAfterOf(err)))
	default:
		if errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusServiceUnavailable, "TEMPORARILY_UNAVAILABLE", "request timed out")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
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
	if code := publictaskdomain.CodeOf(err); code != "" {
		return code
	}
	return ""
}

// errorFieldOf 尝试从所有领域包中提取错误字段。
func errorFieldOf(err error) string {
	if field := domain.FieldOf(err); field != "" {
		return field
	}
	if field := identitydomain.FieldOf(err); field != "" {
		return field
	}
	if field := agentversiondomain.FieldOf(err); field != "" {
		return field
	}
	if field := evaluationdomain.FieldOf(err); field != "" {
		return field
	}
	if field := agentexperiencedomain.FieldOf(err); field != "" {
		return field
	}
	if field := publictaskdomain.FieldOf(err); field != "" {
		return field
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

func mapIdentityError(w http.ResponseWriter, err error, principal identityapp.Principal) {
	code := identitydomain.CodeOf(err)
	switch code {
	case "invalid_argument":
		writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error(), identitydomain.FieldOf(err))
	case "forbidden":
		if principal.IsAdmin {
			writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
			return
		}
		writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case "not_found":
		if principal.IsAdmin {
			writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case "state_conflict":
		writeError(w, http.StatusConflict, "STATE_CONFLICT", err.Error())
	case "token_expired":
		writeError(w, http.StatusUnauthorized, "TOKEN_EXPIRED", err.Error())
	case "token_revoked":
		writeError(w, http.StatusUnauthorized, "TOKEN_REVOKED", err.Error())
	case "rate_limited":
		writeRateLimited(w, err.Error(), retryAfterSeconds(identitydomain.RetryAfterOf(err)))
	default:
		if errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusServiceUnavailable, "TEMPORARILY_UNAVAILABLE", "request timed out")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func retryAfterSeconds(duration time.Duration) int {
	if duration <= 0 {
		return 1
	}
	return int((duration + time.Second - 1) / time.Second)
}
