package application_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
)

func TestProjectorGroupsByKey(t *testing.T) {
	ctx := context.Background()
	signals := []reputationdomain.ReviewSignal{
		{AgentVersionID: "agent-v1", Capability: "go", TaskType: "code", Decision: reviewdomain.DecisionAccepted, CostCents: 100, LatencyMs: 1000},
		{AgentVersionID: "agent-v1", Capability: "go", TaskType: "code", Decision: reviewdomain.DecisionRejected, CostCents: 200, LatencyMs: 2000},
		{AgentVersionID: "agent-v2", Capability: "go", TaskType: "code", Decision: reviewdomain.DecisionAccepted, CostCents: 50, LatencyMs: 500},
		{AgentVersionID: "agent-v1", Capability: "python", TaskType: "code", Decision: reviewdomain.DecisionAccepted, CostCents: 150, LatencyMs: 1500},
	}

	projector := reputationapp.NewProjector()
	projections, err := projector.Project(ctx, signals)
	require.NoError(t, err)
	require.Len(t, projections, 3)

	byKey := make(map[reputationdomain.ProjectionKey]reputationdomain.Projection)
	for _, p := range projections {
		byKey[p.Key] = p
	}

	v1Go := byKey[reputationdomain.ProjectionKey{AgentVersionID: "agent-v1", Capability: "go", TaskType: "code"}]
	require.Equal(t, 2, v1Go.TotalReviews)
	require.InDelta(t, 0.5, v1Go.PassRate, 1e-9)
	require.InDelta(t, 150.0, v1Go.AvgReviewCostCents, 1e-9)

	v2Go := byKey[reputationdomain.ProjectionKey{AgentVersionID: "agent-v2", Capability: "go", TaskType: "code"}]
	require.Equal(t, 1, v2Go.TotalReviews)
	require.InDelta(t, 1.0, v2Go.PassRate, 1e-9)

	v1Python := byKey[reputationdomain.ProjectionKey{AgentVersionID: "agent-v1", Capability: "python", TaskType: "code"}]
	require.Equal(t, 1, v1Python.TotalReviews)
}

func TestProjectorEmptySignals(t *testing.T) {
	ctx := context.Background()
	projector := reputationapp.NewProjector()
	projections, err := projector.Project(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, projections)
}
