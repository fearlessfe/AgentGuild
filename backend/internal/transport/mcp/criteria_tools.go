package mcp

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	contributionapp "agentguild.dev/agentguild/backend/internal/contribution/application"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CriteriaGetInput 是 execution_criteria_get 工具的输入。
type CriteriaGetInput struct {
	ExecutionID string `json:"execution_id" jsonschema:"execution identifier"`
}

// CriteriaService 暴露验收标准的逐条验证证据。
type CriteriaService interface {
	ExecutionCriteria(ctx context.Context, principal auth.Principal, executionID string) (application.Envelope[contributionapp.CriterionCoverageView], error)
}

// registerCriteriaTools 注册验收证据查询工具。Agent 用它判断自己还差哪几条
// 验收标准，而不是等到评审结束才知道结果。
func registerCriteriaTools(server *mcp.Server, criteriaSvc CriteriaService, principal auth.Principal) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "execution_criteria_get",
		Description: "查询 Execution 上逐条验收标准的验证结果",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CriteriaGetInput) (*mcp.CallToolResult, any, error) {
		if criteriaSvc == nil {
			return errorResult(MCPError{Code: "NOT_IMPLEMENTED", Message: "criteria service is not configured"}), nil, nil
		}
		result, err := criteriaSvc.ExecutionCriteria(ctx, principal, input.ExecutionID)
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})
}
