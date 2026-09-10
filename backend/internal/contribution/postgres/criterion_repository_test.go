package postgres_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/contribution/domain"
	contributionpostgres "agentguild.dev/agentguild/backend/internal/contribution/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/stretchr/testify/require"
)

func criterionResult(t *testing.T, criterionID string, passed bool, source domain.SourceKind, sourceID string, observedAt time.Time) *domain.CriterionResult {
	t.Helper()
	result, err := domain.NewCriterionResult(domain.NewCriterionResultParams{
		ResourceTenantID: "tenant-sponsor",
		TaskID:           "task-1",
		ExecutionID:      "execution-1",
		CriterionID:      criterionID,
		Critical:         true,
		VerifierKind:     domain.VerifierCommand,
		Passed:           passed,
		SourceKind:       source,
		SourceID:         sourceID,
		VerifiedBy:       "validation_worker",
		ObservedAt:       observedAt,
	})
	require.NoError(t, err)
	return result
}

func TestCriterionRepositoryIsAppendOnlyAndIdempotentPerSource(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedContributionFacts(t, db)
	repository := contributionpostgres.NewCriterionRepository(db)
	ctx := context.Background()
	observedAt := time.Now().UTC().Truncate(time.Microsecond)

	first := criterionResult(t, "AC-1", true, domain.SourceValidationJob, "validation-1", observedAt)
	stored, created, err := repository.Record(ctx, first)
	require.NoError(t, err)
	require.True(t, created)
	require.Positive(t, stored.ID)

	replayed, created, err := repository.Record(ctx, first)
	require.NoError(t, err)
	require.False(t, created, "the same validation job must not append a second fact")
	require.Equal(t, stored.ID, replayed.ID)

	// 同一来源改写结论必须被拒绝，否则账本就不是事实了。
	tampered := *first
	tampered.Passed = false
	_, _, err = repository.Record(ctx, &tampered)
	require.ErrorIs(t, err, domain.ErrStateConflict)

	_, err = db.Exec(ctx, `UPDATE execution_criterion_results SET passed=false WHERE id=$1`, stored.ID)
	require.Error(t, err, "criterion result updates must be rejected by the database")
	_, err = db.Exec(ctx, `DELETE FROM execution_criterion_results WHERE id=$1`, stored.ID)
	require.Error(t, err, "criterion result deletes must be rejected by the database")
}

func TestCriterionRepositoryLatestPrefersMostRecentObservation(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedContributionFacts(t, db)
	repository := contributionpostgres.NewCriterionRepository(db)
	ctx := context.Background()
	observedAt := time.Now().UTC().Truncate(time.Microsecond)

	// 自动验证先判失败，随后人工复核推翻结论。
	automated := criterionResult(t, "AC-1", false, domain.SourceValidationJob, "validation-1", observedAt)
	_, _, err := repository.Record(ctx, automated)
	require.NoError(t, err)

	manual := criterionResult(t, "AC-1", true, domain.SourceReview, "review-1", observedAt.Add(time.Hour))
	manual.VerifierKind = domain.VerifierManual
	manual.VerifiedBy = "reviewer:alice"
	_, _, err = repository.Record(ctx, manual)
	require.NoError(t, err)

	other := criterionResult(t, "AC-2", true, domain.SourceValidationJob, "validation-1", observedAt)
	_, _, err = repository.Record(ctx, other)
	require.NoError(t, err)

	latest, err := repository.ListLatest(ctx, "tenant-sponsor", "execution-1")
	require.NoError(t, err)
	require.Len(t, latest, 2)
	require.Equal(t, "AC-1", latest[0].CriterionID)
	require.True(t, latest[0].Passed, "the later manual review must win")
	require.Equal(t, domain.SourceReview, latest[0].SourceKind)
	require.Equal(t, "AC-2", latest[1].CriterionID)

	history, err := repository.ListHistory(ctx, "tenant-sponsor", "execution-1")
	require.NoError(t, err)
	require.Len(t, history, 3, "the overturned automated verdict stays in the ledger")

	coverage := domain.SummarizeCriteria(
		[]domain.SpecCriterion{{ID: "AC-1", Critical: true}, {ID: "AC-2", Critical: true}},
		latest,
	)
	require.True(t, coverage.AllRequiredPassed())
}

func TestCriterionRepositoryIsolatesTenants(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedContributionFacts(t, db)
	repository := contributionpostgres.NewCriterionRepository(db)
	ctx := context.Background()

	_, _, err := repository.Record(ctx, criterionResult(t, "AC-1", true,
		domain.SourceValidationJob, "validation-1", time.Now().UTC()))
	require.NoError(t, err)

	latest, err := repository.ListLatest(ctx, "tenant-other", "execution-1")
	require.NoError(t, err)
	require.Empty(t, latest)
}
