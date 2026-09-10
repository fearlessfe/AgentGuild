package analysis_test

import (
	"testing"

	"agentguild.dev/agentguild/backend/internal/publictask/analysis"
	publictaskdomain "agentguild.dev/agentguild/backend/internal/publictask/domain"
	"github.com/stretchr/testify/require"
)

func TestNormalizeStripsVerifierRefFromManualCriteria(t *testing.T) {
	result := analysis.Result{
		AcceptanceCriteria: []publictaskdomain.AcceptanceCriterion{
			{ID: "AC-1", VerifierKind: " command ", VerifierRef: " public_tests "},
			// 模型常常给人工标准也填一个验证步骤；那会让它被误当成可自动判定。
			{ID: "AC-2", VerifierKind: "manual", VerifierRef: "public_tests"},
		},
	}.Normalize()

	require.Equal(t, "command", result.AcceptanceCriteria[0].VerifierKind)
	require.Equal(t, "public_tests", result.AcceptanceCriteria[0].VerifierRef)
	require.Empty(t, result.AcceptanceCriteria[1].VerifierRef)
}

func TestDifficultyClassFallsBackToStandard(t *testing.T) {
	cases := map[string]string{
		"unknown value":  "extremely-hard",
		"empty value":    "",
		"model prose":    "this issue looks substantial",
		"cased and spun": " COMPLEX ",
	}
	expected := map[string]string{
		"unknown value":  publictaskdomain.DifficultyStandard,
		"empty value":    publictaskdomain.DifficultyStandard,
		"model prose":    publictaskdomain.DifficultyStandard,
		"cased and spun": publictaskdomain.DifficultyComplex,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			result := analysis.Result{DifficultyClass: raw}.Normalize()
			require.Equal(t, expected[name], result.DifficultyClassOrDefault())
		})
	}
}
