package application

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

// IssueCredential requests a short-lived Git credential for an execution.
type IssueCredential struct {
	RequestID   string
	ExecutionID string
	Repo        string
	Branch      string
	BaseCommit  string
}

// RevokeCredential revokes a previously issued credential by execution ID.
type RevokeCredential struct {
	ExecutionID string
}

const restrictedBranchPrefix = "agentguild/"

const credentialFallbackTTL = 5 * time.Minute

// IssueCredential returns a branch-scoped proxy token and persisted metadata.
// A same-key retry derives the same token from an HMAC secret without storing
// plaintext; a different request cannot mint a second live credential.
func (s *CredentialService) IssueCredential(ctx context.Context, principal Principal, cmd IssueCredential) (Envelope[IssueCredentialResponse], error) {
	var result Envelope[IssueCredentialResponse]
	if err := requireAgentCaller(principal); err != nil {
		return result, err
	}
	if cmd.ExecutionID == "" {
		return result, invalid("execution_id")
	}
	if cmd.Repo == "" {
		return result, invalid("repo")
	}
	if cmd.BaseCommit == "" {
		return result, invalid("base_commit")
	}
	requestID := cmd.RequestID
	if requestID == "" {
		requestID = "legacy:" + cmd.ExecutionID
	}
	if s.proxyBaseURL == "" || len(s.tokenSecret) < 32 {
		return result, &domain.Error{Code: "state_conflict", Message: "git credential proxy is not configured"}
	}

	branch := restrictedBranchPrefix + cmd.ExecutionID
	if cmd.Branch != "" && cmd.Branch != branch {
		return result, git.ErrInvalidBranch
	}

	requestHash := sha256.Sum256([]byte(requestID))
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		if s.authorizer == nil {
			return domain.ErrForbidden
		}
		grant, err := s.authorizer.AuthorizeCredential(ctx, principal, cmd, now)
		if err != nil {
			return err
		}
		if grant.Repo == "" {
			return domain.ErrForbidden
		}
		resolved, err := s.resolver.Driver(ctx, principal.TenantID, grant.Repo)
		if err != nil {
			return err
		}
		grant.Repo = resolved.FullName
		if grant.BaseCommit == "" {
			baseResolver, ok := s.resolver.(RepositoryBaseResolver)
			if !ok {
				return domain.ErrForbidden
			}
			grant.BaseCommit, err = baseResolver.ResolveBaseCommit(ctx, principal.TenantID, grant.Repo)
			if err != nil {
				return err
			}
		}
		if grant.BaseCommit == "" {
			return domain.ErrForbidden
		}
		if !hasRepoScope(principal.RepoScope, grant.Repo) {
			return domain.ErrForbidden
		}
		if _, err := resolved.Driver.GetCommit(ctx, resolved.FullName, grant.BaseCommit); err != nil {
			return err
		}
		requestHash = sha256.Sum256([]byte(requestID + "\x00" + grant.Repo + "\x00" + grant.BaseCommit + "\x00" + branch))

		existing, err := tx.Credentials().GetByExecutionIDForUpdate(ctx, principal.TenantID, cmd.ExecutionID)
		if err != nil {
			if errors.Is(err, git.ErrCredentialNotFound) {
				existing = nil
			} else {
				return err
			}
		}

		if existing != nil && existing.RevokedAt == nil && now.Before(existing.ExpiresAt) {
			if hmac.Equal(existing.RequestHash, requestHash[:]) {
				token := s.credentialToken(principal.TenantID, existing.ID, requestHash[:])
				result = Envelope[IssueCredentialResponse]{
					Data: IssueCredentialResponse{Credential: credentialView(existing, now), Token: token},
					Meta: Meta{ServerTime: now},
				}
				return nil
			}
			return git.ErrAlreadyIssued
		}
		if existing != nil && existing.RevokedAt == nil {
			if err := tx.Credentials().Revoke(ctx, principal.TenantID, cmd.ExecutionID); err != nil {
				return err
			}
			existing.RevokedAt = &now
		}

		record := existing
		if record != nil && hmac.Equal(record.RequestHash, requestHash[:]) {
			return git.ErrAlreadyIssued
		}
		expiresAt := grant.ExpiresAt
		if expiresAt.IsZero() || expiresAt.After(now.Add(credentialFallbackTTL)) {
			expiresAt = now.Add(credentialFallbackTTL)
		}
		if record != nil {
			record.Provider = s.provider
			record.Repo = grant.Repo
			record.RepoURL = s.proxyURL(principal.TenantID, record.ID, grant.Repo)
			record.Branch = branch
			record.BaseCommit = grant.BaseCommit
			record.ExpiresAt = expiresAt
			record.RevokedAt = nil
			record.Status = gitdomain.CredentialStatusActive
			record.RequestHash = append([]byte(nil), requestHash[:]...)
			token := s.credentialToken(principal.TenantID, record.ID, requestHash[:])
			tokenHash := sha256.Sum256([]byte(token))
			record.TokenHash = append([]byte(nil), tokenHash[:]...)
			if err := tx.Credentials().Reactivate(ctx, record); err != nil {
				return err
			}
		} else {
			id := s.newID()
			token := s.credentialToken(principal.TenantID, id, requestHash[:])
			tokenHash := sha256.Sum256([]byte(token))
			record = &CredentialRecord{
				ID:          id,
				TenantID:    principal.TenantID,
				ExecutionID: cmd.ExecutionID,
				Provider:    s.provider,
				Repo:        grant.Repo,
				RepoURL:     s.proxyURL(principal.TenantID, id, grant.Repo),
				Branch:      branch,
				BaseCommit:  grant.BaseCommit,
				ExpiresAt:   expiresAt,
				Status:      gitdomain.CredentialStatusActive,
				RequestHash: append([]byte(nil), requestHash[:]...),
				TokenHash:   append([]byte(nil), tokenHash[:]...),
				CreatedAt:   now,
			}
			if err := tx.Credentials().Insert(ctx, record); err != nil {
				if isUniqueViolation(err) {
					return git.ErrAlreadyIssued
				}
				return err
			}
		}

		// No external write token is minted during this transaction.
		token := s.credentialToken(principal.TenantID, record.ID, requestHash[:])

		result = Envelope[IssueCredentialResponse]{
			Data: IssueCredentialResponse{
				Credential: credentialView(record, now),
				Token:      token,
			},
			Meta: Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

func (s *CredentialService) credentialToken(tenantID, credentialID string, requestHash []byte) string {
	mac := hmac.New(sha256.New, s.tokenSecret)
	_, _ = mac.Write([]byte(tenantID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(credentialID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(requestHash)
	return "agc_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *CredentialService) proxyURL(tenantID, credentialID, repo string) string {
	owner, name, _ := strings.Cut(repo, "/")
	return fmt.Sprintf("%s/git/%s/%s/%s/%s.git", s.proxyBaseURL,
		url.PathEscape(tenantID), url.PathEscape(credentialID),
		url.PathEscape(owner), url.PathEscape(name))
}

func requireAgentCaller(principal Principal) error {
	if principal.TenantID == "" || principal.AgentID == "" || principal.AgentVersionID == "" {
		return domain.ErrForbidden
	}
	for _, scope := range principal.Scopes {
		if scope == "tasks:execute" {
			return nil
		}
	}
	return domain.ErrForbidden
}

func hasRepoScope(patterns []string, repo string) bool {
	for _, pattern := range patterns {
		if pattern == "*" || pattern == repo {
			return true
		}
		prefix, ok := strings.CutSuffix(pattern, "/*")
		if ok && strings.HasPrefix(repo, prefix+"/") {
			return true
		}
	}
	return false
}

// RevokeCredential marks the credential for the execution as revoked.
func (s *CredentialService) RevokeCredential(ctx context.Context, principal Principal, cmd RevokeCredential) (Envelope[CredentialView], error) {
	var result Envelope[CredentialView]
	if principal.TenantID == "" {
		return result, domain.ErrForbidden
	}
	if !principal.IsAdmin && principal.AgentID == "" {
		return result, domain.ErrForbidden
	}
	if cmd.ExecutionID == "" {
		return result, invalid("execution_id")
	}

	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		record, err := tx.Credentials().GetByExecutionID(ctx, principal.TenantID, cmd.ExecutionID)
		if err != nil {
			return err
		}
		if err := s.authorizeCredentialRecord(ctx, principal, record, now); err != nil {
			return err
		}
		if record.RevokedAt != nil {
			result = Envelope[CredentialView]{
				Data: credentialView(record, now),
				Meta: Meta{ServerTime: now},
			}
			return nil
		}
		if err := tx.Credentials().Revoke(ctx, principal.TenantID, cmd.ExecutionID); err != nil {
			// If another caller revoked between our Get and Revoke, the
			// repository reports the credential as missing. Re-fetch so we can
			// return the now-revoked record idempotently.
			if errors.Is(err, git.ErrCredentialNotFound) {
				record, err = tx.Credentials().GetByExecutionID(ctx, principal.TenantID, cmd.ExecutionID)
				if err != nil {
					return err
				}
				if record.RevokedAt == nil {
					return git.ErrCredentialNotFound
				}
				result = Envelope[CredentialView]{
					Data: credentialView(record, now),
					Meta: Meta{ServerTime: now},
				}
				return nil
			}
			return err
		}
		record.RevokedAt = &now
		record.Status = gitdomain.CredentialStatusRevoked
		result = Envelope[CredentialView]{
			Data: credentialView(record, now),
			Meta: Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

func (s *CredentialService) authorizeCredentialRecord(ctx context.Context, principal Principal, record *CredentialRecord, now time.Time) error {
	if principal.IsAdmin {
		return nil
	}
	if err := requireAgentCaller(principal); err != nil {
		return err
	}
	if s.authorizer == nil {
		return domain.ErrForbidden
	}
	_, err := s.authorizer.AuthorizeCredential(ctx, principal, IssueCredential{
		ExecutionID: record.ExecutionID, Repo: record.Repo, BaseCommit: record.BaseCommit,
	}, now)
	return err
}

func requireCaller(principal Principal) error {
	if principal.TenantID == "" && !principal.IsGlobalAgent() {
		return domain.ErrForbidden
	}
	if principal.IsAdmin || principal.OwnerID != "" {
		return nil
	}
	if principal.AgentID != "" {
		for _, scope := range principal.Scopes {
			if scope == "tasks:execute" {
				return nil
			}
		}
	}
	return domain.ErrForbidden
}

func credentialView(record *CredentialRecord, now time.Time) CredentialView {
	return CredentialView{
		ID:          record.ID,
		TenantID:    record.TenantID,
		ExecutionID: record.ExecutionID,
		Provider:    record.Provider,
		RepoURL:     record.RepoURL,
		Branch:      record.Branch,
		BaseCommit:  record.BaseCommit,
		ExpiresAt:   record.ExpiresAt,
		RevokedAt:   record.RevokedAt,
		Status:      record.Status,
		CreatedAt:   record.CreatedAt,
	}
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func invalid(field string) error {
	return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
