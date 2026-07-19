// Package config loads deployment configuration from environment variables.
package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL, HTTPAddr, CursorSecret                    string
	OAuthIssuer, OAuthAudience, OAuthJWKSURL               string
	OIDCTenantID, OIDCIssuer, OIDCClientID                 string
	OIDCClientSecret, OIDCRedirectURI                      string
	OIDCAuthURL, OIDCTokenURL, OIDCJWKSURL                 string
	OIDCAdminClaim                                         string
	SessionCookieSecret                                    string
	AgentRSAPrivateKeyPath, AgentRSAPrivateKeyPEM          string
	OIDCAdminEmails                                        []string
	MCPEnabled, WebEnabled, LangfuseEnabled                bool
	SessionCookieSecure                                    bool
	ReaperInterval, OutboxInterval, ShutdownTimeout        time.Duration
	ReputationWorkerInterval                               time.Duration
	SyncWorkerInterval, SyncDefaultDeadline                time.Duration
	ReviewSeedTenantID                                     string
	ReviewSeedReviewerUserID                               string
	ValidationWorkerInterval, ValidationLease              time.Duration
	ValidationMaxAttempts                                  int
	ValidationSandboxImage                                 string
	EvaluationExecutor                                     string
	LangfuseBaseURL, LangfusePublicKey, LangfuseSecretKey  string
	LangfuseMode, LangfuseMetricsPath, LangfuseCompleteTag string
	LangfuseSupportsCost                                   bool
	GitHubAppPublicBaseURL, GitHubAppManifestStateSecret   string
	GitHubAllowedHosts                                     []string
	GitHub                                                 struct {
		AppID          int64
		PrivateKey     string
		InstallationID int64
		BaseURL        string
	}
	LocalAdmin struct {
		TenantID   string
		OwnerID    string
		OwnerEmail string
		Password   string
		Enabled    bool
	}
}

type LookupEnv func(string) string

// EvaluationExecutorFixed selects the fixed-pass stub benchmark executor. It is
// intended for local development and demos only; any other executor value is a
// configuration error, and an unset value disables evaluation execution.
const EvaluationExecutorFixed = "fixed"

