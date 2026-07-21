package domain_test

import (
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/publictask/domain"
	"github.com/stretchr/testify/require"
)

func TestPublicProjectionRequiresEveryFailClosedPublicationCheck(t *testing.T) {
	checks := []struct {
		name   string
		mutate func(*domain.PublicationChecks)
	}{
		{"quality", func(c *domain.PublicationChecks) { c.QualityGatePassed = false }},
		{"visibility", func(c *domain.PublicationChecks) { c.VisibilityAllowed = false }},
		{"sensitivity", func(c *domain.PublicationChecks) { c.SensitivityCheckPassed = false }},
		{"repository", func(c *domain.PublicationChecks) { c.RepositoryPublic = false }},
	}
	for _, test := range checks {
		t.Run(test.name, func(t *testing.T) {
			params := validProjectionParams()
			test.mutate(&params.Checks)
			_, err := domain.NewProjection(params)
			require.ErrorIs(t, err, domain.ErrPublicationRejected)
		})
	}
}

func TestPublicProjectionValidatesWhitelistedCriteriaEvidenceAndRevocation(t *testing.T) {
	projection, err := domain.NewProjection(validProjectionParams())
	require.NoError(t, err)
	require.Equal(t, domain.StatusPublished, projection.Status)
	now := time.Now().UTC().Add(time.Hour)
	require.NoError(t, projection.Revoke("governor-1", "source became private", now))
	require.Equal(t, domain.StatusRevoked, projection.Status)
	require.Equal(t, &now, projection.RevokedAt)
	require.ErrorIs(t, projection.Revoke("governor-1", "again", now), domain.ErrStateConflict)
}

func validProjectionParams() domain.NewProjectionParams {
	return domain.NewProjectionParams{
		Checks: domain.PublicationChecks{
			QualityGatePassed: true, VisibilityAllowed: true,
			SensitivityCheckPassed: true, RepositoryPublic: true,
		},
		Projection: domain.Projection{
			ID: "public-task-1", ResourceTenantID: "tenant-sponsor", TaskID: "task-1",
			TaskSpecificationVersionID: "spec-1", CanonicalRepository: "acme/widgets",
			SourceIssueURL: "https://github.com/acme/widgets/issues/41", IssueRevision: "revision-1",
			BaseCommit: "1111111111111111111111111111111111111111",
			Title:      "Fix widget", Summary: "Fix widget crash", ProblemDiagnosis: "Nil state crashes",
			Impact: "Widget users", ProposedSolution: "Guard state and add regression coverage",
			ImplementationSteps: []string{"add regression test", "guard state"},
			Constraints:         []string{"preserve API"}, NonGoals: []string{"redesign widget"}, Risks: []string{"behavior change"},
			AcceptanceCriteria: []domain.AcceptanceCriterion{{
				ID: "criterion-1", Statement: "regression is fixed", Critical: true,
				VerifierKind: "command", ExpectedResult: "tests pass",
			}},
			EvidenceRefs: []domain.EvidenceRef{{
				CommitSHA: "1111111111111111111111111111111111111111", Path: "widget.go",
				StartLine: 10, EndLine: 12, ContentHash: "sha256:evidence",
			}},
			QualityLevel: domain.QualityStandard, PublishedAt: time.Now().UTC(),
		},
	}
}
