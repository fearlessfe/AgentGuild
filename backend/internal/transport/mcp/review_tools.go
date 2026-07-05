package mcp

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	reviewapp "agentguild.dev/agentguild/backend/internal/review/application"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RubricScoreInput 是 review_submit 工具中单个评分维度的输入。
type RubricScoreInput struct {
	Dimension string `json:"dimension" jsonschema:"rubric dimension id"`
	Score     int    `json:"score" jsonschema:"score value"`
}

// ReviewSubmitInput 是 review_submit 工具的输入。
type ReviewSubmitInput struct {
	RequestID string             `json:"request_id" jsonschema:"unique mutation request id"`
	ReviewID  string             `json:"review_id" jsonschema:"review identifier"`
	Decision  string             `json:"decision" jsonschema:"final decision: accepted, rejected, or revision_requested"`
	Scores    []RubricScoreInput `json:"scores,omitempty" jsonschema:"rubric dimension scores"`
	Summary   string             `json:"summary,omitempty" jsonschema:"human-readable review summary"`
}

// ReviewGetInput 是 review_get 工具的输入。
type ReviewGetInput struct {
	ReviewID string `json:"review_id" jsonschema:"review identifier"`
}

// ReputationGetInput 是 reputation_get 工具的输入。
type ReputationGetInput struct {
	AgentVersionID string `json:"agent_version_id" jsonschema:"agent version identifier"`
	Capability     string `json:"capability" jsonschema:"capability name"`
	TaskType       string `json:"task_type" jsonschema:"task type"`
}

// reviewService 是 MCP 层消费的代码评审应用服务边界。
type reviewService interface {
	SubmitDecision(ctx context.Context, principal auth.Principal, cmd reviewapp.SubmitDecision) (application.Envelope[reviewapp.ReviewView], error)
	GetReview(ctx context.Context, principal auth.Principal, query reviewapp.GetReview) (application.Envelope[reviewapp.ReviewView], error)
}

// ReputationQuery 是声望投影查询参数。
type ReputationQuery struct {
	AgentVersionID string
	Capability     string
	TaskType       string
}

// ProjectionView 是声望投影视图，字段与 reputationdomain.Projection 保持一致。
type ProjectionView struct {
	AgentVersionID         string  `json:"agent_version_id"`
	Capability             string  `json:"capability"`
	TaskType               string  `json:"task_type"`
	TotalReviews           int     `json:"total_reviews"`
	AcceptedCount          int     `json:"accepted_count"`
	RejectedCount          int     `json:"rejected_count"`
	RevisionRequestedCount int     `json:"revision_requested_count"`
	PassRate               float64 `json:"pass_rate"`
	ReworkRate             float64 `json:"rework_rate"`
	AvgReviewCostCents     float64 `json:"avg_review_cost_cents"`
	AvgReviewLatencyMs     float64 `json:"avg_review_latency_ms"`
	SampleSizeHint         string  `json:"sample_size_hint"`
	AlgorithmVersion       string  `json:"algorithm_version"`
}

// ReputationService 是声望投影服务边界。
type ReputationService interface {
	GetProjection(ctx context.Context, principal auth.Principal, query ReputationQuery) (application.Envelope[ProjectionView], error)
}

// registerReviewTools 注册代码评审与声望相关 MCP 工具。
func registerReviewTools(server *mcp.Server, reviewSvc reviewService, reputationSvc ReputationService, principal auth.Principal) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "review_submit",
		Description: "提交审核决策",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReviewSubmitInput) (*mcp.CallToolResult, any, error) {
		if reviewSvc == nil {
			return errorResult(MCPError{Code: "NOT_IMPLEMENTED", Message: "review service is not configured"}), nil, nil
		}
		scores := make([]reviewdomain.RubricScore, len(input.Scores))
		for i, s := range input.Scores {
			scores[i] = reviewdomain.RubricScore{Dimension: s.Dimension, Score: s.Score}
		}
		result, err := reviewSvc.SubmitDecision(ctx, principal, reviewapp.SubmitDecision{
			RequestID: input.RequestID,
			ReviewID:  input.ReviewID,
			Decision:  reviewdomain.Decision(input.Decision),
			Scores:    scores,
			Summary:   input.Summary,
		})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "review_get",
		Description: "查询 Review",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReviewGetInput) (*mcp.CallToolResult, any, error) {
		if reviewSvc == nil {
			return errorResult(MCPError{Code: "NOT_IMPLEMENTED", Message: "review service is not configured"}), nil, nil
		}
		result, err := reviewSvc.GetReview(ctx, principal, reviewapp.GetReview{ReviewID: input.ReviewID})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reputation_get",
		Description: "查询声望投影",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReputationGetInput) (*mcp.CallToolResult, any, error) {
		if reputationSvc == nil {
			return errorResult(MCPError{Code: "NOT_IMPLEMENTED", Message: "reputation projection is not implemented"}), nil, nil
		}
		result, err := reputationSvc.GetProjection(ctx, principal, ReputationQuery{
			AgentVersionID: input.AgentVersionID,
			Capability:     input.Capability,
			TaskType:       input.TaskType,
		})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})
}
