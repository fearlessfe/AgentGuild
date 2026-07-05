package domain

import (
	"encoding/json"
	"sort"
	"time"
)

// CandidateStatus is the lifecycle state of an ExperienceCandidate.
type CandidateStatus string

const (
	StatusPendingReview CandidateStatus = "pending_review"
	StatusApproved      CandidateStatus = "approved"
	StatusRejected      CandidateStatus = "rejected"
)

// ExperienceCandidate is an extracted unit of experience awaiting review.
// Once approved it can be bound to a new AgentVersion as a content reference.
// Fields are exported so repositories can scan rows directly; application code
// should use the accessor methods and state-machine methods rather than
// mutating fields directly.
type ExperienceCandidate struct {
	ID                     string
	TenantID               string
	AgentID                string
	SourceTaskID           string
	SourceSubmissionID     string
	SourceReviewID         string
	EvidenceRef            string
	ContentHash            string
	ApplicableCapabilities []string
	TenantScope            string
	SensitivityClass       SensitivityClass
	Status                 CandidateStatus
	PolicyReason           string
	ReviewedBy             string
	ReviewedAt             *time.Time
	CreatedAt              time.Time
}

// NewExperienceCandidate creates a candidate in pending_review. The content
// hash is computed from the evidence reference and the candidate metadata.
func NewExperienceCandidate(
	id, tenantID, agentID,
	sourceTaskID, sourceSubmissionID, sourceReviewID,
	evidenceRef string,
	applicableCapabilities []string,
	tenantScope string,
	createdAt time.Time,
) (*ExperienceCandidate, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
	}
	if agentID == "" {
		return nil, invalidArgument("agent_id")
	}
	if sourceTaskID == "" {
		return nil, invalidArgument("source_task_id")
	}
	if sourceSubmissionID == "" {
		return nil, invalidArgument("source_submission_id")
	}
	if evidenceRef == "" {
		return nil, invalidArgument("evidence_ref")
	}
	if tenantScope == "" {
		tenantScope = tenantID
	}
	if createdAt.IsZero() {
		return nil, invalidArgument("created_at")
	}

	caps := make([]string, len(applicableCapabilities))
	copy(caps, applicableCapabilities)
	sort.Strings(caps)

	payload, _ := json.Marshal([8]any{
		id, tenantID, agentID, sourceTaskID, sourceSubmissionID, sourceReviewID,
		evidenceRef, caps,
	})

	return &ExperienceCandidate{
		ID:                     id,
		TenantID:               tenantID,
		AgentID:                agentID,
		SourceTaskID:           sourceTaskID,
		SourceSubmissionID:     sourceSubmissionID,
		SourceReviewID:         sourceReviewID,
		EvidenceRef:            evidenceRef,
		ContentHash:            ComputeContentHash(payload),
		ApplicableCapabilities: caps,
		TenantScope:            tenantScope,
		SensitivityClass:       SensitivityPublic,
		Status:                 StatusPendingReview,
		CreatedAt:              createdAt,
	}, nil
}

// StatusValue returns the current lifecycle status.
func (c *ExperienceCandidate) StatusValue() CandidateStatus { return c.Status }

// ClassifyAndApply applies the sensitivity policy. If the policy classifies the
// evidence as forbidden, the candidate is automatically rejected.
func (c *ExperienceCandidate) ClassifyAndApply(policy SensitivityPolicy) error {
	if policy == nil {
		return invalidArgument("policy")
	}
	if c.Status != StatusPendingReview {
		return ErrStateConflict
	}
	class, reason := policy.Classify([]byte(c.EvidenceRef))
	c.SensitivityClass = class
	if class == SensitivityForbidden {
		c.Status = StatusRejected
		c.PolicyReason = reason
	}
	return nil
}

// Approve transitions pending_review -> approved.
func (c *ExperienceCandidate) Approve(reviewerID string, now time.Time) error {
	if c.Status != StatusPendingReview {
		return ErrStateConflict
	}
	if reviewerID == "" {
		return invalidArgument("reviewer_id")
	}
	c.Status = StatusApproved
	c.ReviewedBy = reviewerID
	c.ReviewedAt = &now
	return nil
}

// Reject transitions pending_review -> rejected and records the reason.
func (c *ExperienceCandidate) Reject(reviewerID, reason string, now time.Time) error {
	if c.Status != StatusPendingReview {
		return ErrStateConflict
	}
	if reviewerID == "" {
		return invalidArgument("reviewer_id")
	}
	c.Status = StatusRejected
	c.PolicyReason = reason
	c.ReviewedBy = reviewerID
	c.ReviewedAt = &now
	return nil
}

// SetTenantScope refuses to change the tenant scope. Tenant scope is fixed at
// creation time to prevent cross-tenant leakage.
func (c *ExperienceCandidate) SetTenantScope(scope string) error {
	if scope != c.TenantScope {
		return ErrImmutableResource
	}
	return nil
}
