package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

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
		exchanger = oauth2OIDCExchanger{config: oauthConfig, issuer: config.Issuer, clientID: config.ClientID, client: config.HTTPClient}
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
	claims, err := parseOIDCClaims(rawIDToken)
	if err != nil {
		return nil, err
	}
	if e.issuer != "" && oidcStringClaim(claims, "iss") != e.issuer {
		return nil, errors.New("oidc issuer mismatch")
	}
	if e.clientID != "" && !oidcAudienceContains(claims["aud"], e.clientID) {
		return nil, errors.New("oidc audience mismatch")
	}
	if exp := oidcNumericClaim(claims, "exp"); exp > 0 && time.Now().After(time.Unix(exp, 0)) {
		return nil, errors.New("oidc id_token is expired")
	}
	return claims, nil
}

func parseOIDCClaims(rawIDToken string) (OIDCClaims, error) {
	parts := strings.Split(rawIDToken, ".")
	if len(parts) < 2 {
		return nil, errors.New("oidc id_token is malformed")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode oidc claims: %w", err)
	}
	var claims OIDCClaims
	if err := json.Unmarshal(body, &claims); err != nil {
		return nil, fmt.Errorf("parse oidc claims: %w", err)
	}
	return claims, nil
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

func oidcNumericClaim(claims OIDCClaims, key string) int64 {
	switch value := claims[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case json.Number:
		n, _ := value.Int64()
		return n
	default:
		return 0
	}
}

func oidcAudienceContains(raw any, want string) bool {
	switch value := raw.(type) {
	case string:
		return value == want
	case []any:
		for _, item := range value {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}
