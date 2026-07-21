package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/contribution/domain"
)

type ProjectionRepository interface {
	ReplaceAlgorithm(context.Context, string, ProjectionSet) error
	GetAgentLifetime(context.Context, string, string) (*domain.Projection, error)
	ListAgentVersions(context.Context, string, string) ([]domain.Projection, error)
}

type Rebuilder struct {
	facts       Repository
	projections ProjectionRepository
	projector   *Projector
}

func NewRebuilder(facts Repository, projections ProjectionRepository, projector *Projector) (*Rebuilder, error) {
	if facts == nil || projections == nil || projector == nil {
		return nil, domain.ErrInvalidArgument
	}
	return &Rebuilder{facts: facts, projections: projections, projector: projector}, nil
}

func (r *Rebuilder) Rebuild(ctx context.Context, calculatedAt time.Time) (ProjectionSet, error) {
	contributions, err := r.facts.ListVerified(ctx)
	if err != nil {
		return ProjectionSet{}, err
	}
	facts := make([]domain.ContributionFacts, 0, len(contributions))
	for _, contribution := range contributions {
		events, err := r.facts.ListEvents(ctx, contribution.ID)
		if err != nil {
			return ProjectionSet{}, err
		}
		facts = append(facts, domain.ContributionFacts{Contribution: contribution, Events: events})
	}
	set, err := r.projector.Project(facts, calculatedAt)
	if err != nil {
		return ProjectionSet{}, err
	}
	if err := r.projections.ReplaceAlgorithm(ctx, r.projector.algorithmVersion, set); err != nil {
		return ProjectionSet{}, err
	}
	return set, nil
}
