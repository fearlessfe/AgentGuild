package rest

import (
	"context"
	"errors"
	"net/http"

	"agentguild.dev/agentguild/backend/internal/auth"
	syncapp "agentguild.dev/agentguild/backend/internal/sync/application"
	syncdomain "agentguild.dev/agentguild/backend/internal/sync/domain"
	"github.com/go-chi/chi/v5"
)

type SyncEngine interface {
	RunRule(ctx context.Context, tenantID, ruleID string) (syncapp.SyncResult, error)
}

type syncRuleBody struct {
	Repo            string   `json:"repo"`
	IncludeLabels   []string `json:"include_labels"`
	ExcludeLabels   []string `json:"exclude_labels"`
	IssueState      string   `json:"issue_state"`
	TaskType        string   `json:"task_type"`
	DefaultPriority string   `json:"default_priority"`
	DedupeStrategy  string   `json:"dedupe_strategy"`
	SourceAuth      string   `json:"source_auth"`
	Enabled         *bool    `json:"enabled"`
}

type repositoryView struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Visibility    string `json:"visibility"`
}

type syncResultSummary struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Skipped   int `json:"skipped"`
	Cancelled int `json:"cancelled"`
	Failed    int `json:"failed"`
}

func (s *Server) listRepositories(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	source, err := s.gitHubAppManager.IssueSource(r.Context(), principal.TenantID)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	repositories, err := source.ListInstallationRepositories(r.Context())
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	items := make([]repositoryView, 0, len(repositories))
	for _, repo := range repositories {
		items = append(items, repositoryView{
			FullName:      repo.FullName,
			DefaultBranch: repo.DefaultBranch,
			Visibility:    repo.Visibility,
		})
	}
	writeEnvelope(w, http.StatusOK, struct {
		Items []repositoryView `json:"items"`
	}{Items: items})
}

func (s *Server) listSyncRules(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	rules, err := s.syncRules.List(r.Context(), principal)
	if err != nil {
		mapSyncRuleError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusOK, struct {
		Items []syncapp.RuleView `json:"items"`
	}{Items: rules})
}

func (s *Server) createSyncRule(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	if !requireAdminSyncSession(w, principal) {
		return
	}
	var body syncRuleBody
	if !decodeBody(w, r, &body) {
		return
	}
	view, err := s.syncRules.Create(r.Context(), principal, syncapp.CreateRuleCommand{
		Repo:            body.Repo,
		IncludeLabels:   body.IncludeLabels,
		ExcludeLabels:   body.ExcludeLabels,
		IssueState:      body.IssueState,
		TaskType:        body.TaskType,
		DefaultPriority: body.DefaultPriority,
		DedupeStrategy:  body.DedupeStrategy,
		SourceAuth:      body.SourceAuth,
	})
	if err != nil {
		mapSyncRuleError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusCreated, view)
}

func (s *Server) getSyncRule(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	view, err := s.syncRules.Get(r.Context(), principal, chi.URLParam(r, "id"))
	if err != nil {
		mapSyncRuleError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusOK, view)
}

func (s *Server) updateSyncRule(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	if !requireAdminSyncSession(w, principal) {
		return
	}
	var body syncRuleBody
	if !decodeBody(w, r, &body) {
		return
	}
	view, err := s.syncRules.Update(r.Context(), principal, syncapp.UpdateRuleCommand{
		ID:              chi.URLParam(r, "id"),
		Repo:            body.Repo,
		IncludeLabels:   body.IncludeLabels,
		ExcludeLabels:   body.ExcludeLabels,
		IssueState:      body.IssueState,
		TaskType:        body.TaskType,
		DefaultPriority: body.DefaultPriority,
		DedupeStrategy:  body.DedupeStrategy,
		SourceAuth:      body.SourceAuth,
		Enabled:         body.Enabled,
	})
	if err != nil {
		mapSyncRuleError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusOK, view)
}

func (s *Server) deleteSyncRule(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	if !requireAdminSyncSession(w, principal) {
		return
	}
	if err := s.syncRules.Delete(r.Context(), principal, chi.URLParam(r, "id")); err != nil {
		mapSyncRuleError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusOK, struct {
		Deleted bool `json:"deleted"`
	}{Deleted: true})
}

func (s *Server) runSyncRule(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	if !requireAdminSyncSession(w, principal) {
		return
	}
	result, err := s.syncEngine.RunRule(r.Context(), principal.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		mapSyncRuleError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusOK, syncResultSummary{
		Created:   result.Created,
		Updated:   result.Updated,
		Skipped:   result.Skipped,
		Cancelled: result.Cancelled,
		Failed:    result.Failed,
	})
}

func requireAdminSyncSession(w http.ResponseWriter, principal auth.Principal) bool {
	if principal.IsAdmin {
		return true
	}
	writeError(w, http.StatusForbidden, "FORBIDDEN", "admin session is required")
	return false
}

func mapSyncRuleError(w http.ResponseWriter, err error, principal auth.Principal) {
	switch syncRuleErrorCode(err) {
	case "invalid_argument":
		writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error(), syncdomain.FieldOf(err))
	case "forbidden":
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
	case "not_found":
		if principal.IsAdmin {
			writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
	default:
		if errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusServiceUnavailable, "TEMPORARILY_UNAVAILABLE", "request timed out")
			return
		}
		mapDomainError(w, err, principal)
	}
}

func syncRuleErrorCode(err error) string {
	var pointer *syncdomain.Error
	if errors.As(err, &pointer) && pointer != nil {
		return pointer.Code
	}
	var value syncdomain.Error
	if errors.As(err, &value) {
		return value.Code
	}
	return ""
}
