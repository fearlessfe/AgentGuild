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

// RequireTenantOwnerOrAdmin verifies that principal belongs to the tenant and
// is either an admin or a tenant owner. Used for tenant-scoped resources such
// as BenchmarkSets.
func (p *Policy) RequireTenantOwnerOrAdmin(principal identityapp.Principal, tenantID string) error {
	if principal.TenantID == "" {
		return domain.ErrNotFound
	}
	if principal.TenantID != tenantID {
		return domain.ErrNotFound
	}
	if principal.IsAdmin {
		return nil
	}
	if principal.OwnerID != "" {
		return nil
	}
	return domain.ErrNotFound
}

// RequireOwnerOrAdmin verifies that principal is either an admin or the owner
// of the agent identified by tenantID and agentID. For non-admin callers,
// missing or unauthorized agents always return not_found to avoid resource
// probing.
func (p *Policy) RequireOwnerOrAdmin(ctx context.Context, principal identityapp.Principal, tenantID, agentID string) error {
	if principal.TenantID == "" {
		return domain.ErrNotFound
	}
	if principal.TenantID != tenantID {
		return domain.ErrNotFound
	}
	if principal.IsAdmin {
		return nil
	}
	ownerID, err := p.ownerProvider.GetAgentOwner(ctx, tenantID, agentID)
	if err != nil {
		return domain.ErrNotFound
	}
	if principal.OwnerID != "" && principal.OwnerID == ownerID {
		return nil
	}
	return domain.ErrNotFound
}
