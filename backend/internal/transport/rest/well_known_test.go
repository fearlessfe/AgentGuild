package rest_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func TestWellKnownExposesActivationMetadata(t *testing.T) {
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{}).Router()

	res := getNoAuth(t, server, "/.well-known/agentguild")

	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "application/json", res.Header().Get("Content-Type"))

	var body struct {
		Version       string   `json:"version"`
		ActivationURL string   `json:"activation_url"`
		RefreshURL    string   `json:"refresh_url"`
		HeartbeatURL  string   `json:"heartbeat_url"`
		Scopes        []string `json:"scopes"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, "0.1.0", body.Version)
	require.Equal(t, "https://api.agentguild.dev/v1/agents/me:activate", body.ActivationURL)
	require.Equal(t, "https://api.agentguild.dev/v1/agents/me:refresh", body.RefreshURL)
	require.Equal(t, "https://api.agentguild.dev/v1/agents/me:heartbeat", body.HeartbeatURL)
	require.Equal(t, []string{"tasks:read", "tasks:execute", "tasks:publish"}, body.Scopes)
}

func TestWellKnownDocumentedInOpenAPI(t *testing.T) {
	spec := mustReadFile(t, "openapi.yaml")

	require.Contains(t, spec, "/.well-known/agentguild:")
	require.Contains(t, spec, "operationId: getAgentWellKnown")
	require.Contains(t, spec, "summary: 获取 Agent 接入元数据")
	require.Contains(t, spec, "example: https://api.agentguild.dev/v1/agents/me:activate")
	require.Contains(t, spec, "example: https://api.agentguild.dev/v1/agents/me:refresh")
}

func TestSkillGuideIncludesActivationFlow(t *testing.T) {
	skill := mustReadFile(t, filepath.Join("..", "..", "..", "..", "skill.md"))

	require.Contains(t, skill, "Activation Token")
	require.Contains(t, skill, "POST /v1/agents/me:activate")
	require.Contains(t, skill, "Access Token")
	require.Contains(t, skill, "15 分钟")
	require.Contains(t, skill, "POST /v1/agents/me:refresh")
	require.Contains(t, skill, "不要记录 Activation Token")
}

func getNoAuth(t *testing.T, server http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptestRecorder(server, req)
	return rec
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
