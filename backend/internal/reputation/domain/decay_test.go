package domain_test

import (
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/reputation/domain"
	"github.com/stretchr/testify/require"
)

func TestDecayWeight(t *testing.T) {
	evaluatedAt := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	halfLife := domain.DefaultHalfLife
	day := 24 * time.Hour

	cases := []struct {
		name     string
		age      time.Duration
		expected float64
	}{
		{name: "当下观测满权重", age: 0, expected: 1},
		{name: "一个半衰期折半", age: 180 * day, expected: 0.5},
		{name: "两个半衰期四分之一", age: 360 * day, expected: 0.25},
		{name: "未来观测不放大", age: -30 * day, expected: 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := domain.DecayWeight(evaluatedAt.Add(-testCase.age), evaluatedAt, halfLife)
			require.InDelta(t, testCase.expected, got, 1e-9)
		})
	}
}

func TestDecayWeightWithoutHalfLifeDoesNotDecay(t *testing.T) {
	evaluatedAt := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	got := domain.DecayWeight(evaluatedAt.Add(-5*365*24*time.Hour), evaluatedAt, 0)
	require.Equal(t, 1.0, got)
}
