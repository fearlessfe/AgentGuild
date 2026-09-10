package criteria_test

import (
	"context"
	"testing"

	"agentguild.dev/agentguild/backend/internal/criteria"
	publictaskdomain "agentguild.dev/agentguild/backend/internal/publictask/domain"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

const baseCommit = "3333333333333333333333333333333333333333"

func seedPublicTask(t *testing.T, db *pgxpool.Pool, acceptanceCriteria string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO tasks (
			tenant_id, id, publisher_agent_version_id, type, title, problem,
			deadline, status
		) VALUES (
			'tenant-sponsor', 'task-1', 'publisher-version', 'bug',
			'Fix widget', 'Widget fails', clock_timestamp() + interval '1 day', 'open'
		);
		INSERT INTO executions (
			tenant_id, id, task_id, agent_version_id, status, lease_generation
		) VALUES
			('tenant-sponsor', 'execution-1', 'task-1', 'executor-version', 'validating', 1)`)
	require.NoError(t, err)

	_, err = db.Exec(ctx, `
		INSERT INTO public_task_projections (
			id, resource_tenant_id, task_id, task_specification_version_id,
			canonical_repository, source_issue_url, issue_revision, base_commit,
			title, summary, problem_diagnosis, impact, proposed_solution,
			acceptance_criteria, quality_level, status, published_at
		) VALUES (
			'public-1', 'tenant-sponsor', 'task-1', 'specification-1',
			'acme/widgets', 'https://github.com/acme/widgets/issues/41', 'rev-1', $1,
			'Fix widget', 'Widget fails intermittently', 'Race in retry loop',
			'Duplicate writes', 'Guard retries by method',
			$2::jsonb, 'standard', 'published', clock_timestamp()
		)`, baseCommit, acceptanceCriteria)
	require.NoError(t, err)
}

func TestProjectionCriteriaSourceMarksOnlyKnownStepsAutomated(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedPublicTask(t, db, `[
		{"id":"AC-1","statement":"tests pass","critical":true,"verifier_kind":"command","expected_result":"exit 0","verifier_ref":"public_tests"},
		{"id":"AC-2","statement":"no secrets","critical":true,"verifier_kind":"command","expected_result":"clean","verifier_ref":"sniff_test"},
		{"id":"AC-3","statement":"maintainer agrees","critical":false,"verifier_kind":"manual","expected_result":"approval"}
	]`)
	source := criteria.NewProjectionCriteriaSource(db)

	spec, err := source.CriteriaForExecution(context.Background(), "tenant-sponsor", "execution-1")

	require.NoError(t, err)
	require.Equal(t, "task-1", spec.TaskID)
	require.Len(t, spec.Criteria, 3)

	require.True(t, spec.Criteria[0].Automated)
	require.Equal(t, "public_tests", spec.Criteria[0].VerifierRef)

	// 分析器给出的验证步骤不在白名单里，必须降级为不可自动判定。
	require.False(t, spec.Criteria[1].Automated, "an unknown verifier step must not be auto-judged")
	require.Empty(t, spec.Criteria[1].VerifierRef)

	require.False(t, spec.Criteria[2].Automated)
}

func TestProjectionCriteriaSourceReturnsNothingForUnpublishedWork(t *testing.T) {
	db := testdb.StartPostgres(t)
	source := criteria.NewProjectionCriteriaSource(db)

	spec, err := source.CriteriaForExecution(context.Background(), "tenant-sponsor", "missing-execution")

	require.NoError(t, err)
	require.Empty(t, spec.TaskID)
	require.Empty(t, spec.Criteria)
}

func TestMapCriteriaRejectsAutomationWithoutAReference(t *testing.T) {
	specs := criteria.MapCriteria([]publictaskdomain.AcceptanceCriterion{
		{ID: "AC-1", Statement: "tests pass", Critical: true, VerifierKind: "command", ExpectedResult: "exit 0"},
	})

	require.Len(t, specs, 1)
	require.False(t, specs[0].Automated, "a command criterion without verifier_ref cannot be auto-judged")
}
