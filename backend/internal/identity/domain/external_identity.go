package domain

import "time"

type ExternalIdentityProvider string

const (
	ExternalIdentityProviderGitHub ExternalIdentityProvider = "github"
	ExternalIdentityProviderGitLab ExternalIdentityProvider = "gitlab"
	ExternalIdentityProviderGitee  ExternalIdentityProvider = "gitee"

	ExternalIdentityActive  = "active"
	ExternalIdentityRevoked = "revoked"
)

// ExternalIdentity maps a provider-stable subject to a global Agent. Login is
// presentation data and may change without changing contribution attribution.
type ExternalIdentity struct {
	ID                string
	AgentID           string
	Provider          ExternalIdentityProvider
	ProviderSubjectID string
	Login             string
	Status            string
	VerifiedAt        time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	RevokedAt         *time.Time
}

func NewExternalIdentity(id, agentID string, provider ExternalIdentityProvider, providerSubjectID, login string, verifiedAt time.Time) (*ExternalIdentity, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if agentID == "" {
		return nil, invalidArgument("agent_id")
	}
	if !provider.Valid() {
		return nil, invalidArgument("provider")
	}
	if providerSubjectID == "" {
		return nil, invalidArgument("provider_subject_id")
	}
	if login == "" {
		return nil, invalidArgument("login")
	}
	if verifiedAt.IsZero() {
		return nil, invalidArgument("verified_at")
	}
	return &ExternalIdentity{
		ID:                id,
		AgentID:           agentID,
		Provider:          provider,
		ProviderSubjectID: providerSubjectID,
		Login:             login,
		Status:            ExternalIdentityActive,
		VerifiedAt:        verifiedAt,
		CreatedAt:         verifiedAt,
		UpdatedAt:         verifiedAt,
	}, nil
}

func (p ExternalIdentityProvider) Valid() bool {
	return p == ExternalIdentityProviderGitHub || p == ExternalIdentityProviderGitLab || p == ExternalIdentityProviderGitee
}

func (e *ExternalIdentity) RefreshLogin(login string, verifiedAt time.Time) error {
	if e.Status != ExternalIdentityActive {
		return ErrStateConflict
	}
	if login == "" {
		return invalidArgument("login")
	}
	if verifiedAt.IsZero() || verifiedAt.Before(e.VerifiedAt) {
		return invalidArgument("verified_at")
	}
	e.Login = login
	e.VerifiedAt = verifiedAt
	e.UpdatedAt = verifiedAt
	return nil
}

func (e *ExternalIdentity) Revoke(now time.Time) error {
	if e.Status != ExternalIdentityActive {
		return ErrStateConflict
	}
	e.Status = ExternalIdentityRevoked
	e.RevokedAt = &now
	e.UpdatedAt = now
	return nil
}

func (e ExternalIdentity) Attributes(agentID string, provider ExternalIdentityProvider, providerSubjectID string) bool {
	return e.Status == ExternalIdentityActive && e.AgentID == agentID && e.Provider == provider && e.ProviderSubjectID == providerSubjectID
}
