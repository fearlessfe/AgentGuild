// Package source provides a read-only, pinned repository snapshot for task
// analysis. It never executes repository-provided code.
package source

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

	publictaskanalysis "agentguild.dev/agentguild/backend/internal/publictask/analysis"
)

var (
	repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	commitPattern     = regexp.MustCompile(`^[0-9a-fA-F]{40}(?:[0-9a-fA-F]{24})?$`)
)

// GitCloner fetches one immutable public GitHub commit and returns a bounded
// text snapshot. The clone directory is deleted before this method returns.
type GitCloner struct {
	AllowedHosts map[string]struct{}
	MaxFiles     int
	MaxBytes     int
	MaxFileBytes int
}

func NewGitCloner(allowedHosts []string, maxFiles, maxBytes int) *GitCloner {
	hosts := map[string]struct{}{"github.com": {}}
	for _, host := range allowedHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if parsed, err := url.Parse(host); err == nil && parsed.Hostname() != "" {
			host = strings.ToLower(parsed.Hostname())
		}
		if host != "" {
			hosts[host] = struct{}{}
		}
	}
	if maxFiles <= 0 {
		maxFiles = 120
	}
	if maxBytes <= 0 {
		maxBytes = 512 * 1024
	}
	return &GitCloner{AllowedHosts: hosts, MaxFiles: maxFiles, MaxBytes: maxBytes, MaxFileBytes: 64 * 1024}
}

func (c *GitCloner) Snapshot(ctx context.Context, repo, commit string) ([]publictaskanalysis.SourceFile, error) {
	if !repositoryPattern.MatchString(repo) {
		return nil, errors.New("public repository must be owner/name")
	}
	if !commitPattern.MatchString(commit) {
		return nil, errors.New("analysis commit must be immutable")
	}
	if c == nil {
		c = NewGitCloner(nil, 0, 0)
	}
	if _, ok := c.AllowedHosts["github.com"]; !ok {
		return nil, errors.New("public analysis clone requires github.com allowlist")
	}

	dir, err := os.MkdirTemp("", "agentguild-analysis-")
	if err != nil {
		return nil, fmt.Errorf("create analysis workspace: %w", err)
	}
	defer os.RemoveAll(dir)
	env := safeGitEnv(dir)
	remote := "https://github.com/" + repo + ".git"
	commands := [][]string{
		{"init", "--quiet"},
		{"remote", "add", "origin", remote},
		{"fetch", "--quiet", "--depth=1", "origin", strings.ToLower(commit)},
		{"checkout", "--quiet", "--detach", "FETCH_HEAD"},
	}
	for _, args := range commands {
		if _, err := runGit(ctx, dir, env, args...); err != nil {
			return nil, err
		}
	}
	head, err := runGit(ctx, dir, env, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(string(head)), strings.ToLower(commit)) {
		return nil, errors.New("analysis workspace HEAD does not match base commit")
	}
	return readSnapshot(dir, c.MaxFiles, c.MaxBytes, c.MaxFileBytes)
}

func safeGitEnv(dir string) []string {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		pathEnv = "/usr/local/bin:/usr/bin:/bin"
	}
	return []string{
		"PATH=" + pathEnv,
		"HOME=" + dir,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_COUNT=3",
		"GIT_CONFIG_KEY_0=protocol.file.allow",
		"GIT_CONFIG_VALUE_0=never",
		"GIT_CONFIG_KEY_1=http.followRedirects",
		"GIT_CONFIG_VALUE_1=false",
		"GIT_CONFIG_KEY_2=core.hooksPath",
		"GIT_CONFIG_VALUE_2=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
	}
}

func runGit(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s failed: %w", args[0], err)
	}
	return output.Bytes(), nil
}

func readSnapshot(root string, maxFiles, maxBytes, maxFileBytes int) ([]publictaskanalysis.SourceFile, error) {
	files := make([]publictaskanalysis.SourceFile, 0, maxFiles)
	totalBytes := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if ignoredDirectory(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= maxFiles || totalBytes >= maxBytes {
			return filepath.SkipAll
		}
		if entry.Type()&os.ModeSymlink != 0 || ignoredFile(rel) || !textExtension(rel) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if len(body) > maxFileBytes {
			body = body[:maxFileBytes]
		}
		if bytes.IndexByte(body, 0) >= 0 {
			return nil
		}
		remaining := maxBytes - totalBytes
		if remaining <= 0 {
			return filepath.SkipAll
		}
		if len(body) > remaining {
			body = body[:remaining]
		}
		files = append(files, publictaskanalysis.SourceFile{Path: filepath.ToSlash(rel), Content: string(body)})
		totalBytes += len(body)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read analysis snapshot: %w", err)
	}
	return files, nil
}

func ignoredDirectory(path string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range parts {
		switch strings.ToLower(part) {
		case ".git", "node_modules", "vendor", "dist", "build", "target", "coverage", ".next":
			return true
		}
	}
	return false
}

func ignoredFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if strings.HasPrefix(base, ".env") || strings.Contains(base, "credential") || strings.Contains(base, "secret") || strings.Contains(base, "private-key") {
		return true
	}
	return false
}

func textExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		base := strings.ToLower(filepath.Base(path))
		return base == "dockerfile" || base == "makefile" || base == "go.mod" || base == "go.sum"
	}
	switch ext {
	case ".go", ".mod", ".sum", ".ts", ".tsx", ".js", ".jsx", ".json", ".yaml", ".yml", ".toml", ".md", ".html", ".css", ".scss", ".sql", ".rs", ".py", ".java", ".kt", ".swift", ".c", ".h", ".cpp", ".hpp", ".sh":
		return true
	default:
		return false
	}
}
