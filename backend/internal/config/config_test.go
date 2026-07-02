package config_test

import (
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoadAppliesAdapterSwitchesAndWorkerDefaults(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":     "postgres://agentguild:test@localhost/agentguild",
		"CURSOR_SECRET":    strings.Repeat("s", 32),
		"OAUTH_ISSUER":     "https://issuer.example",
		"OAUTH_AUDIENCE":   "agentguild",
		"OAUTH_JWKS_URL":   "https://issuer.example/.well-known/jwks.json",
		"MCP_ENABLED":      "false",
		"WEB_ENABLED":      "true",
		"LANGFUSE_ENABLED": "false",
		"REAPER_INTERVAL":  "7s",
		"OUTBOX_INTERVAL":  "11s",
	}

	cfg, err := config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.False(t, cfg.MCPEnabled)
	require.True(t, cfg.WebEnabled)
	require.False(t, cfg.LangfuseEnabled)
	require.Equal(t, 7*time.Second, cfg.ReaperInterval)
	require.Equal(t, 11*time.Second, cfg.OutboxInterval)
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

func validEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":   "postgres://agentguild:test@localhost/agentguild",
		"CURSOR_SECRET":  strings.Repeat("s", 32),
		"OAUTH_ISSUER":   "https://issuer.example",
		"OAUTH_AUDIENCE": "agentguild",
		"OAUTH_JWKS_URL": "https://issuer.example/.well-known/jwks.json",
	}
}
