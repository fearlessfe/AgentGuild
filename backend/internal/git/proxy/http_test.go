package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"github.com/stretchr/testify/require"
)

func TestEnforceReceivePackRefAllowsOnlyExecutionBranch(t *testing.T) {
	allowed := receivePackBody(
		"0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/agentguild/exec-1\x00report-status\n",
	)
	body, err := enforceReceivePackRef(strings.NewReader(allowed), "refs/heads/agentguild/exec-1")
	require.NoError(t, err)
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, allowed, string(got))
}

func TestEnforceReceivePackRefRejectsDefaultAndMultipleBranches(t *testing.T) {
	for name, body := range map[string]string{
		"default": receivePackBody("0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/main\x00report-status\n"),
		"multiple": receivePackBody(
			"0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/agentguild/exec-1\x00report-status\n",
			"0000000000000000000000000000000000000000 2222222222222222222222222222222222222222 refs/heads/other\n",
		),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := enforceReceivePackRef(strings.NewReader(body), "refs/heads/agentguild/exec-1")
			require.Error(t, err)
		})
	}
}

func receivePackBody(lines ...string) string {
	var body strings.Builder
	for _, line := range lines {
		body.WriteString(fmt.Sprintf("%04x%s", len(line)+4, line))
	}
	body.WriteString("0000PACK")
	return body.String()
}

// --- 拒绝路径审计日志测试 ---

type fakeGitStore struct {
	record *gitapp.CredentialRecord
	err    error
}

func (s *fakeGitStore) WithTx(_ context.Context, fn func(gitapp.Tx) error) error {
	return fn(&fakeGitTx{store: s})
}

type fakeGitTx struct{ store *fakeGitStore }

func (tx *fakeGitTx) Credentials() gitapp.CredentialRepository {
	return &fakeCredentialRepository{store: tx.store}
}
func (tx *fakeGitTx) Submissions() gitapp.SubmissionRepository       { return nil }
func (tx *fakeGitTx) ValidationJobs() gitapp.ValidationJobRepository { return nil }
func (tx *fakeGitTx) GitHubApps() gitapp.GitHubAppRepository         { return nil }
func (tx *fakeGitTx) Now(context.Context) (time.Time, error)         { return time.Now(), nil }

type fakeCredentialRepository struct{ store *fakeGitStore }

func (r *fakeCredentialRepository) GetByID(_ context.Context, tenantID, id string) (*gitapp.CredentialRecord, error) {
	if r.store.err != nil {
		return nil, r.store.err
	}
	if r.store.record == nil || r.store.record.TenantID != tenantID || r.store.record.ID != id {
		return nil, errors.New("credential not found")
	}
	return r.store.record, nil
}
func (r *fakeCredentialRepository) Insert(context.Context, *gitapp.CredentialRecord) error {
	return errors.New("not implemented")
}
func (r *fakeCredentialRepository) GetByExecutionID(context.Context, string, string) (*gitapp.CredentialRecord, error) {
	return nil, errors.New("not implemented")
}
func (r *fakeCredentialRepository) GetByExecutionIDForUpdate(context.Context, string, string) (*gitapp.CredentialRecord, error) {
	return nil, errors.New("not implemented")
}
func (r *fakeCredentialRepository) Update(context.Context, *gitapp.CredentialRecord) error {
	return errors.New("not implemented")
}
func (r *fakeCredentialRepository) Reactivate(context.Context, *gitapp.CredentialRecord) error {
	return errors.New("not implemented")
}
func (r *fakeCredentialRepository) Revoke(context.Context, string, string) error {
	return errors.New("not implemented")
}

type failingResolver struct{}

func (failingResolver) Driver(context.Context, string, string) (git.ResolvedDriver, error) {
	return git.ResolvedDriver{}, errors.New("resolver must not be called for rejected requests")
}
func (failingResolver) IssueSource(context.Context, string, string, string) (git.ResolvedIssueSource, error) {
	return git.ResolvedIssueSource{}, errors.New("resolver must not be called for rejected requests")
}

