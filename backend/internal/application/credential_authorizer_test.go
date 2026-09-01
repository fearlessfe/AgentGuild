package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	participationdomain "agentguild.dev/agentguild/backend/internal/participation/domain"
)

func TestAuthorizeCredentialUsesPersistedIssueRepository(t *testing.T) {
	svc, tx := credentialAuthorizerFixture(t, "agent-version-1")
	principal := credentialPrincipal("agent-version-1")

	grant, err := svc.AuthorizeCredential(context.Background(), principal, gitapp.IssueCredential{
		ExecutionID: "execution-1",
		Repo:        "acme/widgets",
		BaseCommit:  "base-sha",
	}, fixtureNow)

	if err != nil {
		t.Fatalf("AuthorizeCredential() error = %v", err)
	}
	if grant.Repo != "acme/widgets" || grant.BaseCommit != "" {
		t.Fatalf("grant = %#v", grant)
	}
	if tx.executions["execution-1"].AgentID != principal.AgentVersionID {
		t.Fatal("fixture did not bind execution to the Agent Version")
	}
}

func TestAuthorizeCredentialHidesAnotherAgentsExecution(t *testing.T) {
	svc, _ := credentialAuthorizerFixture(t, "agent-version-1")

	_, err := svc.AuthorizeCredential(context.Background(), credentialPrincipal("agent-version-2"), gitapp.IssueCredential{
		ExecutionID: "execution-1",
		Repo:        "acme/widgets",
		BaseCommit:  "base-sha",
	}, fixtureNow)

	if domain.CodeOf(err) != "not_found" {
		t.Fatalf("error = %v, want secure not_found", err)
	}
}

func TestAuthorizeCredentialRejectsExpiredLeaseAndRepositoryMismatch(t *testing.T) {
	svc, tx := credentialAuthorizerFixture(t, "agent-version-1")
	principal := credentialPrincipal("agent-version-1")

	_, err := svc.AuthorizeCredential(context.Background(), principal, gitapp.IssueCredential{
		ExecutionID: "execution-1",
		Repo:        "acme/other",
		BaseCommit:  "base-sha",
	}, fixtureNow)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("repository mismatch error = %v", err)
	}

	tx.executions["execution-1"].Lease.HardExpiry = fixtureNow
	_, err = svc.AuthorizeCredential(context.Background(), principal, gitapp.IssueCredential{
		ExecutionID: "execution-1",
		Repo:        "acme/widgets",
		BaseCommit:  "base-sha",
	}, fixtureNow)
	if !errors.Is(err, domain.ErrStateConflict) {
		t.Fatalf("expired lease error = %v", err)
	}
}

func TestAuthorizeSubmissionUsesParticipationGrantSponsorTenantForGlobalAgent(t *testing.T) {
	tx := newFakeTx()
	tx.seed(application.TaskRecord{
		ID: "task-1", TenantID: "tenant-sponsor",
		Constraints: []byte(`[]`), Requirements: []byte(`[]`), Deadline: fixtureNow.Add(time.Hour),
	})
	tx.executions["execution-1"] = &domain.Execution{
		ID: "execution-1", TaskID: "task-1", TenantID: "tenant-sponsor",
		AgentID: "global-version", Status: domain.ExecutionRunning,
		Lease: domain.Lease{HardExpiry: fixtureNow.Add(time.Hour)},
	}
	tx.seedLiveAgent("", "global-agent", identitydomain.AgentActive, "global-version")
	participation := &recordingParticipationAuthorizer{grant: &participationdomain.Grant{
		ResourceTenantID: "tenant-sponsor", TaskID: "task-1", ExecutionID: "execution-1",
	}}
	svc, err := application.NewService(&fakeStore{tx: tx}, application.Options{
		CursorSecret: []byte("01234567890123456789012345678901"),
		IssueSourceLookup: &fakeIssueSourceLookup{sources: map[string]application.TaskSource{
			"task-1": {Kind: "issue", Repo: "acme/widgets", IssueNumber: 42},
		}},
		Participation: participation,
	})
	if err != nil {
		t.Fatal(err)
	}
	principal := gitapp.Principal{
		SubjectID: auth.AgentSubject("global-agent"), IdentityScope: auth.IdentityScopeGlobal,
		AgentID: "global-agent", AgentVersionID: "global-version", Scopes: []string{"tasks:execute"},
	}
	grant, err := svc.AuthorizeSubmission(context.Background(), principal, gitapp.CreateSubmission{
		ExecutionID: "execution-1", TaskID: "task-1", Repo: "acme/widgets", BaseCommitSHA: "base-sha",
	}, fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	if grant.ResourceTenantID != "tenant-sponsor" || !grant.External || grant.TaskID != "task-1" {
		t.Fatalf("grant=%#v", grant)
	}
	if len(participation.calls) != 1 || participation.calls[0].kind != participationdomain.ResourceExecution || participation.calls[0].scope != participationdomain.ScopeSubmissionCreate {
		t.Fatalf("participation calls=%#v", participation.calls)
	}
}

func credentialAuthorizerFixture(t *testing.T, agentVersionID string) (*application.Service, *fakeTx) {
	t.Helper()
	tx := newFakeTx()
	tx.seed(application.TaskRecord{
		ID:           "task-1",
		TenantID:     "tenant-1",
		Constraints:  []byte(`[]`),
		Requirements: []byte(`[]`),
	})
	tx.executions["execution-1"] = &domain.Execution{
		ID:       "execution-1",
		TaskID:   "task-1",
		TenantID: "tenant-1",
		AgentID:  agentVersionID,
		Status:   domain.ExecutionRunning,
		Lease:    domain.Lease{HardExpiry: fixtureNow.Add(time.Minute)},
	}
	lookup := &fakeIssueSourceLookup{sources: map[string]application.TaskSource{
		"task-1": {Kind: "issue", Repo: "acme/widgets", IssueNumber: 42},
	}}
	svc, err := application.NewService(&fakeStore{tx: tx}, application.Options{
		CursorSecret:      []byte("01234567890123456789012345678901"),
		IssueSourceLookup: lookup,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, tx
}

func credentialPrincipal(agentVersionID string) gitapp.Principal {
	return gitapp.Principal{
		TenantID:       "tenant-1",
		AgentID:        "agent-1",
		AgentVersionID: agentVersionID,
		Scopes:         []string{"tasks:execute"},
		RepoScope:      []string{"acme/*"},
	}
}
