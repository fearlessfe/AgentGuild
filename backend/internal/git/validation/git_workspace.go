package validation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"agentguild.dev/agentguild/backend/internal/git"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
)

var immutableCommitSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}(?:[0-9a-fA-F]{24})?$`)

// RepositoryResolver resolves the tenant-scoped Git driver used to fetch a
// validation workspace.
type RepositoryResolver interface {
	Driver(ctx context.Context, tenantID, fullName string) (git.ResolvedDriver, error)
}

// GitWorkspaceFactory fetches exactly the submitted commit into an isolated
// detached workspace. Credentials are short-lived and never embedded in the
// repository URL or command arguments.
type GitWorkspaceFactory struct {
	resolver     RepositoryResolver
	allowedHosts map[string]struct{}
}

func NewGitWorkspaceFactory(resolver RepositoryResolver, allowedHosts ...string) (*GitWorkspaceFactory, error) {
	if resolver == nil {
		return nil, errors.New("repository resolver is required")
	}
	hosts := map[string]struct{}{"github.com": {}}
	for _, host := range allowedHosts {
		if parsed, err := url.Parse(host); err == nil && parsed.Hostname() != "" {
			host = parsed.Hostname()
		}
		hosts[strings.ToLower(strings.TrimSpace(host))] = struct{}{}
	}
	return &GitWorkspaceFactory{resolver: resolver, allowedHosts: hosts}, nil
}

func (f *GitWorkspaceFactory) Prepare(ctx context.Context, job *gitdomain.ValidationJob) (string, func(), error) {
	if job == nil {
		return "", nil, errors.New("validation job is required")
	}
	resolved, err := f.resolver.Driver(ctx, job.TenantID, job.Repo)
	if err != nil {
		return "", nil, fmt.Errorf("resolve repository: %w", err)
	}
	commit, err := resolved.Driver.GetCommit(ctx, resolved.FullName, job.CommitSHA)
	if err != nil {
		return "", nil, fmt.Errorf("resolve submission commit: %w", err)
	}
	if !immutableCommitSHA.MatchString(commit.SHA) {
		return "", nil, errors.New("git driver did not resolve commit_sha to a full immutable object id")
	}
	canonicalSHA := strings.ToLower(commit.SHA)
	credential, err := resolved.Driver.CreateCredential(ctx, resolved.FullName, job.Branch, canonicalSHA)
	if err != nil {
		return "", nil, fmt.Errorf("create fetch credential: %w", err)
	}
	if err := f.validateCloneURL(credential.RepoURL); err != nil {
		return "", nil, err
	}

	dir, err := os.MkdirTemp("", "agentguild-validation-")
	if err != nil {
		return "", nil, fmt.Errorf("create workspace: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	askpass := filepath.Join(dir, ".git-askpass.sh")
	const askpassScript = "#!/bin/sh\ncase \"$1\" in\n  *Username*) printf '%s\\n' \"$AGENTGUILD_GIT_USERNAME\" ;;\n  *) printf '%s\\n' \"$AGENTGUILD_GIT_TOKEN\" ;;\nesac\n"
	if err := os.WriteFile(askpass, []byte(askpassScript), 0o700); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("create git credential helper: %w", err)
	}
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		pathEnv = "/usr/local/bin:/usr/bin:/bin"
	}
	env := []string{
		"PATH=" + pathEnv,
		"HOME=" + dir,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=protocol.file.allow",
		"GIT_CONFIG_VALUE_0=never",
		"GIT_CONFIG_KEY_1=http.followRedirects",
		"GIT_CONFIG_VALUE_1=false",
		"GIT_ASKPASS=" + askpass,
		"GIT_TERMINAL_PROMPT=0",
		"AGENTGUILD_GIT_USERNAME=x-access-token",
		"AGENTGUILD_GIT_TOKEN=" + credential.Token,
	}
	commands := [][]string{
		{"init", "--quiet"},
		{"remote", "add", "origin", credential.RepoURL},
		{"fetch", "--quiet", "--depth=1", "origin", canonicalSHA},
		{"checkout", "--quiet", "--detach", "FETCH_HEAD"},
	}
	for _, args := range commands {
		if _, err := executeGit(ctx, dir, env, args...); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	head, err := executeGit(ctx, dir, env, "rev-parse", "HEAD")
	if err != nil {
		cleanup()
		return "", nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(string(head)), canonicalSHA) {
		cleanup()
		return "", nil, errors.New("fetched HEAD does not match submission commit_sha")
	}
	if err := os.Remove(askpass); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("remove git credential helper: %w", err)
	}
	return dir, cleanup, nil
}

func (f *GitWorkspaceFactory) CheckIntegrity(ctx context.Context, job *gitdomain.ValidationJob) (bool, error) {
	resolved, err := f.resolver.Driver(ctx, job.TenantID, job.Repo)
	if err != nil {
		return false, err
	}
	branchHead, err := resolved.Driver.GetCommit(ctx, resolved.FullName, job.Branch)
	if err != nil {
		return false, err
	}
	return resolved.Driver.IsAncestor(ctx, resolved.FullName, job.CommitSHA, branchHead.SHA)
}

func (f *GitWorkspaceFactory) validateCloneURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return errors.New("git driver returned an invalid repository URL")
	}
	if u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("git driver returned an invalid repository URL")
	}
	if u.User != nil {
		return errors.New("repository URL must not contain credentials")
	}
	host := strings.ToLower(u.Hostname())
	if _, allowed := f.allowedHosts[host]; !allowed {
		return errors.New("repository URL host is not explicitly allowed")
	}
	return nil
}

func executeGit(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return output.Bytes(), fmt.Errorf("git %s failed: %w: %s", args[0], err, strings.TrimSpace(output.String()))
	}
	return output.Bytes(), nil
}
