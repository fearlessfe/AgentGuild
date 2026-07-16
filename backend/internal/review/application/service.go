package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
)

// Service orchestrates the code-review lifecycle.
type Service struct {
	store       application.Store
	submissions gitapp.SubmissionRepository
	policy      Policy
	allocator   Allocator
	diff        DiffProvider
	validation  ValidationProvider
	newID       func() string
}

// Options configures a new Service.
type Options struct {
	NewID func() string
}

// NewService creates a review application service.
func NewService(store application.Store, submissions gitapp.SubmissionRepository, diff DiffProvider, validation ValidationProvider, options Options) (*Service, error) {
	if store == nil {
		return nil, invalid("store")
	}
	if submissions == nil {
		return nil, invalid("submission_repository")
	}
	if diff == nil {
		return nil, invalid("diff_provider")
	}
	if validation == nil {
		return nil, invalid("validation_provider")
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	return &Service{
		store:       store,
		submissions: submissions,
		policy:      Policy{},
		allocator:   Allocator{},
		diff:        diff,
		validation:  validation,
		newID:       options.NewID,
	}, nil
}

// CreateReview assigns a reviewer to a submission and creates a pending review.
type CreateReview struct {
	RequestID    string
	SubmissionID string
	Capabilities []string
}

// SubmitDecision records the reviewer's final decision and scores.
type SubmitDecision struct {
	RequestID string
	ReviewID  string
	Decision  reviewdomain.Decision
	Scores    []reviewdomain.RubricScore
	Summary   string
}

// AddComment adds a line-level comment to a review.
type AddComment struct {
	RequestID       string
	ReviewID        string
	SubmissionID    string
	FilePath        string
	Side            string
	LineNumber      int
	HunkHash        string
	DiffFingerprint string
	Text            string
}

// GetReview retrieves a single review by ID.
type GetReview struct {
	ReviewID string
}

// GetSubmissionDiff retrieves the structured diff for a submission.
type GetSubmissionDiff struct {
	SubmissionID string
}

// SubmitForReview transitions a running execution to the reviewing state.
type SubmitForReview struct {
	RequestID   string
	ExecutionID string
}

// ReviewView is the serialized representation of a review.
type ReviewView struct {
	ID              string                     `json:"id"`
	TenantID        string                     `json:"tenant_id"`
	SubmissionID    string                     `json:"submission_id"`
	ReviewerID      string                     `json:"reviewer_id"`
	RubricVersionID string                     `json:"rubric_version_id"`
	Capability      string                     `json:"capability"`
	RubricScores    []reviewdomain.RubricScore `json:"rubric_scores"`
	Summary         string                     `json:"summary,omitempty"`
	Status          string                     `json:"status"`
	FinalDecision   string                     `json:"final_decision,omitempty"`
	SubmittedAt     time.Time                  `json:"submitted_at,omitempty"`
	CreatedAt       time.Time                  `json:"created_at"`
	LineComments    []CommentView              `json:"line_comments"`
}

type ListReviews struct {
	Status string
	Limit  int
}

func (s *Service) ListReviews(ctx context.Context, principal auth.Principal, query ListReviews) (application.Envelope[[]ReviewView], error) {
	var result application.Envelope[[]ReviewView]
	if principal.Type != auth.PrincipalTypeHuman || principal.TenantID == "" {
		return result, domain.ErrForbidden
	}
	err := s.store.WithTx(ctx, func(tx application.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		reviewerID := ""
		if !principal.IsAdmin {
			profiles, err := tx.Reviewers().ListActive(ctx, principal.TenantID, 1000)
			if err != nil {
				return err
			}
			for i := range profiles {
				if profiles[i].UserID == principal.OwnerID {
					reviewerID = profiles[i].ID
					break
				}
			}
			if reviewerID == "" {
				result = application.Envelope[[]ReviewView]{Data: []ReviewView{}, Meta: application.Meta{ServerTime: now}}
				return nil
			}
		}
		reviews, err := tx.Reviews().List(ctx, principal.TenantID, reviewerID, query.Status, query.Limit)
		if err != nil {
			return err
		}
		views := make([]ReviewView, len(reviews))
		for i := range reviews {
			views[i] = reviewView(&reviews[i], nil)
		}
		result = application.Envelope[[]ReviewView]{Data: views, Meta: application.Meta{ServerTime: now}}
		return nil
	})
	return result, err
}

// RubricDimensionView is the serialized representation of a rubric dimension.
type RubricDimensionView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RubricView is the serialized representation of the active rubric.
type RubricView struct {
	ID               string                `json:"id"`
	TenantID         string                `json:"tenant_id"`
	VersionNumber    int                   `json:"version_number"`
	Name             string                `json:"name"`
	Dimensions       []RubricDimensionView `json:"dimensions"`
	Weights          map[string]float64    `json:"weights"`
	AlgorithmVersion string                `json:"algorithm_version"`
	IsActive         bool                  `json:"is_active"`
	CreatedAt        time.Time             `json:"created_at"`
}

// CommentView is the serialized representation of a line comment.
type CommentView struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	ReviewID        string    `json:"review_id"`
	SubmissionID    string    `json:"submission_id"`
	FilePath        string    `json:"file_path"`
	Side            string    `json:"side"`
	LineNumber      int       `json:"line_number"`
	HunkHash        string    `json:"hunk_hash"`
	DiffFingerprint string    `json:"diff_fingerprint"`
	Text            string    `json:"text"`
	CreatedAt       time.Time `json:"created_at"`
}

