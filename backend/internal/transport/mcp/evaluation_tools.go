package mcp

import (
	"context"

	evaluationapp "agentguild.dev/agentguild/backend/internal/evaluation/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// EvaluationRunStartInput is the input for evaluation_run_start.
type EvaluationRunStartInput struct {
	AgentID            string `json:"agent_id" jsonschema:"agent identifier"`
	VersionID          string `json:"version_id" jsonschema:"agent version identifier"`
	BenchmarkSetID     string `json:"benchmark_set_id" jsonschema:"benchmark set identifier"`
	EnvironmentDigest  string `json:"environment_digest" jsonschema:"environment digest"`
	ScoringRuleVersion string `json:"scoring_rule_version,omitempty" jsonschema:"scoring rule version"`
}

// EvaluationRunGetInput is the input for evaluation_run_get.
type EvaluationRunGetInput struct {
	EvaluationRunID string `json:"evaluation_run_id" jsonschema:"evaluation run identifier"`
}

func registerEvaluationTools(server *mcp.Server, svc evaluationService, principal auth.Principal) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "evaluation_run_start",
		Description: "对指定版本启动评测运行",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EvaluationRunStartInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.StartEvaluationRun(ctx, evaluationapp.StartEvaluationRun{
			TenantID:           principal.TenantID,
			AgentID:            input.AgentID,
			VersionID:          input.VersionID,
			BenchmarkSetID:     input.BenchmarkSetID,
			EnvironmentDigest:  input.EnvironmentDigest,
			ScoringRuleVersion: input.ScoringRuleVersion,
			ActorID:            principal.OwnerID,
			IsAdmin:            principal.IsAdmin,
		})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(map[string]any{
			"evaluation_run_id": result.EvaluationRun.ID(),
			"status":            result.EvaluationRun.Status(),
		}), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "evaluation_run_get",
		Description: "查询评测运行结果",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EvaluationRunGetInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.GetEvaluationRunDetail(ctx, identityPrincipalFromAuth(principal), principal.TenantID, input.EvaluationRunID)
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})
}
