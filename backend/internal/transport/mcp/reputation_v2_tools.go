package mcp

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/auth"
	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AgentReputationGetInput 是 agent_reputation_get 工具的输入。
// agent_id 省略时返回调用方 Agent 自己的声望。
type AgentReputationGetInput struct {
	AgentID string `json:"agent_id,omitempty" jsonschema:"global agent identifier; defaults to the calling agent"`
}

// AgentReputationService 是声望 v2 的只读服务边界。
type AgentReputationService interface {
	GetAgentReputation(ctx context.Context, agentID string) (reputationapp.Envelope[reputationapp.AgentReputationView], error)
	GetSelfReputation(ctx context.Context, principal auth.Principal) (reputationapp.Envelope[reputationapp.AgentReputationView], error)
}

// registerAgentReputationTools 注册声望 v2 查询工具。
//
// 工具名刻意不叫 reputation_get：那个名字已经属于 v1 的租户内投影查询，
// 复用会静默改变已接入 MCP 客户端拿到的响应结构。两个工具并存，
// 旧客户端完全不受影响。
func registerAgentReputationTools(server *mcp.Server, svc AgentReputationService, principal auth.Principal) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agent_reputation_get",
		Description: "查询全局 Agent 的七维声望（含置信度、样本量与算法版本）",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AgentReputationGetInput) (*mcp.CallToolResult, any, error) {
		if svc == nil {
			return errorResult(MCPError{Code: "NOT_IMPLEMENTED", Message: "reputation v2 is not configured"}), nil, nil
		}
		if input.AgentID == "" {
			result, err := svc.GetSelfReputation(ctx, principal)
			if err != nil {
				return mapDomainError(err, principal), nil, nil
			}
			return successResult(result), nil, nil
		}
		result, err := svc.GetAgentReputation(ctx, input.AgentID)
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})
}
