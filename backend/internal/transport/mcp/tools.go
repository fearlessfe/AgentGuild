package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// PublishTaskInput 是 task_publish 工具的输入。
type PublishTaskInput struct {
	RequestID    string    `json:"request_id" jsonschema:"unique mutation request id"`
	Type         string    `json:"type" jsonschema:"task type"`
	Title        string    `json:"title" jsonschema:"human-readable title"`
	Problem      string    `json:"problem" jsonschema:"problem description"`
	Constraints  []string  `json:"constraints,omitempty" jsonschema:"list of constraints"`
	Requirements []string  `json:"requirements,omitempty" jsonschema:"list of requirements"`
	Deadline     time.Time `json:"deadline" jsonschema:"task deadline in RFC3339"`
}

// ListTasksInput 是 task_list 工具的输入。
type ListTasksInput struct {
	Cursor                  string   `json:"cursor,omitempty" jsonschema:"opaque pagination cursor"`
	Statuses                []string `json:"statuses,omitempty" jsonschema:"filter by task statuses"`
	Type                    string   `json:"type,omitempty" jsonschema:"filter by task type"`
	PublisherAgentVersionID string   `json:"publisher_agent_version_id,omitempty" jsonschema:"filter by publisher agent version id"`
	Limit                   int      `json:"limit,omitempty" jsonschema:"page size, default 20, max 100"`
}

// GetTaskInput 是 task_get 工具的输入。
type GetTaskInput struct {
	TaskID string `json:"task_id" jsonschema:"task identifier"`
}

// ClaimTaskInput 是 task_claim 工具的输入。
type ClaimTaskInput struct {
	RequestID string `json:"request_id" jsonschema:"unique mutation request id"`
	TaskID    string `json:"task_id" jsonschema:"task identifier"`
}

// CancelTaskInput 是 task_cancel 工具的输入。
type CancelTaskInput struct {
	RequestID string `json:"request_id" jsonschema:"unique mutation request id"`
	TaskID    string `json:"task_id" jsonschema:"task identifier"`
	Reason    string `json:"reason,omitempty" jsonschema:"cancellation reason"`
}

// StartExecutionInput 是 execution_start 工具的输入。
type StartExecutionInput struct {
	RequestID       string   `json:"request_id" jsonschema:"unique mutation request id"`
	ExecutionID     string   `json:"execution_id" jsonschema:"execution identifier"`
	LeaseGeneration int64    `json:"lease_generation" jsonschema:"current lease generation"`
	Stage           *string  `json:"stage,omitempty" jsonschema:"optional execution stage"`
	Progress        *float64 `json:"progress,omitempty" jsonschema:"optional progress between 0 and 1"`
}

// HeartbeatInput 是 execution_heartbeat 工具的输入。
type HeartbeatInput struct {
	RequestID       string   `json:"request_id" jsonschema:"unique mutation request id"`
	ExecutionID     string   `json:"execution_id" jsonschema:"execution identifier"`
	LeaseGeneration int64    `json:"lease_generation" jsonschema:"current lease generation"`
	Stage           *string  `json:"stage,omitempty" jsonschema:"optional execution stage"`
	Progress        *float64 `json:"progress,omitempty" jsonschema:"optional progress between 0 and 1"`
}

// GetExecutionInput 是 execution_get 工具的输入。
type GetExecutionInput struct {
	ExecutionID string `json:"execution_id" jsonschema:"execution identifier"`
}

// CreateSubmissionInput 是 submission_create 工具的输入。
type CreateSubmissionInput struct {
	RequestID     string          `json:"request_id" jsonschema:"unique mutation request id"`
	ExecutionID   string          `json:"execution_id" jsonschema:"execution identifier"`
	Repo          string          `json:"repo" jsonschema:"repository in owner/name format"`
	Branch        string          `json:"branch" jsonschema:"branch containing the commit"`
	CommitSHA     string          `json:"commit_sha" jsonschema:"commit sha to submit"`
	BaseCommitSHA string          `json:"base_commit_sha" jsonschema:"base commit sha"`
	Summary       string          `json:"summary" jsonschema:"human-readable summary of changes"`
	Tests         *string         `json:"tests,omitempty" jsonschema:"test declaration or command"`
	Evidence      json.RawMessage `json:"evidence,omitempty" jsonschema:"supporting evidence JSON"`
}

// GetSubmissionInput 是 validation_get 工具的输入。
type GetSubmissionInput struct {
	SubmissionID string `json:"submission_id" jsonschema:"submission identifier"`
}

