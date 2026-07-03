package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/domain"
)

type ListAgents struct {
	OwnerID string
	Limit   int
}

type GetAgent struct {
	AgentID string
}

type GetActivationStatus struct {
	AgentID string
}

type agentLister interface {
	List(context.Context, AgentListQuery) ([]domain.Agent, error)
}

func (s *IdentityService) ListAgents(ctx context.Context, principal Principal, query ListAgents) (Envelope[AgentPage], error) {
	var result Envelope[AgentPage]
	if principal.TenantID == "" || (principal.OwnerID == "" && principal.AgentID == "") {
		return result, domain.ErrForbidden
	}
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		lister, ok := tx.Agents().(agentLister)
		if !ok {
			return invalid("agent_repository")
		}
		ownerID := query.OwnerID
		if !principal.IsAdmin {
			ownerID = principal.OwnerID
		}
		agents, err := lister.List(ctx, AgentListQuery{TenantID: principal.TenantID, OwnerID: ownerID, Limit: query.Limit})
		if err != nil {
			return err
		}
		views := make([]AgentView, 0, len(agents))
		for i := range agents {
			agent := agents[i]
			if err := s.policy.RequireOwnerOrAdmin(principal, &agent); err != nil {
				return err
			}
			version, err := currentVersion(ctx, tx, &agent)
			if err != nil {
				return err
			}
			views = append(views, agentView(&agent, version))
		}
		result = Envelope[AgentPage]{Data: AgentPage{Items: views}, Meta: Meta{ServerTime: now, PollAfterSeconds: 30}}
		return nil
	})
	return result, err
}

func (s *IdentityService) GetAgent(ctx context.Context, principal Principal, query GetAgent) (Envelope[AgentView], error) {
	var result Envelope[AgentView]
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		agent, err := tx.Agents().GetByID(ctx, principal.TenantID, query.AgentID)
		if err != nil {
			return err
		}
		if err := s.policy.RequireOwnerAdminOrAgentSelf(principal, agent); err != nil {
			return err
		}
		version, err := currentVersion(ctx, tx, agent)
		if err != nil {
			return err
		}
		result = Envelope[AgentView]{Data: agentView(agent, version), Meta: Meta{ServerTime: now, PollAfterSeconds: 30}}
		return nil
	})
	return result, err
}

func (s *IdentityService) GetActivationStatus(ctx context.Context, principal Principal, query GetActivationStatus) (Envelope[ActivationStatusView], error) {
	var result Envelope[ActivationStatusView]
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		agent, err := tx.Agents().GetByID(ctx, principal.TenantID, query.AgentID)
		if err != nil {
			return err
		}
		if err := s.policy.RequireOwnerOrAdmin(principal, agent); err != nil {
			return err
		}
		status := "activated"
		var expiresAt *time.Time
		var consumedAt *time.Time
		if credential, err := tx.Credentials().GetPending(ctx, agent.TenantID, agent.ID); err == nil {
			status = credential.Status
			expiresAt = credential.ExpiresAt
			consumedAt = credential.ConsumedAt
		} else if domain.CodeOf(err) != "not_found" {
			return err
		}
		result = Envelope[ActivationStatusView]{
			Data: ActivationStatusView{
				AgentID:             agent.ID,
				Status:              agent.Status,
				ActivationStatus:    status,
				ActivationExpiresAt: expiresAt,
				ActivatedAt:         consumedAt,
			},
			Meta: Meta{ServerTime: now, PollAfterSeconds: 30},
		}
		return nil
	})
	return result, err
}

func agentView(agent *domain.Agent, version *domain.AgentVersion) AgentView {
	view := AgentView{
		ID:             agent.ID,
		TenantID:       agent.TenantID,
		Name:           agent.Name,
		Description:    agent.Description,
		Status:         agent.Status,
		Team:           agent.Team,
		OwnerID:        agent.OwnerID,
		OwnerEmail:     agent.OwnerEmail,
		Scopes:         append([]string(nil), agent.Scopes...),
		RepoScope:      append([]string(nil), agent.RepoScope...),
		LastSeenAt:     agent.LastSeenAt,
		BudgetCents:    agent.BudgetCents,
		BudgetCurrency: agent.BudgetCurrency,
		CreatedAt:      agent.CreatedAt,
		UpdatedAt:      agent.UpdatedAt,
	}
	if version != nil {
		view.CurrentVersion = &AgentVersionView{
			ID:                version.ID,
			TenantID:          version.TenantID,
			AgentID:           version.AgentID,
			VersionNumber:     version.VersionNumber,
			Runtime:           version.Runtime,
			Model:             version.Model,
			Capabilities:      append([]string(nil), version.Capabilities...),
			ConfigFingerprint: version.ConfigFingerprint,
			CreatedAt:         version.CreatedAt,
		}
	}
	return view
}
