package rest_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	agentexperienceapp "agentguild.dev/agentguild/backend/internal/agentexperience/application"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func TestListAgentExperiencesReturnsSnakeCaseFields(t *testing.T) {
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	reviewedAt := now.Add(-time.Hour)
	experiences := &fakeExperienceService{
		list: []agentexperienceapp.CandidateSummary{
			{
				ID:                     "xp-1",
				TenantID:               "tenant-1",
				AgentID:                "agent-1",
				SourceTaskID:           "task-1",
				SourceSubmissionID:     "sub-1",
				SourceReviewID:         "rev-1",
				EvidenceRef:            "sha256:evidence",
				ContentHash:            "sha256:content",
				ApplicableCapabilities: []string{"code"},
				TenantScope:            "tenant",
				SensitivityClass:       "low",
				Status:                 "approved",
				PolicyReason:           "verified safe",
				ReviewedBy:             "owner",
				ReviewedAt:             &reviewedAt,
				CreatedAt:              now,
			},
		},
	}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithSession(testSessionSecret, false),
		rest.WithExperienceService(experiences),
	).Router()

	res := getWithSession(t, server, "/v1/agents/agent-1/experiences?status=approved", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	require.Len(t, experiences.listCalls, 1)
	require.Equal(t, "tenant-1", experiences.listCalls[0].tenantID)
	require.Equal(t, "agent-1", experiences.listCalls[0].agentID)
	require.Equal(t, "approved", experiences.listCalls[0].status)

	body := res.Body.String()
	require.Contains(t, body, `"source_task_id":`)
	require.Contains(t, body, `"source_submission_id":`)
	require.Contains(t, body, `"source_review_id":`)
	require.Contains(t, body, `"evidence_ref":`)
	require.Contains(t, body, `"content_hash":`)
	require.Contains(t, body, `"applicable_capabilities":`)
	require.Contains(t, body, `"tenant_scope":`)
	require.Contains(t, body, `"sensitivity_class":`)
	require.Contains(t, body, `"policy_reason":`)
	require.Contains(t, body, `"reviewed_by":`)
	require.Contains(t, body, `"reviewed_at":`)
	require.Contains(t, body, `"created_at":`)
	require.NotContains(t, body, `"SourceTaskID"`)
	require.NotContains(t, body, `"SourceSubmissionID"`)
	require.NotContains(t, body, `"SourceReviewID"`)
	require.NotContains(t, body, `"EvidenceRef"`)
	require.NotContains(t, body, `"ContentHash"`)
	require.NotContains(t, body, `"ApplicableCapabilities"`)
	require.NotContains(t, body, `"TenantScope"`)
	require.NotContains(t, body, `"SensitivityClass"`)
	require.NotContains(t, body, `"PolicyReason"`)
	require.NotContains(t, body, `"ReviewedBy"`)
	require.NotContains(t, body, `"ReviewedAt"`)
	require.NotContains(t, body, `"CreatedAt"`)
}

type fakeExperienceService struct {
	list      []agentexperienceapp.CandidateSummary
	listErr   error
	listCalls []struct {
		tenantID string
		agentID  string
		status   string
	}
}

func (f *fakeExperienceService) ListCandidates(ctx context.Context, tenantID, agentID, status string) ([]agentexperienceapp.CandidateSummary, error) {
	f.listCalls = append(f.listCalls, struct {
		tenantID string
		agentID  string
		status   string
	}{tenantID: tenantID, agentID: agentID, status: status})
	return f.list, f.listErr
}

func (f *fakeExperienceService) GetCandidate(ctx context.Context, tenantID, agentID, candidateID string) (*agentexperienceapp.CandidateSummary, error) {
	return nil, nil
}

func (f *fakeExperienceService) ExtractCandidate(ctx context.Context, cmd agentexperienceapp.ExtractCandidate) (*agentexperienceapp.ExtractCandidateResponse, error) {
	return nil, nil
}

func (f *fakeExperienceService) ReviewCandidate(ctx context.Context, cmd agentexperienceapp.ReviewCandidate) error {
	return nil
}
