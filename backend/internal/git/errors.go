package git

import "agentguild.dev/agentguild/backend/internal/domain"

var (
	ErrRepoNotFound              = &domain.Error{Code: "repo_not_found", Message: "repository or ref not found"}
	ErrInvalidRepo               = &domain.Error{Code: "invalid_argument", Message: "repo must be in owner/name format"}
	ErrUnauthorized              = &domain.Error{Code: "unauthorized", Message: "GitHub credentials are invalid or expired"}
	ErrGitHubAPI                 = &domain.Error{Code: "external_error", Message: "GitHub API request failed"}
	ErrCredentialNotFound        = &domain.Error{Code: "not_found", Message: "credential not found"}
	ErrCredentialRevoked         = &domain.Error{Code: "state_conflict", Message: "credential has been revoked"}
	ErrAlreadyIssued             = &domain.Error{Code: "already_exists", Message: "credential already issued"}
	ErrInvalidBranch             = &domain.Error{Code: "invalid_argument", Message: "branch must be under agentguild/", Field: "branch"}
	ErrRepositoryBindingConflict = &domain.Error{Code: "state_conflict", Message: "repository source binding cannot be changed"}
	ErrSubmissionNotFound        = &domain.Error{Code: "not_found", Message: "submission not found"}
	ErrValidationJobNotFound     = &domain.Error{Code: "not_found", Message: "validation job not found"}
	ErrGitHubAppNotConfigured    = &domain.Error{Code: "not_configured", Message: "GitHub App is not configured for this tenant"}
)
