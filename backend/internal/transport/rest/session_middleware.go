package rest

import (
	"net/http"

	"agentguild.dev/agentguild/backend/internal/auth"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
)

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(auth.SessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid session")
			return
		}
		session, err := auth.ParseSessionCookie(cookie, s.sessionSecret)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid session")
			return
		}
		principal := auth.Principal{
			TenantID:   session.TenantID,
			Type:       auth.PrincipalTypeHuman,
			OwnerID:    session.OwnerID,
			OwnerEmail: session.OwnerEmail,
			IsAdmin:    session.IsAdmin,
		}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
	})
}

func identityPrincipalFromAuth(principal auth.Principal) identityapp.Principal {
	return identityapp.Principal{
		TenantID:       principal.TenantID,
		OwnerID:        principal.OwnerID,
		OwnerEmail:     principal.OwnerEmail,
		IsAdmin:        principal.IsAdmin,
		AgentID:        principal.AgentID,
		AgentVersionID: principal.AgentVersionID,
		Scopes:         append([]string(nil), principal.Scopes...),
		RepoScope:      append([]string(nil), principal.RepoScope...),
	}
}
