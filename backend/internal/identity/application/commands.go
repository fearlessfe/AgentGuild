package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/domain"
)

const defaultCredentialTTL = 24 * time.Hour

type RegisterAgent struct {
	RequestID      string
	Name           string
	Description    string
	Team           string
	Scopes         []string
	RepoScope      []string
	BudgetCents    int64
	BudgetCurrency string
}

type ActivateAgent struct {
	Token             string
	Runtime           string
	Model             string
	Capabilities      []string
	ConfigFingerprint string
}

type SuspendAgent struct {
	AgentID string
	Reason  string
}

type ResumeAgent struct {
	AgentID string
}

type RevokeAgent struct {
	AgentID string
	Reason  string
}

type IssueAccessToken struct{}

type AgentHeartbeat struct{}

func NewIdentityService(store Store, options IdentityOptions) (*IdentityService, error) {
	if store == nil {
		return nil, invalid("store")
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	if options.CredentialTTL <= 0 {
		options.CredentialTTL = defaultCredentialTTL
	}
	return &IdentityService{
		store:         store,
		newID:         options.NewID,
		tokenIssuer:   options.TokenIssuer,
		credentialTTL: options.CredentialTTL,
	}, nil
}

func (s *IdentityService) RegisterAgent(ctx context.Context, principal Principal, command RegisterAgent) (Envelope[RegisterAgentResponse], error) {
	var result Envelope[RegisterAgentResponse]
	if principal.TenantID == "" || principal.OwnerID == "" {
		return result, domain.ErrForbidden
	}
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		description := command.Description
		agent, err := domain.NewAgent(s.newID(), principal.TenantID, principal.OwnerID, principal.OwnerEmail, command.Team, command.Scopes, &description)
		if err != nil {
			return err
		}
		if command.Name != "" {
			agent.Name = command.Name
		}
		agent.RepoScope = append([]string(nil), command.RepoScope...)
		agent.BudgetCents = command.BudgetCents
		agent.BudgetCurrency = command.BudgetCurrency
		agent.CreatedAt = now
		agent.UpdatedAt = now

		credential, token, err := domain.NewActivationCredential(agent.ID, agent.TenantID, s.credentialTTL)
		if err != nil {
			return err
		}
		if err := tx.Agents().Insert(ctx, agent); err != nil {
			return err
		}
		if err := tx.Credentials().Insert(ctx, credential); err != nil {
			return err
		}
		if err := appendIdentityEvents(ctx, tx, agent); err != nil {
			return err
		}
		result = Envelope[RegisterAgentResponse]{
			Data: RegisterAgentResponse{
				Agent:               agentView(agent, nil),
				ActivationToken:     token,
				ActivationExpiresAt: credential.ExpiresAt,
			},
			Meta: Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

func (s *IdentityService) ActivateAgent(ctx context.Context, command ActivateAgent) (Envelope[AccessTokenView], error) {
	var result Envelope[AccessTokenView]
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		credential, err := tx.Credentials().GetPendingByPlaintext(ctx, command.Token)
		if err != nil {
			return maskCredentialError(err)
		}
		agent, err := tx.Agents().GetByID(ctx, credential.TenantID, credential.AgentID)
		if err != nil {
			return err
		}
		version, err := domain.NewAgentVersion(s.newID(), agent.TenantID, agent.ID, 1, command.Runtime, command.Model, command.Capabilities, command.ConfigFingerprint, now)
		if err != nil {
			return err
		}
		if err := agent.Activate(credential, version, command.Token, now); err != nil {
			return maskCredentialError(err)
		}
		if err := tx.Credentials().Save(ctx, credential); err != nil {
			return maskCredentialError(err)
		}
		if err := tx.Versions().Insert(ctx, version); err != nil {
			return err
		}
		if err := tx.Agents().Update(ctx, agent); err != nil {
			return err
		}
		if err := appendIdentityEvents(ctx, tx, agent); err != nil {
			return err
		}
		token, err := s.issue(ctx, agent, version, now)
		if err != nil {
			return err
		}
		result = Envelope[AccessTokenView]{Data: token, Meta: Meta{ServerTime: now}}
		return nil
	})
	return result, err
}

func (s *IdentityService) SuspendAgent(ctx context.Context, principal Principal, command SuspendAgent) (Envelope[AgentView], error) {
	return s.withManagedAgent(ctx, principal, command.AgentID, func(agent *domain.Agent, now time.Time) error {
		return agent.Suspend(principal.OwnerID, now)
	})
}

func (s *IdentityService) ResumeAgent(ctx context.Context, principal Principal, command ResumeAgent) (Envelope[AgentView], error) {
	return s.withManagedAgent(ctx, principal, command.AgentID, func(agent *domain.Agent, now time.Time) error {
		return agent.Resume(principal.OwnerID, now)
	})
}

func (s *IdentityService) RevokeAgent(ctx context.Context, principal Principal, command RevokeAgent) (Envelope[AgentView], error) {
	return s.withManagedAgent(ctx, principal, command.AgentID, func(agent *domain.Agent, now time.Time) error {
		return agent.Revoke(principal.OwnerID, command.Reason, now)
	})
}

func (s *IdentityService) IssueAccessToken(ctx context.Context, principal Principal, _ IssueAccessToken) (Envelope[AccessTokenView], error) {
	var result Envelope[AccessTokenView]
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		agent, err := tx.Agents().GetByID(ctx, principal.TenantID, principal.AgentID)
		if err != nil {
			return err
		}
		if err := s.policy.RequireAgentSelf(principal, agent); err != nil {
			return err
		}
		if err := s.policy.RequireAgentStatus(agent, domain.AgentActive); err != nil {
			return err
		}
		version, err := tx.Versions().GetByID(ctx, agent.TenantID, agent.CurrentVersionID)
		if err != nil {
			return err
		}
		token, err := s.issue(ctx, agent, version, now)
		if err != nil {
			return err
		}
		result = Envelope[AccessTokenView]{Data: token, Meta: Meta{ServerTime: now}}
		return nil
	})
	return result, err
}

