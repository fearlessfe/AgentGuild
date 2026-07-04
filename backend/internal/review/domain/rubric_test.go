package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
)

func mustNewRubricVersion(t *testing.T) *reviewdomain.RubricVersion {
	t.Helper()
	now := time.Now()
	version, err := reviewdomain.NewRubricVersion(
		"rubric-1",
		"tenant-1",
		"Code Quality Rubric",
		1,
		[]reviewdomain.RubricDimension{
			{ID: "correctness", Name: "Correctness"},
			{ID: "readability", Name: "Readability"},
		},
		map[string]float64{
			"correctness": 0.6,
			"readability": 0.4,
		},
		"v1",
		now,
	)
	require.NoError(t, err)
	return version
}

func TestRubricVersionConstructorRejectsInvalidFields(t *testing.T) {
	now := time.Now()
	validDimensions := []reviewdomain.RubricDimension{{ID: "correctness", Name: "Correctness"}}
	validWeights := map[string]float64{"correctness": 1.0}

	cases := []struct {
		testName         string
		id               string
		tenantID         string
		name             string
		versionNumber    int
		dimensions       []reviewdomain.RubricDimension
		weights          map[string]float64
		algorithmVersion string
		field            string
	}{
		{testName: "empty id", tenantID: "t", name: "n", versionNumber: 1, dimensions: validDimensions, weights: validWeights, algorithmVersion: "v1", field: "id"},
		{testName: "empty tenant_id", id: "id", name: "n", versionNumber: 1, dimensions: validDimensions, weights: validWeights, algorithmVersion: "v1", field: "tenant_id"},
		{testName: "empty name", id: "id", tenantID: "t", versionNumber: 1, dimensions: validDimensions, weights: validWeights, algorithmVersion: "v1", field: "name"},
		{testName: "zero version_number", id: "id", tenantID: "t", name: "n", versionNumber: 0, dimensions: validDimensions, weights: validWeights, algorithmVersion: "v1", field: "version_number"},
		{testName: "empty dimensions", id: "id", tenantID: "t", name: "n", versionNumber: 1, weights: validWeights, algorithmVersion: "v1", field: "dimensions"},
		{testName: "dimension without id", id: "id", tenantID: "t", name: "n", versionNumber: 1, dimensions: []reviewdomain.RubricDimension{{Name: "Correctness"}}, weights: map[string]float64{"": 1.0}, algorithmVersion: "v1", field: "dimension.id"},
		{testName: "dimension without name", id: "id", tenantID: "t", name: "n", versionNumber: 1, dimensions: []reviewdomain.RubricDimension{{ID: "correctness"}}, weights: map[string]float64{"correctness": 1.0}, algorithmVersion: "v1", field: "dimension.name"},
		{testName: "duplicate dimension id", id: "id", tenantID: "t", name: "n", versionNumber: 1, dimensions: []reviewdomain.RubricDimension{{ID: "correctness", Name: "A"}, {ID: "correctness", Name: "B"}}, weights: map[string]float64{"correctness": 1.0}, algorithmVersion: "v1", field: "dimensions"},
		{testName: "missing weight", id: "id", tenantID: "t", name: "n", versionNumber: 1, dimensions: validDimensions, weights: map[string]float64{}, algorithmVersion: "v1", field: "weights"},
		{testName: "zero weight", id: "id", tenantID: "t", name: "n", versionNumber: 1, dimensions: validDimensions, weights: map[string]float64{"correctness": 0}, algorithmVersion: "v1", field: "weights"},
		{testName: "empty algorithm_version", id: "id", tenantID: "t", name: "n", versionNumber: 1, dimensions: validDimensions, weights: validWeights, field: "algorithm_version"},
		{testName: "zero created_at", id: "id", tenantID: "t", name: "n", versionNumber: 1, dimensions: validDimensions, weights: validWeights, algorithmVersion: "v1", field: "created_at"},
	}

	for _, tc := range cases {
		t.Run(tc.testName, func(t *testing.T) {
			var nowArg time.Time
			if tc.field != "created_at" {
				nowArg = now
			}
			version, err := reviewdomain.NewRubricVersion(tc.id, tc.tenantID, tc.name, tc.versionNumber, tc.dimensions, tc.weights, tc.algorithmVersion, nowArg)
			require.Nil(t, version)
			assertInvalidArgument(t, err, tc.field)
		})
	}
}

func TestRubricVersionCompleteRequiresAllDimensions(t *testing.T) {
	version := mustNewRubricVersion(t)

	require.False(t, version.Complete(nil))
	require.False(t, version.Complete([]reviewdomain.RubricScore{
		{Dimension: "correctness", Score: 80},
	}))
	require.True(t, version.Complete([]reviewdomain.RubricScore{
		{Dimension: "correctness", Score: 80},
		{Dimension: "readability", Score: 70},
	}))
}

func TestRubricVersionCompleteRejectsOutOfRangeScores(t *testing.T) {
	version := mustNewRubricVersion(t)

	require.False(t, version.Complete([]reviewdomain.RubricScore{
		{Dimension: "correctness", Score: -1},
		{Dimension: "readability", Score: 70},
	}))
	require.False(t, version.Complete([]reviewdomain.RubricScore{
		{Dimension: "correctness", Score: 80},
		{Dimension: "readability", Score: 101},
	}))
}

func TestRubricVersionIsActiveByDefault(t *testing.T) {
	version := mustNewRubricVersion(t)
	require.True(t, version.IsActive)
}
