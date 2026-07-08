# AgentGuild local dev environment
export DATABASE_URL="postgres://agentguild:agentguild@127.0.0.1:5432/agentguild?sslmode=disable"
export CURSOR_SECRET="$(openssl rand -base64 32)"
export SESSION_COOKIE_SECRET="$(openssl rand -base64 32)"
export AGENT_RSA_PRIVATE_KEY_PATH="$(pwd)/.local/agent-rsa.pem"

# OAuth2 settings (required when MCP_ENABLED or WEB_ENABLED; used for Agent Bearer token validation)
export OAUTH_ISSUER="https://dev-agentguild.local"
export OAUTH_AUDIENCE="agentguild-api"
export OAUTH_JWKS_URL="https://dev-agentguild.local/.well-known/jwks.json"

# Disable OIDC to use local admin fallback
export OIDC_TENANT_ID=""
export LOCAL_ADMIN_PASSWORD="local-dev-password-123"

# Optional but recommended
export MCP_ENABLED="true"
export WEB_ENABLED="true"
export HTTP_ADDR=":8080"

# Issue -> Task 同步
export SYNC_WORKER_INTERVAL="60s"
export SYNC_DEFAULT_DEADLINE="8760h"   # 365 天占位期限（D9）

# GitHub App Manifest 一键接入回调基址（需公网可达；本地可用隧道地址）
# 例：export GITHUB_APP_PUBLIC_BASE_URL="https://<your-tunnel>.trycloudflare.com"
export GITHUB_APP_PUBLIC_BASE_URL=""
