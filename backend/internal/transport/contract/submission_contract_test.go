package contract_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	gitpostgres "agentguild.dev/agentguild/backend/internal/git/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	transportmcp "agentguild.dev/agentguild/backend/internal/transport/mcp"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

// fakeGitDriver is a minimal git.Driver for contract tests.
type fakeGitDriver struct {
	commits      map[string]git.Commit
	compareFiles []git.ChangedFile
	ancestors    map[ancestorKey]bool
}

type ancestorKey struct{ base, head string }

func (f *fakeGitDriver) Driver(_ context.Context, _, repo string) (git.ResolvedDriver, error) {
	return git.ResolvedDriver{Driver: f, FullName: repo}, nil
}

func (f *fakeGitDriver) IssueSource(context.Context, string, string, string) (git.ResolvedIssueSource, error) {
	return git.ResolvedIssueSource{}, nil
}

func (f *fakeGitDriver) CreateCredential(_ context.Context, _, _, _ string) (git.Credential, error) {
	return git.Credential{Token: "fake", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (f *fakeGitDriver) GetCommit(_ context.Context, _, sha string) (git.Commit, error) {
	c, ok := f.commits[sha]
	if !ok {
		return git.Commit{}, git.ErrRepoNotFound
	}
	return c, nil
}

func (f *fakeGitDriver) CompareCommits(_ context.Context, _, _, _ string) ([]git.ChangedFile, error) {
	return f.compareFiles, nil
}

func (f *fakeGitDriver) IsAncestor(_ context.Context, _, base, head string) (bool, error) {
	if base == head {
		return true, nil
	}
	return f.ancestors[ancestorKey{base: base, head: head}], nil
}

func defaultGitDriver() *fakeGitDriver {
	return &fakeGitDriver{
		commits: map[string]git.Commit{
			"head-sha":   {SHA: "head-sha"},
			"base-sha":   {SHA: "base-sha"},
			"head-sha-2": {SHA: "head-sha-2"},
		},
		compareFiles: []git.ChangedFile{{Filename: "src/main.go", Status: "modified"}},
		ancestors: map[ancestorKey]bool{
			{base: "base-sha", head: "head-sha"}:   true,
			{base: "base-sha", head: "head-sha-2"}: true,
		},
	}
}

func newSubmissionService(t *testing.T, svc *application.Service, resolver gitapp.RepositoryGitResolver) (*gitapp.SubmissionService, *gitapp.CredentialService) {
	t.Helper()
	db := testdb.StartPostgres(t)
	store := gitpostgres.NewStore(db)
	verifier := gitapp.NewCommitVerifier(resolver, gitpostgres.NewSubmissionRepository(db))
	authorizer := gitapp.SubmissionAuthorizerFunc(func(_ context.Context, _ gitapp.Principal, cmd gitapp.CreateSubmission, _ time.Time) (gitapp.SubmissionGrant, error) {
		return gitapp.SubmissionGrant{TaskID: cmd.TaskID, Repo: cmd.Repo, BaseCommit: cmd.BaseCommitSHA, AllowedPaths: cmd.AllowedPaths, ForbiddenPaths: cmd.ForbiddenPaths}, nil
	})
	subSvc, err := gitapp.NewSubmissionService(store, verifier, nil, authorizer, nil)
	require.NoError(t, err)
	credSvc, err := gitapp.NewCredentialService(store, resolver, gitapp.Options{
		Authorizer: gitapp.CredentialGrantAuthorizerFunc(func(_ context.Context, _ gitapp.Principal, cmd gitapp.IssueCredential, _ time.Time) (gitapp.CredentialGrant, error) {
			return gitapp.CredentialGrant{Repo: cmd.Repo, BaseCommit: cmd.BaseCommit}, nil
		}),
		ProxyBaseURL: "https://agentguild.example",
		TokenSecret:  []byte("0123456789abcdef0123456789abcdef"),
	})
	require.NoError(t, err)
	return subSvc, credSvc
}

// issueCredential 为执行签发活跃凭证；submission 创建要求凭证绑定该执行的
// repo、agentguild/<execution> 分支与 base commit。
func issueCredential(t *testing.T, credSvc *gitapp.CredentialService, execID, requestID string) {
	t.Helper()
	_, err := credSvc.IssueCredential(context.Background(), gitapp.Principal{
		TenantID: "tenant-1", AgentID: "agent-1", AgentVersionID: "agent-1-v1",
		Scopes: []string{"tasks:execute"}, RepoScope: []string{"owner/*"},
	}, gitapp.IssueCredential{
		RequestID: requestID, ExecutionID: execID, Repo: "owner/repo", BaseCommit: "base-sha",
	})
	require.NoError(t, err)
}

func publishTaskWithConstraints(t *testing.T, svc *application.Service) string {
	t.Helper()
	result, err := svc.PublishTask(context.Background(), publisherPrincipal(), application.PublishTask{
		RequestID:   "pub-1",
		Type:        "code",
		Title:       "Fix parser",
		Problem:     "It races",
		Constraints: []string{"path:allowed:src/*"},
		Deadline:    time.Now().Add(24 * time.Hour),
	})
	require.NoError(t, err)
	return result.Data.ID
}

func startExecutionForAgent(t *testing.T, svc *application.Service, taskID string) string {
	t.Helper()
	claim, err := svc.ClaimTask(context.Background(), agentPrincipal(), application.ClaimTask{RequestID: "claim-" + taskID, TaskID: taskID})
	require.NoError(t, err)
	_, err = svc.StartExecution(context.Background(), agentPrincipal(), application.StartExecution{
		RequestID:       "start-" + taskID,
		ExecutionID:     claim.Data.ID,
		LeaseGeneration: claim.Data.LeaseGeneration,
	})
	require.NoError(t, err)
	return claim.Data.ID
}

type submissionOutcome struct {
	DomainCode   string
	SubmissionID string
	Status       string
}

func createSubmissionViaREST(t *testing.T, svc *application.Service, subSvc *gitapp.SubmissionService, token, executionID, requestID string, commitSHA string) submissionOutcome {
	t.Helper()
	server := rest.NewServer(svc, fakeVerifier{}, rest.WithSubmissionService(subSvc)).Router()
	body := `{"request_id":"` + requestID + `","repo":"owner/repo","branch":"agentguild/` + executionID + `","commit_sha":"` + commitSHA + `","base_commit_sha":"base-sha","summary":"fix parser"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/executions/"+executionID+"/submissions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", requestID)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		var resp struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return submissionOutcome{DomainCode: resp.Error.Code}
	}
	var envelope struct {
		Data gitapp.SubmissionView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	return submissionOutcome{SubmissionID: envelope.Data.ID, Status: string(envelope.Data.Status)}
}

func createSubmissionViaMCP(t *testing.T, svc *application.Service, subSvc *gitapp.SubmissionService, executionID, requestID string, commitSHA string) submissionOutcome {
	t.Helper()
	mcpServer := transportmcp.NewServer(svc, fakeVerifier{}, transportmcp.WithSubmissionService(subSvc))
	handler := mcpServer.Handler()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"submission_create","arguments":{"request_id":"` + requestID + `","execution_id":"` + executionID + `","repo":"owner/repo","branch":"agentguild/` + executionID + `","commit_sha":"` + commitSHA + `","base_commit_sha":"base-sha","summary":"fix parser"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer token-agent-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var rpcResp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			}
			IsError bool `json:"isError"`
		} `json:"result"`
		Error struct {
			Code int `json:"code"`
		}
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rpcResp))
	if rpcResp.Error.Code != 0 {
		return submissionOutcome{DomainCode: "MCP_ERROR"}
	}
	if rpcResp.Result.IsError {
		var mcpErr struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &mcpErr))
		return submissionOutcome{DomainCode: mcpErr.Code}
	}
	var envelope struct {
		Data gitapp.SubmissionView `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &envelope))
	return submissionOutcome{SubmissionID: envelope.Data.ID, Status: string(envelope.Data.Status)}
}

