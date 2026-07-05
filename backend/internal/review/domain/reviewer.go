package domain

import (
	"time"

	appdomain "agentguild.dev/agentguild/backend/internal/domain"
)

// ReviewerProfile captures the capabilities and current load of a reviewer.
type ReviewerProfile struct {
	ID           string
	TenantID     string
	UserID       string
	Capabilities []string
	CurrentLoad  int
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NewReviewerProfile creates an active reviewer profile.
func NewReviewerProfile(id, tenantID, userID string, capabilities []string, now time.Time) (*ReviewerProfile, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
	}
	if userID == "" {
		return nil, invalidArgument("user_id")
	}
	if now.IsZero() {
		return nil, invalidArgument("created_at")
	}
	return &ReviewerProfile{
		ID:           id,
		TenantID:     tenantID,
		UserID:       userID,
		Capabilities: capabilities,
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// SetLoad updates the reviewer's current load. Negative loads are rejected.
func (p *ReviewerProfile) SetLoad(load int, now time.Time) error {
	if load < 0 {
		return invalidArgument("current_load")
	}
	p.CurrentLoad = load
	p.UpdatedAt = now
	return nil
}

// Assign increments the current load when the reviewer accepts a new review assignment.
func (p *ReviewerProfile) Assign(now time.Time) error {
	if !p.IsActive {
		return appdomain.ErrStateConflict
	}
	p.CurrentLoad++
	p.UpdatedAt = now
	return nil
}

// Release decrements the current load when a review assignment is completed or dropped.
func (p *ReviewerProfile) Release(now time.Time) error {
	if p.CurrentLoad <= 0 {
		return appdomain.ErrStateConflict
	}
	p.CurrentLoad--
	p.UpdatedAt = now
	return nil
}

// Activate marks the profile as active.
func (p *ReviewerProfile) Activate(now time.Time) {
	p.IsActive = true
	p.UpdatedAt = now
}

// Deactivate marks the profile as inactive and prevents new assignments.
func (p *ReviewerProfile) Deactivate(now time.Time) {
	p.IsActive = false
	p.UpdatedAt = now
}
