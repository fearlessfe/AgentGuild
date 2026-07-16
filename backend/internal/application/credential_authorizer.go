package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
)

// AuthorizeCredential binds a Git credential request to the persisted task
// execution and its repository source. Request fields are only consistency
// assertions; they never expand the execution's authority.
func (s *Service) AuthorizeCredential(ctx context.Context, principal gitapp.Principal, cmd gitapp.IssueCredential, now time.Time) (gitapp.CredentialGrant, error) {
	var executionTenant, taskID, executionAgentVersion string
	var executionStatus domain.ExecutionStatus
	var hardExpiry time.Time
	var constraints []byte

	err := s.store.WithTx(ctx, func(tx Tx) error {
		execution, _, err := tx.GetExecution(ctx, principal.TenantID, cmd.ExecutionID)
		if err != nil {
			return err
		}
		task, err := tx.GetTask(ctx, principal.TenantID, execution.TaskID)
		if err != nil {
			return err
		}
		executionTenant = execution.TenantID
		taskID = execution.TaskID
		executionAgentVersion = execution.AgentID
		executionStatus = execution.Status
		hardExpiry = execution.Lease.HardExpiry
		constraints = append([]byte(nil), task.Constraints...)
		return nil
	})
	if err != nil {
		return gitapp.CredentialGrant{}, err
	}
	if executionTenant != principal.TenantID || executionAgentVersion != principal.AgentVersionID {
		return gitapp.CredentialGrant{}, notFound()
	}
	if executionStatus != domain.ExecutionLeased && executionStatus != domain.ExecutionRunning {
		return gitapp.CredentialGrant{}, domain.ErrStateConflict
	}
	if hardExpiry.IsZero() || !now.Before(hardExpiry) {
		return gitapp.CredentialGrant{}, domain.ErrStateConflict
	}

	repo, constrainedBase, err := s.credentialTaskBinding(ctx, principal.TenantID, taskID, constraints)
	if err != nil {
		return gitapp.CredentialGrant{}, err
	}
	if repo == "" || cmd.Repo != repo {
		return gitapp.CredentialGrant{}, domain.ErrForbidden
	}
	baseCommit := ""
	if constrainedBase != "" {
		if cmd.BaseCommit != constrainedBase {
			return gitapp.CredentialGrant{}, domain.ErrForbidden
		}
		baseCommit = constrainedBase
	}
	return gitapp.CredentialGrant{Repo: repo, BaseCommit: baseCommit, ExpiresAt: hardExpiry}, nil
}

// AuthorizeSubmission binds a submission to the current Agent Version's live
// running execution and derives task/repository/path constraints server-side.
func (s *Service) AuthorizeSubmission(ctx context.Context, principal gitapp.Principal, cmd gitapp.CreateSubmission, now time.Time) (gitapp.SubmissionGrant, error) {
	var taskID, executionTenant, executionAgentVersion string
	var status domain.ExecutionStatus
	var hardExpiry time.Time
	var rawConstraints []byte
	err := s.store.WithTx(ctx, func(tx Tx) error {
		execution, _, err := tx.GetExecution(ctx, principal.TenantID, cmd.ExecutionID)
		if err != nil {
			return err
		}
		task, err := tx.GetTask(ctx, principal.TenantID, execution.TaskID)
		if err != nil {
			return err
		}
		taskID = execution.TaskID
		executionTenant = execution.TenantID
		executionAgentVersion = execution.AgentID
		status = execution.Status
		hardExpiry = execution.Lease.HardExpiry
		rawConstraints = append([]byte(nil), task.Constraints...)
		return nil
	})
	if err != nil {
		return gitapp.SubmissionGrant{}, err
	}
	if executionTenant != principal.TenantID || executionAgentVersion != principal.AgentVersionID {
		return gitapp.SubmissionGrant{}, notFound()
	}
	switch status {
	case domain.ExecutionRunning:
		if hardExpiry.IsZero() || !now.Before(hardExpiry) {
			return gitapp.SubmissionGrant{}, domain.ErrStateConflict
		}
	case domain.ExecutionSubmitted, domain.ExecutionValidating, domain.ExecutionValidationFailed,
		domain.ExecutionReviewing, domain.ExecutionRevisionRequested, domain.ExecutionAccepted, domain.ExecutionRejected:
		// Downstream states are allowed only to replay an already persisted
		// submission; SubmissionService requires an active credential for new work.
	default:
		return gitapp.SubmissionGrant{}, domain.ErrStateConflict
	}
	if cmd.TaskID != "" && cmd.TaskID != taskID {
		return gitapp.SubmissionGrant{}, domain.ErrForbidden
	}
	allowed, forbidden := pathConstraints(rawConstraints)
	repo, constrainedBase, err := s.credentialTaskBinding(ctx, principal.TenantID, taskID, rawConstraints)
	if err != nil {
		return gitapp.SubmissionGrant{}, err
	}
	if repo == "" || cmd.Repo != repo {
		return gitapp.SubmissionGrant{}, domain.ErrForbidden
	}
	if constrainedBase != "" && cmd.BaseCommitSHA != constrainedBase {
		return gitapp.SubmissionGrant{}, domain.ErrForbidden
	}
	return gitapp.SubmissionGrant{
		TaskID: taskID, Repo: repo, BaseCommit: constrainedBase,
		AllowedPaths: allowed, ForbiddenPaths: forbidden,
	}, nil
}

func (s *Service) credentialTaskBinding(ctx context.Context, tenantID, taskID string, rawConstraints []byte) (string, string, error) {
	if s.issueSourceLookup != nil {
		sources, err := s.issueSourceLookup.LookupByTaskIDs(ctx, tenantID, []string{taskID})
		if err != nil {
			return "", "", err
		}
		if source, ok := sources[taskID]; ok && source.Repo != "" {
			return source.Repo, constraintValue(rawConstraints, "base_commit:"), nil
		}
	}
	return constraintValue(rawConstraints, "repo:"), constraintValue(rawConstraints, "base_commit:"), nil
}

func constraintValue(raw []byte, prefix string) string {
	var constraints []string
	if len(raw) == 0 || json.Unmarshal(raw, &constraints) != nil {
		return ""
	}
	for _, constraint := range constraints {
		if value, ok := strings.CutPrefix(constraint, prefix); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func pathConstraints(raw []byte) (allowed, forbidden []string) {
	var constraints []string
	if len(raw) == 0 || json.Unmarshal(raw, &constraints) != nil {
		return nil, nil
	}
	for _, constraint := range constraints {
		if value, ok := strings.CutPrefix(constraint, "path:allowed:"); ok {
			allowed = append(allowed, strings.TrimSpace(value))
		}
		if value, ok := strings.CutPrefix(constraint, "path:forbidden:"); ok {
			forbidden = append(forbidden, strings.TrimSpace(value))
		}
	}
	return allowed, forbidden
}

var _ gitapp.CredentialGrantAuthorizer = (*Service)(nil)
var _ gitapp.SubmissionAuthorizer = (*Service)(nil)
