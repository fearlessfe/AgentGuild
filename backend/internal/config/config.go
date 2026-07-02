// Package config loads deployment configuration from environment variables.
package config

import (
	"fmt"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL, HTTPAddr, CursorSecret                    string
	OAuthIssuer, OAuthAudience, OAuthJWKSURL               string
	MCPEnabled, WebEnabled, LangfuseEnabled                bool
	ReaperInterval, OutboxInterval, ShutdownTimeout        time.Duration
	LangfuseBaseURL, LangfusePublicKey, LangfuseSecretKey  string
	LangfuseMode, LangfuseMetricsPath, LangfuseCompleteTag string
	LangfuseSupportsCost                                   bool
}

type LookupEnv func(string) string

func Load(get LookupEnv) (Config, error) {
	cfg := Config{
		DatabaseURL: get("DATABASE_URL"), HTTPAddr: value(get, "HTTP_ADDR", ":8080"), CursorSecret: get("CURSOR_SECRET"),
		OAuthIssuer: get("OAUTH_ISSUER"), OAuthAudience: get("OAUTH_AUDIENCE"), OAuthJWKSURL: get("OAUTH_JWKS_URL"),
		LangfuseBaseURL: get("LANGFUSE_BASE_URL"), LangfusePublicKey: get("LANGFUSE_PUBLIC_KEY"), LangfuseSecretKey: get("LANGFUSE_SECRET_KEY"),
		LangfuseMode: value(get, "LANGFUSE_MODE", "cloud"), LangfuseMetricsPath: get("LANGFUSE_METRICS_PATH"), LangfuseCompleteTag: get("LANGFUSE_COMPLETE_COVERAGE_TAG"),
	}
	var err error
	if cfg.MCPEnabled, err = boolean(get, "MCP_ENABLED", true); err != nil {
		return Config{}, err
	}
	if cfg.WebEnabled, err = boolean(get, "WEB_ENABLED", true); err != nil {
		return Config{}, err
	}
	if cfg.LangfuseEnabled, err = boolean(get, "LANGFUSE_ENABLED", false); err != nil {
		return Config{}, err
	}
	if cfg.LangfuseSupportsCost, err = boolean(get, "LANGFUSE_SUPPORTS_COST", true); err != nil {
		return Config{}, err
	}
	if cfg.ReaperInterval, err = duration(get, "REAPER_INTERVAL", 5*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.OutboxInterval, err = duration(get, "OUTBOX_INTERVAL", 3*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = duration(get, "SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
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
		for _, required := range [][2]string{{"OAUTH_ISSUER", cfg.OAuthIssuer}, {"OAUTH_AUDIENCE", cfg.OAuthAudience}, {"OAUTH_JWKS_URL", cfg.OAuthJWKSURL}} {
			if required[1] == "" {
				return Config{}, fmt.Errorf("%s is required when a transport is enabled", required[0])
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
