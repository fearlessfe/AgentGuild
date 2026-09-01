package rest_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
		Version                  string   `json:"version"`
		ActivationURL            string   `json:"activation_url"`
		RefreshURL               string   `json:"refresh_url"`
		HeartbeatURL             string   `json:"heartbeat_url"`
		OpenAPIURL               string   `json:"openapi_url"`
		SkillURL                 string   `json:"skill_url"`
		RegistrationChallengeURL string   `json:"registration_challenge_url"`
		RegistrationURL          string   `json:"registration_url"`
		PublicTasksURL           string   `json:"public_tasks_url"`
		PublicAgentsURL          string   `json:"public_agents_url"`
		Scopes                   []string `json:"scopes"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, "0.1.0", body.Version)
	require.Equal(t, "/v1/agents/me:activate", body.ActivationURL)
	require.Equal(t, "/v1/agents/me:refresh", body.RefreshURL)
	require.Equal(t, "/v1/agents/me:heartbeat", body.HeartbeatURL)
	require.Equal(t, "/openapi.yaml", body.OpenAPIURL)
	require.Equal(t, "/skill.md", body.SkillURL)
	require.Equal(t, "/v1/agents:registration-challenge", body.RegistrationChallengeURL)
	require.Equal(t, "/v1/agents:register", body.RegistrationURL)
	require.Equal(t, "/v1/public/tasks", body.PublicTasksURL)
	require.Equal(t, "/v1/public/agents", body.PublicAgentsURL)
	require.Equal(t, []string{"tasks:read", "tasks:claim", "tasks:execute"}, body.Scopes)
}

func TestWellKnownDocumentedInOpenAPI(t *testing.T) {
	spec := mustReadFile(t, "openapi.yaml")
	wellKnownSection := mustPathSection(t, spec, "/.well-known/agentguild")
	activateSection := mustPathSection(t, spec, "/v1/agents/me:activate")

	require.Contains(t, wellKnownSection, "operationId: getAgentWellKnown")
	require.Contains(t, wellKnownSection, "summary: 获取 Agent 接入元数据")
	require.Contains(t, wellKnownSection, "$ref: '#/components/schemas/AgentWellKnown'")

	require.Contains(t, activateSection, "operationId: activateAgent")
	require.Contains(t, activateSection, "$ref: '#/components/schemas/ActivateAgentRequest'")
	require.Contains(t, activateSection, "examples:")
	require.Contains(t, activateSection, "summary: Codex Agent 激活示例")
	require.Contains(t, activateSection, "activation_token: agt_act_123")
	require.Contains(t, activateSection, "token: access-token-1")
	require.Contains(t, activateSection, "token_type: Bearer")
	require.Contains(t, activateSection, "expires_at: '2026-07-03T10:15:00Z'")
}

func TestSkillGuideIncludesActivationFlow(t *testing.T) {
	skill := mustReadFile(t, filepath.Join("..", "..", "..", "..", "skill.md"))

	require.Contains(t, skill, "Open Registration")
	require.Contains(t, skill, "POST /v1/agents:registration-challenge")
	require.Contains(t, skill, "POST /v1/agents:register")
	require.Contains(t, skill, "Ed25519")
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

func mustPathSection(t *testing.T, spec string, path string) string {
	t.Helper()

	anchor := "\n  " + path + ":\n"
	start := strings.Index(spec, anchor)
	require.NotEqualf(t, -1, start, "expected path %s in OpenAPI spec", path)

	section := spec[start+1:]
	if next := strings.Index(section, "\n  /"); next >= 0 {
		section = section[:next]
	}

	return section
}
