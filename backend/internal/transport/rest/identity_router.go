package rest

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	"github.com/go-chi/chi/v5"
)

const (
	oidcStateCookieName = "agentguild_oidc_state"
	oidcStateCookiePath = "/oauth/oidc/callback"
	oidcStateTTL        = 10 * time.Minute
)

func (s *Server) oidcLogin(w http.ResponseWriter, r *http.Request) {
	state := randomState()
	cookie, err := newOIDCStateCookie(state, s.sessionSecret, s.sessionSecure)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	http.SetCookie(w, cookie)
	http.Redirect(w, r, s.oidc.BeginAuthURL(state), http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if err := s.validateOIDCState(r); err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid oidc state")
		return
	}
	session, err := s.oidc.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "oidc callback failed")
		return
	}
	cookie, err := auth.NewSessionCookie(*session, s.sessionSecret, s.sessionSecure)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	http.SetCookie(w, clearOIDCStateCookie(s.sessionSecure))
	http.SetCookie(w, cookie)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"authenticated": true}})
}

func (s *Server) validateOIDCState(r *http.Request) error {
	queryState := r.URL.Query().Get("state")
	if queryState == "" {
		return errors.New("oidc state query is required")
	}
	cookie, err := r.Cookie(oidcStateCookieName)
	if err != nil {
		return err
	}
	cookieState, err := parseOIDCStateCookie(cookie, s.sessionSecret)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(queryState), []byte(cookieState)) {
		return errors.New("oidc state mismatch")
	}
	return nil
}

func (s *Server) registerAgent(w http.ResponseWriter, r *http.Request) {
	// Agent identities are created exclusively through the public
	// challenge/signature flow. Keep this legacy route as an explicit deny so
	// old clients fail closed instead of creating human-provisioned agents.
	writeError(w, http.StatusForbidden, "AGENT_SELF_REGISTRATION_ONLY", "manual Agent registration is disabled; use the self-registration challenge")
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "limit is invalid", "limit")
		return
	}
	result, err := s.identity.ListAgents(r.Context(), principal, identityapp.ListAgents{
		OwnerID: r.URL.Query().Get("owner_id"),
		Limit:   limit,
	})
	if err != nil {
		mapIdentityError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	result, err := s.identity.GetAgent(r.Context(), principal, identityapp.GetAgent{AgentID: chi.URLParam(r, "id")})
	if err != nil {
		mapIdentityError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) suspendAgent(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	var body struct {
		Reason string `json:"reason"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := s.identity.SuspendAgent(r.Context(), principal, identityapp.SuspendAgent{AgentID: chi.URLParam(r, "id"), Reason: body.Reason})
	if err != nil {
		mapIdentityError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) resumeAgent(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	result, err := s.identity.ResumeAgent(r.Context(), principal, identityapp.ResumeAgent{AgentID: chi.URLParam(r, "id")})
	if err != nil {
		mapIdentityError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) revokeAgent(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	var body struct {
		Reason string `json:"reason"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := s.identity.RevokeAgent(r.Context(), principal, identityapp.RevokeAgent{AgentID: chi.URLParam(r, "id"), Reason: body.Reason})
	if err != nil {
		mapIdentityError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getAgentToken(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	result, err := s.identity.GetActivationStatus(r.Context(), principal, identityapp.GetActivationStatus{AgentID: chi.URLParam(r, "id")})
	if err != nil {
		mapIdentityError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func randomState() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func newOIDCStateCookie(state, secret string, secure bool) (*http.Cookie, error) {
	if state == "" {
		return nil, errors.New("oidc state is required")
	}
	if secret == "" {
		return nil, errors.New("session secret is required")
	}
	payload := base64.RawURLEncoding.EncodeToString([]byte(state))
	signature := signOIDCStatePayload(payload, secret)
	expires := time.Now().Add(oidcStateTTL)
	return &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    payload + "." + signature,
		Path:     oidcStateCookiePath,
		Expires:  expires,
		MaxAge:   int(oidcStateTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}, nil
}

func parseOIDCStateCookie(cookie *http.Cookie, secret string) (string, error) {
	if cookie == nil {
		return "", errors.New("oidc state cookie is required")
	}
	if cookie.Name != oidcStateCookieName {
		return "", errors.New("unexpected oidc state cookie name")
	}
	if secret == "" {
		return "", errors.New("session secret is required")
	}
	payload, signature, ok := strings.Cut(cookie.Value, ".")
	if !ok || payload == "" || signature == "" {
		return "", errors.New("oidc state cookie is malformed")
	}
	if !hmac.Equal([]byte(signature), []byte(signOIDCStatePayload(payload, secret))) {
		return "", errors.New("oidc state cookie signature is invalid")
	}
	body, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", err
	}
	state := string(body)
	if state == "" {
		return "", errors.New("oidc state is empty")
	}
	return state, nil
}

func clearOIDCStateCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    "",
		Path:     oidcStateCookiePath,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

func signOIDCStatePayload(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
