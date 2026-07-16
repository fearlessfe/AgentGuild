package github

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
	"github.com/golang-jwt/jwt/v5"
)

const (
	defaultHTTPTimeout = 10 * time.Second
	appTokenTTL        = 10 * time.Minute
)

// Driver implements git.Driver using the GitHub REST API and GitHub App
// installation tokens.
type Driver struct {
	cfg    Config
	key    *rsa.PrivateKey
	client *http.Client
	now    func() time.Time
}

// Option customises a Driver.
type Option func(*Driver)

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(d *Driver) { d.client = client }
}

// WithClock overrides the default time source. Useful for deterministic tests.
func WithClock(now func() time.Time) Option {
	return func(d *Driver) { d.now = now }
}

// NewDriver creates a GitHub Driver from configuration.
func NewDriver(cfg Config, opts ...Option) (*Driver, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.github.com"
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	key, err := cfg.PrivateRSAKey()
	if err != nil {
		return nil, fmt.Errorf("parse github private key: %w", err)
	}

	base, _ := url.Parse(cfg.BaseURL)
	d := &Driver{
		cfg: cfg,
		key: key,
		client: &http.Client{Timeout: defaultHTTPTimeout, CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if req.URL.Scheme != "https" || !strings.EqualFold(req.URL.Host, base.Host) {
				return fmt.Errorf("github redirect changed origin")
			}
			return nil
		}},
		now: time.Now,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d, nil
}

// CreateCredential returns a repository-scoped GitHub App installation token.
// GitHub cannot constrain installation tokens to a branch; production callers
// keep this token server-side behind the branch-enforcing Git proxy.
func (d *Driver) CreateCredential(ctx context.Context, repo, branch, baseCommit string) (git.Credential, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return git.Credential{}, err
	}

	token, expiresAt, err := d.installationToken(ctx, name)
	if err != nil {
		return git.Credential{}, err
	}

	return git.Credential{
		Token:      token,
		RepoURL:    d.repoURL(owner, name),
		Branch:     branch,
		BaseCommit: baseCommit,
		ExpiresAt:  expiresAt,
	}, nil
}

// GetCommit fetches a commit by SHA.
func (d *Driver) GetCommit(ctx context.Context, repo, sha string) (git.Commit, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return git.Commit{}, err
	}

	token, _, err := d.installationToken(ctx)
	if err != nil {
		return git.Commit{}, err
	}

	url := d.apiURL("/repos/%s/%s/commits/%s", owner, name, url.PathEscape(sha))
	body, status, retryAfter, err := d.get(ctx, token, url)
	if err != nil {
		return git.Commit{}, err
	}
	if status != http.StatusOK {
		return git.Commit{}, mapError(status, body, retryAfter)
	}

	var payload commitPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return git.Commit{}, fmt.Errorf("decode commit: %w", err)
	}

	committedAt, _ := time.Parse(time.RFC3339, payload.Commit.Author.Date)
	return git.Commit{
		SHA:         payload.SHA,
		Message:     payload.Commit.Message,
		Author:      payload.Commit.Author.Name,
		Email:       payload.Commit.Author.Email,
		CommittedAt: committedAt,
		URL:         payload.HTMLURL,
	}, nil
}

// CompareCommits returns the list of changed files between base and head.
func (d *Driver) CompareCommits(ctx context.Context, repo, base, head string) ([]git.ChangedFile, error) {
	_, files, err := d.compare(ctx, repo, base, head)
	return files, err
}

// IsAncestor reports whether base is an ancestor of head. It returns true when
// base and head are identical or when the comparison is strictly ahead.
func (d *Driver) IsAncestor(ctx context.Context, repo, base, head string) (bool, error) {
	cmp, _, err := d.compare(ctx, repo, base, head)
	if err != nil {
		return false, err
	}
	if cmp.Status == "identical" {
		return true, nil
	}
	return cmp.Status == "ahead" && cmp.BehindBy == 0, nil
}

