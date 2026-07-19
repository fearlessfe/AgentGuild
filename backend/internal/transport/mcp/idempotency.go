package mcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"

	canonicaljson "github.com/gibson042/canonicaljson-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	mutationIdempotencyTTL    = 24 * time.Hour
	mutationCompletionTimeout = 5 * time.Second
)

// mutationIdempotencyStore 是 MCP 变更工具幂等包装所需的存储契约；
// 与 REST mutation 中间件复用同一实现，*postgres.Store 天然满足此接口。
type mutationIdempotencyStore interface {
	AcquireMutationIdempotency(context.Context, application.IdempotencyKey, [32]byte, time.Duration) (*application.IdempotencyRecord, error)
	CompleteIdempotency(context.Context, application.IdempotencyKey, string, int, []byte) error
}

// idempotentMutation 为 MCP 变更工具 handler 提供与 REST mutation 中间件等价的幂等语义：
// 同 tenant+actor+工具+request_id 且规范化请求摘要一致时重放首次记录的结果且不重复执行；
// 摘要不同返回 IDEMPOTENCY_MISMATCH；执行中的并发重放返回 IDEMPOTENCY_IN_PROGRESS。
func idempotentMutation[In any](
	store mutationIdempotencyStore,
	tool string,
	principal auth.Principal,
	requestIDOf func(In) string,
	next mcp.ToolHandlerFor[In, any],
) mcp.ToolHandlerFor[In, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, any, error) {
		if store == nil {
			return errorResult(MCPError{Code: "INTERNAL_ERROR", Message: "internal server error"}), nil, nil
		}
		requestID := requestIDOf(input)
		if requestID == "" {
			return errorResult(MCPError{Code: "INVALID_ARGUMENT", Message: "request_id is required"}), nil, nil
		}
		key := application.IdempotencyKey{
			TenantID:  principal.TenantID,
			ActorID:   mutationActorID(principal),
			Operation: "mcp." + tool,
			RequestID: requestID,
		}
		record, err := store.AcquireMutationIdempotency(ctx, key, canonicalMutationArgumentsHash(req.Params.Arguments), mutationIdempotencyTTL)
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		if record.Completed {
			return replayMutationResult(record), nil, nil
		}
		if !record.Acquired {
			return mapDomainError(&domain.Error{Code: "idempotency_in_progress", Message: "an idempotent request is already in progress"}, principal), nil, nil
		}

		result, output, handlerErr := next(ctx, req, input)
		if handlerErr != nil {
			return nil, output, handlerErr
		}
		body, code, recordable := encodeMutationResult(result)
		if recordable {
			completionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mutationCompletionTimeout)
			err := store.CompleteIdempotency(completionCtx, key, record.OwnerToken, code, body)
			cancel()
			if err != nil {
				// 完成失败时挂起的记录会在租约老化后被接管重试；本次调用按内部错误处理。
				return errorResult(MCPError{Code: "INTERNAL_ERROR", Message: "internal server error"}), nil, nil
			}
		}
		return result, nil, nil
	}
}

// mutationActorID 按规格以 Agent 为幂等键的执行者维度；人类调用方回退到 owner。
func mutationActorID(p auth.Principal) string {
	if p.AgentID != "" {
		return "agent:" + p.AgentID
	}
	return "owner:" + p.OwnerID
}

// canonicalMutationArgumentsHash 让 JSON 空白与键序不影响请求摘要。
func canonicalMutationArgumentsHash(raw json.RawMessage) [32]byte {
	canonical := raw
	if len(bytes.TrimSpace(raw)) == 0 {
		canonical = []byte("null")
	} else {
		var value any
		if json.Unmarshal(raw, &value) == nil {
			if encoded, err := canonicaljson.Marshal(value); err == nil {
				canonical = encoded
			}
		}
	}
	return sha256.Sum256(canonical)
}

// encodeMutationResult 把单文本内容的工具结果序列化为可重放形态；code 记录 IsError 标志。
// 5xx 类瞬时错误（INTERNAL_ERROR、TEMPORARILY_UNAVAILABLE）不记录，允许后续重试真正执行。
func encodeMutationResult(result *mcp.CallToolResult) (body []byte, code int, recordable bool) {
	if result == nil || len(result.Content) != 1 {
		return nil, 0, false
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return nil, 0, false
	}
	if !result.IsError {
		return []byte(text.Text), 0, true
	}
	var mcpErr MCPError
	if err := json.Unmarshal([]byte(text.Text), &mcpErr); err != nil {
		return nil, 0, false
	}
	switch mcpErr.Code {
	case "INTERNAL_ERROR", "TEMPORARILY_UNAVAILABLE":
		return nil, 0, false
	}
	return []byte(text.Text), 1, true
}

// replayMutationResult 按记录重放首次执行的工具结果。
func replayMutationResult(record *application.IdempotencyRecord) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: record.ResponseCode != nil && *record.ResponseCode == 1,
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(record.ResponseBody)},
		},
	}
}