// CreateReview assigns a reviewer and creates a pending review for the submission.
func (s *Service) CreateReview(ctx context.Context, principal auth.Principal, cmd CreateReview) (application.Envelope[ReviewView], error) {
	var result application.Envelope[ReviewView]
	if err := requireTenant(principal); err != nil {
		return result, err
	}

	err := s.store.WithTx(ctx, func(tx application.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}

		key, record, err := acquireReview(ctx, tx, principal, "review_create", cmd.RequestID, cmd, now)
		if err != nil {
			return err
		}
		if record.Completed {
			return json.Unmarshal(record.ResponseBody, &result)
		}
		if !record.Acquired {
			return &domain.Error{Code: "state_conflict", Message: "idempotency request is already in progress"}
		}

		execution, err := s.executionForSubmission(ctx, tx, principal.TenantID, cmd.SubmissionID)
		if err != nil {
			if domain.CodeOf(err) == "not_found" {
				return domain.ErrNotFound
			}
			return err
		}
		if execution.Status != domain.ExecutionReviewing {
			return &domain.Error{Code: "state_conflict", Message: "submission is not ready for review"}
		}

		task, err := tx.GetTask(ctx, principal.TenantID, execution.TaskID)
		if err != nil {
			return err
		}

		if err := s.policy.CanViewReview(ctx, principal, ReviewRecord{TenantID: principal.TenantID}, taskSummary(task, execution)); err != nil {
			return err
		}

		rubric, err := tx.Rubrics().GetActive(ctx, principal.TenantID)
		if err != nil {
			return err
		}

		reviewerID, err := s.allocator.Allocate(ctx, tx, principal.TenantID, cmd.Capabilities)
		if err != nil {
			return err
		}

		if err := tx.Reviewers().IncrementLoad(ctx, principal.TenantID, reviewerID); err != nil {
			return err
		}

		capability := task.Type
		if len(cmd.Capabilities) > 0 {
			capability = cmd.Capabilities[0]
		}
		review, err := reviewdomain.NewReview(s.newID(), principal.TenantID, cmd.SubmissionID, reviewerID, rubric.ID, capability, now)
		if err != nil {
			return err
		}
		if err := tx.Reviews().Insert(ctx, review); err != nil {
			return err
		}

		result = application.Envelope[ReviewView]{
			Data: reviewView(review, nil),
			Meta: application.Meta{ServerTime: now},
		}
		return completeReview(ctx, tx, key, record.OwnerToken, result)
	})
	return result, err
}

