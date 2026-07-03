package rest

import "net/http"

type agentWellKnown struct {
	Version       string   `json:"version"`
	ActivationURL string   `json:"activation_url"`
	RefreshURL    string   `json:"refresh_url"`
	HeartbeatURL  string   `json:"heartbeat_url"`
	Scopes        []string `json:"scopes"`
}

var defaultAgentWellKnown = agentWellKnown{
	Version:       "0.1.0",
	ActivationURL: "https://api.agentguild.dev/v1/agents/me:activate",
	RefreshURL:    "https://api.agentguild.dev/v1/agents/me:refresh",
	HeartbeatURL:  "https://api.agentguild.dev/v1/agents/me:heartbeat",
	Scopes:        []string{"tasks:read", "tasks:execute", "tasks:publish"},
}

func (s *Server) getAgentWellKnown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, defaultAgentWellKnown)
}
