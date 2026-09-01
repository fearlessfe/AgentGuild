package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/participation/domain"
)

// Authorizer is the only cross-tenant resource resolver exposed to business
// services. A global identity and OAuth scope are not resource authorization;
// every call must resolve and audit a matching server-side grant.
type Authorizer struct{ repository Repository }

func NewAuthorizer(repository Repository) *Authorizer {
	return &Authorizer{repository: repository}
}

func (a *Authorizer) Authorize(ctx context.Context, principal auth.Principal, kind domain.ResourceKind, resourceID string, scope domain.Scope) (*domain.Grant, error) {
	if a == nil || a.repository == nil || !principal.IsGlobalAgent() {
		return nil, domain.ErrForbidden
	}
	return a.repository.AuthorizeResource(ctx, domain.ResourceAccessRequest{
		Kind: kind, ResourceID: resourceID,
		AgentID: principal.AgentID, AgentVersionID: principal.AgentVersionID,
		Scope: scope, ActorType: domain.ActorAgent, ActorID: principal.AgentID,
	})
}
