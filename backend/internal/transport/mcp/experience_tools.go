package mcp

import (
	"context"

	agentexperienceapp "agentguild.dev/agentguild/backend/internal/agentexperience/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ExperienceCandidateListInput is the input for experience_candidate_list.
type ExperienceCandidateListInput struct {
	AgentID string `json:"agent_id" jsonschema:"agent identifier"`
	Status  string `json:"status,omitempty" jsonschema:"filter by candidate status"`
}

// ExperienceCandidateReviewInput is the input for experience_candidate_review.
type ExperienceCandidateReviewInput struct {
	AgentID     string `json:"agent_id" jsonschema:"agent identifier"`
	CandidateID string `json:"candidate_id" jsonschema:"experience candidate identifier"`
	Action      string `json:"action" jsonschema:"approve or reject"`
	Reason      string `json:"reason,omitempty" jsonschema:"rejection reason"`
}

func registerExperienceTools(server *mcp.Server, svc experienceService, principal auth.Principal) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "experience_candidate_list",
		Description: "列出 Agent 的经验候选",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ExperienceCandidateListInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.ListCandidates(ctx, principal.TenantID, input.AgentID, input.Status)
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "experience_candidate_review",
		Description: "审批或拒绝经验候选",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ExperienceCandidateReviewInput) (*mcp.CallToolResult, any, error) {
		err := svc.ReviewCandidate(ctx, agentexperienceapp.ReviewCandidate{
			TenantID:    principal.TenantID,
			AgentID:     input.AgentID,
			CandidateID: input.CandidateID,
			Action:      input.Action,
			Reason:      input.Reason,
			ReviewerID:  principal.OwnerID,
			IsAdmin:     principal.IsAdmin,
		})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(map[string]any{
			"action":   input.Action,
			"reviewed": true,
		}), nil, nil
	})
}
