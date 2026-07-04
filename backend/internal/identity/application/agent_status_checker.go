package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/identity/domain"
)

type AgentStatusChecker struct {
	store Store
}

func NewAgentStatusChecker(store Store) AgentStatusChecker {
	return AgentStatusChecker{store: store}
}

func (c AgentStatusChecker) CheckAgentStatus(ctx context.Context, principal auth.Principal) error {
	if c.store == nil {
		return invalid("store")
	}
	return c.store.WithTx(ctx, func(tx Tx) error {
		agent, err := tx.Agents().GetByID(ctx, principal.TenantID, principal.AgentID)
		if err != nil {
			if domain.CodeOf(err) == "not_found" {
				return nil
			}
			return err
		}
		if agent.CurrentVersionID != "" && principal.AgentVersionID != agent.CurrentVersionID {
			return domain.ErrForbidden
		}
		switch agent.Status {
		case domain.AgentActive:
			return nil
		case domain.AgentRevoked:
			return domain.ErrTokenRevoked
		default:
			return domain.ErrStateConflict
		}
	})
}
