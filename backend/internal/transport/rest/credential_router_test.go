package rest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	rest "agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func TestIssueCredentialRoute(t *testing.T) {
	svc := &fakeCredentialService{}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithCredentialService(svc),
	).Router()

	body, _ := json.Marshal(map[string]string{"repo": "owner/repo", "base_commit": "abc"})
	req := httptest.NewRequest(http.MethodPost, "/v1/executions/exec-1/credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token-agent-1")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	require.Len(t, svc.issueCalls, 1)
	require.Equal(t, "exec-1", svc.issueCalls[0].ExecutionID)
	require.Equal(t, "owner/repo", svc.issueCalls[0].Repo)
	require.Equal(t, "abc", svc.issueCalls[0].BaseCommit)
}

func TestGetCredentialRoute(t *testing.T) {
	svc := &fakeCredentialService{}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithCredentialService(svc),
	).Router()

	req := httptest.NewRequest(http.MethodGet, "/v1/executions/exec-1/credentials", nil)
	req.Header.Set("Authorization", "Bearer token-agent-1")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Len(t, svc.getCalls, 1)
	require.Equal(t, "exec-1", svc.getCalls[0].ExecutionID)
}

func TestRevokeCredentialRoute(t *testing.T) {
	svc := &fakeCredentialService{}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithCredentialService(svc),
	).Router()

	req := httptest.NewRequest(http.MethodDelete, "/v1/executions/exec-1/credentials", nil)
	req.Header.Set("Authorization", "Bearer token-agent-1")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Len(t, svc.revokeCalls, 1)
	require.Equal(t, "exec-1", svc.revokeCalls[0].ExecutionID)
}

func TestCredentialRoutesRequireAuthentication(t *testing.T) {
	svc := &fakeCredentialService{}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithCredentialService(svc),
	).Router()

	req := httptest.NewRequest(http.MethodPost, "/v1/executions/exec-1/credentials", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

type fakeCredentialService struct {
	issueCalls  []gitapp.IssueCredential
	getCalls    []gitapp.GetCredential
	revokeCalls []gitapp.RevokeCredential
}

func (f *fakeCredentialService) IssueCredential(_ context.Context, _ gitapp.Principal, cmd gitapp.IssueCredential) (gitapp.Envelope[gitapp.IssueCredentialResponse], error) {
	f.issueCalls = append(f.issueCalls, cmd)
	return gitapp.Envelope[gitapp.IssueCredentialResponse]{Data: gitapp.IssueCredentialResponse{
		Credential: gitapp.CredentialView{ID: "cred-1", ExecutionID: cmd.ExecutionID},
		Token:      "tok-1",
	}, Meta: gitapp.Meta{ServerTime: time.Now()}}, nil
}

func (f *fakeCredentialService) GetCredential(_ context.Context, _ gitapp.Principal, q gitapp.GetCredential) (gitapp.Envelope[gitapp.CredentialView], error) {
	f.getCalls = append(f.getCalls, q)
	return gitapp.Envelope[gitapp.CredentialView]{Data: gitapp.CredentialView{ID: "cred-1", ExecutionID: q.ExecutionID}}, nil
}

func (f *fakeCredentialService) RevokeCredential(_ context.Context, _ gitapp.Principal, cmd gitapp.RevokeCredential) (gitapp.Envelope[gitapp.CredentialView], error) {
	f.revokeCalls = append(f.revokeCalls, cmd)
	return gitapp.Envelope[gitapp.CredentialView]{Data: gitapp.CredentialView{ID: "cred-1", ExecutionID: cmd.ExecutionID}}, nil
}
