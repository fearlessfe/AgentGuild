package application

import (
	"context"
	"time"

	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
)

// ProjectionRecord binds a projection to its owning tenant. It is used by the
// global transaction boundary so that tenant isolation is explicit.
type ProjectionRecord struct {
	TenantID   string
	Projection reputationdomain.Projection
}

// GetProjection is a reputation query.
type GetProjection struct {
	AgentVersionID string
	Capability     string
	TaskType       string
}

// ProjectionView is the serialized representation of a reputation projection.
type ProjectionView struct {
	AgentVersionID         string  `json:"agent_version_id"`
	Capability             string  `json:"capability"`
	TaskType               string  `json:"task_type"`
	TotalReviews           int     `json:"total_reviews"`
	AcceptedCount          int     `json:"accepted_count"`
	RejectedCount          int     `json:"rejected_count"`
	RevisionRequestedCount int     `json:"revision_requested_count"`
	PassRate               float64 `json:"pass_rate"`
	ReworkRate             float64 `json:"rework_rate"`
	AvgReviewCostCents     float64 `json:"avg_review_cost_cents,omitempty"`
	AvgReviewLatencyMs     float64 `json:"avg_review_latency_ms,omitempty"`
	SampleSizeHint         string  `json:"sample_size_hint"`
	AlgorithmVersion       string  `json:"algorithm_version"`
}

// ProjectionRepository stores projected reputation slices.
type ProjectionRepository interface {
	GetByKey(ctx context.Context, tenantID string, key reputationdomain.ProjectionKey) (*reputationdomain.Projection, error)
	Save(ctx context.Context, record ProjectionRecord) error
	Upsert(ctx context.Context, record ProjectionRecord) error
	ListByAgentVersion(ctx context.Context, tenantID, agentVersionID string) ([]ProjectionRecord, error)
}

// SignalSource produces review signals for projection.
type SignalSource interface {
	ListSignals(ctx context.Context, since time.Time) ([]reputationdomain.ReviewSignal, error)
}
