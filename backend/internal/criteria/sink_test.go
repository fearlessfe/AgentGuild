package criteria_test

import (
	"context"
	"testing"
	"time"

	contributionapp "agentguild.dev/agentguild/backend/internal/contribution/application"
	contributiondomain "agentguild.dev/agentguild/backend/internal/contribution/domain"
	contributionpostgres "agentguild.dev/agentguild/backend/internal/contribution/postgres"
	"agentguild.dev/agentguild/backend/internal/criteria"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	reviewapp "agentguild.dev/agentguild/backend/internal/review/application"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// 一份混合规格：两条自动化标准（一条绑定会跑的步骤，一条绑定不会跑的步骤）、
// 一条人工标准，以及一条绑定了未知步骤的标准。
const mixedCriteria = `[
	{"id":"AC-1","statement":"tests pass","critical":true,"verifier_kind":"command","expected_result":"exit 0","verifier_ref":"public_tests"},
	{"id":"AC-2","statement":"no vulnerabilities","critical":true,"verifier_kind":"command","expected_result":"clean","verifier_ref":"security_scan"},
	{"id":"AC-3","statement":"maintainer confirms the fix","critical":true,"verifier_kind":"manual","expected_result":"approval"},
	{"id":"AC-4","statement":"vibes are good","critical":false,"verifier_kind":"command","expected_result":"ok","verifier_ref":"vibe_check"}
]`

func newSinks(t *testing.T, db *pgxpool.Pool) (*criteria.ValidationSink, *criteria.ReviewSink, contributionapp.CriterionRepository) {
	t.Helper()
	repository := contributionpostgres.NewCriterionRepository(db)
	recorder, err := contributionapp.NewCriterionRecorder(repository, contributionapp.CriterionRecorderOptions{})
	require.NoError(t, err)
	source := criteria.NewProjectionCriteriaSource(db)
	validationRecorder, err := contributionapp.NewValidationCriterionRecorder(recorder, source)
	require.NoError(t, err)
	return criteria.NewValidationSink(validationRecorder), criteria.NewReviewSink(recorder, source), repository
}

func coverageFor(t *testing.T, repository contributionapp.CriterionRepository) contributiondomain.CriterionCoverage {
	t.Helper()
	latest, err := repository.ListLatest(context.Background(), "tenant-sponsor", "execution-1")
	require.NoError(t, err)
	return contributiondomain.SummarizeCriteria([]contributiondomain.SpecCriterion{
		{ID: "AC-1", Critical: true}, {ID: "AC-2", Critical: true},
		{ID: "AC-3", Critical: true}, {ID: "AC-4", Critical: false},
	}, latest)
}

func TestValidationAndReviewTogetherCloseOutAcceptance(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedPublicTask(t, db, mixedCriteria)
	validationSink, reviewSink, repository := newSinks(t, db)
	ctx := context.Background()
	observedAt := time.Now().UTC().Truncate(time.Microsecond)

	// 验证只跑了 public_tests 与 security_scan。
	require.NoError(t, validationSink.RecordValidationOutcome(ctx, gitapp.ValidationCriterionOutcome{
		TenantID:      "tenant-sponsor",
		ExecutionID:   "execution-1",
		JobID:         "validation-1",
		ConfigVersion: "config-7",
		StepResults:   map[string]bool{"public_tests": true, "security_scan": true},
		ObservedAt:    observedAt,
	}))

	coverage := coverageFor(t, repository)
	require.Equal(t, 2, coverage.RequiredPassed)
	require.False(t, coverage.AllRequiredPassed(), "the manual criterion is still unverified")
	require.ElementsMatch(t, []string{"AC-3", "AC-4"}, coverage.Unverified,
		"a criterion bound to an unknown step must stay unverified, not pass")

	// 评审补上人工标准。
	require.NoError(t, reviewSink.RecordReviewOutcome(ctx, reviewapp.ReviewCriterionOutcome{
		TenantID:    "tenant-sponsor",
		ExecutionID: "execution-1",
		ReviewID:    "review-1",
		ReviewerID:  "alice",
		Verdicts: []reviewapp.CriterionVerdict{
			{CriterionID: "AC-3", Passed: true, EvidenceURI: "https://example.test/review/1"},
			// 规格之外的标识必须被丢弃。
			{CriterionID: "AC-INVENTED", Passed: true},
		},
		ObservedAt: observedAt.Add(time.Hour),
	}))

	coverage = coverageFor(t, repository)
	require.Equal(t, 3, coverage.RequiredPassed)
	require.True(t, coverage.AllRequiredPassed(), "all required criteria are now verified and passing")
	require.NotContains(t, coverage.ResultsByID, "AC-INVENTED")
}

func TestValidationSinkIsIdempotentAgainstWorkerRetries(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedPublicTask(t, db, mixedCriteria)
	validationSink, _, repository := newSinks(t, db)
	ctx := context.Background()
	outcome := gitapp.ValidationCriterionOutcome{
		TenantID:    "tenant-sponsor",
		ExecutionID: "execution-1",
		JobID:       "validation-1",
		StepResults: map[string]bool{"public_tests": true},
		ObservedAt:  time.Now().UTC(),
	}

	require.NoError(t, validationSink.RecordValidationOutcome(ctx, outcome))
	require.NoError(t, validationSink.RecordValidationOutcome(ctx, outcome))

	history, err := repository.ListHistory(ctx, "tenant-sponsor", "execution-1")
	require.NoError(t, err)
	require.Len(t, history, 1, "a retried validation job must not append a second fact")
}
