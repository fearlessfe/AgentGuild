package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"time"
)

const (
	ActivationCredentialPending  = "pending"
	ActivationCredentialConsumed = "consumed"
	ActivationCredentialExpired  = "expired"
	activationTokenLength        = 32
)

type ActivationCredential struct {
	ID         string
	TenantID   string
	AgentID    string
	Hash       []byte
	Status     string
	ExpiresAt  *time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

func NewActivationCredential(agentID, tenantID string, ttl time.Duration) (*ActivationCredential, string, error) {
	if agentID == "" {
		return nil, "", invalidArgument("agent_id")
	}
	if tenantID == "" {
		return nil, "", invalidArgument("tenant_id")
	}
	if ttl <= 0 {
		return nil, "", invalidArgument("ttl")
	}

	plaintext := make([]byte, activationTokenLength)
	if _, err := rand.Read(plaintext); err != nil {
		return nil, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(plaintext)
	hash := hashToken(token)

	expiresAt := time.Now().Add(ttl)
	return &ActivationCredential{
		ID:        generateID("cred"),
		TenantID:  tenantID,
		AgentID:   agentID,
		Hash:      hash,
		Status:    ActivationCredentialPending,
		ExpiresAt: &expiresAt,
		CreatedAt: time.Now(),
	}, token, nil
}

func (c *ActivationCredential) Consume(plaintext string, now time.Time) error {
	if c.Status == ActivationCredentialConsumed {
		return ErrTokenExpired
	}
	if c.ExpiresAt != nil && !now.Before(*c.ExpiresAt) {
		c.Status = ActivationCredentialExpired
		return ErrTokenExpired
	}
	if subtle.ConstantTimeCompare(c.Hash, hashToken(plaintext)) != 1 {
		return ErrTokenExpired
	}
	c.Status = ActivationCredentialConsumed
	c.ConsumedAt = &now
	return nil
}

func hashToken(plaintext string) []byte {
	sum := sha256.Sum256([]byte(plaintext))
	return sum[:]
}
