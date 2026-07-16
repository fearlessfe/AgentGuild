package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
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
	if cmd.Repo == "" {
		return result, invalid("repo")
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
		if s.authorizer == nil {
			return domain.ErrForbidden
		}
		grant, err := s.authorizer.AuthorizeSubmission(ctx, principal, cmd, now)
		if err != nil {
			return err
		}
		if grant.TaskID == "" || grant.Repo == "" {
			return domain.ErrForbidden
		}
		if !hasRepoScope(principal.RepoScope, grant.Repo) {
			return domain.ErrForbidden
		}
		expectedBranch := restrictedBranchPrefix + cmd.ExecutionID
		// Idempotency remains available after the proxy credential is revoked.
		existing, err := tx.Submissions().GetByExecutionID(ctx, principal.TenantID, cmd.ExecutionID)
		if err != nil {
			return err
		}
		for _, sub := range existing {
			if sub.CommitSHA == cmd.CommitSHA {
				if cmd.Repo != sub.Repo || cmd.Branch != sub.Branch || cmd.BaseCommitSHA != sub.BaseCommitSHA {
					return domain.ErrForbidden
				}
				result = Envelope[SubmissionView]{Data: submissionView(sub, now), Meta: Meta{ServerTime: now}}
				return nil
			}
		}
		credential, err := tx.Credentials().GetByExecutionID(ctx, principal.TenantID, cmd.ExecutionID)
		if err != nil {
			return err
		}
		if credential.Status != gitdomain.CredentialStatusActive || credential.RevokedAt != nil ||
			!now.Before(credential.ExpiresAt) || credential.Repo != grant.Repo || credential.Branch != expectedBranch {
			return domain.ErrForbidden
		}
		if grant.BaseCommit != "" && credential.BaseCommit != grant.BaseCommit {
			return domain.ErrForbidden
		}
		if cmd.Branch != credential.Branch {
			return git.ErrInvalidBranch
		}
		if cmd.Repo != credential.Repo || cmd.BaseCommitSHA != credential.BaseCommit {
			return domain.ErrForbidden
		}
		cmd.TaskID = grant.TaskID
		cmd.Repo = credential.Repo
		cmd.BaseCommitSHA = credential.BaseCommit
		cmd.Branch = credential.Branch
		cmd.AllowedPaths = append([]string(nil), grant.AllowedPaths...)
		cmd.ForbiddenPaths = append([]string(nil), grant.ForbiddenPaths...)

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

		files, err := s.verifier.ChangedFiles(ctx, principal.TenantID, cmd.Repo, cmd.BaseCommitSHA, cmd.CommitSHA)
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
			cmd.Repo,
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
		if err := tx.Credentials().Revoke(ctx, principal.TenantID, cmd.ExecutionID); err != nil {
			return err
		}

		configVersion := cmd.ConfigVersion
		if configVersion == "" {
			configVersion = "default"
		}
		job, err := gitdomain.NewValidationJob(principal.TenantID, sub.ID, cmd.ExecutionID, cmd.Repo, cmd.Branch, cmd.CommitSHA, configVersion, now, s.newID)
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
	if err != nil {
		return result, err
	}

	// A failed state transition remains retryable: the idempotent submission
	// lookup above returns the same record, then retries this notification.
	if err := s.notifier.Notify(ctx, ExecutionStateCommand{
		TenantID:    principal.TenantID,
		ExecutionID: cmd.ExecutionID,
		Intent:      domain.IntentSubmit,
		Actor:       domain.Actor{Type: domain.ActorAgent, ID: principal.AgentID},
	}, time.Now()); err != nil {
		return result, err
	}

	return result, nil
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

// ListSubmissions returns tenant-scoped submissions for an execution. The
// transport must first authorize access to the owning execution.
func (s *SubmissionService) ListSubmissions(ctx context.Context, principal Principal, query ListSubmissions) (Envelope[[]SubmissionView], error) {
	var result Envelope[[]SubmissionView]
	if err := requireCaller(principal); err != nil {
		return result, err
	}
	if query.ExecutionID == "" {
		return result, invalid("execution_id")
	}
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		submissions, err := tx.Submissions().GetByExecutionID(ctx, principal.TenantID, query.ExecutionID)
		if err != nil {
			return err
		}
		views := make([]SubmissionView, len(submissions))
		for i := range submissions {
			views[i] = submissionView(submissions[i], now)
		}
		result = Envelope[[]SubmissionView]{Data: views, Meta: Meta{ServerTime: now}}
		return nil
	})
	return result, err
}

// CheckSubmissionIntegrity verifies the submission commit is still reachable
// from its branch. If the branch has been force-pushed and the commit is no
// longer reachable, the submission is marked invalid.
func (s *SubmissionService) CheckSubmissionIntegrity(ctx context.Context, principal Principal, query CheckSubmissionIntegrity) error {
	if err := requireCaller(principal); err != nil {
		return err
	}
	if query.SubmissionID == "" {
		return invalid("submission_id")
	}

	return s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		sub, err := tx.Submissions().GetByID(ctx, principal.TenantID, query.SubmissionID)
		if err != nil {
			return err
		}
		reachable, err := s.verifier.IsCommitReachable(ctx, principal.TenantID, sub.Repo, sub.Branch, sub.CommitSHA)
		if err != nil {
			return err
		}
		if !reachable {
			if err := sub.MarkInvalid(now); err != nil {
				return err
			}
			return tx.Submissions().Save(ctx, sub)
		}
		return nil
	})
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
		Repo:            sub.Repo,
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
