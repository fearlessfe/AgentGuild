package domain_test

import (
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/publictask/domain"
	"github.com/stretchr/testify/require"
)

const specBaseCommit = "4444444444444444444444444444444444444444"

func specProjectionParams() domain.NewProjectionParams {
	return domain.NewProjectionParams{
		Projection: domain.Projection{
			ID:                         "public-1",
			ResourceTenantID:           "tenant-sponsor",
			TaskID:                     "task-1",
			TaskSpecificationVersionID: "specification-1",
			CanonicalRepository:        "acme/widgets",
			SourceIssueURL:             "https://github.com/acme/widgets/issues/41",
			IssueRevision:              "rev-1",
			BaseCommit:                 specBaseCommit,
			Title:                      "Fix widget",
			Summary:                    "Widget fails intermittently",
			ProblemDiagnosis:           "Race in retry loop",
			Impact:                     "Duplicate writes",
			ProposedSolution:           "Guard retries by method",
			Constraints:                []string{"keep API"},
			NonGoals:                   []string{"redesign"},
			AcceptanceCriteria: []domain.AcceptanceCriterion{
				{ID: "AC-1", Statement: "tests pass", Critical: true, VerifierKind: "command", ExpectedResult: "exit 0", VerifierRef: "public_tests"},
			},
			QualityLevel: domain.QualityStandard,
			PublishedAt:  time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC),
		},
		Checks: domain.PublicationChecks{
			QualityGatePassed: true, VisibilityAllowed: true,
			SensitivityCheckPassed: true, RepositoryPublic: true,
		},
	}
}

func TestSpecHashIsStableAndComputedByThePlatform(t *testing.T) {
	params := specProjectionParams()
	// 调用方传入的 spec_hash 必须被忽略。
	params.Projection.SpecHash = "not-a-real-hash"

	first, err := domain.NewProjection(params)
	require.NoError(t, err)
	require.Len(t, first.SpecHash, 64)
	require.NotEqual(t, "not-a-real-hash", first.SpecHash)

	second, err := domain.NewProjection(specProjectionParams())
	require.NoError(t, err)
	require.Equal(t, first.SpecHash, second.SpecHash, "identical specs must hash identically")
}

func TestSpecHashIgnoresDescriptiveProseButTracksTheContract(t *testing.T) {
	baseline, err := domain.NewProjection(specProjectionParams())
	require.NoError(t, err)

	t.Run("rewording the summary keeps the contract identity", func(t *testing.T) {
		params := specProjectionParams()
		params.Projection.Summary = "完全不同的描述文字"
		params.Projection.ProposedSolution = "换一种说法描述同一个方案"
		changed, err := domain.NewProjection(params)
		require.NoError(t, err)
		require.Equal(t, baseline.SpecHash, changed.SpecHash)
	})

	contractChanges := map[string]func(*domain.NewProjectionParams){
		"base commit": func(p *domain.NewProjectionParams) {
			p.Projection.BaseCommit = "5555555555555555555555555555555555555555"
		},
		"acceptance criteria": func(p *domain.NewProjectionParams) {
			p.Projection.AcceptanceCriteria[0].Critical = false
		},
		"verifier binding": func(p *domain.NewProjectionParams) {
			p.Projection.AcceptanceCriteria[0].VerifierRef = "hidden_tests"
		},
		"non goals": func(p *domain.NewProjectionParams) {
			p.Projection.NonGoals = []string{"redesign", "重写模块"}
		},
		"difficulty": func(p *domain.NewProjectionParams) {
			p.Projection.DifficultyClass = domain.DifficultyComplex
		},
	}
	for name, mutate := range contractChanges {
		t.Run("changing the "+name+" changes the hash", func(t *testing.T) {
			params := specProjectionParams()
			mutate(&params)
			changed, err := domain.NewProjection(params)
			require.NoError(t, err)
			require.NotEqual(t, baseline.SpecHash, changed.SpecHash)
		})
	}
}

func TestDifficultyClassDefaultsToStandardAndRejectsUnknownValues(t *testing.T) {
	params := specProjectionParams()
	projection, err := domain.NewProjection(params)
	require.NoError(t, err)
	require.Equal(t, domain.DifficultyStandard, projection.DifficultyClass)

	params = specProjectionParams()
	params.Projection.DifficultyClass = "impossible"
	_, err = domain.NewProjection(params)
	require.Error(t, err, "an unknown difficulty class must not reach the projection")
}
