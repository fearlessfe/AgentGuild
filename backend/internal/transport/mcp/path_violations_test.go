package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// 越界 submission 的 PathViolationError 必须在 MCP 结构化错误中透出违规项。
func TestSubmissionCreateReturnsPathViolations(t *testing.T) {
	app := &fakeApplication{
		getExecution: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{
				ID:              "exe-1",
				TaskID:          "task-1",
				TenantID:        "tenant-1",
				AgentVersionID:  "agent-1-v1",
				TaskConstraints: []string{"path:allowed:src/*"},
			},
		},
	}
	sub := &fakeSubmissionService{createErr: &gitapp.PathViolationError{
		DomainErr: &domain.Error{Code: "invalid_argument", Message: "changed paths violate path constraints", Field: "changed_paths"},
		Violations: []gitapp.PathViolation{
			{Path: "secrets/prod.env", Reason: "path is outside allowed set or matches a forbidden pattern"},
			{Path: "cmd/backdoor.go", Reason: "path is outside allowed set or matches a forbidden pattern"},
		},
	}}
	server := newMCPServerWithAppAndSubmissions(t, app, sub)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "submission_create",
		Arguments: map[string]any{
			"request_id":      "req-1",
			"execution_id":    "exe-1",
			"repo":            "owner/repo",
			"branch":          "agentguild/exe-1",
			"commit_sha":      "abc",
			"base_commit_sha": "def",
			"summary":         "fix",
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)

	var mcpErr MCPError
	require.NoError(t, json.Unmarshal([]byte(text.Text), &mcpErr))
	require.Equal(t, "INVALID_ARGUMENT", mcpErr.Code)
	require.Equal(t, "changed paths violate path constraints", mcpErr.Message)
	require.Len(t, mcpErr.Violations, 2)
	require.Equal(t, ViolationView{Path: "secrets/prod.env", Reason: "path is outside allowed set or matches a forbidden pattern"}, mcpErr.Violations[0])
	require.Equal(t, "cmd/backdoor.go", mcpErr.Violations[1].Path)
}

// 普通 invalid_argument 错误不得携带 violations 键。
func TestSubmissionCreateInvalidArgumentOmitsViolations(t *testing.T) {
	app := &fakeApplication{
		getExecution: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{ID: "exe-1", TaskID: "task-1", AgentVersionID: "agent-1-v1"},
		},
	}
	sub := &fakeSubmissionService{createErr: &domain.Error{Code: "invalid_argument", Message: "commit not found", Field: "commit_sha"}}
	server := newMCPServerWithAppAndSubmissions(t, app, sub)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "submission_create",
		Arguments: map[string]any{
			"request_id":      "req-1",
			"execution_id":    "exe-1",
			"repo":            "owner/repo",
			"branch":          "agentguild/exe-1",
			"commit_sha":      "abc",
			"base_commit_sha": "def",
			"summary":         "fix",
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
	require.Equal(t, "INVALID_ARGUMENT", payload["code"])
	require.NotContains(t, payload, "violations")
}
