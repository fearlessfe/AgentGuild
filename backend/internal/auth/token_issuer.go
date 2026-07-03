package auth

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/domain"
	"github.com/golang-jwt/jwt/v5"
)

var ErrTokenExpired = errors.New("token expired")

const agentAccessTokenTTL = 15 * time.Minute

type TokenIssuerConfig struct {
	Issuer   string
	Audience string
	KeyID    string
	TTL      time.Duration
}

type TokenIssuer struct {
	privateKey *rsa.PrivateKey
	config     TokenIssuerConfig
}

func NewRS256TokenIssuer(privateKey *rsa.PrivateKey, config TokenIssuerConfig) (*TokenIssuer, error) {
	if privateKey == nil {
		return nil, errors.New("private key is required")
	}
	if config.TTL <= 0 {
		config.TTL = agentAccessTokenTTL
	}
	return &TokenIssuer{privateKey: privateKey, config: config}, nil
}

func (i *TokenIssuer) Issue(agent *domain.Agent, version *domain.AgentVersion, now time.Time) (string, error) {
	if agent == nil {
		return "", errors.New("agent is required")
	}
	if version == nil {
		return "", errors.New("agent version is required")
	}
	if agent.TenantID == "" || agent.ID == "" {
		return "", errors.New("agent identity is incomplete")
	}
	if version.TenantID != agent.TenantID || version.AgentID != agent.ID {
		return "", errors.New("agent version does not belong to agent")
	}
	if version.ID == "" {
		return "", errors.New("agent version id is required")
	}
	claims := jwt.MapClaims{
		"tenant_id":        agent.TenantID,
		"agent_id":         agent.ID,
		"agent_version_id": version.ID,
		"scopes":           append([]string(nil), agent.Scopes...),
		"repo_scope":       append([]string(nil), agent.RepoScope...),
		"iat":              now.Unix(),
		"exp":              now.Add(i.config.TTL).Unix(),
	}
	if i.config.Issuer != "" {
		claims["iss"] = i.config.Issuer
	}
	if i.config.Audience != "" {
		claims["aud"] = i.config.Audience
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if i.config.KeyID != "" {
		token.Header["kid"] = i.config.KeyID
	}
	return token.SignedString(i.privateKey)
}

type TokenVerifierConfig struct {
	Issuer   string
	Audience string
	Now      func() time.Time
}

type RS256Verifier struct {
	publicKey *rsa.PublicKey
	config    TokenVerifierConfig
}

func NewRS256Verifier(publicKey *rsa.PublicKey, config TokenVerifierConfig) *RS256Verifier {
	if config.Now == nil {
		config.Now = time.Now
	}
	return &RS256Verifier{publicKey: publicKey, config: config}
}

func (v *RS256Verifier) Verify(_ context.Context, rawToken string) (Principal, error) {
	var principal Principal
	if rawToken == "" {
		return principal, errors.New("token is empty")
	}
	if v.publicKey == nil {
		return principal, errors.New("public key is required")
	}
	claims := jwt.MapClaims{}
	options := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithTimeFunc(v.config.Now),
	}
	if v.config.Issuer != "" {
		options = append(options, jwt.WithIssuer(v.config.Issuer))
	}
	if v.config.Audience != "" {
		options = append(options, jwt.WithAudience(v.config.Audience))
	}
	token, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		return v.publicKey, nil
	}, options...)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return principal, ErrTokenExpired
		}
		return principal, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return principal, errors.New("token is invalid")
	}
	expiresAt, err := claims.GetExpirationTime()
	if err != nil {
		return principal, fmt.Errorf("invalid exp claim: %w", err)
	}
	if expiresAt == nil {
		return principal, errors.New("token is missing exp claim")
	}
	principal = Principal{
		TenantID:       stringClaim(claims, "tenant_id"),
		Type:           PrincipalTypeAgent,
		AgentID:        stringClaim(claims, "agent_id"),
		AgentVersionID: stringClaim(claims, "agent_version_id"),
		Scopes:         stringSliceClaim(claims, "scopes"),
		RepoScope:      stringSliceClaim(claims, "repo_scope"),
	}
	if principal.TenantID == "" || principal.AgentID == "" || principal.AgentVersionID == "" {
		return principal, errors.New("token is missing required identity claims")
	}
	return principal, nil
}
