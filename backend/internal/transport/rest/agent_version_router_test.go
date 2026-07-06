package rest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	agentversionapp "agentguild.dev/agentguild/backend/internal/agentversion/application"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

type fakeVersionService struct {
	list      []agentversionapp.VersionSummary
	listErr   error
	listCalls []struct {
		tenantID string
		agentID  string
	}

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
	f.listCalls = append(f.listCalls, struct {
		tenantID string
		agentID  string
	}{tenantID: tenantID, agentID: agentID})
	return f.list, f.listErr
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

func TestListAgentVersionsReturnsSnakeCaseFields(t *testing.T) {
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	versions := &fakeVersionService{
		list: []agentversionapp.VersionSummary{
			{
				ID:                "v1",
				VersionNumber:     1,
				Status:            "active",
				ParentVersionID:   "v0",
				ConfigFingerprint: "fp-1",
				CreatedAt:         now,
			},
		},
	}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithSession(testSessionSecret, false),
		rest.WithVersionService(versions),
	).Router()

	res := getWithSession(t, server, "/v1/agents/agent-1/versions", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	require.Len(t, versions.listCalls, 1)
	require.Equal(t, "tenant-1", versions.listCalls[0].tenantID)
	require.Equal(t, "agent-1", versions.listCalls[0].agentID)

	body := res.Body.String()
	require.Contains(t, body, `"data":`)
	require.Contains(t, body, `"items":`)
	require.Contains(t, body, `"version_number":`)
	require.Contains(t, body, `"parent_version_id":`)
	require.Contains(t, body, `"config_fingerprint":`)
	require.Contains(t, body, `"created_at":`)
	require.Contains(t, body, `"status":`)
	require.NotContains(t, body, `"VersionNumber"`)
	require.NotContains(t, body, `"ParentVersionID"`)
	require.NotContains(t, body, `"ConfigFingerprint"`)
	require.NotContains(t, body, `"CreatedAt"`)

	var envelope struct {
		Data struct {
			Items []agentversionapp.VersionSummary `json:"items"`
		} `json:"data"`
		Meta struct {
			ServerTime time.Time `json:"server_time"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.Items, 1)
	require.Equal(t, "v1", envelope.Data.Items[0].ID)
	require.NotZero(t, envelope.Meta.ServerTime)
}

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
		Data struct {
			BaseVersionID       string   `json:"base_version_id"`
			TargetVersionID     string   `json:"target_version_id"`
			AddedCapabilities   []string `json:"added_capabilities"`
			RemovedCapabilities []string `json:"removed_capabilities"`
			ChangedRefs         []struct {
				Field string  `json:"field"`
				From  *string `json:"from,omitempty"`
				To    *string `json:"to,omitempty"`
			} `json:"changed_refs"`
		} `json:"data"`
		Meta struct {
			ServerTime time.Time `json:"server_time"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, "base-v1", body.Data.BaseVersionID)
	require.Equal(t, "v2", body.Data.TargetVersionID)
	require.ElementsMatch(t, []string{"go", "lint", "github"}, body.Data.AddedCapabilities)
	require.Equal(t, []string{"python"}, body.Data.RemovedCapabilities)
	require.Len(t, body.Data.ChangedRefs, 2)
	require.NotZero(t, body.Meta.ServerTime)

	changedByField := make(map[string]struct{ From, To *string })
	for _, c := range body.Data.ChangedRefs {
		changedByField[c.Field] = struct{ From, To *string }{From: c.From, To: c.To}
	}
	require.Contains(t, changedByField, "runtime")
	require.NotNil(t, changedByField["runtime"].From)
	require.Equal(t, "r1", *changedByField["runtime"].From)
	require.NotNil(t, changedByField["runtime"].To)
	require.Equal(t, "r2", *changedByField["runtime"].To)
	require.Contains(t, changedByField, "model")
}
