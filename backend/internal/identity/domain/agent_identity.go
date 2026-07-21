package domain

import "time"

// AgentIdentity is the platform-global identity of an Agent. Repository access,
// organization membership, operator responsibility, and execution grants are
// deliberately modeled outside this aggregate.
type AgentIdentity struct {
	ID               string
	Handle           string
	DisplayName      string
	Description      string
	Status           string
	CurrentVersionID string
	LastSeenAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Events           []AgentIdentityEvent
}

// AgentIdentityEvent is an auditable global identity transition. It does not
// carry a tenant because tenant membership is not part of Agent identity.
type AgentIdentityEvent struct {
	AgentID   string
	ActorType ActorType
	ActorID   string
	Intent    string
	FromState string
	ToState   string
	Reason    string
	Payload   map[string]any
	CreatedAt time.Time
}

// NewAgentIdentity creates a pending platform-global identity.
func NewAgentIdentity(id, handle, displayName string, description *string, now time.Time) (*AgentIdentity, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if handle == "" {
		return nil, invalidArgument("handle")
	}
	if displayName == "" {
		displayName = handle
	}
	descriptionValue := ""
	if description != nil {
		descriptionValue = *description
	}
	agent := &AgentIdentity{
		ID:          id,
		Handle:      handle,
		DisplayName: displayName,
		Description: descriptionValue,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	agent.transition(ActorSystem, "system", "register", AgentPendingActivation, "", nil, now)
	return agent, nil
}

// Activate binds the first immutable version and activates the global identity.
func (a *AgentIdentity) Activate(version *AgentVersion, actorID string, now time.Time) error {
	if a.Status != AgentPendingActivation {
		return ErrStateConflict
	}
	if version == nil {
		return invalidArgument("agent_version")
	}
	if version.AgentID != a.ID {
		return ErrForbidden
	}
	if actorID == "" {
		return ErrForbidden
	}
	a.CurrentVersionID = version.ID
	a.transition(ActorAgent, actorID, "activate", AgentActive, "", map[string]any{"version_id": version.ID}, now)
	return nil
}

// Rename changes public presentation fields without changing the stable ID.
func (a *AgentIdentity) Rename(handle, displayName, actorID string, now time.Time) error {
	if handle == "" {
		return invalidArgument("handle")
	}
	if actorID == "" {
		return ErrForbidden
	}
	if a.Status == AgentRevoked {
		return ErrStateConflict
	}
	if displayName == "" {
		displayName = handle
	}
	a.Handle = handle
	a.DisplayName = displayName
	a.UpdatedAt = now
	a.Events = append(a.Events, AgentIdentityEvent{
		AgentID:   a.ID,
		ActorType: ActorHuman,
		ActorID:   actorID,
		Intent:    "rename",
		FromState: a.Status,
		ToState:   a.Status,
		Payload:   map[string]any{"handle": handle, "display_name": displayName},
		CreatedAt: now,
	})
	return nil
}

// Suspend prevents protected operations while retaining identity and history.
func (a *AgentIdentity) Suspend(actorID, reason string, now time.Time) error {
	if actorID == "" {
		return ErrForbidden
	}
	if a.Status != AgentActive {
		return ErrStateConflict
	}
	a.transition(ActorHuman, actorID, "suspend", AgentSuspended, reason, nil, now)
	return nil
}

// Resume returns a suspended global identity to active status.
func (a *AgentIdentity) Resume(actorID string, now time.Time) error {
	if actorID == "" {
		return ErrForbidden
	}
	if a.Status != AgentSuspended {
		return ErrStateConflict
	}
	a.transition(ActorHuman, actorID, "resume", AgentActive, "", nil, now)
	return nil
}

// Revoke is terminal but keeps historical contribution attribution intact.
func (a *AgentIdentity) Revoke(actorID, reason string, now time.Time) error {
	if actorID == "" {
		return ErrForbidden
	}
	if a.Status == AgentRevoked {
		return ErrStateConflict
	}
	a.transition(ActorHuman, actorID, "revoke", AgentRevoked, reason, nil, now)
	return nil
}

// Heartbeat records liveness independently from resource authorization.
func (a *AgentIdentity) Heartbeat(now time.Time) error {
	if a.Status != AgentActive && a.Status != AgentSuspended {
		return ErrStateConflict
	}
	a.LastSeenAt = &now
	a.UpdatedAt = now
	return nil
}

func (a *AgentIdentity) transition(actorType ActorType, actorID, intent, toState, reason string, payload map[string]any, now time.Time) {
	fromState := a.Status
	a.Status = toState
	a.UpdatedAt = now
	a.Events = append(a.Events, AgentIdentityEvent{
		AgentID:   a.ID,
		ActorType: actorType,
		ActorID:   actorID,
		Intent:    intent,
		FromState: fromState,
		ToState:   toState,
		Reason:    reason,
		Payload:   payload,
		CreatedAt: now,
	})
}
