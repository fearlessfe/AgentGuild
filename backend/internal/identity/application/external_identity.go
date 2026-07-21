package application

import (
	"context"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/domain"
)

// VerifiedProviderIdentity is returned by a provider-specific proof verifier.
// The opaque proof itself must never be persisted.
type VerifiedProviderIdentity struct {
	Provider          domain.ExternalIdentityProvider
	ProviderSubjectID string
	Login             string
	VerifiedAt        time.Time
}

type ExternalIdentityVerifier interface {
	Verify(context.Context, domain.ExternalIdentityProvider, string) (VerifiedProviderIdentity, error)
}

type ExternalIdentityRepository interface {
	Insert(context.Context, *domain.ExternalIdentity) error
	GetByProviderSubject(context.Context, domain.ExternalIdentityProvider, string) (*domain.ExternalIdentity, error)
	ListByAgent(context.Context, string) ([]domain.ExternalIdentity, error)
	Update(context.Context, *domain.ExternalIdentity) error
}

type ExternalIdentityService struct {
	repository ExternalIdentityRepository
	verifier   ExternalIdentityVerifier
	newID      func() string
}

func NewExternalIdentityService(repository ExternalIdentityRepository, verifier ExternalIdentityVerifier, newID func() string) (*ExternalIdentityService, error) {
	if repository == nil {
		return nil, invalid("external_identity_repository")
	}
	if verifier == nil {
		return nil, invalid("external_identity_verifier")
	}
	if newID == nil {
		newID = randomID
	}
	return &ExternalIdentityService{repository: repository, verifier: verifier, newID: newID}, nil
}

// Bind verifies provider proof and binds the stable provider subject. Rebinding
// the same provider subject to another Agent is always rejected.
func (s *ExternalIdentityService) Bind(ctx context.Context, agentID string, provider domain.ExternalIdentityProvider, proof string) (*domain.ExternalIdentity, error) {
	if agentID == "" {
		return nil, invalid("agent_id")
	}
	if proof == "" {
		return nil, invalid("proof")
	}
	verified, err := s.verifier.Verify(ctx, provider, proof)
	if err != nil {
		return nil, domain.ErrForbidden
	}
	if verified.Provider != provider || verified.ProviderSubjectID == "" || verified.Login == "" || verified.VerifiedAt.IsZero() {
		return nil, invalid("provider_identity")
	}
	existing, err := s.repository.GetByProviderSubject(ctx, provider, verified.ProviderSubjectID)
	if err == nil {
		if existing.AgentID != agentID {
			return nil, domain.ErrStateConflict
		}
		if err := existing.RefreshLogin(verified.Login, verified.VerifiedAt); err != nil {
			return nil, err
		}
		if err := s.repository.Update(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	identity, err := domain.NewExternalIdentity(s.newID(), agentID, provider, verified.ProviderSubjectID, verified.Login, verified.VerifiedAt)
	if err != nil {
		return nil, err
	}
	if err := s.repository.Insert(ctx, identity); err != nil {
		return nil, err
	}
	return identity, nil
}

// VerifyAttribution confirms that a PR author subject is actively bound to the
// authenticated global Agent. Login alone is deliberately ignored.
func (s *ExternalIdentityService) VerifyAttribution(ctx context.Context, agentID string, provider domain.ExternalIdentityProvider, providerSubjectID string) error {
	identity, err := s.repository.GetByProviderSubject(ctx, provider, providerSubjectID)
	if err != nil {
		return domain.ErrForbidden
	}
	if !identity.Attributes(agentID, provider, providerSubjectID) {
		return domain.ErrForbidden
	}
	return nil
}

func (s *ExternalIdentityService) Revoke(ctx context.Context, agentID string, provider domain.ExternalIdentityProvider, providerSubjectID string, now time.Time) error {
	identity, err := s.repository.GetByProviderSubject(ctx, provider, providerSubjectID)
	if err != nil {
		return err
	}
	if identity.AgentID != agentID {
		return domain.ErrForbidden
	}
	if err := identity.Revoke(now); err != nil {
		return err
	}
	return s.repository.Update(ctx, identity)
}
