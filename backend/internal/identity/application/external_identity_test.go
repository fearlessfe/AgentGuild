package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/identity/domain"
)

func TestExternalIdentityServiceUsesStableSubjectAndRefreshesLogin(t *testing.T) {
	now := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	repo := &memoryExternalIdentityRepository{}
	verifier := &fixedExternalIdentityVerifier{identity: application.VerifiedProviderIdentity{
		Provider: domain.ExternalIdentityProviderGitHub, ProviderSubjectID: "node-123", Login: "old-login", VerifiedAt: now,
	}}
	service, err := application.NewExternalIdentityService(repo, verifier, func() string { return "external-1" })
	require.NoError(t, err)

	bound, err := service.Bind(context.Background(), "agent-1", domain.ExternalIdentityProviderGitHub, "proof")
	require.NoError(t, err)
	require.Equal(t, "node-123", bound.ProviderSubjectID)
	require.NoError(t, service.VerifyAttribution(context.Background(), "agent-1", domain.ExternalIdentityProviderGitHub, "node-123"))

	verifier.identity.Login = "new-login"
	verifier.identity.VerifiedAt = now.Add(time.Minute)
	refreshed, err := service.Bind(context.Background(), "agent-1", domain.ExternalIdentityProviderGitHub, "proof-2")
	require.NoError(t, err)
	require.Equal(t, "external-1", refreshed.ID)
	require.Equal(t, "new-login", refreshed.Login)
	require.Len(t, repo.identities, 1)
}

func TestExternalIdentityServiceRejectsDuplicateBindingAndUnverifiedAuthor(t *testing.T) {
	now := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	repo := &memoryExternalIdentityRepository{}
	verifier := &fixedExternalIdentityVerifier{identity: application.VerifiedProviderIdentity{
		Provider: domain.ExternalIdentityProviderGitHub, ProviderSubjectID: "node-123", Login: "bot", VerifiedAt: now,
	}}
	service, err := application.NewExternalIdentityService(repo, verifier, func() string { return "external-1" })
	require.NoError(t, err)
	_, err = service.Bind(context.Background(), "agent-1", domain.ExternalIdentityProviderGitHub, "proof")
	require.NoError(t, err)

	_, err = service.Bind(context.Background(), "agent-2", domain.ExternalIdentityProviderGitHub, "proof")
	require.ErrorIs(t, err, domain.ErrStateConflict)
	require.ErrorIs(t, service.VerifyAttribution(context.Background(), "agent-2", domain.ExternalIdentityProviderGitHub, "node-123"), domain.ErrForbidden)
	require.ErrorIs(t, service.VerifyAttribution(context.Background(), "agent-1", domain.ExternalIdentityProviderGitHub, "unknown"), domain.ErrForbidden)
}

func TestExternalIdentityServiceRevocationStopsAttribution(t *testing.T) {
	now := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	repo := &memoryExternalIdentityRepository{}
	verifier := &fixedExternalIdentityVerifier{identity: application.VerifiedProviderIdentity{
		Provider: domain.ExternalIdentityProviderGitLab, ProviderSubjectID: "subject-7", Login: "bot", VerifiedAt: now,
	}}
	service, err := application.NewExternalIdentityService(repo, verifier, func() string { return "external-1" })
	require.NoError(t, err)
	_, err = service.Bind(context.Background(), "agent-1", domain.ExternalIdentityProviderGitLab, "proof")
	require.NoError(t, err)
	require.NoError(t, service.Revoke(context.Background(), "agent-1", domain.ExternalIdentityProviderGitLab, "subject-7", now.Add(time.Minute)))
	require.ErrorIs(t, service.VerifyAttribution(context.Background(), "agent-1", domain.ExternalIdentityProviderGitLab, "subject-7"), domain.ErrForbidden)
}

type fixedExternalIdentityVerifier struct {
	identity application.VerifiedProviderIdentity
	err      error
}

func (v *fixedExternalIdentityVerifier) Verify(context.Context, domain.ExternalIdentityProvider, string) (application.VerifiedProviderIdentity, error) {
	return v.identity, v.err
}

type memoryExternalIdentityRepository struct {
	identities []*domain.ExternalIdentity
}

func (r *memoryExternalIdentityRepository) Insert(_ context.Context, identity *domain.ExternalIdentity) error {
	for _, existing := range r.identities {
		if existing.Provider == identity.Provider && existing.ProviderSubjectID == identity.ProviderSubjectID {
			return domain.ErrStateConflict
		}
	}
	copy := *identity
	r.identities = append(r.identities, &copy)
	return nil
}

func (r *memoryExternalIdentityRepository) GetByProviderSubject(_ context.Context, provider domain.ExternalIdentityProvider, subject string) (*domain.ExternalIdentity, error) {
	for _, identity := range r.identities {
		if identity.Provider == provider && identity.ProviderSubjectID == subject {
			copy := *identity
			return &copy, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *memoryExternalIdentityRepository) ListByAgent(_ context.Context, agentID string) ([]domain.ExternalIdentity, error) {
	var result []domain.ExternalIdentity
	for _, identity := range r.identities {
		if identity.AgentID == agentID {
			result = append(result, *identity)
		}
	}
	return result, nil
}

func (r *memoryExternalIdentityRepository) Update(_ context.Context, identity *domain.ExternalIdentity) error {
	for i, existing := range r.identities {
		if existing.ID == identity.ID {
			copy := *identity
			r.identities[i] = &copy
			return nil
		}
	}
	return domain.ErrNotFound
}

var _ application.ExternalIdentityVerifier = (*fixedExternalIdentityVerifier)(nil)
var _ application.ExternalIdentityRepository = (*memoryExternalIdentityRepository)(nil)
