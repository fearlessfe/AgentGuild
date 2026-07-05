package application

import (
	"context"

	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
)

// Projector aggregates review signals into reputation projections.
// It is stateless and deterministic: the same input signals always produce
// the same output projections.
type Projector struct{}

// NewProjector creates a new projector.
func NewProjector() *Projector {
	return &Projector{}
}

// Project groups signals by projection key and returns one projection per key.
func (pr *Projector) Project(ctx context.Context, signals []reputationdomain.ReviewSignal) ([]reputationdomain.Projection, error) {
	groups := make(map[reputationdomain.ProjectionKey]*reputationdomain.Projection)
	for _, s := range signals {
		key := reputationdomain.ProjectionKey{
			AgentVersionID: s.AgentVersionID,
			Capability:     s.Capability,
			TaskType:       s.TaskType,
		}
		p, ok := groups[key]
		if !ok {
			p = reputationdomain.NewProjection(s.AgentVersionID, s.Capability, s.TaskType)
			groups[key] = p
		}
		p.Apply(s)
	}

	projections := make([]reputationdomain.Projection, 0, len(groups))
	for _, p := range groups {
		projections = append(projections, *p)
	}
	return projections, nil
}
