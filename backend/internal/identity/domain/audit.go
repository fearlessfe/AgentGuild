package domain

import "time"

type ActorType string

const (
	ActorHuman  ActorType = "human"
	ActorAgent  ActorType = "agent"
	ActorSystem ActorType = "system"
)

type IdentityEvent struct {
	TenantID  string
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

func NewIdentityEvent(
	tenantID, agentID string,
	actorType ActorType,
	actorID, intent, fromState, toState, reason string,
	payload map[string]any,
	now time.Time,
) IdentityEvent {
	return IdentityEvent{
		TenantID:  tenantID,
		AgentID:   agentID,
		ActorType: actorType,
		ActorID:   actorID,
		Intent:    intent,
		FromState: fromState,
		ToState:   toState,
		Reason:    reason,
		Payload:   payload,
		CreatedAt: now,
	}
}
