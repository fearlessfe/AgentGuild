package application_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
	"github.com/stretchr/testify/require"
)

func TestIssueCredentialRequiresTenantAndAuthorizedCaller(t *testing.T) {
	fixture := newCredentialFixture(t)

	_, err := fixture.svc.IssueCredential(context.Background(), application.Principal{}, application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.ErrorIs(t, err, domain.ErrForbidden)

	_, err = fixture.svc.IssueCredential(context.Background(), application.Principal{TenantID: "tenant-1"}, application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestIssueCredentialRejectsNonRestrictedBranch(t *testing.T) {
	fixture := newCredentialFixture(t)

	_, err := fixture.svc.IssueCredential(context.Background(), ownerPrincipal(), application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", Branch: "main", BaseCommit: "abc",
	})
	require.ErrorIs(t, err, git.ErrInvalidBranch)
}

func TestIssueCredentialReturnsTokenAndPersistsMetadata(t *testing.T) {
	fixture := newCredentialFixture(t)

	got, err := fixture.svc.IssueCredential(context.Background(), ownerPrincipal(), application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})

	require.NoError(t, err)
	require.Equal(t, "tok-exec-1-a", got.Data.Token)
	require.Equal(t, "tenant-1", got.Data.Credential.TenantID)
	require.Equal(t, "exec-1", got.Data.Credential.ExecutionID)
	require.Equal(t, "github", got.Data.Credential.Provider)
	require.Equal(t, "agentguild/exec-1", got.Data.Credential.Branch)
	require.Equal(t, "abc", got.Data.Credential.BaseCommit)
	require.True(t, got.Data.Credential.ExpiresAt.After(fixture.now))

	record := fixture.store.credentialByExecution("tenant-1", "exec-1")
	require.NotNil(t, record)
	require.Equal(t, "agentguild/exec-1", record.Branch)
	require.Empty(t, record.RevokedAt)
}

func TestIssueCredentialUpdatesExistingExecutionMetadata(t *testing.T) {
	fixture := newCredentialFixture(t)
	first, err := fixture.svc.IssueCredential(context.Background(), ownerPrincipal(), application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.NoError(t, err)

	second, err := fixture.svc.IssueCredential(context.Background(), ownerPrincipal(), application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", Branch: "agentguild/exec-1", BaseCommit: "def",
	})

	require.NoError(t, err)
	require.Equal(t, first.Data.Credential.ID, second.Data.Credential.ID)
	require.NotEqual(t, first.Data.Token, second.Data.Token)
	require.Equal(t, "def", second.Data.Credential.BaseCommit)
	require.Equal(t, "agentguild/exec-1", second.Data.Credential.Branch)
}

func TestIssueCredentialRejectsReissueAfterRevoke(t *testing.T) {
	fixture := newCredentialFixture(t)
	_, err := fixture.svc.IssueCredential(context.Background(), ownerPrincipal(), application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.NoError(t, err)

	_, err = fixture.svc.RevokeCredential(context.Background(), ownerPrincipal(), application.RevokeCredential{ExecutionID: "exec-1"})
	require.NoError(t, err)

	_, err = fixture.svc.IssueCredential(context.Background(), ownerPrincipal(), application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.ErrorIs(t, err, git.ErrCredentialRevoked)
}

func TestRevokeCredentialSetsRevokedAt(t *testing.T) {
	fixture := newCredentialFixture(t)
	issued, err := fixture.svc.IssueCredential(context.Background(), ownerPrincipal(), application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.NoError(t, err)

	got, err := fixture.svc.RevokeCredential(context.Background(), ownerPrincipal(), application.RevokeCredential{ExecutionID: "exec-1"})

	require.NoError(t, err)
	require.NotNil(t, got.Data.RevokedAt)
	require.Equal(t, issued.Data.Credential.ID, got.Data.ID)
}

func TestRevokeCredentialRequiresOwnerOrAdmin(t *testing.T) {
	fixture := newCredentialFixture(t)
	_, err := fixture.svc.IssueCredential(context.Background(), ownerPrincipal(), application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.NoError(t, err)

	_, err = fixture.svc.RevokeCredential(context.Background(), application.Principal{
		TenantID: "tenant-1", AgentID: "agent-1", Scopes: []string{"tasks:execute"},
	}, application.RevokeCredential{ExecutionID: "exec-1"})
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestTenantIsolationHidesForeignCredential(t *testing.T) {
	fixture := newCredentialFixture(t)
	_, err := fixture.svc.IssueCredential(context.Background(), ownerPrincipal(), application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.NoError(t, err)

	_, err = fixture.svc.GetCredential(context.Background(), application.Principal{
		TenantID: "tenant-2", OwnerID: "owner-1",
	}, application.GetCredential{ExecutionID: "exec-1"})
	require.ErrorIs(t, err, git.ErrCredentialNotFound)
}

func TestAgentWithExecuteScopeCanIssueCredential(t *testing.T) {
	fixture := newCredentialFixture(t)

	got, err := fixture.svc.IssueCredential(context.Background(), application.Principal{
		TenantID: "tenant-1", AgentID: "agent-1", Scopes: []string{"tasks:execute"},
	}, application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})

	require.NoError(t, err)
	require.Equal(t, "exec-1", got.Data.Credential.ExecutionID)
}

func TestRevokeCredentialIsIdempotent(t *testing.T) {
	fixture := newCredentialFixture(t)
	issued, err := fixture.svc.IssueCredential(context.Background(), ownerPrincipal(), application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.NoError(t, err)

	_, err = fixture.svc.RevokeCredential(context.Background(), ownerPrincipal(), application.RevokeCredential{ExecutionID: "exec-1"})
	require.NoError(t, err)

	got, err := fixture.svc.RevokeCredential(context.Background(), ownerPrincipal(), application.RevokeCredential{ExecutionID: "exec-1"})
	require.NoError(t, err)
	require.NotNil(t, got.Data.RevokedAt)
	require.Equal(t, issued.Data.Credential.ID, got.Data.ID)
}

func TestGetCredentialRequiresAuthorizedCaller(t *testing.T) {
	fixture := newCredentialFixture(t)
	_, err := fixture.svc.IssueCredential(context.Background(), ownerPrincipal(), application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.NoError(t, err)

	_, err = fixture.svc.GetCredential(context.Background(), application.Principal{}, application.GetCredential{ExecutionID: "exec-1"})
	require.ErrorIs(t, err, domain.ErrForbidden)

	_, err = fixture.svc.GetCredential(context.Background(), application.Principal{TenantID: "tenant-1"}, application.GetCredential{ExecutionID: "exec-1"})
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestIssueCredentialDoesNotIssueTokenForRevokedCredential(t *testing.T) {
	fixture := newCredentialFixture(t)

	_, err := fixture.svc.IssueCredential(context.Background(), ownerPrincipal(), application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.NoError(t, err)
	require.Equal(t, 1, fixture.issuer.counter)

	_, err = fixture.svc.RevokeCredential(context.Background(), ownerPrincipal(), application.RevokeCredential{ExecutionID: "exec-1"})
	require.NoError(t, err)

	_, err = fixture.svc.IssueCredential(context.Background(), ownerPrincipal(), application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.ErrorIs(t, err, git.ErrCredentialRevoked)
	require.Equal(t, 1, fixture.issuer.counter)
}

func ownerPrincipal() application.Principal {
	return application.Principal{TenantID: "tenant-1", OwnerID: "owner-1", OwnerEmail: "owner@example.com"}
}

type credentialFixture struct {
	svc    *application.CredentialService
	store  *memoryStore
	issuer *fakeIssuer
	now    time.Time
}

func newCredentialFixture(t *testing.T) *credentialFixture {
	t.Helper()
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	store := newMemoryStore(now)
	issuer := &fakeIssuer{}
	svc, err := application.NewCredentialService(store, application.Options{
		Issuer: issuer,
		NewID:  sequenceIDs("cred-1"),
	})
	require.NoError(t, err)
	return &credentialFixture{svc: svc, store: store, issuer: issuer, now: now}
}

type fakeIssuer struct {
	counter int
}

func (f *fakeIssuer) Issue(_ context.Context, tenantID, executionID, repo, branch, baseCommit string) (git.Credential, error) {
	_ = tenantID
	f.counter++
	return git.Credential{
		Token:      "tok-" + executionID + "-" + string(rune('a'+f.counter-1)),
		RepoURL:    "https://github.com/" + repo + ".git",
		Branch:     branch,
		BaseCommit: baseCommit,
		ExpiresAt:  time.Now().Add(15 * time.Minute),
	}, nil
}

type memoryStore struct {
	now         time.Time
	credentials map[string]*application.CredentialRecord
}

func newMemoryStore(now time.Time) *memoryStore {
	return &memoryStore{now: now, credentials: map[string]*application.CredentialRecord{}}
}

func (s *memoryStore) WithTx(ctx context.Context, fn func(application.Tx) error) error {
	return fn(&memoryTx{ctx: ctx, store: s, now: s.now})
}

type memoryTx struct {
	ctx   context.Context
	store *memoryStore
	now   time.Time
}

func (tx *memoryTx) Credentials() application.CredentialRepository {
	return &memoryCredentialRepository{store: tx.store, now: tx.now}
}

func (tx *memoryTx) Now(context.Context) (time.Time, error) { return tx.now, nil }

type memoryCredentialRepository struct {
	store *memoryStore
	now   time.Time
}

func (r *memoryCredentialRepository) Insert(_ context.Context, record *application.CredentialRecord) error {
	if r.store.credentialByExecution(record.TenantID, record.ExecutionID) != nil {
		return git.ErrAlreadyIssued
	}
	r.store.credentials[record.ID] = cloneRecord(record)
	return nil
}

func (r *memoryCredentialRepository) GetByID(_ context.Context, tenantID, id string) (*application.CredentialRecord, error) {
	record := r.store.credentials[id]
	if record == nil || record.TenantID != tenantID {
		return nil, git.ErrCredentialNotFound
	}
	return cloneRecord(record), nil
}

func (r *memoryCredentialRepository) GetByExecutionID(_ context.Context, tenantID, executionID string) (*application.CredentialRecord, error) {
	record := r.store.credentialByExecution(tenantID, executionID)
	if record == nil {
		return nil, git.ErrCredentialNotFound
	}
	return cloneRecord(record), nil
}

func (r *memoryCredentialRepository) Update(_ context.Context, record *application.CredentialRecord) error {
	existing := r.store.credentials[record.ID]
	if existing == nil || existing.TenantID != record.TenantID {
		return git.ErrCredentialNotFound
	}
	if existing.RevokedAt != nil {
		return git.ErrCredentialRevoked
	}
	r.store.credentials[record.ID] = cloneRecord(record)
	return nil
}

func (r *memoryCredentialRepository) Revoke(_ context.Context, tenantID, executionID string) error {
	record := r.store.credentialByExecution(tenantID, executionID)
	if record == nil {
		return git.ErrCredentialNotFound
	}
	if record.RevokedAt != nil {
		return git.ErrCredentialNotFound
	}
	revoked := cloneRecord(record)
	revoked.RevokedAt = &r.now
	r.store.credentials[record.ID] = revoked
	return nil
}

func (s *memoryStore) credentialByExecution(tenantID, executionID string) *application.CredentialRecord {
	for _, record := range s.credentials {
		if record.TenantID == tenantID && record.ExecutionID == executionID {
			return record
		}
	}
	return nil
}

func cloneRecord(record *application.CredentialRecord) *application.CredentialRecord {
	if record == nil {
		return nil
	}
	clone := *record
	if record.RevokedAt != nil {
		v := *record.RevokedAt
		clone.RevokedAt = &v
	}
	return &clone
}

func sequenceIDs(values ...string) func() string {
	next := 0
	return func() string {
		if next >= len(values) {
			return values[len(values)-1]
		}
		value := values[next]
		next++
		return value
	}
}
