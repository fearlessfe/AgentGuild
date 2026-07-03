package auth

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

type OIDCConfig struct {
	TenantID     string
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	AuthURL      string
	TokenURL     string
	JWKSURL      string
	Scopes       []string
	AdminClaim   string
	AdminEmails  []string
	HTTPClient   *http.Client
}

type OIDCClaims map[string]any

type OIDCExchanger interface {
	ExchangeOIDC(context.Context, string) (OIDCClaims, error)
}

type OIDCProvider struct {
	config      OIDCConfig
	oauthConfig oauth2.Config
	exchanger   OIDCExchanger
	adminEmails map[string]struct{}
}

func NewOIDCProvider(config OIDCConfig, exchanger OIDCExchanger) (*OIDCProvider, error) {
	if config.TenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if config.Issuer == "" {
		return nil, errors.New("issuer is required")
	}
	if config.ClientID == "" {
		return nil, errors.New("client_id is required")
	}
	if config.RedirectURI == "" {
		return nil, errors.New("redirect_uri is required")
	}
	if config.AuthURL == "" {
		return nil, errors.New("auth_url is required")
	}
	if config.TokenURL == "" {
		return nil, errors.New("token_url is required")
	}
	if len(config.Scopes) == 0 {
		config.Scopes = []string{"openid", "email", "profile"}
	}
	if exchanger == nil && config.JWKSURL == "" {
		return nil, errors.New("jwks_url is required for default oidc exchanger")
	}
	oauthConfig := oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURI,
		Scopes:       append([]string(nil), config.Scopes...),
		Endpoint: oauth2.Endpoint{
			AuthURL:  config.AuthURL,
			TokenURL: config.TokenURL,
		},
	}
	if exchanger == nil {
		client := config.HTTPClient
		if client == nil {
			client = &http.Client{Timeout: 10 * time.Second}
		}
		exchanger = oauth2OIDCExchanger{config: oauthConfig, issuer: config.Issuer, clientID: config.ClientID, jwksURL: config.JWKSURL, client: client}
	}
	adminEmails := make(map[string]struct{}, len(config.AdminEmails))
	for _, email := range config.AdminEmails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email != "" {
			adminEmails[email] = struct{}{}
		}
	}
	return &OIDCProvider{config: config, oauthConfig: oauthConfig, exchanger: exchanger, adminEmails: adminEmails}, nil
}

func (p *OIDCProvider) BeginAuthURL(state string) string {
	return p.oauthConfig.AuthCodeURL(state)
}

func (p *OIDCProvider) Exchange(ctx context.Context, code string) (*Session, error) {
	if code == "" {
		return nil, errors.New("code is required")
	}
	claims, err := p.exchanger.ExchangeOIDC(ctx, code)
	if err != nil {
		return nil, err
	}
	ownerID := oidcStringClaim(claims, "sub")
	ownerEmail := oidcStringClaim(claims, "email")
	if ownerID == "" || ownerEmail == "" {
		return nil, errors.New("oidc claims missing sub or email")
	}
	return &Session{
		TenantID:   p.config.TenantID,
		OwnerID:    ownerID,
		OwnerEmail: ownerEmail,
		IsAdmin:    p.isAdmin(claims, ownerEmail),
	}, nil
}

func (p *OIDCProvider) isAdmin(claims OIDCClaims, email string) bool {
	if p.config.AdminClaim != "" && oidcBoolClaim(claims, p.config.AdminClaim) {
		return true
	}
	_, ok := p.adminEmails[strings.ToLower(strings.TrimSpace(email))]
	return ok
}

type oauth2OIDCExchanger struct {
	config   oauth2.Config
	issuer   string
	clientID string
	jwksURL  string
	client   *http.Client
}

func (e oauth2OIDCExchanger) ExchangeOIDC(ctx context.Context, code string) (OIDCClaims, error) {
	if e.client != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, e.client)
	}
	token, err := e.config.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, errors.New("oidc id_token is missing")
	}
	claims, err := e.verifyIDToken(ctx, rawIDToken)
	if err != nil {
		return nil, err
	}
	return claims, nil
}

func (e oauth2OIDCExchanger) verifyIDToken(ctx context.Context, rawIDToken string) (OIDCClaims, error) {
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(rawIDToken, claims, e.keyFunc(ctx),
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(e.issuer),
		jwt.WithAudience(e.clientID),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("verify oidc id_token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("oidc id_token is invalid")
	}
	out := make(OIDCClaims, len(claims))
	for key, value := range claims {
		out[key] = value
	}
	return out, nil
}

func (e oauth2OIDCExchanger) keyFunc(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, errors.New("oidc id_token header missing kid")
		}
		keys, err := e.fetchKeys(ctx)
		if err != nil {
			return nil, err
		}
		key, ok := keys[kid]
		if !ok {
			return nil, fmt.Errorf("oidc signing key %q not found in JWKS", kid)
		}
		return key, nil
	}
}

func (e oauth2OIDCExchanger) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.jwksURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch OIDC JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch OIDC JWKS: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	keys, err := parseJWKS(body)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func oidcStringClaim(claims OIDCClaims, key string) string {
	value, ok := claims[key].(string)
	if !ok {
		return ""
	}
	return value
}

func oidcBoolClaim(claims OIDCClaims, key string) bool {
	switch value := claims[key].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(value, "true") || value == "1"
	default:
		return false
	}
}
