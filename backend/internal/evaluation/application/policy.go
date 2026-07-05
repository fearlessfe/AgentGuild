package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
)

// Policy enforces ownership and admin access for evaluation management.
type Policy struct {
	ownerProvider AgentOwnerProvider
}

// NewPolicy creates a Policy that reads agent ownership from ownerProvider.
func NewPolicy(ownerProvider AgentOwnerProvider) *Policy {
	return &Policy{ownerProvider: ownerProvider}
}

// RequireTenantAdmin verifies that principal belongs to the tenant and is an
// admin. Used for tenant-scoped resources such as BenchmarkSets.
func (p *Policy) RequireTenantAdmin(ctx context.Context, principal identityapp.Principal, tenantID string) error {
	if principal.TenantID == "" {
		return domain.ErrForbidden
	}
	if principal.TenantID != tenantID {
		return domain.ErrForbidden
	}
	if principal.IsAdmin {
		return nil
	}
	return domain.ErrForbidden
}

// RequireOwnerOrAdmin verifies that principal is either an admin or the owner
// of the agent identified by tenantID and agentID.
func (p *Policy) RequireOwnerOrAdmin(ctx context.Context, principal identityapp.Principal, tenantID, agentID string) error {
	if principal.TenantID == "" {
		return domain.ErrForbidden
	}
	if principal.TenantID != tenantID {
		return domain.ErrForbidden
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
	return domain.ErrForbidden
}