func activeCredential(token string) *gitapp.CredentialRecord {
	hash := sha256.Sum256([]byte(token))
	return &gitapp.CredentialRecord{
		ID: "cred-1", TenantID: "tenant-1", ExecutionID: "exec-1",
		Repo: "owner/repo", Branch: "agentguild/exec-1",
		Status: gitdomain.CredentialStatusActive, ExpiresAt: time.Now().Add(time.Hour),
		TokenHash: hash[:],
	}
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

func serveProxy(t *testing.T, store *fakeGitStore, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	handler, err := NewHandler(store, failingResolver{})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func TestProxyLogsCredentialRejectionWithoutLeakingToken(t *testing.T) {
	tests := []struct {
		name         string
		buildRequest func() *http.Request
		store        *fakeGitStore
		wantStatus   int
		wantReason   string
	}{
		{
			name: "token mismatch",
			buildRequest: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/git/tenant-1/cred-1/owner/repo.git/git-receive-pack", strings.NewReader("body"))
				req.SetBasicAuth("x-access-token", "bad-token")
				return req
			},
			store:      &fakeGitStore{record: activeCredential("good-token")},
			wantStatus: http.StatusUnauthorized,
			wantReason: "credential token mismatch",
		},
		{
			name: "repo mismatch",
			buildRequest: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/git/tenant-1/cred-1/owner/other.git/git-receive-pack", strings.NewReader("body"))
				req.SetBasicAuth("x-access-token", "good-token")
				return req
			},
			store:      &fakeGitStore{record: activeCredential("good-token")},
			wantStatus: http.StatusUnauthorized,
			wantReason: "credential repo mismatch",
		},
		{
			name: "unknown credential",
			buildRequest: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/git/tenant-1/cred-9/owner/repo.git/git-receive-pack", strings.NewReader("body"))
				req.SetBasicAuth("x-access-token", "good-token")
				return req
			},
			store:      &fakeGitStore{record: activeCredential("good-token")},
			wantStatus: http.StatusUnauthorized,
			wantReason: "credential lookup failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logs := captureLogs(t)
			recorder := serveProxy(t, test.store, test.buildRequest())
			require.Equal(t, test.wantStatus, recorder.Code)
			output := logs.String()
			require.Contains(t, output, "git proxy rejected credential")
			require.Contains(t, output, test.wantReason)
			require.Contains(t, output, `"tenant":"tenant-1"`)
			require.Contains(t, output, `"credential":"cred-`)
			require.NotContains(t, output, "bad-token", "日志不得包含 token 本体")
			require.NotContains(t, output, "good-token", "日志不得包含 token 本体")
		})
	}
}

func TestProxyLogsRepoAndExecutionOnCredentialRejection(t *testing.T) {
	logs := captureLogs(t)
	req := httptest.NewRequest(http.MethodPost, "/git/tenant-1/cred-1/owner/repo.git/git-receive-pack", strings.NewReader("body"))
	req.SetBasicAuth("x-access-token", "bad-token")
	recorder := serveProxy(t, &fakeGitStore{record: activeCredential("good-token")}, req)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	output := logs.String()
	require.Contains(t, output, `"execution":"exec-1"`)
	require.Contains(t, output, `"repo":"owner/repo"`)
}

func TestProxyLogsPushOutsideExecutionBranch(t *testing.T) {
	logs := captureLogs(t)
	body := receivePackBody("0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/main\x00report-status\n")
	req := httptest.NewRequest(http.MethodPost, "/git/tenant-1/cred-1/owner/repo.git/git-receive-pack", strings.NewReader(body))
	req.SetBasicAuth("x-access-token", "good-token")
	recorder := serveProxy(t, &fakeGitStore{record: activeCredential("good-token")}, req)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	output := logs.String()
	require.Contains(t, output, "git proxy rejected push")
	require.Contains(t, output, `"allowed_ref":"refs/heads/agentguild/exec-1"`)
	require.Contains(t, output, `"execution":"exec-1"`)
	require.Contains(t, output, `"tenant":"tenant-1"`)
	require.NotContains(t, output, "good-token", "日志不得包含 token 本体")
}

func TestProxyLogsDisallowedGitOperation(t *testing.T) {
	logs := captureLogs(t)
	req := httptest.NewRequest(http.MethodGet, "/git/tenant-1/cred-1/owner/repo.git/info/refs", nil)
	req.SetBasicAuth("x-access-token", "good-token")
	recorder := serveProxy(t, &fakeGitStore{record: activeCredential("good-token")}, req)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	output := logs.String()
	require.Contains(t, output, "git proxy rejected request")
	require.Contains(t, output, "git operation is not allowed")
	require.Contains(t, output, `"execution":"exec-1"`)
}

func TestProxyLogsMissingBasicAuth(t *testing.T) {
	logs := captureLogs(t)
	req := httptest.NewRequest(http.MethodGet, "/git/tenant-1/cred-1/owner/repo.git/info/refs?service=git-upload-pack", nil)
	recorder := serveProxy(t, &fakeGitStore{record: activeCredential("good-token")}, req)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	output := logs.String()
	require.Contains(t, output, "git proxy rejected request")
	require.Contains(t, output, "missing or malformed basic auth")
}
