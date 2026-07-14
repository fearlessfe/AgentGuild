package github

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// InstallationClientConfig configures App-authenticated installation metadata requests.
type InstallationClientConfig struct {
	BaseURL    string
	AppID      int64
	PrivateKey string
	HTTPClient *http.Client
}

// InstallationClient fetches installation metadata using a GitHub App JWT.
type InstallationClient struct {
	baseURL string
	appID   int64
	key     *rsa.PrivateKey
	client  *http.Client
	now     func() time.Time
}

// NewInstallationClient creates a client without exposing App credentials to callers.
func NewInstallationClient(cfg InstallationClientConfig) (*InstallationClient, error) {
	if cfg.AppID == 0 {
		return nil, fmt.Errorf("github app_id is required")
	}
	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("github private_key is required")
	}
	key, err := (Config{PrivateKey: cfg.PrivateKey}).PrivateRSAKey()
	if err != nil {
		return nil, fmt.Errorf("parse github private key: %w", err)
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &InstallationClient{baseURL: baseURL, appID: cfg.AppID, key: key, client: client, now: time.Now}, nil
}

// InstallationAccount returns the owner login attached to an installation.
func (c *InstallationClient) InstallationAccount(ctx context.Context, installationID int64) (string, error) {
	if installationID == 0 {
		return "", fmt.Errorf("github installation_id is required")
	}
	token, err := c.appJWT()
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/app/installations/%d", c.baseURL, installationID), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request installation metadata: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", mapError(resp.StatusCode, body, resp.Header.Get("Retry-After"))
	}
	var payload struct {
		Account struct {
			Login string `json:"login"`
		} `json:"account"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode installation metadata: %w", err)
	}
	login := strings.TrimSpace(payload.Account.Login)
	if login == "" {
		return "", fmt.Errorf("github installation account login is missing")
	}
	return login, nil
}

func (c *InstallationClient) appJWT() (string, error) {
	now := c.now()
	claims := jwt.MapClaims{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(appTokenTTL).Unix(),
		"iss": c.appID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(c.key)
}
