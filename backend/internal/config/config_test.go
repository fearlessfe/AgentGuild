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
	require.Equal(t, "public", cfg.OpenAgentOrganizationID)
}

func TestLoadAllowsOpenAgentOrganizationOverride(t *testing.T) {
	env := validEnv()
	env["OPEN_AGENT_ORGANIZATION_ID"] = "community"
	cfg, err := config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, "community", cfg.OpenAgentOrganizationID)
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
		"GITHUB_APP_PUBLIC_BASE_URL": "https://agentguild.example.com",
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

func TestLoadRequiresGitHubAppPublicBaseURLWhenWebEnabled(t *testing.T) {
	env := validEnv()
	delete(env, "GITHUB_APP_PUBLIC_BASE_URL")

	_, err := config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "GITHUB_APP_PUBLIC_BASE_URL is required")

	env["WEB_ENABLED"] = "false"
	env["MCP_ENABLED"] = "false"
	_, err = config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
}

func TestLoadValidatesGitHubAppPublicBaseURL(t *testing.T) {
	tests := []string{
		"/relative", "ftp://agentguild.example.com", "https:///missing-host",
		"https://user@agentguild.example.com", "https://agentguild.example.com/app",
		"https://agentguild.example.com?tenant=1", "https://agentguild.example.com#fragment",
		"https://agentguild.example.com?", "https://agentguild.example.com/?",
		"https://agentguild.example.com#", "https://agentguild.example.com/#",
	}
	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			env := validEnv()
			env["GITHUB_APP_PUBLIC_BASE_URL"] = value
			_, err := config.Load(func(key string) string { return env[key] })
			require.ErrorContains(t, err, "GITHUB_APP_PUBLIC_BASE_URL must be an absolute HTTP(S) origin")
		})
	}
}

func TestLoadNormalizesGitHubAppPublicBaseURL(t *testing.T) {
	env := validEnv()
	env["GITHUB_APP_PUBLIC_BASE_URL"] = "https://agentguild.example.com/"
	cfg, err := config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, "https://agentguild.example.com", cfg.GitHubAppPublicBaseURL)
}

func TestLoadValidatesEvaluationExecutor(t *testing.T) {
	// Unset means evaluation runs are disabled (fail closed at execution time).
	env := validEnv()
	cfg, err := config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, "", cfg.EvaluationExecutor)

	// "fixed" explicitly selects the fixed-pass stub for local development.
	env = validEnv()
	env["EVALUATION_EXECUTOR"] = "fixed"
	cfg, err = config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, config.EvaluationExecutorFixed, cfg.EvaluationExecutor)

	// "platform" selects the real executor that publishes platform tasks.
	env = validEnv()
	env["EVALUATION_EXECUTOR"] = "platform"
	cfg, err = config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, config.EvaluationExecutorPlatform, cfg.EvaluationExecutor)
	require.Equal(t, 2*time.Hour, cfg.EvaluationTaskDeadline)

	// The evaluation harvest worker has its own interval and run timeout defaults.
	require.Equal(t, 30*time.Second, cfg.EvaluationWorkerInterval)
	require.Equal(t, 24*time.Hour, cfg.EvaluationRunTimeout)

	// EVALUATION_TASK_DEADLINE overrides the default evaluation task deadline.
	env = validEnv()
	env["EVALUATION_EXECUTOR"] = "platform"
	env["EVALUATION_TASK_DEADLINE"] = "30m"
	cfg, err = config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, 30*time.Minute, cfg.EvaluationTaskDeadline)

	// Any other value is a startup configuration error.
	env = validEnv()
	env["EVALUATION_EXECUTOR"] = "real-runner"
	_, err = config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "EVALUATION_EXECUTOR")
}

