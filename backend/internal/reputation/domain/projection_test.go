package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
)

func TestProjectionShowsLowSampleHint(t *testing.T) {
	p := reputationdomain.NewProjection("agent-v1", "go", "code")
	p.Apply(reputationdomain.ReviewSignal{Decision: reviewdomain.DecisionAccepted, CostCents: 100, LatencyMs: 5000})
	if p.SampleSizeHint != "low" {
		t.Fatalf("hint=%s, want low", p.SampleSizeHint)
	}
}

func TestProjectionSampleSizeThresholds(t *testing.T) {
	cases := []struct {
		n    int
		hint string
	}{
		{0, "low"},
		{1, "low"},
		{4, "low"},
		{5, "medium"},
		{19, "medium"},
		{20, "high"},
		{25, "high"},
	}
	for _, tc := range cases {
		t.Run(tc.hint, func(t *testing.T) {
			p := reputationdomain.NewProjection("agent-v1", "go", "code")
			for i := 0; i < tc.n; i++ {
				p.Apply(reputationdomain.ReviewSignal{Decision: reviewdomain.DecisionAccepted, CostCents: 100, LatencyMs: 1000})
			}
			require.Equal(t, tc.hint, p.SampleSizeHint)
		})
	}
}

func TestProjectionCountsAndRates(t *testing.T) {
	p := reputationdomain.NewProjection("agent-v1", "go", "code")
	signals := []reputationdomain.ReviewSignal{
		{Decision: reviewdomain.DecisionAccepted, CostCents: 100, LatencyMs: 1000},
		{Decision: reviewdomain.DecisionAccepted, CostCents: 200, LatencyMs: 2000},
		{Decision: reviewdomain.DecisionRejected, CostCents: 50, LatencyMs: 500},
		{Decision: reviewdomain.DecisionRevisionRequested, CostCents: 150, LatencyMs: 1500},
	}
	for _, s := range signals {
		p.Apply(s)
	}

	require.Equal(t, 4, p.TotalReviews)
	require.Equal(t, 2, p.AcceptedCount)
	require.Equal(t, 1, p.RejectedCount)
	require.Equal(t, 1, p.RevisionRequestedCount)
	require.InDelta(t, 0.5, p.PassRate, 1e-9)
	require.InDelta(t, 0.25, p.ReworkRate, 1e-9)
	require.InDelta(t, 125.0, p.AvgReviewCostCents, 1e-9)
	require.InDelta(t, 1250.0, p.AvgReviewLatencyMs, 1e-9)
}

func TestProjectionAlgorithmVersion(t *testing.T) {
	p := reputationdomain.NewProjection("agent-v1", "go", "code")
	require.Equal(t, "2026-07-04-v1", p.AlgorithmVersion)
	p.Apply(reputationdomain.ReviewSignal{Decision: reviewdomain.DecisionAccepted, CostCents: 100, LatencyMs: 1000})
	require.Equal(t, "2026-07-04-v1", p.AlgorithmVersion)
}

func TestProjectionKey(t *testing.T) {
	p := reputationdomain.NewProjection("agent-v1", "go", "code")
	require.Equal(t, "agent-v1", p.Key.AgentVersionID)
	require.Equal(t, "go", p.Key.Capability)
	require.Equal(t, "code", p.Key.TaskType)
}
