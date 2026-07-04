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
	if err := s.requireCaller(principal); err != nil {
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

	credential, err := s.issuer.Issue(ctx, principal.TenantID, cmd.ExecutionID, cmd.Repo, branch, cmd.BaseCommit)
	if err != nil {
		return result, err
	}

	err = s.store.WithTx(ctx, func(tx Tx) error {
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

		if existing != nil {
			if existing.RevokedAt != nil {
				return git.ErrCredentialRevoked
			}
			existing.Provider = s.provider
			existing.RepoURL = credential.RepoURL
			existing.Branch = branch
			existing.BaseCommit = credential.BaseCommit
			existing.ExpiresAt = credential.ExpiresAt
			if err := tx.Credentials().Update(ctx, existing); err != nil {
				return err
			}
			result = Envelope[IssueCredentialResponse]{
				Data: IssueCredentialResponse{
					Credential: credentialView(existing, now),
					Token:      credential.Token,
				},
				Meta: Meta{ServerTime: now},
			}
			return nil
		}

		record := &CredentialRecord{
			ID:          s.newID(),
			TenantID:    principal.TenantID,
			ExecutionID: cmd.ExecutionID,
			Provider:    s.provider,
			RepoURL:     credential.RepoURL,
			Branch:      branch,
			BaseCommit:  credential.BaseCommit,
			ExpiresAt:   credential.ExpiresAt,
			CreatedAt:   now,
		}
		if err := tx.Credentials().Insert(ctx, record); err != nil {
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
			return git.ErrCredentialRevoked
		}
		if err := tx.Credentials().Revoke(ctx, principal.TenantID, cmd.ExecutionID); err != nil {
			return err
		}
		record.RevokedAt = &now
		result = Envelope[CredentialView]{
			Data: credentialView(record, now),
			Meta: Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

func (s *CredentialService) requireCaller(principal Principal) error {
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
