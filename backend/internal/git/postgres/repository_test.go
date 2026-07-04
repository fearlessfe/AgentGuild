package postgres_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
	"agentguild.dev/agentguild/backend/internal/git/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestCredentialRepositoryRoundTrip(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	record := sampleRecord("tenant-1", "exec-1")

	repo := postgres.NewCredentialRepository(db)
	require.NoError(t, repo.Insert(ctx, record))

	got, err := repo.GetByID(ctx, record.TenantID, record.ID)
	require.NoError(t, err)
	require.Equal(t, record.ID, got.ID)
	require.Equal(t, record.ExecutionID, got.ExecutionID)
	require.Equal(t, record.Provider, got.Provider)
	require.Equal(t, record.RepoURL, got.RepoURL)
	require.Equal(t, record.Branch, got.Branch)
	require.Equal(t, record.BaseCommit, got.BaseCommit)
	require.True(t, record.ExpiresAt.Equal(got.ExpiresAt))
	require.Nil(t, got.RevokedAt)

	byExecution, err := repo.GetByExecutionID(ctx, record.TenantID, record.ExecutionID)
	require.NoError(t, err)
	require.Equal(t, record.ID, byExecution.ID)
}

func TestCredentialRepositoryTenantIsolation(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	record := sampleRecord("tenant-1", "exec-1")
	require.NoError(t, postgres.NewCredentialRepository(db).Insert(ctx, record))

	repo := postgres.NewCredentialRepository(db)
	_, err := repo.GetByID(ctx, "tenant-2", record.ID)
	require.ErrorIs(t, err, git.ErrCredentialNotFound)

	_, err = repo.GetByExecutionID(ctx, "tenant-2", record.ExecutionID)
	require.ErrorIs(t, err, git.ErrCredentialNotFound)
}

func TestCredentialRepositoryUniqueConstraintOnExecution(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	repo := postgres.NewCredentialRepository(db)

	require.NoError(t, repo.Insert(ctx, sampleRecord("tenant-1", "exec-1")))
	err := repo.Insert(ctx, sampleRecord("tenant-1", "exec-1"))
	require.Error(t, err)
}

func TestCredentialRepositoryRevokeSetsRevokedAt(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	record := sampleRecord("tenant-1", "exec-1")

	store := postgres.NewStore(db)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.Credentials().Insert(ctx, record)
	}))

	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.Credentials().Revoke(ctx, record.TenantID, record.ExecutionID)
	}))

	got, err := postgres.NewCredentialRepository(db).GetByID(ctx, record.TenantID, record.ID)
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt)
}

func TestCredentialRepositoryRevokeIsIdempotent(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	record := sampleRecord("tenant-1", "exec-1")

	store := postgres.NewStore(db)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.Credentials().Insert(ctx, record)
	}))
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.Credentials().Revoke(ctx, record.TenantID, record.ExecutionID)
	}))

	err := store.WithTx(ctx, func(tx application.Tx) error {
		return tx.Credentials().Revoke(ctx, record.TenantID, record.ExecutionID)
	})
	require.ErrorIs(t, err, git.ErrCredentialNotFound)
}

func TestCredentialServiceIssuesCredentialThroughPostgresStore(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	svc, err := application.NewCredentialService(postgres.NewStore(db), application.Options{
		Issuer: &fakeIssuer{},
		NewID:  sequenceIDs("cred-1"),
	})
	require.NoError(t, err)

	got, err := svc.IssueCredential(ctx, application.Principal{
		TenantID: "tenant-1", OwnerID: "owner-1", OwnerEmail: "owner@example.com",
	}, application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.NoError(t, err)
	require.Equal(t, "tok-exec-1", got.Data.Token)
	require.Equal(t, "agentguild/exec-1", got.Data.Credential.Branch)

	repo := postgres.NewCredentialRepository(db)
	record, err := repo.GetByID(ctx, "tenant-1", "cred-1")
	require.NoError(t, err)
	require.Equal(t, "exec-1", record.ExecutionID)
	require.Empty(t, record.RevokedAt)
}

func TestCredentialServiceRevokeThroughPostgresStore(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	svc, err := application.NewCredentialService(postgres.NewStore(db), application.Options{
		Issuer: &fakeIssuer{},
		NewID:  sequenceIDs("cred-1"),
	})
	require.NoError(t, err)
	_, err = svc.IssueCredential(ctx, application.Principal{
		TenantID: "tenant-1", OwnerID: "owner-1", OwnerEmail: "owner@example.com",
	}, application.IssueCredential{
		ExecutionID: "exec-1", Repo: "owner/repo", BaseCommit: "abc",
	})
	require.NoError(t, err)

	got, err := svc.RevokeCredential(ctx, application.Principal{
		TenantID: "tenant-1", OwnerID: "owner-1",
	}, application.RevokeCredential{ExecutionID: "exec-1"})
	require.NoError(t, err)
	require.NotNil(t, got.Data.RevokedAt)

	record, err := postgres.NewCredentialRepository(db).GetByID(ctx, "tenant-1", "cred-1")
	require.NoError(t, err)
	require.NotNil(t, record.RevokedAt)
}

func sampleRecord(tenantID, executionID string) *application.CredentialRecord {
	return &application.CredentialRecord{
		ID:          "cred-" + executionID,
		TenantID:    tenantID,
		ExecutionID: executionID,
		Provider:    "github",
		RepoURL:     "https://github.com/owner/repo.git",
		Branch:      "agentguild/" + executionID,
		BaseCommit:  "abc",
		ExpiresAt:   time.Now().Add(15 * time.Minute).UTC().Truncate(time.Microsecond),
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}
}

type fakeIssuer struct{}

func (fakeIssuer) Issue(_ context.Context, tenantID, executionID, repo, branch, baseCommit string) (git.Credential, error) {
	_ = tenantID
	return git.Credential{
		Token:      "tok-" + executionID,
		RepoURL:    "https://github.com/" + repo + ".git",
		Branch:     branch,
		BaseCommit: baseCommit,
		ExpiresAt:  time.Now().Add(15 * time.Minute),
	}, nil
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