// IssueCredentialInput 是 credential_issue 工具的输入。
type IssueCredentialInput struct {
	ExecutionID string `json:"execution_id" jsonschema:"execution identifier"`
	Repo        string `json:"repo" jsonschema:"repository in owner/name format"`
	Branch      string `json:"branch,omitempty" jsonschema:"optional restricted branch"`
	BaseCommit  string `json:"base_commit" jsonschema:"base commit sha"`
}

// GetCredentialInput 是 credential_get 工具的输入。
type GetCredentialInput struct {
	ExecutionID string `json:"execution_id" jsonschema:"execution identifier"`
}

// RevokeCredentialInput 是 credential_revoke 工具的输入。
type RevokeCredentialInput struct {
	ExecutionID string `json:"execution_id" jsonschema:"execution identifier"`
}

// registerTools 注册任务生命周期、Submission 与 Credential 工具。
// Principal 已按请求注入，每个 handler 只调用共享 applicationService。
func registerTools(server *mcp.Server, svc applicationService, submissions submissionService, credentials credentialService, principal auth.Principal) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_publish",
		Description: "发布新任务",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PublishTaskInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.PublishTask(ctx, principal, application.PublishTask{
			RequestID:    input.RequestID,
			Type:         input.Type,
			Title:        input.Title,
			Problem:      input.Problem,
			Constraints:  input.Constraints,
			Requirements: input.Requirements,
			Deadline:     input.Deadline,
		})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_list",
		Description: "列出任务",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListTasksInput) (*mcp.CallToolResult, any, error) {
		query := application.ListTasks{
			Cursor:                  input.Cursor,
			PublisherAgentVersionID: input.PublisherAgentVersionID,
			Type:                    input.Type,
			Limit:                   input.Limit,
		}
		if len(input.Statuses) > 0 {
			query.Statuses = make([]domain.TaskStatus, 0, len(input.Statuses))
			for _, s := range input.Statuses {
				query.Statuses = append(query.Statuses, domain.TaskStatus(s))
			}
		}
		result, err := svc.ListTasks(ctx, principal, query)
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_get",
		Description: "获取单个任务",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetTaskInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.GetTask(ctx, principal, application.GetTask{TaskID: input.TaskID})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_claim",
		Description: "领取任务并开始执行",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ClaimTaskInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.ClaimTask(ctx, principal, application.ClaimTask{
			RequestID: input.RequestID,
			TaskID:    input.TaskID,
		})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_cancel",
		Description: "取消任务",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CancelTaskInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.CancelTask(ctx, principal, application.CancelTask{
			RequestID: input.RequestID,
			TaskID:    input.TaskID,
			Reason:    input.Reason,
		})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "execution_start",
		Description: "开始执行已领取的任务",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StartExecutionInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.StartExecution(ctx, principal, application.StartExecution{
			RequestID:       input.RequestID,
			ExecutionID:     input.ExecutionID,
			LeaseGeneration: input.LeaseGeneration,
			Stage:           input.Stage,
			Progress:        input.Progress,
		})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "execution_heartbeat",
		Description: "续租当前执行",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input HeartbeatInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.HeartbeatExecution(ctx, principal, application.HeartbeatExecution{
			RequestID:       input.RequestID,
			ExecutionID:     input.ExecutionID,
			LeaseGeneration: input.LeaseGeneration,
			Stage:           input.Stage,
			Progress:        input.Progress,
		})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "execution_get",
		Description: "获取执行状态",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetExecutionInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.GetExecution(ctx, principal, application.GetExecution{ExecutionID: input.ExecutionID})
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})

	if submissions != nil {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "submission_create",
			Description: "提交代码成果以触发自动验证",
		}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateSubmissionInput) (*mcp.CallToolResult, any, error) {
			execResult, err := svc.GetExecution(ctx, principal, application.GetExecution{ExecutionID: input.ExecutionID})
			if err != nil {
				return mapDomainError(err, principal), nil, nil
			}
			allowed, forbidden := parsePathConstraints(execResult.Data.TaskConstraints)
			result, err := submissions.CreateSubmission(ctx, gitPrincipal(principal), gitapp.CreateSubmission{
				RequestID:      input.RequestID,
				ExecutionID:    input.ExecutionID,
				TaskID:         execResult.Data.TaskID,
				Repo:           input.Repo,
				Branch:         input.Branch,
				CommitSHA:      input.CommitSHA,
				BaseCommitSHA:  input.BaseCommitSHA,
				Summary:        input.Summary,
				Tests:          input.Tests,
				Evidence:       []byte(input.Evidence),
				AllowedPaths:   allowed,
				ForbiddenPaths: forbidden,
			})
			if err != nil {
				return mapDomainError(err, principal), nil, nil
			}
			return successResult(result), nil, nil
		})

		mcp.AddTool(server, &mcp.Tool{
			Name:        "validation_get",
			Description: "查询 Submission 与验证状态",
		}, func(ctx context.Context, req *mcp.CallToolRequest, input GetSubmissionInput) (*mcp.CallToolResult, any, error) {
			result, err := submissions.GetSubmission(ctx, gitPrincipal(principal), gitapp.GetSubmission{SubmissionID: input.SubmissionID})
			if err != nil {
				return mapDomainError(err, principal), nil, nil
			}
			_, err = svc.GetExecution(ctx, principal, application.GetExecution{ExecutionID: result.Data.ExecutionID})
			if err != nil {
				return mapDomainError(err, principal), nil, nil
			}
			return successResult(result), nil, nil
		})
	}

	if credentials != nil {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "credential_issue",
			Description: "为执行签发短期 Git 凭证",
		}, func(ctx context.Context, req *mcp.CallToolRequest, input IssueCredentialInput) (*mcp.CallToolResult, any, error) {
			result, err := credentials.IssueCredential(ctx, gitPrincipal(principal), gitapp.IssueCredential{
				ExecutionID: input.ExecutionID,
				Repo:        input.Repo,
				Branch:      input.Branch,
				BaseCommit:  input.BaseCommit,
			})
			if err != nil {
				return mapDomainError(err, principal), nil, nil
			}
			return successResult(result), nil, nil
		})

		mcp.AddTool(server, &mcp.Tool{
			Name:        "credential_get",
			Description: "查询执行关联的凭证元数据",
		}, func(ctx context.Context, req *mcp.CallToolRequest, input GetCredentialInput) (*mcp.CallToolResult, any, error) {
			result, err := credentials.GetCredential(ctx, gitPrincipal(principal), gitapp.GetCredential{ExecutionID: input.ExecutionID})
			if err != nil {
				return mapDomainError(err, principal), nil, nil
			}
			return successResult(result), nil, nil
		})

		mcp.AddTool(server, &mcp.Tool{
			Name:        "credential_revoke",
			Description: "撤销执行关联的凭证",
		}, func(ctx context.Context, req *mcp.CallToolRequest, input RevokeCredentialInput) (*mcp.CallToolResult, any, error) {
			result, err := credentials.RevokeCredential(ctx, gitPrincipal(principal), gitapp.RevokeCredential{ExecutionID: input.ExecutionID})
			if err != nil {
				return mapDomainError(err, principal), nil, nil
			}
			return successResult(result), nil, nil
		})
	}
	wrapSchemaValidationErrors(server)
}

