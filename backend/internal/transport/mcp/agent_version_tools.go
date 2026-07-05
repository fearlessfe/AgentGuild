package mcp

import (
	"context"

	agentversionapp "agentguild.dev/agentguild/backend/internal/agentversion/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AgentVersionListInput is the input for agent_version_list.
type AgentVersionListInput struct {
	AgentID string `json:"agent_id" jsonschema:"agent identifier"`
}

// AgentVersionCreateInput is the input for agent_version_create.
type AgentVersionCreateInput struct {
	AgentID               string   `json:"agent_id" jsonschema:"agent identifier"`
	Runtime               string   `json:"runtime" jsonschema:"runtime identifier"`
	Model                 string   `json:"model" jsonschema:"model identifier"`
	Capabilities          []string `json:"capabilities,omitempty" jsonschema:"capability tags"`
	PromptRef             string   `json:"prompt_ref,omitempty" jsonschema:"prompt content reference"`
	SkillRefs             []string `json:"skill_refs,omitempty" jsonschema:"skill content references"`
	MemoryRef             string   `json:"memory_ref,omitempty" jsonschema:"memory content reference"`
	ToolRefs              []string `json:"tool_refs,omitempty" jsonschema:"tool content references"`
	EnvironmentDigest     string   `json:"environment_digest,omitempty" jsonschema:"environment digest"`
	ApprovedExperienceIDs []string `json:"approved_experience_ids,omitempty" jsonschema:"approved experience candidate ids to bind"`
}

// AgentVersionPromoteInput is the input for agent_version_promote.
type AgentVersionPromoteInput struct {
	AgentID   string `json:"agent_id" jsonschema:"agent identifier"`
	VersionID string `json:"version_id" jsonschema:"version identifier"`
}

// AgentVersionRollbackInput is the input for agent_version_rollback.
type AgentVersionRollbackInput struct {
	AgentID   string `json:"agent_id" jsonschema:"agent identifier"`
	VersionID string `json:"version_id" jsonschema:"version identifier"`
}

func registerAgentVersionTools(server *mcp.Server, svc versionService, principal auth.Principal) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agent_version_list",
		Description: "列出 Agent 的版本谱系",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AgentVersionListInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.ListVersions(ctx, principal.TenantID, input.AgentID)
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "agent_version_create",
		Description: "为 Agent 创建一个新的 Draft 版本",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AgentVersionCreateInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.CreateDraft(ctx, agentversionapp.CreateDraft{
			TenantID:              principal.TenantID,
			AgentID:               input.AgentID,
			CreatedBy:             principal.OwnerID,
			IsAdmin:               principal.IsAdmin,
			Runtime:               input.Runtime,
			Model:                 input.Model,
			Capabilities:          input.Capabilities,
			PromptRef:             input.PromptRef,
			SkillRefs:             input.SkillRefs,
			MemoryRef:             input.MemoryRef,
			ToolRefs:              input.ToolRefs,
			EnvironmentDigest:     input.EnvironmentDigest,
			ApprovedExperienceIDs: input.ApprovedExperienceIDs,
		})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(map[string]any{
			"version_id":     result.Version.ID,
			"version_number": result.Version.VersionNumber,
			"status":         result.Version.Status,
		}), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "agent_version_promote",
		Description: "将 Eligible 版本晋级为 Active",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AgentVersionPromoteInput) (*mcp.CallToolResult, any, error) {
		err := svc.Promote(ctx, agentversionapp.Promote{
			TenantID:  principal.TenantID,
			AgentID:   input.AgentID,
			VersionID: input.VersionID,
			ActorID:   principal.OwnerID,
			IsAdmin:   principal.IsAdmin,
		})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(map[string]any{"promoted": true}), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "agent_version_rollback",
		Description: "回滚到指定的历史版本",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AgentVersionRollbackInput) (*mcp.CallToolResult, any, error) {
		err := svc.Rollback(ctx, agentversionapp.Rollback{
			TenantID:  principal.TenantID,
			AgentID:   input.AgentID,
			VersionID: input.VersionID,
			ActorID:   principal.OwnerID,
			IsAdmin:   principal.IsAdmin,
		})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(map[string]any{"rolled_back": true}), nil, nil
	})
}

// identityPrincipalFromAuth converts an auth principal to the identity package
// principal type used by the version, evaluation, and experience services.
func identityPrincipalFromAuth(principal auth.Principal) identityapp.Principal {
	return identityapp.Principal{
		TenantID:       principal.TenantID,
		OwnerID:        principal.OwnerID,
		OwnerEmail:     principal.OwnerEmail,
		IsAdmin:        principal.IsAdmin,
		AgentID:        principal.AgentID,
		AgentVersionID: principal.AgentVersionID,
		Scopes:         append([]string(nil), principal.Scopes...),
		RepoScope:      append([]string(nil), principal.RepoScope...),
	}
}
