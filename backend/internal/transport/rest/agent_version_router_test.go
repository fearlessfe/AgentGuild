package rest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	agentversionapp "agentguild.dev/agentguild/backend/internal/agentversion/application"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func TestDiffAgentVersionReturnsStructuredView(t *testing.T) {
	versions := &fakeVersionService{
		diff: &agentversionapp.VersionDiff{
			BaseVersionID: "base-v1",
			Added: map[string]agentversionapp.RefChange{
				"capability:go": {From: "", To: "go"},
				"skill:lint":    {From: "", To: "lint"},
				"tool:github":   {From: "", To: "github"},
			},
			Removed: map[string]agentversionapp.RefChange{
				"capability:python": {From: "python", To: ""},
			},
			Changed: map[string]agentversionapp.RefChange{
				"runtime": {From: "r1", To: "r2"},
				"model":   {From: "m1", To: "m2"},
			},
		},
	}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithSession(testSessionSecret, false),
		rest.WithVersionService(versions),
	).Router()

	res := postJSONWithSession(t, server, "/v1/agents/agent-1/versions/v2/diff", `{}`, sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	require.Len(t, versions.diffCalls, 1)
	require.Equal(t, "agent-1", versions.diffCalls[0].agentID)
	require.Equal(t, "v2", versions.diffCalls[0].versionID)

	var body struct {
		BaseVersionID       string   `json:"base_version_id"`
		TargetVersionID     string   `json:"target_version_id"`
		AddedCapabilities   []string `json:"added_capabilities"`
		RemovedCapabilities []string `json:"removed_capabilities"`
		ChangedRefs         []struct {
			Field string  `json:"field"`
			From  *string `json:"from,omitempty"`
			To    *string `json:"to,omitempty"`
		} `json:"changed_refs"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, "base-v1", body.BaseVersionID)
	require.Equal(t, "v2", body.TargetVersionID)
	require.ElementsMatch(t, []string{"go", "lint", "github"}, body.AddedCapabilities)
	require.Equal(t, []string{"python"}, body.RemovedCapabilities)
	require.Len(t, body.ChangedRefs, 2)

	changedByField := make(map[string]struct{ From, To *string })
	for _, c := range body.ChangedRefs {
		changedByField[c.Field] = struct{ From, To *string }{From: c.From, To: c.To}
	}
	require.Contains(t, changedByField, "runtime")
	require.NotNil(t, changedByField["runtime"].From)
	require.Equal(t, "r1", *changedByField["runtime"].From)
	require.NotNil(t, changedByField["runtime"].To)
	require.Equal(t, "r2", *changedByField["runtime"].To)
	require.Contains(t, changedByField, "model")
}

type fakeVersionService struct {
	diff      *agentversionapp.VersionDiff
	diffErr   error
	diffCalls []struct {
		tenantID      string
		agentID       string
		versionID     string
		baseVersionID string
	}
}

func (f *fakeVersionService) ListVersions(ctx context.Context, tenantID, agentID string) ([]agentversionapp.VersionSummary, error) {
	return nil, nil
}

func (f *fakeVersionService) GetVersion(ctx context.Context, tenantID, agentID, versionID string) (*agentversionapp.VersionDetail, error) {
	return nil, nil
}

func (f *fakeVersionService) GetVersionDiff(ctx context.Context, tenantID, agentID, versionID, baseVersionID string) (*agentversionapp.VersionDiff, error) {
	f.diffCalls = append(f.diffCalls, struct {
		tenantID      string
		agentID       string
		versionID     string
		baseVersionID string
	}{tenantID: tenantID, agentID: agentID, versionID: versionID, baseVersionID: baseVersionID})
	return f.diff, f.diffErr
}

func (f *fakeVersionService) CreateDraft(ctx context.Context, cmd agentversionapp.CreateDraft) (*agentversionapp.CreateDraftResponse, error) {
	return nil, nil
}

func (f *fakeVersionService) StartEvaluation(ctx context.Context, principal identityapp.Principal, cmd agentversionapp.StartEvaluation) error {
	return nil
}

func (f *fakeVersionService) Promote(ctx context.Context, cmd agentversionapp.Promote) error {
	return nil
}

func (f *fakeVersionService) Rollback(ctx context.Context, cmd agentversionapp.Rollback) error {
	return nil
}