// SubmitDecision validates authorization and hard gates, then submits the review decision.
func (s *Service) SubmitDecision(ctx context.Context, principal auth.Principal, cmd SubmitDecision) (application.Envelope[ReviewView], error) {
	var result application.Envelope[ReviewView]
	if err := requireTenant(principal); err != nil {
		return result, err
	}

	err := s.store.WithTx(ctx, func(tx application.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}

		key, record, err := acquireReview(ctx, tx, principal, "review_submit", cmd.RequestID, cmd, now)
		if err != nil {
			return err
		}
		if record.Completed {
			return json.Unmarshal(record.ResponseBody, &result)
		}
		if !record.Acquired {
			return &domain.Error{Code: "state_conflict", Message: "idempotency request is already in progress"}
		}

		review, err := tx.Reviews().GetByID(ctx, principal.TenantID, cmd.ReviewID)
		if err != nil {
			return err
		}

		reviewer, err := tx.Reviewers().GetByID(ctx, principal.TenantID, review.ReviewerID)
		if err != nil {
			return err
		}

		rec := reviewRecord(review, reviewer)
		if err := s.policy.CanSubmitDecision(principal, rec); err != nil {
			return err
		}

		if cmd.Decision == reviewdomain.DecisionAccepted {
			status, err := s.validation.GetValidationStatus(ctx, principal.TenantID, review.SubmissionID)
			if err != nil {
				return err
			}
			if !status.AllHardGatesPassed() {
				return &domain.Error{Code: "hard_gates_failed", Message: "cannot accept submission with failed hard gates"}
			}
		}

		rubric, err := tx.Rubrics().GetByID(ctx, principal.TenantID, review.RubricVersionID)
		if err != nil {
			return err
		}

		if err := review.Submit(cmd.Decision, cmd.Scores, rubric, now); err != nil {
			return err
		}
		review.Summary = cmd.Summary

		if err := tx.Reviews().Update(ctx, review); err != nil {
			return err
		}

		if err := tx.Reviewers().DecrementLoad(ctx, principal.TenantID, review.ReviewerID); err != nil {
			return err
		}

		if err := s.applyDecisionToExecution(ctx, tx, review, now); err != nil {
			return err
		}

		result = application.Envelope[ReviewView]{
			Data: reviewView(review, nil),
			Meta: application.Meta{ServerTime: now},
		}
		return completeReview(ctx, tx, key, record.OwnerToken, result)
	})
	return result, err
}

// AddComment adds a line-level comment to a review.
func (s *Service) AddComment(ctx context.Context, principal auth.Principal, cmd AddComment) (application.Envelope[CommentView], error) {
	var result application.Envelope[CommentView]
	if err := requireTenant(principal); err != nil {
		return result, err
	}

	err := s.store.WithTx(ctx, func(tx application.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}

		key, record, err := acquireReview(ctx, tx, principal, "review_comment", cmd.RequestID, cmd, now)
		if err != nil {
			return err
		}
		if record.Completed {
			return json.Unmarshal(record.ResponseBody, &result)
		}
		if !record.Acquired {
			return &domain.Error{Code: "state_conflict", Message: "idempotency request is already in progress"}
		}

		review, err := tx.Reviews().GetByID(ctx, principal.TenantID, cmd.ReviewID)
		if err != nil {
			return err
		}

		if cmd.SubmissionID != review.SubmissionID {
			return invalid("submission_id")
		}

		reviewer, err := tx.Reviewers().GetByID(ctx, principal.TenantID, review.ReviewerID)
		if err != nil {
			return err
		}

		if err := s.policy.CanAddComment(principal, reviewRecord(review, reviewer)); err != nil {
			return err
		}

		side := reviewdomain.DiffSide(cmd.Side)
		comment, err := reviewdomain.NewLineComment(
			s.newID(), principal.TenantID, cmd.ReviewID, cmd.SubmissionID,
			cmd.FilePath, side, cmd.LineNumber, cmd.HunkHash, cmd.DiffFingerprint,
			cmd.Text, now,
		)
		if err != nil {
			return err
		}

		if err := tx.LineComments().Insert(ctx, comment); err != nil {
			return err
		}

		result = application.Envelope[CommentView]{
			Data: commentView(comment),
			Meta: application.Meta{ServerTime: now},
		}
		return completeReview(ctx, tx, key, record.OwnerToken, result)
	})
	return result, err
}

