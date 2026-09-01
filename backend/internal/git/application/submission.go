package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	participationdomain "agentguild.dev/agentguild/backend/internal/participation/domain"
)

// TaskDeadlineResolver resolves the persisted deadline for a task. It is
// wired in production from the core task store; when nil the deadline guard
// is disabled (tests and local development).
type TaskDeadlineResolver interface {
	TaskDeadline(ctx context.Context, tenantID, taskID string) (time.Time, error)
}

// TaskDeadlineResolverFunc adapts a function to TaskDeadlineResolver.
type TaskDeadlineResolverFunc func(context.Context, string, string) (time.Time, error)

// TaskDeadline implements TaskDeadlineResolver.
func (f TaskDeadlineResolverFunc) TaskDeadline(ctx context.Context, tenantID, taskID string) (time.Time, error) {
	return f(ctx, tenantID, taskID)
}

// SetTaskDeadlineResolver wires the task deadline guard after construction.
func (s *SubmissionService) SetTaskDeadlineResolver(resolver TaskDeadlineResolver) {
	s.deadlines = resolver
}

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

	var resourceTenantID string
	var external bool
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
		resourceTenantID = grant.ResourceTenantID
		if resourceTenantID == "" {
			resourceTenantID = principal.TenantID
		}
		external = grant.External
		if resourceTenantID == "" || external != principal.IsGlobalAgent() {
			return domain.ErrForbidden
		}
		if !external && !hasRepoScope(principal.RepoScope, grant.Repo) {
			return domain.ErrForbidden
		}
		expectedBranch := restrictedBranchPrefix + cmd.ExecutionID
		// Idempotency remains available after the proxy credential is revoked.
		existing, err := tx.Submissions().GetByExecutionID(ctx, resourceTenantID, cmd.ExecutionID)
		if err != nil {
			return err
		}
		for _, sub := range existing {
			if sub.CommitSHA == cmd.CommitSHA {
				if cmd.Repo != sub.Repo || cmd.Branch != sub.Branch || cmd.BaseCommitSHA != sub.BaseCommitSHA {
					return domain.ErrForbidden
				}
				result = Envelope[SubmissionView]{Data: submissionViewForAccess(sub, now, external), Meta: Meta{ServerTime: now}}
				return nil
			}
		}
		// The task deadline is a hard cutoff for result submission, aligned
		// with the heartbeat guard: a live lease must not extend the window
		// past the deadline.
		if s.deadlines != nil {
			deadline, err := s.deadlines.TaskDeadline(ctx, resourceTenantID, grant.TaskID)
			if err != nil {
				return err
			}
			if !now.Before(deadline) {
				return &domain.Error{Code: "deadline_exceeded", Message: "task deadline has passed"}
			}
		}
		credential, err := tx.Credentials().GetByExecutionID(ctx, resourceTenantID, cmd.ExecutionID)
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
			TenantID:       resourceTenantID,
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

		files, err := s.verifier.ChangedFiles(ctx, resourceTenantID, cmd.Repo, cmd.BaseCommitSHA, cmd.CommitSHA)
		if err != nil {
			return err
		}
		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.Filename
		}

		sub, err := gitdomain.NewSubmission(
			resourceTenantID,
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
		if err := tx.Credentials().Revoke(ctx, resourceTenantID, cmd.ExecutionID); err != nil {
			return err
		}

		configVersion := cmd.ConfigVersion
		if configVersion == "" {
			configVersion = "default"
		}
		job, err := gitdomain.NewValidationJob(resourceTenantID, sub.ID, cmd.ExecutionID, cmd.Repo, cmd.Branch, cmd.CommitSHA, configVersion, now, s.newID)
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
			Data: submissionViewForAccess(sub, now, external),
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
		TenantID:    resourceTenantID,
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
	resourceTenantID, external, err := s.authorizeRead(ctx, principal, participationdomain.ResourceSubmission, query.SubmissionID)
	if err != nil {
		return result, err
	}

	err = s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		sub, err := tx.Submissions().GetByID(ctx, resourceTenantID, query.SubmissionID)
		if err != nil {
			return err
		}
		result = Envelope[SubmissionView]{
			Data: submissionViewForAccess(sub, now, external),
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
	resourceTenantID, external, err := s.authorizeRead(ctx, principal, participationdomain.ResourceExecution, query.ExecutionID)
	if err != nil {
		return result, err
	}
	err = s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		submissions, err := tx.Submissions().GetByExecutionID(ctx, resourceTenantID, query.ExecutionID)
		if err != nil {
			return err
		}
		views := make([]SubmissionView, len(submissions))
		for i := range submissions {
			views[i] = submissionViewForAccess(submissions[i], now, external)
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
	if principal.IsGlobalAgent() {
		return domain.ErrForbidden
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
	if principal.AgentID == "" || principal.AgentVersionID == "" || (principal.TenantID == "" && !principal.IsGlobalAgent()) {
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

func submissionViewForAccess(sub *gitdomain.Submission, now time.Time, external bool) SubmissionView {
	view := submissionView(sub, now)
	if external {
		view.TenantID = ""
	}
	return view
}

func (s *SubmissionService) authorizeRead(ctx context.Context, principal Principal, kind participationdomain.ResourceKind, resourceID string) (string, bool, error) {
	if !principal.IsGlobalAgent() {
		if principal.TenantID == "" {
			return "", false, domain.ErrForbidden
		}
		return principal.TenantID, false, nil
	}
	if s.participation == nil {
		return "", true, domain.ErrForbidden
	}
	grant, err := s.participation.Authorize(ctx, participationPrincipal(principal), kind, resourceID, participationdomain.ScopeExecutionRead)
	if err != nil {
		return "", true, err
	}
	return grant.ResourceTenantID, true, nil
}

func participationPrincipal(principal Principal) auth.Principal {
	return auth.Principal{
		SubjectID: principal.SubjectID, IdentityScope: principal.IdentityScope,
		TenantID: principal.TenantID, Type: auth.PrincipalTypeAgent,
		AgentID: principal.AgentID, AgentVersionID: principal.AgentVersionID,
		Scopes: append([]string(nil), principal.Scopes...),
	}
}
