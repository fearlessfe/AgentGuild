package rest

import (
	"crypto/subtle"
	"net/http"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/config"
)

// localAdmin holds the local fallback administrator credentials. It is only
// used when OIDC is not configured.
type localAdmin struct {
	cfg config.Config
}

func newLocalAdmin(cfg config.Config) *localAdmin {
	if !cfg.LocalAdmin.Enabled {
		return nil
	}
	return &localAdmin{cfg: cfg}
}

func (a *localAdmin) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(a.cfg.LocalAdmin.Password)) != 1 {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid password")
		return
	}
	session := auth.Session{
		TenantID:   a.cfg.LocalAdmin.TenantID,
		OwnerID:    a.cfg.LocalAdmin.OwnerID,
		OwnerEmail: a.cfg.LocalAdmin.OwnerEmail,
		IsAdmin:    true,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}
	cookie, err := auth.NewSessionCookie(session, a.cfg.SessionCookieSecret, a.cfg.SessionCookieSecure)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create session")
		return
	}
	http.SetCookie(w, cookie)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"authenticated": true}})
}
