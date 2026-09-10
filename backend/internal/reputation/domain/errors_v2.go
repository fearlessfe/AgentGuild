package domain

import (
	coredomain "agentguild.dev/agentguild/backend/internal/domain"
)

// 声望 v2 直接复用核心领域错误类型，这样 REST/MCP 的 errorCodeOf 不需要为
// 本模块再加一条分支。

var (
	// ErrNotFound 表示请求的算法版本或投影不存在。
	ErrNotFound = coredomain.ErrNotFound
	// ErrInvalidEvaluatedAt 表示缺少固定的评估时刻。有 180 天衰减后，
	// "可完整重算"只在固定评估时刻成立，因此它不能为空。
	ErrInvalidEvaluatedAt = invalidArgument("evaluated_at")
	// ErrInvalidFact 表示事实缺少全局 Agent 或 Agent Version 归属。
	ErrInvalidFact = invalidArgument("contribution_fact")
	// ErrInvalidArgument 是通用参数错误。
	ErrInvalidArgument = invalidArgument("argument")
)

func invalidArgument(field string) error {
	return &coredomain.Error{
		Code:    "invalid_argument",
		Message: field + " is invalid",
		Field:   field,
	}
}