func TestLoadConfiguresPublicTaskAnalyzer(t *testing.T) {
	env := validEnv()
	cfg, err := config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, "deterministic", cfg.PublicTaskAnalysisProvider)
	require.Equal(t, 120, cfg.PublicTaskAnalysisMaxFiles)
	require.Equal(t, 512*1024, cfg.PublicTaskAnalysisMaxBytes)

	env["PUBLIC_TASK_ANALYZER"] = "anthropic"
	env["ANTHROPIC_API_KEY"] = "test-key"
	env["PUBLIC_TASK_ANALYSIS_MAX_FILES"] = "20"
	env["PUBLIC_TASK_ANALYSIS_MAX_BYTES"] = "4096"
	cfg, err = config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, "anthropic", cfg.PublicTaskAnalysisProvider)
	require.Equal(t, 20, cfg.PublicTaskAnalysisMaxFiles)
	require.Equal(t, 4096, cfg.PublicTaskAnalysisMaxBytes)

	env["ANTHROPIC_API_KEY"] = ""
	_, err = config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "ANTHROPIC_API_KEY")

	env = validEnv()
	env["PUBLIC_TASK_ANALYZER"] = "openai"
	env["OPENAI_API_KEY"] = "test-openai-key"
	env["BASE_URL"] = "https://llm.example.com/v1"
	env["MODEL"] = "gpt-5.6-sol"
	cfg, err = config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, "openai", cfg.PublicTaskAnalysisProvider)
	require.Equal(t, "test-openai-key", cfg.PublicTaskAnalysisAPIKey)
	require.Equal(t, "https://llm.example.com/v1", cfg.PublicTaskAnalysisBaseURL)
	require.Equal(t, "gpt-5.6-sol", cfg.PublicTaskAnalysisModel)

	env["OPENAI_API_KEY"] = ""
	_, err = config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "OPENAI_API_KEY")
}

func TestLoadValidatesEvaluationWorkerTiming(t *testing.T) {
	// EVALUATION_WORKER_INTERVAL and EVALUATION_RUN_TIMEOUT override the defaults.
	env := validEnv()
	env["EVALUATION_WORKER_INTERVAL"] = "45s"
	env["EVALUATION_RUN_TIMEOUT"] = "12h"
	cfg, err := config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, 45*time.Second, cfg.EvaluationWorkerInterval)
	require.Equal(t, 12*time.Hour, cfg.EvaluationRunTimeout)

	// Non-positive or unparsable durations are rejected.
	env = validEnv()
	env["EVALUATION_WORKER_INTERVAL"] = "0s"
	_, err = config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "EVALUATION_WORKER_INTERVAL")

	env = validEnv()
	env["EVALUATION_RUN_TIMEOUT"] = "-1h"
	_, err = config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "EVALUATION_RUN_TIMEOUT")

	env = validEnv()
	env["EVALUATION_RUN_TIMEOUT"] = "soon"
	_, err = config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "EVALUATION_RUN_TIMEOUT")
}

func TestLoadParsesEvaluationAuto(t *testing.T) {
	// EVALUATION_AUTO defaults to off.
	env := validEnv()
	cfg, err := config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.False(t, cfg.EvaluationAuto)

	// It follows the same boolean parsing as the other toggles.
	for _, raw := range []string{"true", "1", "TRUE", "True"} {
		env = validEnv()
		env["EVALUATION_AUTO"] = raw
		cfg, err = config.Load(func(key string) string { return env[key] })
		require.NoError(t, err)
		require.True(t, cfg.EvaluationAuto, "EVALUATION_AUTO=%q", raw)
	}

	env = validEnv()
	env["EVALUATION_AUTO"] = "false"
	cfg, err = config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.False(t, cfg.EvaluationAuto)

	env = validEnv()
	env["EVALUATION_AUTO"] = "sometimes"
	_, err = config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "EVALUATION_AUTO")
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
		"DATABASE_URL":               "postgres://agentguild:test@localhost/agentguild",
		"CURSOR_SECRET":              strings.Repeat("s", 32),
		"OAUTH_ISSUER":               "https://issuer.example",
		"OAUTH_AUDIENCE":             "agentguild",
		"OAUTH_JWKS_URL":             "https://issuer.example/.well-known/jwks.json",
		"SESSION_COOKIE_SECRET":      strings.Repeat("c", 32),
		"OIDC_TENANT_ID":             "tenant-1",
		"OIDC_ISSUER":                "https://issuer.example",
		"OIDC_CLIENT_ID":             "client-1",
		"OIDC_CLIENT_SECRET":         "secret-1",
		"OIDC_REDIRECT_URI":          "https://issuer.example/callback",
		"OIDC_AUTH_URL":              "https://issuer.example/oauth/authorize",
		"OIDC_TOKEN_URL":             "https://issuer.example/oauth/token",
		"OIDC_JWKS_URL":              "https://issuer.example/.well-known/openid-jwks.json",
		"OIDC_ADMIN_CLAIM":           "agentguild_admin",
		"OIDC_ADMIN_EMAILS":          "admin@example.com",
		"AGENT_RSA_PRIVATE_KEY_PEM":  string(privateKeyPEM),
		"GITHUB_APP_PUBLIC_BASE_URL": "https://agentguild.example.com",
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
