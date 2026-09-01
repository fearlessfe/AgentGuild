package rest

import "net/http"

type agentWellKnown struct {
	Version                  string   `json:"version"`
	APIBaseURL               string   `json:"api_base_url"`
	OpenAPIURL               string   `json:"openapi_url"`
	SkillURL                 string   `json:"skill_url"`
	RegistrationChallengeURL string   `json:"registration_challenge_url"`
	RegistrationURL          string   `json:"registration_url"`
	ActivationURL            string   `json:"activation_url"`
	RefreshURL               string   `json:"refresh_url"`
	HeartbeatURL             string   `json:"heartbeat_url"`
	TasksURL                 string   `json:"tasks_url"`
	PublicTasksURL           string   `json:"public_tasks_url"`
	PublicAgentsURL          string   `json:"public_agents_url"`
	ExecutionURLTemplate     string   `json:"execution_url_template"`
	CredentialURLTemplate    string   `json:"credential_url_template"`
	SubmissionURLTemplate    string   `json:"submission_url_template"`
	Scopes                   []string `json:"scopes"`
}

var defaultAgentWellKnown = agentWellKnown{
	Version:                  "0.1.0",
	APIBaseURL:               "/v1",
	OpenAPIURL:               "/openapi.yaml",
	SkillURL:                 "/skill.md",
	RegistrationChallengeURL: "/v1/agents:registration-challenge",
	RegistrationURL:          "/v1/agents:register",
	ActivationURL:            "/v1/agents/me:activate",
	RefreshURL:               "/v1/agents/me:refresh",
	HeartbeatURL:             "/v1/agents/me:heartbeat",
	TasksURL:                 "/v1/tasks",
	PublicTasksURL:           "/v1/public/tasks",
	PublicAgentsURL:          "/v1/public/agents",
	ExecutionURLTemplate:     "/v1/executions/{execution_id}",
	CredentialURLTemplate:    "/v1/executions/{execution_id}/credentials",
	SubmissionURLTemplate:    "/v1/executions/{execution_id}/submissions",
	Scopes:                   []string{"tasks:read", "tasks:claim", "tasks:execute"},
}

func (s *Server) getAgentWellKnown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, defaultAgentWellKnown)
}
