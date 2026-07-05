package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
)

// CreateSubmission validates the commit and persists a new submission.
func (s *SubmissionService) CreateSubmission(ctx context.Context, principal Principal, cmd CreateSubmission) (Envelope[SubmissionView], error) {
	var result Envelope[SubmissionView]
	if err := s.requireAgent(principal); err != nil {
		return result, err
	}
	if cmd.ExecutionID == "" {
		return result, invalid("execution_id")
	}
	if cmd.TaskID == "" {
		return result, invalid("task_id")
	}
	if cmd.Repo == "" {
		return result, invalid("repo")
	}
	if cmd.Branch == "" {
		return result, invalid("branch")
	}
	if cmd.CommitSHA == "" {
		return result, invalid("commit_sha")
	}
	if cmd.BaseCommitSHA == "" {
		return result, invalid("base_commit_sha")
	}
	if cmd.Summary == "" {
		return result, invalid("summary")
	}

	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}

		// Idempotency: same execution + same commit SHA returns the existing
		// submission regardless of request id.
		existing, err := tx.Submissions().GetByExecutionID(ctx, principal.TenantID, cmd.ExecutionID)
		if err != nil {
			return err
		}
		for _, sub := range existing {
			if sub.CommitSHA == cmd.CommitSHA {
				result = Envelope[SubmissionView]{
					Data: submissionView(sub, now),
					Meta: Meta{ServerTime: now},
				}
				return nil
			}
		}

		if err := s.verifier.Verify(ctx, VerifyCommit{
			TenantID:       principal.TenantID,
			ExecutionID:    cmd.ExecutionID,
			Repo:           cmd.Repo,
			Branch:         cmd.Branch,
			CommitSHA:      cmd.CommitSHA,
			BaseCommitSHA:  cmd.BaseCommitSHA,
			AllowedPaths:   cmd.AllowedPaths,
			ForbiddenPaths: cmd.ForbiddenPaths,
		}); err != nil {
			return err
		}

		files, err := s.verifier.ChangedFiles(ctx, cmd.Repo, cmd.BaseCommitSHA, cmd.CommitSHA)
		if err != nil {
			return err
		}
		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.Filename
		}

		sub, err := gitdomain.NewSubmission(
			principal.TenantID,
			cmd.TaskID,
			cmd.ExecutionID,
			cmd.Branch,
			cmd.CommitSHA,
			cmd.BaseCommitSHA,
			cmd.Summary,
			cmd.Tests,
			cmd.Evidence,
			paths,
			now,
			s.newID,
		)
		if err != nil {
			return err
		}
		if err := tx.Submissions().Save(ctx, sub); err != nil {
			return err
		}

		configVersion := cmd.ConfigVersion
		if configVersion == "" {
			configVersion = "default"
		}
		job, err := gitdomain.NewValidationJob(principal.TenantID, sub.ID, cmd.Repo, cmd.Branch, cmd.CommitSHA, configVersion, now, s.newID)
		if err != nil {
			return err
		}
		if err := tx.ValidationJobs().Insert(ctx, job); err != nil {
			return err
		}
		sub.SetValidationJobID(job.ID, now)
		if err := tx.Submissions().Save(ctx, sub); err != nil {
			return err
		}

		result = Envelope[SubmissionView]{
			Data: submissionView(sub, now),
			Meta: Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

// GetSubmission returns a submission by ID for an authorized caller.
func (s *SubmissionService) GetSubmission(ctx context.Context, principal Principal, query GetSubmission) (Envelope[SubmissionView], error) {
	var result Envelope[SubmissionView]
	if err := requireCaller(principal); err != nil {
		return result, err
	}
	if query.SubmissionID == "" {
		return result, invalid("submission_id")
	}

	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		sub, err := tx.Submissions().GetByID(ctx, principal.TenantID, query.SubmissionID)
		if err != nil {
			return err
		}
		result = Envelope[SubmissionView]{
			Data: submissionView(sub, now),
			Meta: Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

func (s *SubmissionService) requireAgent(principal Principal) error {
	if principal.TenantID == "" {
		return domain.ErrForbidden
	}
	if principal.AgentID == "" {
		return domain.ErrForbidden
	}
	for _, scope := range principal.Scopes {
		if scope == "tasks:execute" {
			return nil
		}
	}
	return domain.ErrForbidden
}

func submissionView(sub *gitdomain.Submission, now time.Time) SubmissionView {
	return SubmissionView{
		ID:              sub.ID,
		TenantID:        sub.TenantID,
		TaskID:          sub.TaskID,
		ExecutionID:     sub.ExecutionID,
		Branch:          sub.Branch,
		CommitSHA:       sub.CommitSHA,
		BaseCommitSHA:   sub.BaseCommitSHA,
		Summary:         sub.Summary,
		Tests:           sub.TestDeclaration,
		Evidence:        sub.Evidence,
		DiffFingerprint: sub.DiffFingerprint,
		Status:          sub.Status,
		ValidationJobID: sub.ValidationJobID,
		CreatedAt:       sub.CreatedAt,
		UpdatedAt:       sub.UpdatedAt,
	}
}
