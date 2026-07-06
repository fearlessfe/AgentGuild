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

func (f *fakeGitDriver) Driver(_ context.Context, _ string) (git.Driver, error) {
	return f, nil
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
			"head-sha":          {SHA: "head-sha"},
			"base-sha":          {SHA: "base-sha"},
			"agentguild/exe-1":  {SHA: "head-sha"},
			"head-sha-2":        {SHA: "head-sha-2"},
			"agentguild/exe-2":  {SHA: "head-sha-2"},
		},
		compareFiles: []git.ChangedFile{{Filename: "src/main.go", Status: "modified"}},
		ancestors: map[ancestorKey]bool{
			{base: "base-sha", head: "head-sha"}:   true,
			{base: "base-sha", head: "head-sha-2"}: true,
		},
	}
}

func newSubmissionService(t *testing.T, svc *application.Service, appService gitapp.GitHubAppService) *gitapp.SubmissionService {
	t.Helper()
	db := testdb.StartPostgres(t)
	store := gitpostgres.NewStore(db)
	verifier := gitapp.NewCommitVerifier(appService, gitpostgres.NewSubmissionRepository(db))
	subSvc, err := gitapp.NewSubmissionService(store, verifier, nil, nil)
	require.NoError(t, err)
	return subSvc
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
	DomainCode string
	SubmissionID string
	Status string
}

func createSubmissionViaREST(t *testing.T, svc *application.Service, subSvc *gitapp.SubmissionService, token, executionID, requestID string, commitSHA string) submissionOutcome {
	t.Helper()
	server := rest.NewServer(svc, fakeVerifier{}, rest.WithSubmissionService(subSvc)).Router()
	body := `{"request_id":"` + requestID + `","repo":"owner/repo","branch":"` + commitSHA + `","commit_sha":"` + commitSHA + `","base_commit_sha":"base-sha","summary":"fix parser"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/executions/"+executionID+"/submissions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", requestID)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		var resp struct {
			Error struct{ Code string `json:"code"` } `json:"error"`
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
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"submission_create","arguments":{"request_id":"` + requestID + `","execution_id":"` + executionID + `","repo":"owner/repo","branch":"` + commitSHA + `","commit_sha":"` + commitSHA + `","base_commit_sha":"base-sha","summary":"fix parser"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer token-agent-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var rpcResp struct {
		Result struct {
			Content []struct{ Text string `json:"text"` }
			IsError bool `json:"isError"`
		} `json:"result"`
		Error struct{ Code int `json:"code"` }
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rpcResp))
	if rpcResp.Error.Code != 0 {
		return submissionOutcome{DomainCode: "MCP_ERROR"}
	}
	if rpcResp.Result.IsError {
		var mcpErr struct{ Code string `json:"code"` }
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
			Error struct{ Code string `json:"code"` } `json:"error"`
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
	subSvc := newSubmissionService(t, svc, defaultGitDriver())
	taskID := publishTaskWithConstraints(t, svc)
	execID := startExecutionForAgent(t, svc, taskID)

	rest := createSubmissionViaREST(t, svc, subSvc, "token-agent-1", execID, "req-rest", "head-sha")
	mcp := createSubmissionViaMCP(t, svc, subSvc, execID, "req-mcp", "head-sha-2")

	require.Equal(t, "pending_verification", rest.Status)
	require.Equal(t, "pending_verification", mcp.Status)
	require.NotEmpty(t, rest.SubmissionID)
	require.NotEmpty(t, mcp.SubmissionID)
}

func TestRESTAndMCPSubmissionIdempotencyAreEquivalent(t *testing.T) {
	svc := newService(t)
	subSvc := newSubmissionService(t, svc, defaultGitDriver())
	taskID := publishTaskWithConstraints(t, svc)
	execID := startExecutionForAgent(t, svc, taskID)

	first := createSubmissionViaREST(t, svc, subSvc, "token-agent-1", execID, "idem-1", "head-sha")
	second := createSubmissionViaREST(t, svc, subSvc, "token-agent-1", execID, "idem-1", "head-sha")
	require.Equal(t, first.SubmissionID, second.SubmissionID)

	third := createSubmissionViaMCP(t, svc, subSvc, execID, "idem-2", "head-sha-2")
	fourth := createSubmissionViaMCP(t, svc, subSvc, execID, "idem-2", "head-sha-2")
	require.Equal(t, third.SubmissionID, fourth.SubmissionID)
}

func TestRESTAndMCPSubmissionOutOfBoundPathAreEquivalent(t *testing.T) {
	svc := newService(t)
	driver := defaultGitDriver()
	driver.compareFiles = []git.ChangedFile{{Filename: "README.md", Status: "modified"}}
	subSvc := newSubmissionService(t, svc, driver)
	taskID := publishTaskWithConstraints(t, svc)
	execID := startExecutionForAgent(t, svc, taskID)

	rest := createSubmissionViaREST(t, svc, subSvc, "token-agent-1", execID, "req-path", "head-sha")
	mcp := createSubmissionViaMCP(t, svc, subSvc, execID, "req-path2", "head-sha-2")

	require.Equal(t, "INVALID_ARGUMENT", rest.DomainCode)
	require.Equal(t, "INVALID_ARGUMENT", mcp.DomainCode)
}

func TestSubmissionQueryRejectsCrossOwner(t *testing.T) {
	svc := newService(t)
	subSvc := newSubmissionService(t, svc, defaultGitDriver())
	taskID := publishTaskWithConstraints(t, svc)
	execID := startExecutionForAgent(t, svc, taskID)
	rest := createSubmissionViaREST(t, svc, subSvc, "token-agent-1", execID, "req-owner", "head-sha")

	outcome := getSubmissionViaREST(t, svc, subSvc, "token-agent-2", rest.SubmissionID)
	require.Equal(t, "NOT_FOUND", outcome.DomainCode)
}

func TestSubmissionCreateRejectsNonOwnerExecution(t *testing.T) {
	svc := newService(t)
	subSvc := newSubmissionService(t, svc, defaultGitDriver())
	taskID := publishTaskWithConstraints(t, svc)

	// Claim by agent-1 but attempt to submit with agent-2 token.
	claim, err := svc.ClaimTask(context.Background(), agentPrincipal(), application.ClaimTask{RequestID: "claim-1", TaskID: taskID})
	require.NoError(t, err)

	outcome := createSubmissionViaREST(t, svc, subSvc, "token-agent-2", claim.Data.ID, "req-nonowner", "head-sha")
	require.Equal(t, "NOT_FOUND", outcome.DomainCode)
}

func TestRESTAndMCPGetSubmissionAreEquivalent(t *testing.T) {
	svc := newService(t)
	subSvc := newSubmissionService(t, svc, defaultGitDriver())
	taskID := publishTaskWithConstraints(t, svc)
	execID := startExecutionForAgent(t, svc, taskID)
	created := createSubmissionViaREST(t, svc, subSvc, "token-agent-1", execID, "req-get", "head-sha")

	rest := getSubmissionViaREST(t, svc, subSvc, "token-agent-1", created.SubmissionID)
	require.Equal(t, "pending_verification", rest.Status)
}
