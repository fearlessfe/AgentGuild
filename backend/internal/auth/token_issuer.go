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
		"sub":              AgentSubject(agent.ID),
		"identity_scope":   IdentityScopeTenant,
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
	return i.sign(claims)
}

// IssueGlobal issues a token for a platform-global Agent identity. The token
// intentionally contains no tenant or repository scope; resource access must be
// granted independently by membership or a task participation grant.
func (i *TokenIssuer) IssueGlobal(agent *domain.AgentIdentity, version *domain.AgentVersion, scopes []string, now time.Time) (string, error) {
	if agent == nil || agent.ID == "" {
		return "", errors.New("agent identity is incomplete")
	}
	if agent.Status != domain.AgentActive {
		return "", errors.New("agent identity is not active")
	}
	if version == nil || version.ID == "" {
		return "", errors.New("agent version is required")
	}
	if version.AgentID != agent.ID {
		return "", errors.New("agent version does not belong to agent")
	}
	if agent.CurrentVersionID != "" && agent.CurrentVersionID != version.ID {
		return "", errors.New("agent version is not current")
	}
	claims := jwt.MapClaims{
		"sub":              AgentSubject(agent.ID),
		"identity_scope":   IdentityScopeGlobal,
		"agent_id":         agent.ID,
		"agent_version_id": version.ID,
		"scopes":           append([]string(nil), scopes...),
		"iat":              now.Unix(),
		"exp":              now.Add(i.config.TTL).Unix(),
	}
	if i.config.Issuer != "" {
		claims["iss"] = i.config.Issuer
	}
	if i.config.Audience != "" {
		claims["aud"] = i.config.Audience
	}
	return i.sign(claims)
}

func (i *TokenIssuer) sign(claims jwt.MapClaims) (string, error) {
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
		SubjectID:      stringClaim(claims, "sub"),
		IdentityScope:  stringClaim(claims, "identity_scope"),
		TenantID:       stringClaim(claims, "tenant_id"),
		Type:           PrincipalTypeAgent,
		AgentID:        stringClaim(claims, "agent_id"),
		AgentVersionID: stringClaim(claims, "agent_version_id"),
		Scopes:         stringSliceClaim(claims, "scopes"),
		RepoScope:      stringSliceClaim(claims, "repo_scope"),
	}
	if principal.AgentID == "" || principal.AgentVersionID == "" {
		return principal, errors.New("token is missing required identity claims")
	}
	if principal.SubjectID != "" && principal.SubjectID != AgentSubject(principal.AgentID) {
		return principal, errors.New("token subject does not match agent identity")
	}
	if principal.TenantID == "" {
		if principal.IdentityScope != IdentityScopeGlobal || principal.SubjectID != AgentSubject(principal.AgentID) {
			return principal, errors.New("global token is missing required identity claims")
		}
		principal.RepoScope = nil
	} else if principal.IdentityScope == "" {
		// Backward compatibility for tenant-scoped tokens issued before the global
		// identity migration.
		principal.IdentityScope = IdentityScopeTenant
	}
	return principal, nil
}
