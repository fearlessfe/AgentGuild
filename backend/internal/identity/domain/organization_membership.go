package domain

import "time"

const (
	OrganizationMembershipActive  = "active"
	OrganizationMembershipRevoked = "revoked"
)

// OrganizationMembership grants an Agent organization-scoped responsibility
// and policy. It is not an identity record and intentionally contains no
// repository scope.
type OrganizationMembership struct {
	AgentID          string
	OrganizationID   string
	OperatorID       string
	OperatorEmail    string
	Team             string
	Scopes           []string
	BudgetCents      int64
	BudgetCurrency   string
	Status           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	RevokedAt        *time.Time
	RevocationActor  string
	RevocationReason string
}

// NewOrganizationMembership creates an active optional organization binding.
func NewOrganizationMembership(agentID, organizationID, operatorID, operatorEmail, team string, scopes []string, now time.Time) (*OrganizationMembership, error) {
	if agentID == "" {
		return nil, invalidArgument("agent_id")
	}
	if organizationID == "" {
		return nil, invalidArgument("organization_id")
	}
	if operatorID == "" {
		return nil, invalidArgument("operator_id")
	}
	if operatorEmail == "" {
		return nil, invalidArgument("operator_email")
	}
	return &OrganizationMembership{
		AgentID:        agentID,
		OrganizationID: organizationID,
		OperatorID:     operatorID,
		OperatorEmail:  operatorEmail,
		Team:           team,
		Scopes:         append([]string(nil), scopes...),
		Status:         OrganizationMembershipActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// Revoke removes organization authority without changing the global identity.
func (m *OrganizationMembership) Revoke(actorID, reason string, now time.Time) error {
	if actorID == "" {
		return ErrForbidden
	}
	if m.Status != OrganizationMembershipActive {
		return ErrStateConflict
	}
	m.Status = OrganizationMembershipRevoked
	m.RevokedAt = &now
	m.RevocationActor = actorID
	m.RevocationReason = reason
	m.UpdatedAt = now
	return nil
}
