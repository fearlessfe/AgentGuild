package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
)

// Service orchestrates the code-review lifecycle.
type Service struct {
	store       application.Store
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
func NewService(store application.Store, diff DiffProvider, validation ValidationProvider, options Options) (*Service, error) {
	if store == nil {
		return nil, invalid("store")
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
		store:      store,
		policy:     Policy{},
		allocator:  Allocator{},
		diff:       diff,
		validation: validation,
		newID:      options.NewID,
	}, nil
}

// CreateReview assigns a reviewer to a submission and creates a pending review.
type CreateReview struct {
	SubmissionID string
	Capabilities []string
}

// SubmitDecision records the reviewer's final decision and scores.
type SubmitDecision struct {
	ReviewID string
	Decision reviewdomain.Decision
	Scores   []reviewdomain.RubricScore
	Summary  string
}

// AddComment adds a line-level comment to a review.
type AddComment struct {
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

// ReviewView is the serialized representation of a review.
type ReviewView struct {
	ID              string
	TenantID        string
	SubmissionID    string
	ReviewerID      string
	RubricVersionID string
	Scores          []reviewdomain.RubricScore
	Summary         string
	Status          string
	FinalDecision   string
	SubmittedAt     time.Time
	CreatedAt       time.Time
}

// CommentView is the serialized representation of a line comment.
type CommentView struct {
	ID              string
	TenantID        string
	ReviewID        string
	SubmissionID    string
	FilePath        string
	Side            string
	LineNumber      int
	HunkHash        string
	DiffFingerprint string
	Text            string
	CreatedAt       time.Time
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

		execution, _, err := tx.GetExecution(ctx, principal.TenantID, cmd.SubmissionID)
		if err != nil {
			if domain.CodeOf(err) == "not_found" {
				return domain.ErrNotFound
			}
			return err
		}
		if execution.Status != domain.ExecutionReviewing {
			// Accept a small window where the submission has just been handed off.
			if execution.Status != domain.ExecutionSubmitted && execution.Status != domain.ExecutionValidating {
				return &domain.Error{Code: "state_conflict", Message: "submission is not ready for review"}
			}
		}

		task, err := tx.GetTask(ctx, principal.TenantID, execution.TaskID)
		if err != nil {
			return err
		}

		if err := s.policy.CanViewReview(ctx, principal, ReviewRecord{TenantID: principal.TenantID}, taskSummary(task)); err != nil {
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

		reviewer, err := tx.Reviewers().GetByID(ctx, principal.TenantID, reviewerID)
		if err != nil {
			return err
		}

		if err := reviewer.Assign(now); err != nil {
			return err
		}
		if err := tx.Reviewers().IncrementLoad(ctx, principal.TenantID, reviewerID); err != nil {
			return err
		}

		review, err := reviewdomain.NewReview(s.newID(), principal.TenantID, cmd.SubmissionID, reviewerID, rubric.ID, now)
		if err != nil {
			return err
		}
		if err := tx.Reviews().Insert(ctx, review); err != nil {
			return err
		}

		result = application.Envelope[ReviewView]{
			Data: reviewView(review),
			Meta: application.Meta{ServerTime: now},
		}
		return nil
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

		review, err := tx.Reviews().GetByID(ctx, principal.TenantID, cmd.ReviewID)
		if err != nil {
			return err
		}

		reviewer, err := tx.Reviewers().GetByID(ctx, principal.TenantID, review.ReviewerID)
		if err != nil {
			return err
		}

		record := reviewRecord(review, reviewer)
		if err := s.policy.CanSubmitDecision(principal, record); err != nil {
			return err
		}

		if cmd.Decision == reviewdomain.DecisionAccepted {
			status, err := s.validation.GetValidationStatus(ctx, review.SubmissionID)
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

		if err := tx.Reviews().Update(ctx, review); err != nil {
			return err
		}

		if err := reviewer.Release(now); err != nil {
			return err
		}
		if err := tx.Reviewers().DecrementLoad(ctx, principal.TenantID, review.ReviewerID); err != nil {
			return err
		}

		if err := applyDecisionToExecution(ctx, tx, review, now); err != nil {
			return err
		}

		result = application.Envelope[ReviewView]{
			Data: reviewView(review),
			Meta: application.Meta{ServerTime: now},
		}
		return nil
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

		review, err := tx.Reviews().GetByID(ctx, principal.TenantID, cmd.ReviewID)
		if err != nil {
			return err
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
		return nil
	})
	return result, err
}

// GetReview returns a review with its line comments.
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

		execution, _, err := tx.GetExecution(ctx, principal.TenantID, review.SubmissionID)
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

		if err := s.policy.CanViewReview(ctx, principal, reviewRecord(review, reviewer), taskSummary(task)); err != nil {
			return err
		}

		result = application.Envelope[ReviewView]{
			Data: reviewView(review),
			Meta: application.Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

func applyDecisionToExecution(ctx context.Context, tx application.Tx, review *reviewdomain.Review, now time.Time) error {
	execution, version, err := tx.GetExecutionForUpdate(ctx, review.TenantID, review.SubmissionID)
	if err != nil {
		return err
	}

	actor := domain.Actor{Type: domain.ActorReviewer, ID: review.ReviewerID}
	switch review.FinalDecision {
	case reviewdomain.DecisionAccepted:
		if err := execution.Accept(actor, now); err != nil {
			return err
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

func taskSummary(task *application.TaskRecord) application.TaskSummary {
	return application.TaskSummary{
		TenantID:                task.TenantID,
		PublisherAgentVersionID: task.PublisherAgentVersionID,
	}
}

func reviewView(review *reviewdomain.Review) ReviewView {
	return ReviewView{
		ID:              review.ID,
		TenantID:        review.TenantID,
		SubmissionID:    review.SubmissionID,
		ReviewerID:      review.ReviewerID,
		RubricVersionID: review.RubricVersionID,
		Scores:          append([]reviewdomain.RubricScore(nil), review.RubricScores...),
		Summary:         review.Summary,
		Status:          string(review.Status),
		FinalDecision:   string(review.FinalDecision),
		SubmittedAt:     review.SubmittedAt,
		CreatedAt:       review.CreatedAt,
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
