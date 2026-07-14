package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

// IssueCredential requests a short-lived Git credential for an execution.
type IssueCredential struct {
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

// IssueCredential returns a plaintext token and the persisted credential
// metadata. Re-issuing for the same execution updates the stored metadata and
// returns a fresh token unless the credential has been revoked.
func (s *CredentialService) IssueCredential(ctx context.Context, principal Principal, cmd IssueCredential) (Envelope[IssueCredentialResponse], error) {
	var result Envelope[IssueCredentialResponse]
	if err := requireCaller(principal); err != nil {
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

	branch := cmd.Branch
	if branch == "" {
		branch = restrictedBranchPrefix + cmd.ExecutionID
	}
	if !strings.HasPrefix(branch, restrictedBranchPrefix) {
		return result, git.ErrInvalidBranch
	}

	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}

		existing, err := tx.Credentials().GetByExecutionID(ctx, principal.TenantID, cmd.ExecutionID)
		if err != nil {
			if errors.Is(err, git.ErrCredentialNotFound) {
				existing = nil
			} else {
				return err
			}
		}

		if existing != nil && existing.RevokedAt != nil {
			return git.ErrCredentialRevoked
		}

		record := existing
		if record != nil {
			// Mark the existing record as pending before calling the external
			// issuer. If any later step fails, the transaction rolls back and
			// the previous active metadata is restored.
			record.Status = gitdomain.CredentialStatusPending
			if err := tx.Credentials().Update(ctx, record); err != nil {
				return err
			}
		} else {
			record = &CredentialRecord{
				ID:          s.newID(),
				TenantID:    principal.TenantID,
				ExecutionID: cmd.ExecutionID,
				Provider:    s.provider,
				RepoURL:     "pending",
				Branch:      branch,
				BaseCommit:  cmd.BaseCommit,
				ExpiresAt:   now,
				Status:      gitdomain.CredentialStatusPending,
				CreatedAt:   now,
			}
			if err := tx.Credentials().Insert(ctx, record); err != nil {
				if isUniqueViolation(err) {
					return git.ErrAlreadyIssued
				}
				return err
			}
		}

		// Only call the external issuer after a placeholder record has been
		// persisted. This prevents a live token from being created without any
		// corresponding metadata row.
		resolved, err := s.resolver.Driver(ctx, principal.TenantID, cmd.Repo)
		if err != nil {
			return err
		}
		issuer := git.NewIssuer(resolved.Driver)
		credential, err := issuer.Issue(ctx, principal.TenantID, cmd.ExecutionID, resolved.FullName, branch, cmd.BaseCommit)
		if err != nil {
			return err
		}

		record.Provider = s.provider
		record.RepoURL = credential.RepoURL
		record.Branch = branch
		record.BaseCommit = credential.BaseCommit
		record.ExpiresAt = credential.ExpiresAt
		record.Status = gitdomain.CredentialStatusActive
		if err := tx.Credentials().Update(ctx, record); err != nil {
			return err
		}

		result = Envelope[IssueCredentialResponse]{
			Data: IssueCredentialResponse{
				Credential: credentialView(record, now),
				Token:      credential.Token,
			},
			Meta: Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

// RevokeCredential marks the credential for the execution as revoked.
func (s *CredentialService) RevokeCredential(ctx context.Context, principal Principal, cmd RevokeCredential) (Envelope[CredentialView], error) {
	var result Envelope[CredentialView]
	if principal.TenantID == "" {
		return result, domain.ErrForbidden
	}
	if !principal.IsAdmin && principal.OwnerID == "" {
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

func requireCaller(principal Principal) error {
	if principal.TenantID == "" {
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