func Load(get LookupEnv) (Config, error) {
	cfg := Config{
		DatabaseURL: get("DATABASE_URL"), HTTPAddr: value(get, "HTTP_ADDR", ":8080"), CursorSecret: get("CURSOR_SECRET"),
		OAuthIssuer: get("OAUTH_ISSUER"), OAuthAudience: get("OAUTH_AUDIENCE"), OAuthJWKSURL: get("OAUTH_JWKS_URL"),
		OIDCTenantID: get("OIDC_TENANT_ID"), OIDCIssuer: get("OIDC_ISSUER"), OIDCClientID: get("OIDC_CLIENT_ID"),
		OIDCClientSecret: get("OIDC_CLIENT_SECRET"), OIDCRedirectURI: get("OIDC_REDIRECT_URI"),
		OIDCAuthURL: get("OIDC_AUTH_URL"), OIDCTokenURL: get("OIDC_TOKEN_URL"), OIDCJWKSURL: get("OIDC_JWKS_URL"),
		OIDCAdminClaim: get("OIDC_ADMIN_CLAIM"), SessionCookieSecret: get("SESSION_COOKIE_SECRET"),
		AgentRSAPrivateKeyPath: get("AGENT_RSA_PRIVATE_KEY_PATH"), AgentRSAPrivateKeyPEM: get("AGENT_RSA_PRIVATE_KEY_PEM"),
		OIDCAdminEmails: splitCSV(get("OIDC_ADMIN_EMAILS")),
		LangfuseBaseURL: get("LANGFUSE_BASE_URL"), LangfusePublicKey: get("LANGFUSE_PUBLIC_KEY"), LangfuseSecretKey: get("LANGFUSE_SECRET_KEY"),
		LangfuseMode: value(get, "LANGFUSE_MODE", "cloud"), LangfuseMetricsPath: get("LANGFUSE_METRICS_PATH"), LangfuseCompleteTag: get("LANGFUSE_COMPLETE_COVERAGE_TAG"),
		ReviewSeedTenantID:           get("REVIEW_SEED_TENANT_ID"),
		ReviewSeedReviewerUserID:     value(get, "REVIEW_SEED_REVIEWER_USER_ID", "default-reviewer"),
		ValidationSandboxImage:       get("VALIDATION_SANDBOX_IMAGE"),
		EvaluationExecutor:           strings.ToLower(strings.TrimSpace(get("EVALUATION_EXECUTOR"))),
		GitHubAppPublicBaseURL:       get("GITHUB_APP_PUBLIC_BASE_URL"),
		GitHubAppManifestStateSecret: get("SESSION_COOKIE_SECRET"),
		GitHubAllowedHosts:           splitCSV(get("GITHUB_ALLOWED_HOSTS")),
		LocalAdmin: struct {
			TenantID   string
			OwnerID    string
			OwnerEmail string
			Password   string
			Enabled    bool
		}{
			TenantID:   value(get, "LOCAL_ADMIN_TENANT_ID", "local"),
			OwnerID:    value(get, "LOCAL_ADMIN_OWNER_ID", "local-admin"),
			OwnerEmail: value(get, "LOCAL_ADMIN_OWNER_EMAIL", "admin@local"),
			Password:   get("LOCAL_ADMIN_PASSWORD"),
		},
	}
	var err error
	if cfg.MCPEnabled, err = boolean(get, "MCP_ENABLED", true); err != nil {
		return Config{}, err
	}
	if cfg.WebEnabled, err = boolean(get, "WEB_ENABLED", true); err != nil {
		return Config{}, err
	}
	cfg.GitHubAppPublicBaseURL, err = parseGitHubAppPublicBaseURL(cfg.GitHubAppPublicBaseURL, cfg.WebEnabled || cfg.MCPEnabled)
	if err != nil {
		return Config{}, err
	}
	if cfg.SessionCookieSecure, err = boolean(get, "SESSION_COOKIE_SECURE", false); err != nil {
		return Config{}, err
	}
	if cfg.LangfuseEnabled, err = boolean(get, "LANGFUSE_ENABLED", false); err != nil {
		return Config{}, err
	}
	if cfg.LangfuseSupportsCost, err = boolean(get, "LANGFUSE_SUPPORTS_COST", true); err != nil {
		return Config{}, err
	}
	cfg.GitHub.BaseURL = value(get, "GITHUB_BASE_URL", "https://api.github.com")
	cfg.GitHub.PrivateKey = get("GITHUB_PRIVATE_KEY")
	if cfg.GitHub.AppID, err = integer(get, "GITHUB_APP_ID"); err != nil {
		return Config{}, err
	}
	if cfg.GitHub.InstallationID, err = integer(get, "GITHUB_INSTALLATION_ID"); err != nil {
		return Config{}, err
	}
	localPassword := get("LOCAL_ADMIN_PASSWORD")
	if localPassword != "" && len(localPassword) < 12 {
		return Config{}, fmt.Errorf("LOCAL_ADMIN_PASSWORD must be at least 12 characters")
	}
	cfg.LocalAdmin.Enabled = cfg.WebEnabled && cfg.OIDCTenantID == "" && localPassword != ""
	if cfg.ReaperInterval, err = duration(get, "REAPER_INTERVAL", 5*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.OutboxInterval, err = duration(get, "OUTBOX_INTERVAL", 3*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ReputationWorkerInterval, err = duration(get, "REPUTATION_WORKER_INTERVAL", 30*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.SyncWorkerInterval, err = duration(get, "SYNC_WORKER_INTERVAL", 60*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.SyncDefaultDeadline, err = duration(get, "SYNC_DEFAULT_DEADLINE", 365*24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = duration(get, "SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ValidationWorkerInterval, err = duration(get, "VALIDATION_WORKER_INTERVAL", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ValidationLease, err = duration(get, "VALIDATION_LEASE", 5*time.Minute); err != nil {
		return Config{}, err
	}
	var maxAttempts int64
	if maxAttempts, err = integer(get, "VALIDATION_MAX_ATTEMPTS"); err != nil {
		return Config{}, err
	}
	if maxAttempts == 0 {
		cfg.ValidationMaxAttempts = 3
	} else {
		cfg.ValidationMaxAttempts = int(maxAttempts)
	}
	switch cfg.EvaluationExecutor {
	case "", EvaluationExecutorFixed:
	default:
		return Config{}, fmt.Errorf("EVALUATION_EXECUTOR must be %q or unset (evaluation execution disabled)", EvaluationExecutorFixed)
	}
	for _, required := range [][2]string{{"DATABASE_URL", cfg.DatabaseURL}, {"CURSOR_SECRET", cfg.CursorSecret}} {
		if required[1] == "" {
			return Config{}, fmt.Errorf("%s is required", required[0])
		}
	}
	if len(cfg.CursorSecret) < 32 {
		return Config{}, fmt.Errorf("CURSOR_SECRET must contain at least 32 bytes")
	}
	if cfg.WebEnabled || cfg.MCPEnabled {
		for _, required := range [][2]string{{"OAUTH_ISSUER", cfg.OAuthIssuer}, {"OAUTH_AUDIENCE", cfg.OAuthAudience}} {
			if required[1] == "" {
				return Config{}, fmt.Errorf("%s is required when a transport is enabled", required[0])
			}
		}
		if !cfg.LocalAdmin.Enabled && cfg.OAuthJWKSURL == "" {
			return Config{}, fmt.Errorf("OAUTH_JWKS_URL is required when a transport is enabled")
		}
	}
	if cfg.WebEnabled {
		for _, required := range [][2]string{
			{"SESSION_COOKIE_SECRET", cfg.SessionCookieSecret},
		} {
			if required[1] == "" {
				return Config{}, fmt.Errorf("%s is required when WEB_ENABLED=true", required[0])
			}
		}
		if len(cfg.SessionCookieSecret) < 32 {
			return Config{}, fmt.Errorf("SESSION_COOKIE_SECRET must contain at least 32 bytes")
		}
		if cfg.AgentRSAPrivateKeyPEM == "" && cfg.AgentRSAPrivateKeyPath == "" {
			return Config{}, fmt.Errorf("AGENT_RSA_PRIVATE_KEY_PEM or AGENT_RSA_PRIVATE_KEY_PATH is required when WEB_ENABLED=true")
		}
		if cfg.OIDCTenantID == "" && !cfg.LocalAdmin.Enabled {
			for _, required := range [][2]string{
				{"OIDC_TENANT_ID", cfg.OIDCTenantID},
				{"OIDC_ISSUER", cfg.OIDCIssuer},
				{"OIDC_CLIENT_ID", cfg.OIDCClientID},
				{"OIDC_REDIRECT_URI", cfg.OIDCRedirectURI},
				{"OIDC_AUTH_URL", cfg.OIDCAuthURL},
				{"OIDC_TOKEN_URL", cfg.OIDCTokenURL},
				{"OIDC_JWKS_URL", cfg.OIDCJWKSURL},
			} {
				if required[1] == "" {
					return Config{}, fmt.Errorf("%s is required when WEB_ENABLED=true and local admin fallback is not enabled", required[0])
				}
			}
		}
	}
	if cfg.LangfuseEnabled {
		for _, required := range [][2]string{{"LANGFUSE_BASE_URL", cfg.LangfuseBaseURL}, {"LANGFUSE_PUBLIC_KEY", cfg.LangfusePublicKey}, {"LANGFUSE_SECRET_KEY", cfg.LangfuseSecretKey}} {
			if required[1] == "" {
				return Config{}, fmt.Errorf("%s is required when LANGFUSE_ENABLED=true", required[0])
			}
		}
	}
	return cfg, nil
}

func parseGitHubAppPublicBaseURL(raw string, required bool) (string, error) {
	if strings.TrimSpace(raw) == "" {
		if required {
			return "", fmt.Errorf("GITHUB_APP_PUBLIC_BASE_URL is required when WEB_ENABLED=true or MCP_ENABLED=true")
		}
		return "", nil
	}
	if strings.ContainsAny(raw, "?#") {
		return "", fmt.Errorf("GITHUB_APP_PUBLIC_BASE_URL must be an absolute HTTP(S) origin")
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("GITHUB_APP_PUBLIC_BASE_URL must be an absolute HTTP(S) origin")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func value(get LookupEnv, key, fallback string) string {
	if v := get(key); v != "" {
		return v
	}
	return fallback
}
func boolean(get LookupEnv, key string, fallback bool) (bool, error) {
	v := get(key)
	if v == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}
func duration(get LookupEnv, key string, fallback time.Duration) (time.Duration, error) {
	v := get(key)
	if v == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(v)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return parsed, nil
}

func integer(get LookupEnv, key string) (int64, error) {
	v := get(key)
	if v == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return parsed, nil
}
