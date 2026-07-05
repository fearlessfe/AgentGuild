package domain

import (
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
)

// ProjectionKey identifies a reputation projection slice.
type ProjectionKey struct {
	AgentVersionID string
	Capability     string
	TaskType       string
}

// Projection is an aggregated view of review signals for a single key.
// All rates are fractions in the range [0, 1].
type Projection struct {
	Key                    ProjectionKey
	TotalReviews           int
	AcceptedCount          int
	RejectedCount          int
	RevisionRequestedCount int
	PassRate               float64
	ReworkRate             float64
	AvgReviewCostCents     float64
	AvgReviewLatencyMs     float64
	SampleSizeHint         string
	AlgorithmVersion       string
}

// ReviewSignal is the input to the projection algorithm.
// Decision comes from the review domain; the remaining fields are observed
// execution/task metadata.
type ReviewSignal struct {
	Decision       reviewdomain.Decision
	CostCents      int64
	LatencyMs      int64
	Capability     string
	TaskType       string
	AgentVersionID string
}

// NewProjection creates a fresh projection with the correct initial state.
func NewProjection(agentVersionID, capability, taskType string) *Projection {
	return &Projection{
		Key: ProjectionKey{
			AgentVersionID: agentVersionID,
			Capability:     capability,
			TaskType:       taskType,
		},
		SampleSizeHint:   "low",
		AlgorithmVersion: "2026-07-04-v1",
	}
}

// Apply incorporates a single review signal into the projection.
func (p *Projection) Apply(s ReviewSignal) {
	p.TotalReviews++
	switch s.Decision {
	case reviewdomain.DecisionAccepted:
		p.AcceptedCount++
	case reviewdomain.DecisionRejected:
		p.RejectedCount++
	case reviewdomain.DecisionRevisionRequested:
		p.RevisionRequestedCount++
	}

	p.PassRate = float64(p.AcceptedCount) / float64(p.TotalReviews)
	p.ReworkRate = float64(p.RevisionRequestedCount) / float64(p.TotalReviews)
	p.AvgReviewCostCents = p.AvgReviewCostCents + (float64(s.CostCents)-p.AvgReviewCostCents)/float64(p.TotalReviews)
	p.AvgReviewLatencyMs = p.AvgReviewLatencyMs + (float64(s.LatencyMs)-p.AvgReviewLatencyMs)/float64(p.TotalReviews)
	p.SampleSizeHint = sampleSizeHint(p.TotalReviews)
	p.AlgorithmVersion = "2026-07-04-v1"
}

func sampleSizeHint(n int) string {
	switch {
	case n < 5:
		return "low"
	case n < 20:
		return "medium"
	default:
		return "high"
	}
}
