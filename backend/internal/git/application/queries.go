package application

import (
	"context"
)

// GetCredential retrieves the metadata for a credential by execution ID.
type GetCredential struct {
	ExecutionID string
}

// GetCredential returns the credential metadata if the caller is authorized.
func (s *CredentialService) GetCredential(ctx context.Context, principal Principal, query GetCredential) (Envelope[CredentialView], error) {
	var result Envelope[CredentialView]
	if err := requireCaller(principal); err != nil {
		return result, err
	}
	if query.ExecutionID == "" {
		return result, invalid("execution_id")
	}

	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		record, err := tx.Credentials().GetByExecutionID(ctx, principal.TenantID, query.ExecutionID)
		if err != nil {
			return err
		}
		if err := s.authorizeCredentialRecord(ctx, principal, record, now); err != nil {
			return err
		}
		result = Envelope[CredentialView]{
			Data: credentialView(record, now),
			Meta: Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}
