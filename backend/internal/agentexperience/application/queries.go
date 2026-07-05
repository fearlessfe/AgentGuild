package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/agentexperience/domain"
)

// ListCandidates returns experience candidates for an agent, optionally
// filtered by status. Empty status returns all candidates.
func (s *CandidateService) ListCandidates(
	ctx context.Context,
	tenantID, agentID string,
	status string,
) ([]CandidateSummary, error) {
	var candidates []domain.ExperienceCandidate
	var err error

	if status == "" {
		candidates, err = s.candidates.ListByAgent(ctx, tenantID, agentID)
	} else {
		candidates, err = s.candidates.ListByAgentAndStatus(ctx, tenantID, agentID, domain.CandidateStatus(status))
	}
	if err != nil {
		return nil, err
	}

	summaries := make([]CandidateSummary, 0, len(candidates))
	for _, c := range candidates {
		summaries = append(summaries, toCandidateSummary(&c))
	}
	return summaries, nil
}

// GetCandidate returns a single candidate by ID.
func (s *CandidateService) GetCandidate(
	ctx context.Context,
	tenantID, agentID, candidateID string,
) (*CandidateSummary, error) {
	candidate, err := s.candidates.GetByID(ctx, tenantID, agentID, candidateID)
	if err != nil {
		return nil, err
	}
	summary := toCandidateSummary(candidate)
	return &summary, nil
}

func toCandidateSummary(c *domain.ExperienceCandidate) CandidateSummary {
	return CandidateSummary{
		ID:                     c.ID,
		TenantID:               c.TenantID,
		AgentID:                c.AgentID,
		SourceTaskID:           c.SourceTaskID,
		SourceSubmissionID:     c.SourceSubmissionID,
		SourceReviewID:         c.SourceReviewID,
		EvidenceRef:            c.EvidenceRef,
		ContentHash:            c.ContentHash,
		ApplicableCapabilities: append([]string(nil), c.ApplicableCapabilities...),
		TenantScope:            c.TenantScope,
		SensitivityClass:       string(c.SensitivityClass),
		Status:                 string(c.Status),
		PolicyReason:           c.PolicyReason,
		ReviewedBy:             c.ReviewedBy,
		ReviewedAt:             c.ReviewedAt,
		CreatedAt:              c.CreatedAt,
	}
}
