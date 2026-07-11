package config_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoadAppliesAdapterSwitchesAndWorkerDefaults(t *testing.T) {
	env := validEnv()
	env["MCP_ENABLED"] = "false"
	env["WEB_ENABLED"] = "true"
	env["LANGFUSE_ENABLED"] = "false"
	env["REAPER_INTERVAL"] = "7s"
	env["OUTBOX_INTERVAL"] = "11s"

	cfg, err := config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.False(t, cfg.MCPEnabled)
	require.True(t, cfg.WebEnabled)
	require.False(t, cfg.LangfuseEnabled)
	require.Equal(t, 7*time.Second, cfg.ReaperInterval)
	require.Equal(t, 11*time.Second, cfg.OutboxInterval)
	require.Equal(t, 60*time.Second, cfg.SyncWorkerInterval)
	require.Equal(t, 365*24*time.Hour, cfg.SyncDefaultDeadline)
}

func TestLoadRejectsInvalidBoolean(t *testing.T) {
	env := validEnv()
	env["WEB_ENABLED"] = "sometimes"

	_, err := config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "WEB_ENABLED")
}

func TestLoadRequiresLangfuseCredentialsOnlyWhenEnabled(t *testing.T) {
	env := validEnv()
	env["LANGFUSE_ENABLED"] = "true"

	_, err := config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "LANGFUSE_BASE_URL")

	env["LANGFUSE_ENABLED"] = "false"
	_, err = config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
}

func TestLoadRequiresIdentityRuntimeConfigurationWhenWebEnabled(t *testing.T) {
	env := validEnv()
	env["SESSION_COOKIE_SECRET"] = ""

	_, err := config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "SESSION_COOKIE_SECRET")

	env = validEnv()
	env["OIDC_TENANT_ID"] = ""
	env["LOCAL_ADMIN_PASSWORD"] = ""
	_, err = config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "OIDC_TENANT_ID")

	env = validEnv()
	env["AGENT_RSA_PRIVATE_KEY_PEM"] = ""
	env["AGENT_RSA_PRIVATE_KEY_PATH"] = ""
	_, err = config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "AGENT_RSA_PRIVATE_KEY_PEM")
}

func TestLoadParsesIdentityRuntimeConfiguration(t *testing.T) {
	env := validEnv()
	env["OIDC_ADMIN_EMAILS"] = "admin@example.com, owner@example.com "
	env["SESSION_COOKIE_SECURE"] = "true"

	cfg, err := config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, "tenant-1", cfg.OIDCTenantID)
	require.Equal(t, "client-1", cfg.OIDCClientID)
	require.Equal(t, "https://issuer.example/callback", cfg.OIDCRedirectURI)
	require.Equal(t, []string{"admin@example.com", "owner@example.com"}, cfg.OIDCAdminEmails)
	require.Equal(t, "agentguild_admin", cfg.OIDCAdminClaim)
	require.Equal(t, "-----BEGIN RSA PRIVATE KEY-----", cfg.AgentRSAPrivateKeyPEM[:31])
	require.True(t, cfg.SessionCookieSecure)
}

func TestLoadLocalAdminJWKSRequirement(t *testing.T) {
	privateKey := rsaKeyPath(t)
	baseEnv := map[string]string{
		"DATABASE_URL":               "postgres://agentguild:test@localhost/agentguild",
		"CURSOR_SECRET":              strings.Repeat("s", 32),
		"SESSION_COOKIE_SECRET":      strings.Repeat("c", 32),
		"LOCAL_ADMIN_PASSWORD":       "local-password",
		"OAUTH_ISSUER":               "http://agentguild.local",
		"OAUTH_AUDIENCE":             "agentguild",
		"AGENT_RSA_PRIVATE_KEY_PATH": privateKey,
	}

	tests := []struct {
		name      string
		configure func(map[string]string)
		wantErr   bool
	}{
		{
			name: "accepts empty external JWKS in local admin mode",
		},
		{
			name: "rejects empty external JWKS for non-local web transport",
			configure: func(env map[string]string) {
				env["LOCAL_ADMIN_PASSWORD"] = ""
				env["MCP_ENABLED"] = "false"
			},
			wantErr: true,
		},
		{
			name: "rejects empty external JWKS for non-local MCP transport",
			configure: func(env map[string]string) {
				env["LOCAL_ADMIN_PASSWORD"] = ""
				env["WEB_ENABLED"] = "false"
			},
			wantErr: true,
		},
		{
			name: "rejects empty external JWKS for MCP-only transport despite local admin password",
			configure: func(env map[string]string) {
				env["WEB_ENABLED"] = "false"
				env["MCP_ENABLED"] = "true"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := make(map[string]string, len(baseEnv))
			for key, value := range baseEnv {
				env[key] = value
			}
			if tt.configure != nil {
				tt.configure(env)
			}

			cfg, err := config.Load(func(key string) string { return env[key] })
			if tt.wantErr {
				require.ErrorContains(t, err, "OAUTH_JWKS_URL is required when a transport is enabled")
				return
			}
			require.NoError(t, err)
			require.True(t, cfg.LocalAdmin.Enabled)
		})
	}
}