func (s *IdentityService) AgentHeartbeat(ctx context.Context, principal Principal, _ AgentHeartbeat) (Envelope[AgentView], error) {
	var result Envelope[AgentView]
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		agent, err := tx.Agents().GetByID(ctx, principal.TenantID, principal.AgentID)
		if err != nil {
			return err
		}
		if err := s.policy.RequireAgentSelf(principal, agent); err != nil {
			return err
		}
		if err := agent.Heartbeat(now); err != nil {
			return err
		}
		if err := tx.Agents().Update(ctx, agent); err != nil {
			return err
		}
		version, err := currentVersion(ctx, tx, agent)
		if err != nil {
			return err
		}
		result = Envelope[AgentView]{Data: agentView(agent, version), Meta: Meta{ServerTime: now}}
		return nil
	})
	return result, err
}

func (s *IdentityService) withManagedAgent(ctx context.Context, principal Principal, agentID string, apply func(*domain.Agent, time.Time) error) (Envelope[AgentView], error) {
	var result Envelope[AgentView]
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		agent, err := tx.Agents().GetByID(ctx, principal.TenantID, agentID)
		if err != nil {
			return err
		}
		if err := s.policy.RequireOwnerOrAdmin(principal, agent); err != nil {
			return err
		}
		if err := apply(agent, now); err != nil {
			return err
		}
		if err := tx.Agents().Update(ctx, agent); err != nil {
			return err
		}
		if err := appendIdentityEvents(ctx, tx, agent); err != nil {
			return err
		}
		version, err := currentVersion(ctx, tx, agent)
		if err != nil {
			return err
		}
		result = Envelope[AgentView]{Data: agentView(agent, version), Meta: Meta{ServerTime: now}}
		return nil
	})
	return result, err
}

func (s *IdentityService) issue(ctx context.Context, agent *domain.Agent, version *domain.AgentVersion, now time.Time) (AccessTokenView, error) {
	if s.tokenIssuer == nil {
		return AccessTokenView{}, invalid("token_issuer")
	}
	return s.tokenIssuer.IssueAccessToken(ctx, agent, version, now)
}

func appendIdentityEvents(ctx context.Context, tx Tx, agent *domain.Agent) error {
	for _, event := range agent.Events {
		if err := tx.Audits().Append(ctx, event); err != nil {
			return err
		}
	}
	agent.Events = nil
	return nil
}

func currentVersion(ctx context.Context, tx Tx, agent *domain.Agent) (*domain.AgentVersion, error) {
	if agent.CurrentVersionID == "" {
		return nil, nil
	}
	return tx.Versions().GetByID(ctx, agent.TenantID, agent.CurrentVersionID)
}

func maskCredentialError(err error) error {
	if domain.CodeOf(err) == "not_found" {
		return domain.ErrTokenExpired
	}
	return err
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func invalid(field string) error {
	return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}
