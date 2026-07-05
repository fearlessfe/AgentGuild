package git

import "context"

// CredentialIssuer creates task-scoped Git credentials. The first
// implementation delegates directly to a Driver.
type CredentialIssuer interface {
	Issue(ctx context.Context, tenantID, executionID, repo, branch, baseCommit string) (Credential, error)
}

// NewIssuer returns a CredentialIssuer backed by driver. tenantID and
// executionID are accepted for future multi-tenant routing but are currently
// unused.
func NewIssuer(driver Driver) CredentialIssuer {
	return &issuer{driver: driver}
}

type issuer struct {
	driver Driver
}

func (i *issuer) Issue(ctx context.Context, tenantID, executionID, repo, branch, baseCommit string) (Credential, error) {
	return i.driver.CreateCredential(ctx, repo, branch, baseCommit)
}
