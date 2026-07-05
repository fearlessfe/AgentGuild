package rest

import (
	"context"
	"net/http"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	reviewapp "agentguild.dev/agentguild/backend/internal/review/application"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
	"github.com/go-chi/chi/v5"
)

// ReviewService 是 REST 层消费的代码评审应用服务边界。
type ReviewService interface {
	CreateReview(ctx context.Context, principal auth.Principal, cmd reviewapp.CreateReview) (application.Envelope[reviewapp.ReviewView], error)
	SubmitDecision(ctx context.Context, principal auth.Principal, cmd reviewapp.SubmitDecision) (application.Envelope[reviewapp.ReviewView], error)
	AddComment(ctx context.Context, principal auth.Principal, cmd reviewapp.AddComment) (application.Envelope[reviewapp.CommentView], error)
	GetReview(ctx context.Context, principal auth.Principal, query reviewapp.GetReview) (application.Envelope[reviewapp.ReviewView], error)
}

// RubricService 是 REST 层消费的评分标准服务边界。
type RubricService interface {
	GetActiveRubric(ctx context.Context, principal auth.Principal) (application.Envelope[reviewapp.RubricView], error)
}

// ReputationQuery 是声望投影查询参数；实际投影逻辑尚未实现。
type ReputationQuery struct {
	AgentVersionID string
	Capability     string
	TaskType       string
}

// ProjectionView 是声望投影视图，字段与 reputationdomain.Projection 保持一致。
type ProjectionView struct {
	AgentVersionID         string  `json:"agent_version_id"`
	Capability             string  `json:"capability"`
	TaskType               string  `json:"task_type"`
	TotalReviews           int     `json:"total_reviews"`
	AcceptedCount          int     `json:"accepted_count"`
	RejectedCount          int     `json:"rejected_count"`
	RevisionRequestedCount int     `json:"revision_requested_count"`
	PassRate               float64 `json:"pass_rate"`
	ReworkRate             float64 `json:"rework_rate"`
	AvgReviewCostCents     float64 `json:"avg_review_cost_cents"`
	AvgReviewLatencyMs     float64 `json:"avg_review_latency_ms"`
	SampleSizeHint         string  `json:"sample_size_hint"`
	AlgorithmVersion       string  `json:"algorithm_version"`
}

// ReputationService 是声望投影占位服务边界。
type ReputationService interface {
	GetProjection(ctx context.Context, principal auth.Principal, query ReputationQuery) (application.Envelope[ProjectionView], error)
}

// WithReviewService 挂载代码评审 REST API。
func WithReviewService(svc ReviewService) Option {
	return func(s *Server) { s.reviewSvc = svc }
}

// WithRubricService 挂载评分标准 REST API。
func WithRubricService(svc RubricService) Option {
	return func(s *Server) { s.rubricSvc = svc }
}

// WithReputationService 挂载声望投影 REST API。
func WithReputationService(svc ReputationService) Option {
	return func(s *Server) { s.reputationSvc = svc }
}

func (s *Server) getReview(w http.ResponseWriter, r *http.Request) {
	if s.reviewSvc == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "review service is not configured")
		return
	}
	principal := mustPrincipal(r)
	result, err := s.reviewSvc.GetReview(r.Context(), principal, reviewapp.GetReview{ReviewID: chi.URLParam(r, "id")})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createReview(w http.ResponseWriter, r *http.Request) {
	if s.reviewSvc == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "review service is not configured")
		return
	}
	principal := mustPrincipal(r)
	var body struct {
		RequestID    string   `json:"request_id"`
		Capabilities []string `json:"capabilities"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.reviewSvc.CreateReview(r.Context(), principal, reviewapp.CreateReview{
		RequestID:    idempotencyKey,
		SubmissionID: chi.URLParam(r, "id"),
		Capabilities: body.Capabilities,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) submitDecision(w http.ResponseWriter, r *http.Request) {
	if s.reviewSvc == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "review service is not configured")
		return
	}
	principal := mustPrincipal(r)
	var body struct {
		RequestID string                     `json:"request_id"`
		Decision  string                     `json:"decision"`
		Scores    []reviewdomain.RubricScore `json:"scores"`
		Summary   string                     `json:"summary"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.reviewSvc.SubmitDecision(r.Context(), principal, reviewapp.SubmitDecision{
		RequestID: idempotencyKey,
		ReviewID:  chi.URLParam(r, "id"),
		Decision:  reviewdomain.Decision(body.Decision),
		Scores:    body.Scores,
		Summary:   body.Summary,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) addComment(w http.ResponseWriter, r *http.Request) {
	if s.reviewSvc == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "review service is not configured")
		return
	}
	principal := mustPrincipal(r)
	var body struct {
		RequestID       string `json:"request_id"`
		SubmissionID    string `json:"submission_id"`
		FilePath        string `json:"file_path"`
		Side            string `json:"side"`
		LineNumber      int    `json:"line_number"`
		HunkHash        string `json:"hunk_hash"`
		DiffFingerprint string `json:"diff_fingerprint"`
		Text            string `json:"text"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.reviewSvc.AddComment(r.Context(), principal, reviewapp.AddComment{
		RequestID:       idempotencyKey,
		ReviewID:        chi.URLParam(r, "id"),
		SubmissionID:    body.SubmissionID,
		FilePath:        body.FilePath,
		Side:            body.Side,
		LineNumber:      body.LineNumber,
		HunkHash:        body.HunkHash,
		DiffFingerprint: body.DiffFingerprint,
		Text:            body.Text,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) getActiveRubric(w http.ResponseWriter, r *http.Request) {
	if s.rubricSvc == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "rubric service is not configured")
		return
	}
	principal := mustPrincipal(r)
	result, err := s.rubricSvc.GetActiveRubric(r.Context(), principal)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getReputation(w http.ResponseWriter, r *http.Request) {
	if s.reputationSvc == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "reputation projection is not implemented")
		return
	}
	principal := mustPrincipal(r)
	query := ReputationQuery{
		AgentVersionID: r.URL.Query().Get("agent_version_id"),
		Capability:     r.URL.Query().Get("capability"),
		TaskType:       r.URL.Query().Get("task_type"),
	}
	result, err := s.reputationSvc.GetProjection(r.Context(), principal, query)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