// GetActiveRubric returns the currently active rubric version for the tenant.
func (s *Service) GetActiveRubric(ctx context.Context, principal auth.Principal) (application.Envelope[RubricView], error) {
	var result application.Envelope[RubricView]
	if err := requireTenant(principal); err != nil {
		return result, err
	}
	if err := requireHumanOrScope(principal, "reviews:read"); err != nil {
		return result, err
	}
	err := s.store.WithTx(ctx, func(tx application.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		rubric, err := tx.Rubrics().GetActive(ctx, principal.TenantID)
		if err != nil {
			return err
		}
		result = application.Envelope[RubricView]{
			Data: rubricView(rubric),
			Meta: application.Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

// GetReview returns a review by ID.
func (s *Service) GetReview(ctx context.Context, principal auth.Principal, query GetReview) (application.Envelope[ReviewView], error) {
	var result application.Envelope[ReviewView]
	if err := requireTenant(principal); err != nil {
		return result, err
	}

	err := s.store.WithTx(ctx, func(tx application.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}

		review, err := tx.Reviews().GetByID(ctx, principal.TenantID, query.ReviewID)
		if err != nil {
			return err
		}

		execution, err := s.executionForSubmission(ctx, tx, principal.TenantID, review.SubmissionID)
		if err != nil {
			return err
		}

		task, err := tx.GetTask(ctx, principal.TenantID, execution.TaskID)
		if err != nil {
			return err
		}

		reviewer, err := tx.Reviewers().GetByID(ctx, principal.TenantID, review.ReviewerID)
		if err != nil {
			return err
		}

		if err := s.policy.CanViewReview(ctx, principal, reviewRecord(review, reviewer), taskSummary(task, execution)); err != nil {
			return err
		}

		comments, err := tx.LineComments().ListByReview(ctx, principal.TenantID, review.ID)
		if err != nil {
			return err
		}
		commentViews := make([]CommentView, len(comments))
		for i := range comments {
			commentViews[i] = commentView(&comments[i])
		}

		result = application.Envelope[ReviewView]{
			Data: reviewView(review, commentViews),
			Meta: application.Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

func (s *Service) applyDecisionToExecution(ctx context.Context, tx application.Tx, review *reviewdomain.Review, now time.Time) error {
	submission, err := s.submissions.GetByID(ctx, review.TenantID, review.SubmissionID)
	if err != nil {
		return err
	}
	execution, version, err := tx.GetExecutionForUpdate(ctx, review.TenantID, submission.ExecutionID)
	if err != nil {
		return err
	}

	actor := domain.Actor{Type: domain.ActorReviewer, ID: review.ReviewerID}
	switch review.FinalDecision {
	case reviewdomain.DecisionAccepted:
		if err := execution.Accept(actor, now); err != nil {
			return err
		}
		taskRecord, err := tx.GetTask(ctx, review.TenantID, execution.TaskID)
		if err != nil {
			return err
		}
		task := &domain.Task{
			ID: taskRecord.ID, TenantID: taskRecord.TenantID,
			PublisherID: taskRecord.PublisherAgentVersionID, Deadline: taskRecord.Deadline,
			Status: taskRecord.Status, ClaimedBy: taskRecord.ClaimedBy,
		}
		if err := task.Apply(domain.IntentComplete, actor, now); err != nil {
			return err
		}
		taskRecord.Status = task.Status
		updatedTask, err := tx.UpdateTask(ctx, *taskRecord, taskRecord.StateVersion, execution.ID)
		if err != nil {
			return err
		}
		if !updatedTask {
			return &domain.Error{Code: "state_conflict", Message: "task changed concurrently"}
		}
	case reviewdomain.DecisionRejected:
		if err := execution.Reject(actor, now); err != nil {
			return err
		}
	case reviewdomain.DecisionRevisionRequested:
		if err := execution.RequestRevision(actor, now); err != nil {
			return err
		}
	}

	updated, err := tx.UpdateExecution(ctx, execution, version)
	if err != nil {
		return err
	}
	if !updated {
		return &domain.Error{Code: "state_conflict", Message: "execution changed concurrently"}
	}
	return nil
}

func (s *Service) executionForSubmission(ctx context.Context, tx application.Tx, tenantID, submissionID string) (*domain.Execution, error) {
	submission, err := s.submissions.GetByID(ctx, tenantID, submissionID)
	if err != nil {
		return nil, err
	}
	execution, _, err := tx.GetExecution(ctx, tenantID, submission.ExecutionID)
	return execution, err
}

func reviewRecord(review *reviewdomain.Review, reviewer *reviewdomain.ReviewerProfile) ReviewRecord {
	return ReviewRecord{
		TenantID:       review.TenantID,
		ID:             review.ID,
		SubmissionID:   review.SubmissionID,
		ReviewerID:     review.ReviewerID,
		ReviewerUserID: reviewer.UserID,
		Status:         string(review.Status),
	}
}

func taskSummary(task *application.TaskRecord, execution *domain.Execution) application.TaskSummary {
	return application.TaskSummary{
		TenantID:                task.TenantID,
		PublisherAgentVersionID: task.PublisherAgentVersionID,
		ExecutionAgentVersionID: execution.AgentID,
	}
}

// GetSubmissionDiff returns the structured diff for a submission.
// The caller must be allowed to view the associated review.
func (s *Service) GetSubmissionDiff(ctx context.Context, principal auth.Principal, query GetSubmissionDiff) (application.Envelope[[]FileDiff], error) {
	var result application.Envelope[[]FileDiff]
	if err := requireTenant(principal); err != nil {
		return result, err
	}

	err := s.store.WithTx(ctx, func(tx application.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}

		execution, err := s.executionForSubmission(ctx, tx, principal.TenantID, query.SubmissionID)
		if err != nil {
			return err
		}

		task, err := tx.GetTask(ctx, principal.TenantID, execution.TaskID)
		if err != nil {
			return err
		}

		if err := s.canViewSubmissionDiff(ctx, tx, principal, query.SubmissionID, task, execution); err != nil {
			return err
		}

		diff, err := s.diff.GetDiff(ctx, principal.TenantID, query.SubmissionID)
		if err != nil {
			return err
		}

		result = application.Envelope[[]FileDiff]{
			Data: diff,
			Meta: application.Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

func (s *Service) canViewSubmissionDiff(ctx context.Context, tx application.Tx, principal auth.Principal, submissionID string, task *application.TaskRecord, execution *domain.Execution) error {
	if principal.IsAdmin && principal.TenantID == task.TenantID {
		return nil
	}
	reviews, err := tx.Reviews().ListBySubmission(ctx, principal.TenantID, submissionID)
	if err != nil {
		return err
	}
	for i := range reviews {
		reviewer, err := tx.Reviewers().GetByID(ctx, principal.TenantID, reviews[i].ReviewerID)
		if err != nil {
			return err
		}
		if s.policy.CanViewReview(ctx, principal, reviewRecord(&reviews[i], reviewer), taskSummary(task, execution)) == nil {
			return nil
		}
	}
	return domain.ErrForbidden
}

// SubmitForReview moves a running execution to the reviewing state.
// TODO: replace with git-delivery-and-validation integration
func (s *Service) SubmitForReview(ctx context.Context, principal auth.Principal, cmd SubmitForReview) (application.Envelope[application.ExecutionView], error) {
	var result application.Envelope[application.ExecutionView]
	if err := requireTenant(principal); err != nil {
		return result, err
	}

	err := s.store.WithTx(ctx, func(tx application.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}

		key, record, err := acquireReview(ctx, tx, principal, "execution_submit_for_review", cmd.RequestID, cmd, now)
		if err != nil {
			return err
		}
		if record.Completed {
			return json.Unmarshal(record.ResponseBody, &result)
		}
		if !record.Acquired {
			return &domain.Error{Code: "state_conflict", Message: "idempotency request is already in progress"}
		}

		execution, version, err := tx.GetExecutionForUpdate(ctx, principal.TenantID, cmd.ExecutionID)
		if err != nil {
			return err
		}

		task, err := tx.GetTask(ctx, principal.TenantID, execution.TaskID)
		if err != nil {
			return err
		}

		if err := s.policy.CanSubmitForReview(principal, taskSummary(task, execution), execution); err != nil {
			return err
		}

		actor := domain.Actor{Type: domain.ActorSystem, ID: "system"}
		if principal.Type == auth.PrincipalTypeAgent {
			actor = domain.Actor{Type: domain.ActorAgent, ID: principal.AgentID}
		} else if principal.Type == auth.PrincipalTypeHuman {
			actor = domain.Actor{Type: domain.ActorPublisher, ID: principal.OwnerID}
		}
		if err := execution.SubmitForReview(actor, now); err != nil {
			return err
		}

		updated, err := tx.UpdateExecution(ctx, execution, version)
		if err != nil {
			return err
		}
		if !updated {
			return &domain.Error{Code: "state_conflict", Message: "execution changed concurrently"}
		}

		result = application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{
				ID:             execution.ID,
				TaskID:         execution.TaskID,
				TenantID:       execution.TenantID,
				AgentVersionID: execution.AgentID,
				Status:         execution.Status,
			},
			Meta: application.Meta{ServerTime: now},
		}
		return completeReview(ctx, tx, key, record.OwnerToken, result)
	})
	return result, err
}

const reviewIdempotencyTTL = 24 * time.Hour

func acquireReview(ctx context.Context, tx application.Tx, principal auth.Principal, operation, requestID string, request any, now time.Time) (application.IdempotencyKey, *application.IdempotencyRecord, error) {
	if requestID == "" {
		return application.IdempotencyKey{}, nil, invalid("request_id")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return application.IdempotencyKey{}, nil, err
	}
	key := application.IdempotencyKey{TenantID: principal.TenantID, ActorID: actorID(principal), Operation: operation, RequestID: requestID}
	record, err := tx.AcquireIdempotency(ctx, key, sha256.Sum256(payload), now.Add(reviewIdempotencyTTL))
	return key, record, err
}

func completeReview[T any](ctx context.Context, tx application.Tx, key application.IdempotencyKey, owner string, response application.Envelope[T]) error {
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return tx.CompleteIdempotency(ctx, key, owner, 200, body)
}

func actorID(principal auth.Principal) string {
	if principal.AgentVersionID != "" {
		return principal.AgentVersionID
	}
	return principal.OwnerID
}

func reviewView(review *reviewdomain.Review, comments []CommentView) ReviewView {
	if comments == nil {
		comments = []CommentView{}
	}
	return ReviewView{
		ID:              review.ID,
		TenantID:        review.TenantID,
		SubmissionID:    review.SubmissionID,
		ReviewerID:      review.ReviewerID,
		RubricVersionID: review.RubricVersionID,
		Capability:      review.Capability,
		RubricScores:    append([]reviewdomain.RubricScore(nil), review.RubricScores...),
		Summary:         review.Summary,
		Status:          string(review.Status),
		FinalDecision:   string(review.FinalDecision),
		SubmittedAt:     review.SubmittedAt,
		CreatedAt:       review.CreatedAt,
		LineComments:    comments,
	}
}

func commentView(comment *reviewdomain.LineComment) CommentView {
	return CommentView{
		ID:              comment.ID,
		TenantID:        comment.TenantID,
		ReviewID:        comment.ReviewID,
		SubmissionID:    comment.SubmissionID,
		FilePath:        comment.FilePath,
		Side:            string(comment.Side),
		LineNumber:      comment.LineNumber,
		HunkHash:        comment.HunkHash,
		DiffFingerprint: comment.DiffFingerprint,
		Text:            comment.Text,
		CreatedAt:       comment.CreatedAt,
	}
}

func rubricView(rubric *reviewdomain.RubricVersion) RubricView {
	dims := make([]RubricDimensionView, len(rubric.Dimensions))
	for i, d := range rubric.Dimensions {
		dims[i] = RubricDimensionView{ID: d.ID, Name: d.Name}
	}
	weights := make(map[string]float64, len(rubric.Weights))
	for k, v := range rubric.Weights {
		weights[k] = v
	}
	return RubricView{
		ID:               rubric.ID,
		TenantID:         rubric.TenantID,
		VersionNumber:    rubric.VersionNumber,
		Name:             rubric.Name,
		Dimensions:       dims,
		Weights:          weights,
		AlgorithmVersion: rubric.AlgorithmVersion,
		IsActive:         rubric.IsActive,
		CreatedAt:        rubric.CreatedAt,
	}
}

func requireTenant(principal auth.Principal) error {
	if principal.TenantID == "" {
		return domain.ErrForbidden
	}
	return nil
}

func invalid(field string) error {
	return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
