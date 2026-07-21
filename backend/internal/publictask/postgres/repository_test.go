package postgres_test

import (
	"context"
	"testing"
	"time"

	publictaskapp "agentguild.dev/agentguild/backend/internal/publictask/application"
	"agentguild.dev/agentguild/backend/internal/publictask/domain"
	publictaskpostgres "agentguild.dev/agentguild/backend/internal/publictask/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestRepositoryPublishesWhitelistedProjectionAndRevokesItFromCatalog(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO tasks (
			tenant_id, id, publisher_agent_version_id, type, title, problem, deadline, status
		) VALUES (
			'tenant-sponsor', 'task-1', 'publisher-version', 'bug', 'Fix widget',
			'Widget fails', clock_timestamp() + interval '1 day', 'open'
		)`)
	require.NoError(t, err)
	repository := publictaskpostgres.NewRepository(db)
	projection := newProjection(t)
	require.NoError(t, repository.Insert(ctx, projection))

	got, err := repository.GetByID(ctx, projection.ID)
	require.NoError(t, err)
	require.Equal(t, projection.TaskSpecificationVersionID, got.TaskSpecificationVersionID)
	require.Equal(t, projection.AcceptanceCriteria, got.AcceptanceCriteria)
	require.Equal(t, projection.EvidenceRefs, got.EvidenceRefs)

	items, err := repository.ListPublished(ctx, 20)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "public-task-1", items[0].ID)
	items, err = repository.ListPublishedPage(ctx, publictaskapp.PublishedPageQuery{
		AfterPublishedAt: projection.PublishedAt,
		AfterID:          projection.ID,
		Limit:            20,
	})
	require.NoError(t, err)
	require.Empty(t, items)

	now := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, projection.Revoke("governor-1", "source became private", now))
	require.NoError(t, repository.Update(ctx, projection))
	items, err = repository.ListPublished(ctx, 20)
	require.NoError(t, err)
	require.Empty(t, items)
	got, err = repository.GetByID(ctx, projection.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusRevoked, got.Status)
	require.Equal(t, "source became private", got.RevocationReason)
}

func newProjection(t *testing.T) *domain.Projection {
	t.Helper()
	p, err := domain.NewProjection(domain.NewProjectionParams{
		Checks: domain.PublicationChecks{QualityGatePassed: true, VisibilityAllowed: true, SensitivityCheckPassed: true, RepositoryPublic: true},
		Projection: domain.Projection{
			ID: "public-task-1", ResourceTenantID: "tenant-sponsor", TaskID: "task-1",
			TaskSpecificationVersionID: "spec-1", CanonicalRepository: "acme/widgets",
			SourceIssueURL: "https://github.com/acme/widgets/issues/41", IssueRevision: "revision-1",
			BaseCommit: "1111111111111111111111111111111111111111",
			Title:      "Fix widget", Summary: "Fix crash", ProblemDiagnosis: "Nil state crashes",
			Impact: "Widget users", ProposedSolution: "Guard state",
			ImplementationSteps: []string{"test", "fix"}, Constraints: []string{"preserve API"},
			NonGoals: []string{"redesign"}, Risks: []string{"behavior"},
			AcceptanceCriteria: []domain.AcceptanceCriterion{{ID: "c1", Statement: "tests pass", Critical: true, VerifierKind: "command", ExpectedResult: "exit 0"}},
			EvidenceRefs:       []domain.EvidenceRef{{CommitSHA: "1111111111111111111111111111111111111111", Path: "widget.go", StartLine: 1, EndLine: 2, ContentHash: "sha256:content"}},
			QualityLevel:       domain.QualityStandard, PublishedAt: time.Now().UTC().Truncate(time.Microsecond),
		},
	})
	require.NoError(t, err)
	return p
}