func TestLoadRejectsShortLocalAdminPasswordForMCPOnlyTransport(t *testing.T) {
	env := validEnv()
	env["WEB_ENABLED"] = "false"
	env["MCP_ENABLED"] = "true"
	env["LOCAL_ADMIN_PASSWORD"] = "short"

	_, err := config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "LOCAL_ADMIN_PASSWORD must be at least 12 characters")
}

func TestLoadParsesGitHubConfiguration(t *testing.T) {
	env := validEnv()
	env["GITHUB_APP_ID"] = "42"
	env["GITHUB_INSTALLATION_ID"] = "123"
	env["GITHUB_PRIVATE_KEY"] = "-----BEGIN RSA PRIVATE KEY-----\nMIIB"
	env["GITHUB_BASE_URL"] = "https://github.example.com/api/v3"
	env["GITHUB_APP_PUBLIC_BASE_URL"] = "https://agentguild.example.com"

	cfg, err := config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, int64(42), cfg.GitHub.AppID)
	require.Equal(t, int64(123), cfg.GitHub.InstallationID)
	require.Equal(t, "https://github.example.com/api/v3", cfg.GitHub.BaseURL)
	require.Equal(t, "-----BEGIN RSA PRIVATE KEY-----\nMIIB", cfg.GitHub.PrivateKey)
	require.Equal(t, "https://agentguild.example.com", cfg.GitHubAppPublicBaseURL)
	require.Equal(t, env["SESSION_COOKIE_SECRET"], cfg.GitHubAppManifestStateSecret)
}

func TestLoadRejectsInvalidGitHubAppID(t *testing.T) {
	env := validEnv()
	env["GITHUB_APP_ID"] = "not-a-number"

	_, err := config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "GITHUB_APP_ID")
}

func validEnv() map[string]string {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	return map[string]string{
		"DATABASE_URL":              "postgres://agentguild:test@localhost/agentguild",
		"CURSOR_SECRET":             strings.Repeat("s", 32),
		"OAUTH_ISSUER":              "https://issuer.example",
		"OAUTH_AUDIENCE":            "agentguild",
		"OAUTH_JWKS_URL":            "https://issuer.example/.well-known/jwks.json",
		"SESSION_COOKIE_SECRET":     strings.Repeat("c", 32),
		"OIDC_TENANT_ID":            "tenant-1",
		"OIDC_ISSUER":               "https://issuer.example",
		"OIDC_CLIENT_ID":            "client-1",
		"OIDC_CLIENT_SECRET":        "secret-1",
		"OIDC_REDIRECT_URI":         "https://issuer.example/callback",
		"OIDC_AUTH_URL":             "https://issuer.example/oauth/authorize",
		"OIDC_TOKEN_URL":            "https://issuer.example/oauth/token",
		"OIDC_JWKS_URL":             "https://issuer.example/.well-known/openid-jwks.json",
		"OIDC_ADMIN_CLAIM":          "agentguild_admin",
		"OIDC_ADMIN_EMAILS":         "admin@example.com",
		"AGENT_RSA_PRIVATE_KEY_PEM": string(privateKeyPEM),
	}
}

func rsaKeyPath(t *testing.T) string {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "agent-private-key.pem")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}), 0o600))
	return path
}
