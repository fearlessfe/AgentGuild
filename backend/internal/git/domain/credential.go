package domain

// CredentialStatus is the lifecycle state of a git credential record. It is
// stored separately from revoked_at so that a pending placeholder can be
// persisted before an external token is issued, closing the leak path where a
// token is created but never recorded.
type CredentialStatus string

const (
	// CredentialStatusPending is written before the external issuer is called.
	// The record exists to reserve the execution_id and prevent duplicate
	// issuance, but the metadata is not yet authoritative.
	CredentialStatusPending CredentialStatus = "pending"
	// CredentialStatusActive means the credential has been issued and its
	// metadata reflects the live external token.
	CredentialStatusActive CredentialStatus = "active"
	// CredentialStatusRevoked means the credential has been explicitly revoked
	// and must not be used or re-issued.
	CredentialStatusRevoked CredentialStatus = "revoked"
)
