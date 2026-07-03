package domain

import "time"

const (
	AgentPendingActivation = "pending_activation"
	AgentActive            = "active"
	AgentSuspended         = "suspended"
	AgentRevoked           = "revoked"
)

type Agent struct {
	ID               string
	TenantID         string
	OwnerID          string
	OwnerEmail       string
	Team             string
	Name             string
	Description      string
	Status           string
	CurrentVersionID string
	Scopes           []string
	RepoScope        []string
	BudgetCents      int64
	BudgetCurrency   string
	LastSeenAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Events           []IdentityEvent
}

func NewAgent(
	id, tenantID, ownerID, ownerEmail, team string,
	scopes []string,
	description *string,
) (*Agent, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
	}
	if ownerID == "" {
		return nil, invalidArgument("owner_id")
	}
	if ownerEmail == "" {
		return nil, invalidArgument("owner_email")
	}

	now := time.Now()
	desc := ""
	if description != nil {
		desc = *description
	}

	agent := &Agent{
		ID:          id,
		TenantID:    tenantID,
		OwnerID:     ownerID,
		OwnerEmail:  ownerEmail,
		Team:        team,
		Name:        id,
		Description: desc,
		Status:      "",
		Scopes:      scopes,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := agent.Register(); err != nil {
		return nil, err
	}
	return agent, nil
}

func (a *Agent) Register() error {
	if a.Status != "" {
		return ErrStateConflict
	}
	a.Status = AgentPendingActivation
	a.UpdatedAt = time.Now()
	a.recordEvent(ActorSystem, "system", "register", "", AgentPendingActivation, "", nil)
	return nil
}

func (a *Agent) Activate(cred *ActivationCredential, version *AgentVersion, token string, now time.Time) error {
	if a.Status != AgentPendingActivation {
		return ErrStateConflict
	}
	if cred == nil {
		return invalidArgument("activation_credential")
	}
	if version == nil {
		return invalidArgument("agent_version")
	}
	if token == "" {
		return invalidArgument("token")
	}
	if cred.TenantID != a.TenantID || cred.AgentID != a.ID {
		return ErrForbidden
	}
	if version.AgentID != a.ID || version.TenantID != a.TenantID {
		return ErrForbidden
	}
	if err := cred.Consume(token, now); err != nil {
		return err
	}

	a.CurrentVersionID = version.ID
	a.Status = AgentActive
	a.UpdatedAt = now
	a.recordEvent(ActorAgent, a.ID, "activate", AgentPendingActivation, AgentActive, "", map[string]any{
		"version_id": version.ID,
	})
	return nil
}

func (a *Agent) Suspend(actorID string, now time.Time) error {
	if actorID == "" {
		return ErrForbidden
	}
	if a.Status != AgentActive {
		return ErrStateConflict
	}
	a.Status = AgentSuspended
	a.UpdatedAt = now
	a.recordEvent(ActorHuman, actorID, "suspend", AgentActive, AgentSuspended, "", nil)
	return nil
}

func (a *Agent) Resume(actorID string, now time.Time) error {
	if actorID == "" {
		return ErrForbidden
	}
	if a.Status != AgentSuspended {
		return ErrStateConflict
	}
	a.Status = AgentActive
	a.UpdatedAt = now
	a.recordEvent(ActorHuman, actorID, "resume", AgentSuspended, AgentActive, "", nil)
	return nil
}

func (a *Agent) Revoke(actorID, reason string, now time.Time) error {
	if actorID == "" {
		return ErrForbidden
	}
	if a.Status == AgentRevoked {
		return ErrStateConflict
	}
	from := a.Status
	a.Status = AgentRevoked
	a.UpdatedAt = now
	a.recordEvent(ActorHuman, actorID, "revoke", from, AgentRevoked, reason, nil)
	return nil
}

func (a *Agent) Heartbeat(now time.Time) error {
	switch a.Status {
	case AgentActive, AgentSuspended:
		a.LastSeenAt = &now
		a.UpdatedAt = now
		return nil
	default:
		return ErrStateConflict
	}
}

func (a *Agent) recordEvent(actorType ActorType, actorID, intent, fromState, toState, reason string, payload map[string]any) {
	a.Events = append(a.Events, NewIdentityEvent(
		a.TenantID,
		a.ID,
		actorType,
		actorID,
		intent,
		fromState,
		toState,
		reason,
		payload,
		time.Now(),
	))
}
