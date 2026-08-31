package domain

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"time"
)

const (
	RegistrationChallengePending  = "pending"
	RegistrationChallengeConsumed = "consumed"
	registrationNonceLength       = 32
)

// AgentRegistrationChallenge binds a short-lived server challenge to the
// public key that will become the Agent's durable identity.
type AgentRegistrationChallenge struct {
	ID                  string
	PublicKey           []byte
	Nonce               []byte
	Status              string
	ExpiresAt           time.Time
	ConsumedAt          *time.Time
	RegisteredAgentID   string
	RegisteredVersionID string
	CreatedAt           time.Time
}

type AgentIdentityKey struct {
	AgentID    string
	KeyID      string
	Algorithm  string
	Thumbprint string
	PublicKey  []byte
	CreatedAt  time.Time
}

func NewAgentRegistrationChallenge(id string, publicKey ed25519.PublicKey, ttl time.Duration, now time.Time) (*AgentRegistrationChallenge, error) {
	if id == "" {
		return nil, invalidArgument("challenge_id")
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, invalidArgument("public_key")
	}
	if ttl <= 0 {
		return nil, invalidArgument("ttl")
	}
	nonce := make([]byte, registrationNonceLength)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return &AgentRegistrationChallenge{
		ID:        id,
		PublicKey: append([]byte(nil), publicKey...),
		Nonce:     nonce,
		Status:    RegistrationChallengePending,
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
	}, nil
}

func (c *AgentRegistrationChallenge) Consume(now time.Time) error {
	if c.Status != RegistrationChallengePending {
		return ErrTokenExpired
	}
	if !now.Before(c.ExpiresAt) {
		return ErrTokenExpired
	}
	c.Status = RegistrationChallengeConsumed
	c.ConsumedAt = &now
	return nil
}

// RegistrationProofMessage provides a length-delimited, domain-separated
// message for Ed25519 registration signatures.
func RegistrationProofMessage(challengeID string, nonce, publicKey []byte) []byte {
	var payload bytes.Buffer
	payload.WriteString("AGENTGUILD/REGISTER/v1")
	for _, part := range [][]byte{[]byte(challengeID), nonce, publicKey} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(part)))
		payload.Write(length[:])
		payload.Write(part)
	}
	return payload.Bytes()
}

func PublicKeyThumbprint(publicKey []byte) string {
	sum := sha256.Sum256(publicKey)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
