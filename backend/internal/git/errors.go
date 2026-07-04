package git

import "agentguild.dev/agentguild/backend/internal/domain"

var (
	ErrRepoNotFound = &domain.Error{Code: "repo_not_found", Message: "repository or ref not found"}
	ErrInvalidRepo  = &domain.Error{Code: "invalid_argument", Message: "repo must be in owner/name format"}
	ErrUnauthorized = &domain.Error{Code: "unauthorized", Message: "GitHub credentials are invalid or expired"}
	ErrGitHubAPI    = &domain.Error{Code: "external_error", Message: "GitHub API request failed"}
)
