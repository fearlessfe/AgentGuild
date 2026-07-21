package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/contribution/domain"
	"github.com/stretchr/testify/require"
)

const (
	commitOne = "1111111111111111111111111111111111111111"
)

func TestNewContributionRequiresCompleteGlobalAndResourceAttribution(t *testing.T) {
	params := validContributionParams()
	contribution, err := domain.NewContribution(params)
	require.NoError(t, err)
	require.Equal(t, "agent-global", contribution.AgentID)
	require.Equal(t, "version-global", contribution.AgentVersionID)
	require.Equal(t, "tenant-sponsor", contribution.ResourceTenantID)
	require.Equal(t, "acme/widgets", contribution.CanonicalRepository)

	tests := []struct {
		name   string
		mutate func(*domain.NewContributionParams)
		field  string
	}{
		{"agent", func(p *domain.NewContributionParams) { p.AgentID = "" }, "agent_id"},
		{"version", func(p *domain.NewContributionParams) { p.AgentVersionID = "" }, "agent_version_id"},
		{"task", func(p *domain.NewContributionParams) { p.TaskID = "" }, "task_id"},
		{"execution", func(p *domain.NewContributionParams) { p.ExecutionID = "" }, "execution_id"},
		{"specification", func(p *domain.NewContributionParams) { p.TaskSpecificationVersionID = "" }, "task_specification_version_id"},
		{"repository", func(p *domain.NewContributionParams) { p.CanonicalRepository = "" }, "canonical_repository"},
		{"issue", func(p *domain.NewContributionParams) { p.IssueNumber = 0 }, "issue_number"},
		{"pull request", func(p *domain.NewContributionParams) { p.PullRequestNumber = 0 }, "pull_request_number"},
		{"commit", func(p *domain.NewContributionParams) { p.CommitSHA = "branch-name" }, "commit_sha"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := validContributionParams()
			test.mutate(&candidate)
			_, err := domain.NewContribution(candidate)
			require.ErrorIs(t, err, domain.ErrInvalidArgument)
			require.Equal(t, test.field, domain.FieldOf(err))
		})
	}
}

func TestNewContributionEventRequiresStableIdempotencyAndCompatibleOutcome(t *testing.T) {
	now := time.Now().UTC()
	event, err := domain.NewContributionEvent(domain.NewContributionEventParams{
		ContributionID:     "contribution-1",
		Provider:           domain.ProviderGitHub,
		ProviderDeliveryID: "delivery-1",
		ObjectVersion:      "check-run:10:completed",
		Type:               domain.EventCI,
		Outcome:            domain.OutcomeCIPassed,
		CommitSHA:          commitOne,
		Payload:            json.RawMessage(`{"z":1,"a":"ok"}`),
		OccurredAt:         now,
		ReceivedAt:         now,
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"a":"ok","z":1}`, string(event.Payload))

	_, err = domain.NewContributionEvent(domain.NewContributionEventParams{
		ContributionID: "contribution-1",
		Provider:       domain.ProviderGitHub,
		Type:           domain.EventCI,
		Outcome:        domain.OutcomeCIPassed,
		CommitSHA:      commitOne,
		Payload:        json.RawMessage(`{}`),
		OccurredAt:     now,
		ReceivedAt:     now,
	})
	require.ErrorIs(t, err, domain.ErrInvalidArgument)
	require.Equal(t, "idempotency_source", domain.FieldOf(err))

	_, err = domain.NewContributionEvent(domain.NewContributionEventParams{
		ContributionID: "contribution-1",
		Provider:       domain.ProviderGitHub,
		ObjectVersion:  "merge:1",
		Type:           domain.EventMerged,
		Outcome:        domain.OutcomeCIPassed,
		CommitSHA:      commitOne,
		Payload:        json.RawMessage(`{}`),
		OccurredAt:     now,
		ReceivedAt:     now,
	})
	require.ErrorIs(t, err, domain.ErrInvalidArgument)
	require.Equal(t, "outcome", domain.FieldOf(err))
}

func validContributionParams() domain.NewContributionParams {
	return domain.NewContributionParams{
		ID:                         "contribution-1",
		ResourceTenantID:           "tenant-sponsor",
		AgentID:                    "agent-global",
		AgentVersionID:             "version-global",
		TaskID:                     "task-1",
		ExecutionID:                "execution-1",
		TaskSpecificationVersionID: "specification-1",
		CanonicalRepository:        "acme/widgets",
		IssueNumber:                41,
		IssueURL:                   "https://github.com/acme/widgets/issues/41",
		Provider:                   domain.ProviderGitHub,
		PullRequestNumber:          52,
		PullRequestURL:             "https://github.com/acme/widgets/pull/52",
		CommitSHA:                  commitOne,
		AttributionStatus:          domain.AttributionVerified,
		Outcome:                    domain.OutcomeAttempt,
		CreatedAt:                  time.Now().UTC(),
	}
}
