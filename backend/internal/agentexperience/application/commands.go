package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"agentguild.dev/agentguild/backend/internal/agentexperience/domain"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
)

// NewCandidateService creates the experience candidate service.
func NewCandidateService(
	store Store,
	candidates ExperienceCandidateRepository,
	submissions SubmissionStore,
	executions ExecutionStore,
	policy *Policy,
	classifier domain.SensitivityPolicy,
	options CandidateOptions,
) (*CandidateService, error) {
	if store == nil {
		return nil, invalidArg("store")
	}
	if candidates == nil {
		return nil, invalidArg("candidates")
	}
	if submissions == nil {
		return nil, invalidArg("submissions")
	}
	if executions == nil {
		return nil, invalidArg("executions")
	}
	if policy == nil {
		return nil, invalidArg("policy")
	}
	if classifier == nil {
		return nil, invalidArg("classifier")
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	return &CandidateService{
		store:       store,
		candidates:  candidates,
		submissions: submissions,
		executions:  executions,
		policy:      policy,
		classifier:  classifier,
		newID:       options.NewID,
	}, nil
}

// ExtractCandidate creates an ExperienceCandidate from an accepted submission.
// It reads the submission, resolves capabilities from its execution, computes
// the evidence content hash from EvidenceBytes, and applies the sensitivity
// policy to the actual evidence content.
func (s *CandidateService) ExtractCandidate(
	ctx context.Context,
	cmd ExtractCandidate,
) (*ExtractCandidateResponse, error) {
	principal := identityapp.Principal{
		TenantID: cmd.TenantID,
		OwnerID:  cmd.CreatedBy,
		IsAdmin:  cmd.IsAdmin,
	}
	if err := s.policy.RequireOwnerOrAdmin(ctx, principal, cmd.TenantID, cmd.AgentID); err != nil {
		return nil, err
	}
	if len(cmd.EvidenceBytes) == 0 {
		return nil, invalidArg("evidence_bytes")
	}

	submission, err := s.submissions.GetAcceptedSubmission(ctx, cmd.TenantID, cmd.SubmissionID)
	if err != nil {
		return nil, err
	}
	if submission.AgentID != cmd.AgentID {
		return nil, domain.ErrNotFound
	}

	var capabilities []string
	if submission.ExecutionID != "" {
		execution, err := s.executions.GetExecution(ctx, cmd.TenantID, submission.ExecutionID)
		if err != nil {
			return nil, err
		}
		if execution.AgentID != cmd.AgentID {
			return nil, domain.ErrNotFound
		}
		capabilities = append([]string(nil), execution.Capabilities...)
	}

	var candidate *domain.ExperienceCandidate
	if err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		candidate, err = domain.NewExperienceCandidate(
			s.newID(), cmd.TenantID, cmd.AgentID,
			submission.TaskID, submission.ID, submission.ReviewID,
			cmd.EvidenceBytes, capabilities, cmd.TenantID, now,
		)
		if err != nil {
			return err
		}
		if err := candidate.ClassifyAndApply(s.classifier, cmd.EvidenceBytes); err != nil {
			return err
		}
		return s.candidates.Create(ctx, tx, candidate)
	}); err != nil {
		return nil, err
	}
	return &ExtractCandidateResponse{Candidate: candidate}, nil
}

// ReviewCandidate approves or rejects a pending candidate.
func (s *CandidateService) ReviewCandidate(ctx context.Context, cmd ReviewCandidate) error {
	principal := identityapp.Principal{
		TenantID: cmd.TenantID,
		OwnerID:  cmd.ReviewerID,
		IsAdmin:  cmd.IsAdmin,
	}
	if err := s.policy.RequireOwnerOrAdmin(ctx, principal, cmd.TenantID, cmd.AgentID); err != nil {
		return err
	}

	return s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		candidate, err := s.candidates.GetByID(ctx, cmd.TenantID, cmd.AgentID, cmd.CandidateID)
		if err != nil {
			return err
		}
		switch cmd.Action {
		case "approve":
			if err := candidate.Approve(cmd.ReviewerID, now); err != nil {
				return err
			}
		case "reject":
			if err := candidate.Reject(cmd.ReviewerID, cmd.Reason, now); err != nil {
				return err
			}
		default:
			return invalidArg("action")
		}
		return s.candidates.UpdateStatus(ctx, tx, candidate)
	})
}

func invalidArg(field string) error {
	return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
