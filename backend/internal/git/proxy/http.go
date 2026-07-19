// Package proxy implements the branch-enforcing Git smart HTTP gateway.
package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
)

const maxCommandSection = 1 << 20

// Handler authenticates an execution credential, enforces its one writable
// ref, and forwards Git smart HTTP requests using a server-side installation
// token. Installation tokens are never returned to the Agent.
type Handler struct {
	store    gitapp.Store
	resolver gitapp.RepositoryGitResolver
	client   *http.Client
}

func NewHandler(store gitapp.Store, resolver gitapp.RepositoryGitResolver) (*Handler, error) {
	if store == nil || resolver == nil {
		return nil, errors.New("git proxy store and resolver are required")
	}
	return &Handler{
		store:    store,
		resolver: resolver,
		client: &http.Client{
			Timeout: 30 * time.Minute,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("git upstream redirects are disabled")
			},
		},
	}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tenantID, credentialID, repo, servicePath, ok := parseProxyPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	username, token, ok := r.BasicAuth()
	if !ok || username != "x-access-token" || token == "" {
		// 不记录 token 本体，只记录定位所需的非敏感字段。
		slog.WarnContext(r.Context(), "git proxy rejected request",
			"tenant", tenantID, "credential", credentialID, "repo", repo,
			"reason", "missing or malformed basic auth")
		w.Header().Set("WWW-Authenticate", `Basic realm="AgentGuild Git"`)
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	record, err := h.authorize(r.Context(), tenantID, credentialID, repo, token)
	if err != nil {
		attrs := []any{"tenant", tenantID, "credential", credentialID, "repo", repo, "reason", err.Error()}
		if record != nil {
			attrs = append(attrs, "execution", record.ExecutionID, "branch", record.Branch)
		}
		slog.WarnContext(r.Context(), "git proxy rejected credential", attrs...)
		http.Error(w, "credential is invalid or expired", http.StatusUnauthorized)
		return
	}
	if !allowedGitRequest(r.Method, servicePath, r.URL.Query().Get("service")) {
		slog.WarnContext(r.Context(), "git proxy rejected request",
			"tenant", tenantID, "credential", credentialID, "execution", record.ExecutionID,
			"repo", repo, "method", r.Method, "path", servicePath, "service", r.URL.Query().Get("service"),
			"reason", "git operation is not allowed")
		http.Error(w, "git operation is not allowed", http.StatusForbidden)
		return
	}

	body := r.Body
	if r.Method == http.MethodPost && servicePath == "/git-receive-pack" {
		body, err = enforceReceivePackRef(r.Body, "refs/heads/"+record.Branch)
		if err != nil {
			slog.WarnContext(r.Context(), "git proxy rejected push",
				"tenant", tenantID, "credential", credentialID, "execution", record.ExecutionID,
				"repo", repo, "allowed_ref", "refs/heads/"+record.Branch,
				"reason", err.Error())
			http.Error(w, "push updates a ref outside the execution branch", http.StatusForbidden)
			return
		}
	}

	resolved, err := h.resolver.Driver(r.Context(), tenantID, record.Repo)
	if err != nil {
		http.Error(w, "repository is unavailable", http.StatusBadGateway)
		return
	}
	upstreamCredential, err := resolved.Driver.CreateCredential(r.Context(), resolved.FullName, record.Branch, record.BaseCommit)
	if err != nil {
		http.Error(w, "repository credential is unavailable", http.StatusBadGateway)
		return
	}
	upstreamURL, err := joinUpstreamURL(upstreamCredential.RepoURL, servicePath, r.URL.RawQuery)
	if err != nil {
		http.Error(w, "repository URL is invalid", http.StatusBadGateway)
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, body)
	if err != nil {
		http.Error(w, "git request is invalid", http.StatusBadRequest)
		return
	}
	request.SetBasicAuth("x-access-token", upstreamCredential.Token)
	copyRequestHeader(request.Header, r.Header, "Content-Type")
	copyRequestHeader(request.Header, r.Header, "Accept")
	copyRequestHeader(request.Header, r.Header, "Git-Protocol")
	request.Header.Set("User-Agent", "AgentGuild-Git-Proxy/1.0")

	response, err := h.client.Do(request)
	if err != nil {
		http.Error(w, "git upstream request failed", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	copyResponseHeader(w.Header(), response.Header, "Content-Type")
	copyResponseHeader(w.Header(), response.Header, "Cache-Control")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

// authorize 校验凭证。校验失败时返回加载到的凭证（如有）与具体原因，
// 便于拒绝路径输出结构化审计日志；对客户端始终返回统一的 401 文案。
func (h *Handler) authorize(ctx context.Context, tenantID, credentialID, repo, token string) (*gitapp.CredentialRecord, error) {
	var record *gitapp.CredentialRecord
	err := h.store.WithTx(ctx, func(tx gitapp.Tx) error {
		var err error
		record, err = tx.Credentials().GetByID(ctx, tenantID, credentialID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("credential lookup failed: %w", err)
	}
	if record == nil {
		return nil, errors.New("credential not found")
	}
	if record.Repo != repo {
		return record, errors.New("credential repo mismatch")
	}
	if record.Status != gitdomain.CredentialStatusActive || record.RevokedAt != nil {
		return record, errors.New("credential is inactive")
	}
	if !time.Now().Before(record.ExpiresAt) {
		return record, errors.New("credential is expired")
	}
	hash := sha256.Sum256([]byte(token))
	if len(record.TokenHash) != len(hash) || subtle.ConstantTimeCompare(record.TokenHash, hash[:]) != 1 {
		return record, errors.New("credential token mismatch")
	}
	return record, nil
}

func parseProxyPath(path string) (tenantID, credentialID, repo, servicePath string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/git/"), "/")
	if len(parts) < 5 || !strings.HasSuffix(parts[3], ".git") {
		return "", "", "", "", false
	}
	name := strings.TrimSuffix(parts[3], ".git")
	if parts[0] == "" || parts[1] == "" || parts[2] == "" || name == "" {
		return "", "", "", "", false
	}
	return parts[0], parts[1], parts[2] + "/" + name, "/" + strings.Join(parts[4:], "/"), true
}

func allowedGitRequest(method, path, service string) bool {
	if method == http.MethodGet && path == "/info/refs" {
		return service == "git-upload-pack" || service == "git-receive-pack"
	}
	return method == http.MethodPost && (path == "/git-upload-pack" || path == "/git-receive-pack")
}

func enforceReceivePackRef(body io.Reader, allowedRef string) (io.ReadCloser, error) {
	var prefix bytes.Buffer
	updates := 0
	for prefix.Len() <= maxCommandSection {
		header := make([]byte, 4)
		if _, err := io.ReadFull(body, header); err != nil {
			return nil, err
		}
		prefix.Write(header)
		if bytes.Equal(header, []byte("0000")) {
			if updates == 0 {
				return nil, errors.New("receive-pack contains no ref updates")
			}
			return io.NopCloser(io.MultiReader(&prefix, body)), nil
		}
		lengthBytes, err := hex.DecodeString(string(header))
		if err != nil || len(lengthBytes) != 2 {
			return nil, errors.New("invalid pkt-line length")
		}
		length := int(lengthBytes[0])<<8 | int(lengthBytes[1])
		if length < 4 || length > 65520 || prefix.Len()+length-4 > maxCommandSection {
			return nil, errors.New("invalid receive-pack command section")
		}
		payload := make([]byte, length-4)
		if _, err := io.ReadFull(body, payload); err != nil {
			return nil, err
		}
		prefix.Write(payload)
		line := strings.TrimSuffix(string(payload), "\n")
		if nul := strings.IndexByte(line, 0); nul >= 0 {
			line = line[:nul]
		}
		fields := strings.Fields(line)
		if len(fields) != 3 || len(fields[0]) != 40 || len(fields[1]) != 40 || fields[2] != allowedRef {
			return nil, errors.New("receive-pack ref is not allowed")
		}
		updates++
	}
	return nil, errors.New("receive-pack command section is too large")
}

func joinUpstreamURL(repoURL, servicePath, rawQuery string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", errors.New("invalid upstream repository URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + servicePath
	u.RawQuery = rawQuery
	return u.String(), nil
}

func copyRequestHeader(dst, src http.Header, name string) {
	if value := src.Get(name); value != "" {
		dst.Set(name, value)
	}
}

func copyResponseHeader(dst, src http.Header, name string) {
	if value := src.Get(name); value != "" {
		dst.Set(name, value)
	}
}

var _ http.Handler = (*Handler)(nil)
