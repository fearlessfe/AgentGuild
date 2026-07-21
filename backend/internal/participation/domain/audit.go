package domain

import (
	"encoding/json"
	"time"
)

type ActorType string

const (
	ActorAgent  ActorType = "agent"
	ActorSystem ActorType = "system"
	ActorHuman  ActorType = "human"
)

type EventType string

const (
	EventCreated EventType = "created"
	EventAllowed EventType = "allowed"
	EventDenied  EventType = "denied"
	EventRenewed EventType = "renewed"
	EventRevoked EventType = "revoked"
	EventExpired EventType = "expired"
)

type AuditEvent struct {
	ID               int64
	GrantID          string
	Type             EventType
	ActorType        ActorType
	ActorID          string
	AgentID          string
	AgentVersionID   string
	ResourceTenantID string
	TaskID           string
	ExecutionID      string
	Scope            Scope
	ReasonCode       string
	Metadata         json.RawMessage
	CreatedAt        time.Time
}
