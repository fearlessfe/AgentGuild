package application

import (
	"context"
	"time"
)

// Store begins a transaction against the credential repository.
type Store interface {
	WithTx(context.Context, func(Tx) error) error
}

// Tx is the set of operations available inside one credential transaction.
type Tx interface {
	Credentials() CredentialRepository
	Now(context.Context) (time.Time, error)
}

// CredentialRepository persists credential metadata without the token
// plaintext.
type CredentialRepository interface {
	Insert(context.Context, *CredentialRecord) error
	GetByID(context.Context, string, string) (*CredentialRecord, error)
	GetByExecutionID(context.Context, string, string) (*CredentialRecord, error)
	Update(context.Context, *CredentialRecord) error
	Revoke(context.Context, string, string) error
}