func gitPrincipal(p auth.Principal) gitapp.Principal {
	return gitapp.Principal{
		TenantID:       p.TenantID,
		OwnerID:        p.OwnerID,
		OwnerEmail:     p.OwnerEmail,
		IsAdmin:        p.IsAdmin,
		AgentID:        p.AgentID,
		AgentVersionID: p.AgentVersionID,
		Scopes:         p.Scopes,
		RepoScope:      p.RepoScope,
	}
}

func parsePathConstraints(constraints []string) (allowed, forbidden []string) {
	for _, c := range constraints {
		if strings.HasPrefix(c, "path:allowed:") {
			allowed = append(allowed, strings.TrimPrefix(c, "path:allowed:"))
			continue
		}
		if strings.HasPrefix(c, "path:forbidden:") {
			forbidden = append(forbidden, strings.TrimPrefix(c, "path:forbidden:"))
		}
	}
	return
}

// wrapSchemaValidationErrors 为 mcp.Server 增加接收中间件，把 SDK 在参数 schema 校验阶段
// 产生的原始文本错误转换为稳定的 INVALID_ARGUMENT JSON 错误内容。
func wrapSchemaValidationErrors(server *mcp.Server) {
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if method != "tools/call" || err != nil {
				return res, err
			}
			result, ok := res.(*mcp.CallToolResult)
			if !ok || result == nil || !result.IsError || len(result.Content) == 0 {
				return res, err
			}
			text, ok := result.Content[0].(*mcp.TextContent)
			if !ok || !strings.HasPrefix(text.Text, `validating "arguments":`) {
				return res, err
			}
			return errorResult(MCPError{
				Code:    "INVALID_ARGUMENT",
				Message: text.Text,
			}), nil
		}
	})
}
