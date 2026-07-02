package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
)

// ErrorResponse 是 REST 暴露的稳定错误结构；code 为全大写领域错误码。
type ErrorResponse struct {
	Error struct {
		Code              string `json:"code"`
		Message           string `json:"message"`
		Field             string `json:"field,omitempty"`
		RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
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
	w.Header().Set("Content-Type", "application/json")
	if field != "" {
		w.Header().Set("X-Error-Field", field)
	}
	w.WriteHeader(status)
	resp := ErrorResponse{}
	resp.Error.Code = code
	resp.Error.Message = message
	resp.Error.Field = field
	_ = json.NewEncoder(w).Encode(resp)
}

// mapDomainError 把领域错误映射为 HTTP 状态与响应体。
// 对非管理员，forbidden 与 not_found 返回一致的安全响应，避免资源探测。
func mapDomainError(w http.ResponseWriter, err error, principal auth.Principal) {
	code := domain.CodeOf(err)
	switch code {
	case "invalid_argument":
		writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error(), domain.FieldOf(err))
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
	case "state_conflict":
		writeError(w, http.StatusConflict, "STATE_CONFLICT", err.Error())
	case "lease_expired":
		writeError(w, http.StatusConflict, "LEASE_EXPIRED", err.Error())
	case "idempotency_mismatch":
		writeError(w, http.StatusConflict, "IDEMPOTENCY_MISMATCH", err.Error())
	case "deadline_exceeded":
		writeError(w, http.StatusConflict, "DEADLINE_EXCEEDED", err.Error())
	case "rate_limited":
		writeRateLimited(w, err.Error(), retryAfterSeconds(domain.RetryAfterOf(err)))
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
