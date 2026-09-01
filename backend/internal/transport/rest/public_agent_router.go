package rest

import (
	"net/http"

	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	"github.com/go-chi/chi/v5"
)

func (s *Server) listPublicAgents(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "limit is invalid", "limit")
		return
	}
	result, err := s.publicAgents.List(r.Context(), identityapp.ListPublicAgents{
		Status: r.URL.Query().Get("status"), Limit: limit,
	})
	if err != nil {
		mapIdentityError(w, err, identityapp.Principal{})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getPublicAgent(w http.ResponseWriter, r *http.Request) {
	result, err := s.publicAgents.Get(r.Context(), identityapp.GetPublicAgent{AgentID: chi.URLParam(r, "id")})
	if err != nil {
		mapIdentityError(w, err, identityapp.Principal{})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
