package application

import (
	"context"
	"errors"
	"path"
	"strings"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
)

// CommitVerifier validates that a Git commit referenced by an Agent submission
// exists, is reachable from the expected base and branch, and only touches
// allowed paths.
type CommitVerifier struct {
	appService  GitHubAppService
	submissions SubmissionRepository
}

// NewCommitVerifier creates a CommitVerifier backed by the supplied GitHub App
// service and submission repository. The GitHub Driver is resolved per-tenant at
// verification time.
func NewCommitVerifier(appService GitHubAppService, repo SubmissionRepository) *CommitVerifier {
	return &CommitVerifier{appService: appService, submissions: repo}
}

func (v *CommitVerifier) driver(ctx context.Context, tenantID string) (git.Driver, error) {
	return v.appService.Driver(ctx, tenantID)
}

// VerifyCommit carries the inputs required to validate a commit.
type VerifyCommit struct {
	TenantID       string
	ExecutionID    string
	Repo           string
	Branch         string
	CommitSHA      string
	BaseCommitSHA  string
	AllowedPaths   []string
	ForbiddenPaths []string
}

// PathViolation describes one changed file that violates the path constraints.
type PathViolation struct {
	Path   string
	Reason string
}

// PathViolationError reports that one or more changed paths are outside the
// permitted set. It carries structured violations while still identifying as an
// invalid_argument domain error.
type PathViolationError struct {
	DomainErr  *domain.Error
	Violations []PathViolation
}

func newPathViolationError(violations []PathViolation) *PathViolationError {
	return &PathViolationError{
		DomainErr: &domain.Error{
			Code:    "invalid_argument",
			Message: "changed paths violate path constraints",
			Field:   "changed_paths",
		},
		Violations: violations,
	}
}

func (e *PathViolationError) Error() string {
	return e.DomainErr.Error()
}

func (e *PathViolationError) Is(target error) bool {
	return e.DomainErr.Is(target)
}

// Unwrap returns the underlying domain error so errors.As can extract it.
func (e *PathViolationError) Unwrap() error {
	return e.DomainErr
}

// Verify runs the commit existence, ancestry, branch and changed-path checks.
// All checks must pass for Verify to return nil.
func (v *CommitVerifier) Verify(ctx context.Context, cmd VerifyCommit) error {
	if err := validateVerifyCommit(cmd); err != nil {
		return err
	}

	// Guard against duplicate submissions before running expensive Git checks.
	// A submission for the same execution with the identical commit SHA is
	// treated as idempotent.
	existing, err := v.submissions.GetByExecutionID(ctx, cmd.TenantID, cmd.ExecutionID)
	if err != nil {
		return err
	}
	for _, sub := range existing {
		if sub.CommitSHA == cmd.CommitSHA {
			return nil
		}
	}

	// 1. Commit exists.
	driver, err := v.driver(ctx, cmd.TenantID)
	if err != nil {
		return err
	}
	if _, err := driver.GetCommit(ctx, cmd.Repo, cmd.CommitSHA); err != nil {
		if errors.Is(err, git.ErrRepoNotFound) {
			return &domain.Error{Code: "not_found", Message: "commit not found", Field: "commit_sha"}
		}
		return err
	}

	// 2. Base is an ancestor of head.
	isAncestor, err := driver.IsAncestor(ctx, cmd.Repo, cmd.BaseCommitSHA, cmd.CommitSHA)
	if err != nil {
		if errors.Is(err, git.ErrRepoNotFound) {
			return &domain.Error{Code: "invalid_argument", Message: "base commit not found", Field: "base_commit_sha"}
		}
		return err
	}
	if !isAncestor {
		return &domain.Error{Code: "invalid_argument", Message: "commit is not a descendant of base commit", Field: "base_commit_sha"}
	}

	// 3. Commit appears on the expected branch.
	if err := v.verifyBranch(ctx, cmd); err != nil {
		return err
	}

	// 4. Changed paths stay within the allowed set and avoid forbidden paths.
	files, err := driver.CompareCommits(ctx, cmd.Repo, cmd.BaseCommitSHA, cmd.CommitSHA)
	if err != nil {
		return err
	}

	var violations []PathViolation
	for _, f := range files {
		if !isPathAllowed(f.Filename, cmd.AllowedPaths, cmd.ForbiddenPaths) {
			violations = append(violations, PathViolation{
				Path:   f.Filename,
				Reason: "path is outside allowed set or matches a forbidden pattern",
			})
		}
	}
	if len(violations) > 0 {
		return newPathViolationError(violations)
	}

	return nil
}

func validateVerifyCommit(cmd VerifyCommit) error {
	if cmd.TenantID == "" {
		return invalid("tenant_id")
	}
	if cmd.ExecutionID == "" {
		return invalid("execution_id")
	}
	if cmd.Repo == "" {
		return invalid("repo")
	}
	if cmd.Branch == "" {
		return invalid("branch")
	}
	if cmd.CommitSHA == "" {
		return invalid("commit_sha")
	}
	if cmd.BaseCommitSHA == "" {
		return invalid("base_commit_sha")
	}
	return nil
}

// ChangedFiles returns the files changed between base and head.
func (v *CommitVerifier) ChangedFiles(ctx context.Context, tenantID, repo, base, head string) ([]git.ChangedFile, error) {
	driver, err := v.driver(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return driver.CompareCommits(ctx, repo, base, head)
}

// IsCommitReachable reports whether commitSHA is still an ancestor of the
// current branch head. A false result means the branch was force-pushed or the
// commit was removed.
func (v *CommitVerifier) IsCommitReachable(ctx context.Context, tenantID, repo, branch, commitSHA string) (bool, error) {
	driver, err := v.driver(ctx, tenantID)
	if err != nil {
		return false, err
	}
	branchHead, err := driver.GetCommit(ctx, repo, branch)
	if err != nil {
		return false, err
	}
	return driver.IsAncestor(ctx, repo, commitSHA, branchHead.SHA)
}

func (v *CommitVerifier) verifyBranch(ctx context.Context, cmd VerifyCommit) error {
	driver, err := v.driver(ctx, cmd.TenantID)
	if err != nil {
		return err
	}
	branchHead, err := driver.GetCommit(ctx, cmd.Repo, cmd.Branch)
	if err != nil {
		if errors.Is(err, git.ErrRepoNotFound) {
			return &domain.Error{Code: "invalid_argument", Message: "branch not found", Field: "branch"}
		}
		return err
	}

	onBranch, err := driver.IsAncestor(ctx, cmd.Repo, cmd.CommitSHA, branchHead.SHA)
	if err != nil {
		return err
	}
	if !onBranch {
		return &domain.Error{Code: "invalid_argument", Message: "commit is not on the specified branch", Field: "commit_sha"}
	}
	return nil
}

func isPathAllowed(filename string, allowed, forbidden []string) bool {
	for _, p := range forbidden {
		if matchPath(p, filename) {
			return false
		}
	}
	if len(allowed) == 0 {
		return true
	}
	for _, p := range allowed {
		if matchPath(p, filename) {
			return true
		}
	}
	return false
}

func matchPath(pattern, value string) bool {
	if pattern == value {
		return true
	}
	if matched, _ := path.Match(pattern, value); matched {
		return true
	}
	// Support trailing-wildcard directory patterns such as "src/*" matching
	// files directly under src/.
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		if prefix != "" && strings.HasPrefix(value, prefix+"/") {
			return !strings.Contains(strings.TrimPrefix(value, prefix+"/"), "/")
		}
	}
	return false
}