func (d *Driver) compare(ctx context.Context, repo, base, head string) (comparePayload, []git.ChangedFile, error) {
	var empty comparePayload
	owner, name, err := splitRepo(repo)
	if err != nil {
		return empty, nil, err
	}

	token, _, err := d.installationToken(ctx)
	if err != nil {
		return empty, nil, err
	}

	url := d.apiURL("/repos/%s/%s/compare/%s...%s", owner, name, url.PathEscape(base), url.PathEscape(head))
	body, status, retryAfter, err := d.get(ctx, token, url)
	if err != nil {
		return empty, nil, err
	}
	if status != http.StatusOK {
		return empty, nil, mapError(status, body, retryAfter)
	}

	var payload comparePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return empty, nil, fmt.Errorf("decode compare: %w", err)
	}

	files := make([]git.ChangedFile, 0, len(payload.Files))
	for _, f := range payload.Files {
		files = append(files, git.ChangedFile{
			Filename:  f.Filename,
			Status:    f.Status,
			Additions: f.Additions,
			Deletions: f.Deletions,
			Patch:     f.Patch,
		})
	}
	return payload, files, nil
}

func (d *Driver) installationToken(ctx context.Context, repositories ...string) (string, time.Time, error) {
	jwtToken, err := d.createJWT()
	if err != nil {
		return "", time.Time{}, err
	}

	url := d.apiURL("/app/installations/%d/access_tokens", d.cfg.InstallationID)
	var requestBody io.Reader
	if len(repositories) > 0 {
		body, err := json.Marshal(struct {
			Repositories []string          `json:"repositories"`
			Permissions  map[string]string `json:"permissions"`
		}{
			Repositories: repositories,
			Permissions:  map[string]string{"contents": "write"},
		})
		if err != nil {
			return "", time.Time{}, err
		}
		requestBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, requestBody)
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("request installation token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Time{}, err
	}

	if resp.StatusCode != http.StatusCreated {
		return "", time.Time{}, mapError(resp.StatusCode, body, resp.Header.Get("Retry-After"))
	}

	var tokenResp struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", time.Time{}, fmt.Errorf("decode installation token: %w", err)
	}

	expiresAt, _ := time.Parse(time.RFC3339, tokenResp.ExpiresAt)
	return tokenResp.Token, expiresAt, nil
}

func (d *Driver) createJWT() (string, error) {
	now := d.now()
	claims := jwt.MapClaims{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(appTokenTTL).Unix(),
		"iss": d.cfg.AppID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(d.key)
}

func (d *Driver) get(ctx context.Context, token, url string) ([]byte, int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, 0, "", fmt.Errorf("request github api: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, resp.Header.Get("Retry-After"), err
	}
	return body, resp.StatusCode, resp.Header.Get("Retry-After"), nil
}

func (d *Driver) apiURL(path string, args ...any) string {
	return strings.TrimRight(d.cfg.BaseURL, "/") + fmt.Sprintf(path, args...)
}

func (d *Driver) repoURL(owner, name string) string {
	base := strings.TrimRight(d.cfg.BaseURL, "/")
	if strings.HasSuffix(base, "/api/v3") {
		return strings.TrimSuffix(base, "/api/v3") + fmt.Sprintf("/%s/%s.git", owner, name)
	}
	if strings.Contains(base, "api.github.com") {
		return fmt.Sprintf("https://github.com/%s/%s.git", owner, name)
	}
	return base + fmt.Sprintf("/%s/%s.git", owner, name)
}

func splitRepo(repo string) (owner, name string, err error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", git.ErrInvalidRepo
	}
	return parts[0], parts[1], nil
}

func mapError(status int, body []byte, retryAfter string) error {
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(status)
	}
	switch status {
	case http.StatusNotFound:
		return git.ErrRepoNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return git.ErrUnauthorized
	case http.StatusTooManyRequests:
		return &domain.Error{Code: domain.ErrRateLimited.Code, Message: "GitHub rate limit exceeded", RetryAfter: parseRetryAfter(retryAfter, message)}
	case http.StatusUnprocessableEntity:
		return &domain.Error{Code: "invalid_argument", Message: message}
	}
	if status >= 500 {
		return &domain.Error{Code: "external_error", Message: fmt.Sprintf("GitHub API error %d: %s", status, message)}
	}
	return &domain.Error{Code: "external_error", Message: fmt.Sprintf("GitHub API error %d: %s", status, message)}
}

func parseRetryAfter(header, body string) time.Duration {
	if d := retryAfterSeconds(header); d > 0 {
		return d
	}
	return retryAfterSeconds(body)
}

func retryAfterSeconds(value string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 0
}

type commitPayload struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Date  string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
	HTMLURL string `json:"html_url"`
}

type filePayload struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch"`
}

type comparePayload struct {
	Status   string        `json:"status"`
	AheadBy  int           `json:"ahead_by"`
	BehindBy int           `json:"behind_by"`
	Files    []filePayload `json:"files"`
}