func getSubmissionViaREST(t *testing.T, svc *application.Service, subSvc *gitapp.SubmissionService, token, submissionID string) submissionOutcome {
	t.Helper()
	server := rest.NewServer(svc, fakeVerifier{}, rest.WithSubmissionService(subSvc)).Router()
	req := httptest.NewRequest(http.MethodGet, "/v1/submissions/"+submissionID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		var resp struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return submissionOutcome{DomainCode: resp.Error.Code}
	}
	var envelope struct {
		Data gitapp.SubmissionView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	return submissionOutcome{SubmissionID: envelope.Data.ID, Status: string(envelope.Data.Status)}
}

func TestRESTAndMCPCreateSubmissionAreEquivalent(t *testing.T) {
	svc := newService(t)
	driver := defaultGitDriver()
	subSvc, credSvc := newSubmissionService(t, svc, driver)
	taskID := publishTaskWithConstraints(t, svc)
	execID := startExecutionForAgent(t, svc, taskID)
	branch := "agentguild/" + execID

	issueCredential(t, credSvc, execID, "cred-rest")
	driver.commits[branch] = git.Commit{SHA: "head-sha"}
	rest := createSubmissionViaREST(t, svc, subSvc, "token-agent-1", execID, "req-rest", "head-sha")
	// 提交后凭证即被撤销，再次提交需要重新签发。
	issueCredential(t, credSvc, execID, "cred-mcp")
	driver.commits[branch] = git.Commit{SHA: "head-sha-2"}
	mcp := createSubmissionViaMCP(t, svc, subSvc, execID, "req-mcp", "head-sha-2")

	require.Equal(t, "pending_verification", rest.Status)
	require.Equal(t, "pending_verification", mcp.Status)
	require.NotEmpty(t, rest.SubmissionID)
	require.NotEmpty(t, mcp.SubmissionID)
}

func TestRESTAndMCPSubmissionIdempotencyAreEquivalent(t *testing.T) {
	svc := newService(t)
	driver := defaultGitDriver()
	subSvc, credSvc := newSubmissionService(t, svc, driver)
	taskID := publishTaskWithConstraints(t, svc)
	execID := startExecutionForAgent(t, svc, taskID)
	branch := "agentguild/" + execID

	issueCredential(t, credSvc, execID, "cred-rest")
	driver.commits[branch] = git.Commit{SHA: "head-sha"}
	first := createSubmissionViaREST(t, svc, subSvc, "token-agent-1", execID, "idem-1", "head-sha")
	second := createSubmissionViaREST(t, svc, subSvc, "token-agent-1", execID, "idem-1", "head-sha")
	require.NotEmpty(t, first.SubmissionID)
	require.Equal(t, first.SubmissionID, second.SubmissionID)

	issueCredential(t, credSvc, execID, "cred-mcp")
	driver.commits[branch] = git.Commit{SHA: "head-sha-2"}
	third := createSubmissionViaMCP(t, svc, subSvc, execID, "idem-2", "head-sha-2")
	fourth := createSubmissionViaMCP(t, svc, subSvc, execID, "idem-2", "head-sha-2")
	require.NotEmpty(t, third.SubmissionID)
	require.Equal(t, third.SubmissionID, fourth.SubmissionID)
}

func TestRESTAndMCPSubmissionOutOfBoundPathAreEquivalent(t *testing.T) {
	svc := newService(t)
	driver := defaultGitDriver()
	driver.compareFiles = []git.ChangedFile{{Filename: "README.md", Status: "modified"}}
	subSvc, credSvc := newSubmissionService(t, svc, driver)
	taskID := publishTaskWithConstraints(t, svc)
	execID := startExecutionForAgent(t, svc, taskID)
	branch := "agentguild/" + execID

	issueCredential(t, credSvc, execID, "cred-path")
	driver.commits[branch] = git.Commit{SHA: "head-sha"}
	rest := createSubmissionViaREST(t, svc, subSvc, "token-agent-1", execID, "req-path", "head-sha")
	// 路径校验失败会回滚事务，凭证仍然有效，无需重新签发。
	driver.commits[branch] = git.Commit{SHA: "head-sha-2"}
	mcp := createSubmissionViaMCP(t, svc, subSvc, execID, "req-path2", "head-sha-2")

	require.Equal(t, "INVALID_ARGUMENT", rest.DomainCode)
	require.Equal(t, "INVALID_ARGUMENT", mcp.DomainCode)
}

func TestSubmissionQueryRejectsCrossOwner(t *testing.T) {
	svc := newService(t)
	driver := defaultGitDriver()
	subSvc, credSvc := newSubmissionService(t, svc, driver)
	taskID := publishTaskWithConstraints(t, svc)
	execID := startExecutionForAgent(t, svc, taskID)
	issueCredential(t, credSvc, execID, "cred-owner")
	driver.commits["agentguild/"+execID] = git.Commit{SHA: "head-sha"}
	rest := createSubmissionViaREST(t, svc, subSvc, "token-agent-1", execID, "req-owner", "head-sha")
	require.NotEmpty(t, rest.SubmissionID)

	outcome := getSubmissionViaREST(t, svc, subSvc, "token-agent-2", rest.SubmissionID)
	require.Equal(t, "NOT_FOUND", outcome.DomainCode)
}

func TestSubmissionCreateRejectsNonOwnerExecution(t *testing.T) {
	svc := newService(t)
	subSvc, _ := newSubmissionService(t, svc, defaultGitDriver())
	taskID := publishTaskWithConstraints(t, svc)

	// Claim by agent-1 but attempt to submit with agent-2 token.
	claim, err := svc.ClaimTask(context.Background(), agentPrincipal(), application.ClaimTask{RequestID: "claim-1", TaskID: taskID})
	require.NoError(t, err)

	outcome := createSubmissionViaREST(t, svc, subSvc, "token-agent-2", claim.Data.ID, "req-nonowner", "head-sha")
	require.Equal(t, "NOT_FOUND", outcome.DomainCode)
}

func TestRESTAndMCPGetSubmissionAreEquivalent(t *testing.T) {
	svc := newService(t)
	driver := defaultGitDriver()
	subSvc, credSvc := newSubmissionService(t, svc, driver)
	taskID := publishTaskWithConstraints(t, svc)
	execID := startExecutionForAgent(t, svc, taskID)
	issueCredential(t, credSvc, execID, "cred-get")
	driver.commits["agentguild/"+execID] = git.Commit{SHA: "head-sha"}
	created := createSubmissionViaREST(t, svc, subSvc, "token-agent-1", execID, "req-get", "head-sha")
	require.NotEmpty(t, created.SubmissionID)

	rest := getSubmissionViaREST(t, svc, subSvc, "token-agent-1", created.SubmissionID)
	require.Equal(t, "pending_verification", rest.Status)
}
