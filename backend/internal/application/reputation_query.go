package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
)

// ReputationQueryService reads reputation projections from the store.
type ReputationQueryService struct {
	store Store
}

// NewReputationQueryService creates a reputation query service.
func NewReputationQueryService(store Store) *ReputationQueryService {
	return &ReputationQueryService{store: store}
}

// GetProjection returns the projection for a single key.
func (s *ReputationQueryService) GetProjection(ctx context.Context, principal auth.Principal, query reputationapp.GetProjection) (Envelope[reputationapp.ProjectionView], error) {
	var result Envelope[reputationapp.ProjectionView]
	if principal.TenantID == "" {
		return result, domain.ErrForbidden
	}
	if err := requireReputationScope(principal, "reputation:read"); err != nil {
		return result, err
	}

	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}

		records, err := tx.ListReputationProjectionsByAgentVersion(ctx, principal.TenantID, query.AgentVersionID)
		if err != nil {
			return err
		}

		var proj *reputationapp.ProjectionView
		for _, rec := range records {
			if rec.Projection.Key.Capability == query.Capability && rec.Projection.Key.TaskType == query.TaskType {
				view := toReputationProjectionView(rec.Projection)
				proj = &view
				break
			}
		}
		if proj == nil {
			proj = &reputationapp.ProjectionView{
				AgentVersionID: query.AgentVersionID,
				Capability:     query.Capability,
				TaskType:       query.TaskType,
				SampleSizeHint: "low",
			}
		}

		result = Envelope[reputationapp.ProjectionView]{
			Data: *proj,
			Meta: Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

func toReputationProjectionView(p reputationdomain.Projection) reputationapp.ProjectionView {
	return reputationapp.ProjectionView{
		AgentVersionID:         p.Key.AgentVersionID,
		Capability:             p.Key.Capability,
		TaskType:               p.Key.TaskType,
		TotalReviews:           p.TotalReviews,
		AcceptedCount:          p.AcceptedCount,
		RejectedCount:          p.RejectedCount,
		RevisionRequestedCount: p.RevisionRequestedCount,
		PassRate:               p.PassRate,
		ReworkRate:             p.ReworkRate,
		AvgReviewCostCents:     p.AvgReviewCostCents,
		AvgReviewLatencyMs:     p.AvgReviewLatencyMs,
		SampleSizeHint:         p.SampleSizeHint,
		AlgorithmVersion:       p.AlgorithmVersion,
	}
}

func requireReputationScope(principal auth.Principal, scope string) error {
	if principal.IsAdmin {
		return nil
	}
	if principal.TenantID == "" {
		return domain.ErrForbidden
	}
	for _, s := range principal.Scopes {
		if s == scope {
			return nil
		}
	}
	return domain.ErrForbidden
}
