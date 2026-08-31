package rest

import (
	"net/http"

	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
)

func (s *Server) activateAgent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ActivationToken   string   `json:"activation_token"`
		Token             string   `json:"token"`
		Runtime           string   `json:"runtime"`
		Model             string   `json:"model"`
		Capabilities      []string `json:"capabilities"`
		ConfigFingerprint string   `json:"config_fingerprint"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	token := body.ActivationToken
	if token == "" {
		token = body.Token
	}
	result, err := s.identity.ActivateAgent(r.Context(), identityapp.ActivateAgent{
		Token:             token,
		Runtime:           body.Runtime,
		Model:             body.Model,
		Capabilities:      body.Capabilities,
		ConfigFingerprint: body.ConfigFingerprint,
	})
	if err != nil {
		mapIdentityError(w, err, identityapp.Principal{})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) refreshAgentToken(w http.ResponseWriter, r *http.Request) {
	authPrincipal := mustPrincipal(r)
	if authPrincipal.IsGlobalAgent() && s.openRegistration != nil {
		result, err := s.openRegistration.Refresh(r.Context(), authPrincipal.AgentID, authPrincipal.AgentVersionID)
		if err != nil {
			mapOpenRegistrationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	principal := identityPrincipalFromAuth(authPrincipal)
	result, err := s.identity.IssueAccessToken(r.Context(), principal, identityapp.IssueAccessToken{})
	if err != nil {
		mapIdentityError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) agentHeartbeat(w http.ResponseWriter, r *http.Request) {
	authPrincipal := mustPrincipal(r)
	if authPrincipal.IsGlobalAgent() && s.openRegistration != nil {
		result, err := s.openRegistration.Heartbeat(r.Context(), authPrincipal.AgentID, authPrincipal.AgentVersionID)
		if err != nil {
			mapOpenRegistrationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	principal := identityPrincipalFromAuth(authPrincipal)
	result, err := s.identity.AgentHeartbeat(r.Context(), principal, identityapp.AgentHeartbeat{})
	if err != nil {
		mapIdentityError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getSelfAgent(w http.ResponseWriter, r *http.Request) {
	authPrincipal := mustPrincipal(r)
	if authPrincipal.IsGlobalAgent() && s.openRegistration != nil {
		result, err := s.openRegistration.GetSelf(r.Context(), authPrincipal.AgentID, authPrincipal.AgentVersionID)
		if err != nil {
			mapOpenRegistrationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	principal := identityPrincipalFromAuth(authPrincipal)
	result, err := s.identity.GetAgent(r.Context(), principal, identityapp.GetAgent{AgentID: principal.AgentID})
	if err != nil {
		mapIdentityError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
