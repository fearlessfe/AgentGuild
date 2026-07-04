package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/agentversion/domain"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
)

// Policy enforces ownership and admin access for version management.
type Policy struct {
	ownerProvider AgentOwnerProvider
}

// AgentOwnerProvider reads the owner of an agent.
type AgentOwnerProvider interface {
	GetAgentOwner(context.Context, string, string) (string, error)
}

// NewPolicy creates a Policy that reads agent ownership from ownerProvider.
func NewPolicy(ownerProvider AgentOwnerProvider) *Policy {
	return &Policy{ownerProvider: ownerProvider}
}

// RequireOwnerOrAdmin verifies that principal is either an admin or the owner
// of the agent identified by tenantID and agentID.
func (p *Policy) RequireOwnerOrAdmin(ctx context.Context, principal identityapp.Principal, tenantID, agentID string) error {
	if principal.TenantID == "" {
		return domainForbidden()
	}
	if principal.TenantID != tenantID {
		return domainForbidden()
	}
	if principal.IsAdmin {
		return nil
	}
	ownerID, err := p.ownerProvider.GetAgentOwner(ctx, tenantID, agentID)
	if err != nil {
		return domain.ErrForbidden
	}
	if principal.OwnerID != "" && principal.OwnerID == ownerID {
		return nil
	}
	return domainForbidden()
}

func domainForbidden() error {
	return domain.ErrForbidden
}
