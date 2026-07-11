# Local Admin Production Compose Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a `docker compose up --build --wait` deployment in which Nginx is the only host-facing service and local-admin login works without external OIDC configuration.

**Architecture:** Compose starts PostgreSQL, an idempotent one-shot migration service, the Go API, and an Nginx-hosted React SPA in dependency order. Nginx proxies `/api`, `/oauth`, and `/mcp` to the internal backend; the backend and database have no published ports. Local mode uses a persistent container-generated RSA key for Agent tokens and does not construct an external JWKS verifier.

**Tech Stack:** Docker Compose, PostgreSQL 18.4, Go 1.26.4, React/Vite, Nginx, POSIX shell.

## Global Constraints

- Only `${APP_PORT:-8080}` on `frontend` is published to the host.
- `backend` and `postgres` communicate only through the Compose network.
- `LOCAL_ADMIN_PASSWORD` is at least 12 characters; `CURSOR_SECRET` and `SESSION_COOKIE_SECRET` are at least 32 bytes.
- No password, private key, token, or deployable default secret is committed or copied into an image layer.
- Database migrations are versioned, ordered, transactional, and safe to rerun against an existing volume.
- Images run as non-root users after required startup preparation.

---

### Task 1: Make local authentication independent of external JWKS

**Files:**
- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/config_test.go`
- Modify: `backend/cmd/agentguild-api/main.go`
- Modify: `backend/cmd/agentguild-api/main_test.go`

**Interfaces:**
- Consumes: `config.Config.LocalAdmin.Enabled`, local `OAUTH_ISSUER` and `OAUTH_AUDIENCE` values used to issue Agent access tokens.
- Produces: `buildTokenVerifier(cfg, publicKey) auth.TokenVerifier`, which includes the external JWKS verifier only when `OAUTH_JWKS_URL` is non-empty.

- [ ] **Step 1: Add failing config tests**

Add table cases proving that local-admin mode accepts an empty `OAUTH_JWKS_URL`, while non-local transports still reject it. Use a base environment containing `DATABASE_URL`, 32-byte secrets, `LOCAL_ADMIN_PASSWORD`, `OAUTH_ISSUER`, `OAUTH_AUDIENCE`, and a test RSA key path.

- [ ] **Step 2: Run the focused tests and confirm failure**

Run: `cd backend && go test ./internal/config -run 'TestLoad.*LocalAdmin' -count=1`

Expected: FAIL because `OAUTH_JWKS_URL is required when a transport is enabled`.

- [ ] **Step 3: Relax only the local-mode JWKS requirement**

Keep issuer and audience mandatory for locally issued Agent tokens. Require `OAUTH_JWKS_URL` only when `LocalAdmin.Enabled` is false; do not weaken OIDC validation or secret-length checks.

- [ ] **Step 4: Add verifier-construction tests**

Extract verifier composition from `buildIdentityRuntime` and test these exact cases:

```go
func TestBuildTokenVerifierLocalOnly(t *testing.T) {
    verifier := buildTokenVerifier(config.Config{OAuthIssuer: "http://agentguild.local", OAuthAudience: "agentguild"}, &testKey.PublicKey)
    require.NotNil(t, verifier)
}

func TestBuildTokenVerifierIncludesExternalJWKSWhenConfigured(t *testing.T) {
    verifier := buildTokenVerifier(config.Config{OAuthIssuer: "issuer", OAuthAudience: "audience", OAuthJWKSURL: server.URL}, &testKey.PublicKey)
    require.NotNil(t, verifier)
}
```

- [ ] **Step 5: Implement verifier composition and run tests**

Build the local RS256 verifier unconditionally. Append `auth.NewJWKSVerifier(...)` only when `cfg.OAuthJWKSURL != ""`; return the single local verifier directly in local-only mode.

Run: `cd backend && go test ./internal/config ./cmd/agentguild-api -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/config/config.go backend/internal/config/config_test.go backend/cmd/agentguild-api/main.go backend/cmd/agentguild-api/main_test.go
git commit -m "feat: support local-only authentication runtime"
```

### Task 2: Build production backend and migration containers

**Files:**
- Create: `.dockerignore`
- Create: `backend/Dockerfile`
- Create: `deploy/backend-entrypoint.sh`
- Create: `deploy/migrate.sh`
- Create: `deploy/migrate_test.sh`

**Interfaces:**
- Consumes: `/migrations/*.up.sql`, `DATABASE_URL`, and optional `AGENT_RSA_PRIVATE_KEY_PATH`.
- Produces: API image command `/usr/local/bin/backend-entrypoint`; migration image command `/usr/local/bin/migrate`; persistent key at `/var/lib/agentguild/agent-rsa.pem`.

- [ ] **Step 1: Add shell contract tests**

`deploy/migrate_test.sh` must assert that migration files sort from `000001` through `000012`, reject duplicate numeric prefixes, and that both scripts pass `sh -n`.

- [ ] **Step 2: Run contract tests and confirm failure**

Run: `sh deploy/migrate_test.sh`

Expected: FAIL because the entrypoint and migration script do not exist.

- [ ] **Step 3: Implement the migration runner**

Use `psql -v ON_ERROR_STOP=1`. Acquire `pg_advisory_lock(hashtext('agentguild-schema-migrations'))`, create `schema_migrations(version text primary key, applied_at timestamptz default clock_timestamp())`, and for each sorted `*.up.sql`, execute the SQL and version insert in one transaction when the version is absent. Release the lock on exit.

- [ ] **Step 4: Implement backend key preparation**

When the configured key file is absent, create its parent directory and run `openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$AGENT_RSA_PRIVATE_KEY_PATH"`; set mode `0600`, then `exec /usr/local/bin/agentguild-api`. Never print the key.

- [ ] **Step 5: Implement multi-stage images and ignore rules**

Build `./cmd/agentguild-api` with Go 1.26.4. The API runtime contains CA certificates, OpenSSL, the entrypoint, and an unprivileged `agentguild` user. The migration target contains PostgreSQL 18.4 client tools, migration SQL, and the migration runner. Exclude `.git`, `.env*` except `.env.example`, frontend build/test artifacts, coverage, and editor files from build context.

- [ ] **Step 6: Verify scripts and images**

Run:

```bash
sh deploy/migrate_test.sh
docker build --target backend -t agentguild-backend:test -f backend/Dockerfile .
docker build --target migrate -t agentguild-migrate:test -f backend/Dockerfile .
```

Expected: contract tests PASS and both images build successfully.

- [ ] **Step 7: Commit**

```bash
git add .dockerignore backend/Dockerfile deploy/backend-entrypoint.sh deploy/migrate.sh deploy/migrate_test.sh
git commit -m "build: add backend and migration containers"
```

### Task 3: Build the Nginx frontend and same-origin gateway

**Files:**
- Create: `frontend/Dockerfile`
- Create: `frontend/nginx.conf`
- Create: `frontend/nginx_test.sh`
- Modify: `frontend/src/features/auth/LoginPage.tsx`
- Modify: `frontend/src/features/auth/LoginPage.test.tsx`

**Interfaces:**
- Consumes: backend DNS name `backend:8080` and frontend build output `frontend/dist`.
- Produces: host-facing routes `/`, `/healthz`, `/api/*`, `/oauth/*`, and `/mcp`.

- [ ] **Step 1: Add failing login-page tests**

Assert the default page presents local administration as the primary login method and does not render the enterprise OIDC button when no public OIDC runtime configuration exists.

- [ ] **Step 2: Run the focused frontend test and confirm failure**

Run: `cd frontend && npm test -- --run src/features/auth/LoginPage.test.tsx`

Expected: FAIL because the OIDC button is currently always rendered.

- [ ] **Step 3: Simplify the default login page**

Keep POST `/oauth/local/login`, `credentials: "same-origin"`, password error handling, and redirect to `/agents`. Change the card copy to local administration and remove the unconditional OIDC action and stale API note.

- [ ] **Step 4: Add and run Nginx configuration tests**

`frontend/nginx_test.sh` must check for SPA fallback, a no-store API policy, forwarded request headers, `/api/` prefix removal to backend root routes, direct `/oauth/` and `/mcp` proxying, and the absence of `listen` directives other than container port 8080.

Run: `sh frontend/nginx_test.sh`

Expected before implementation: FAIL because `nginx.conf` does not exist.

- [ ] **Step 5: Implement the frontend image and gateway**

Use a Node builder to run `npm ci && npm run build`, then copy `dist` and `nginx.conf` into an unprivileged Nginx image. Configure `try_files $uri $uri/ /index.html`, `/healthz`, proxy timeouts, `X-Request-ID`, `X-Forwarded-*`, `Authorization`, and `Cache-Control: no-store` on proxied responses.

- [ ] **Step 6: Verify frontend behavior and image**

Run:

```bash
cd frontend && npm test -- --run src/features/auth/LoginPage.test.tsx && npm run build
cd .. && sh frontend/nginx_test.sh
docker build -t agentguild-frontend:test -f frontend/Dockerfile frontend
```

Expected: all tests PASS and the image builds.

- [ ] **Step 7: Commit**

```bash
git add frontend/Dockerfile frontend/nginx.conf frontend/nginx_test.sh frontend/src/features/auth/LoginPage.tsx frontend/src/features/auth/LoginPage.test.tsx
git commit -m "build: add frontend gateway container"
```

### Task 4: Wire Compose and operator configuration

**Files:**
- Modify: `docker-compose.yml`
- Create: `.env.example`
- Create: `deploy/compose_config_test.sh`
- Modify: `README.md` if present; otherwise create `docs/docker-compose.md`

**Interfaces:**
- Consumes: images and commands from Tasks 2-3.
- Produces: services `postgres`, `migrate`, `backend`, and `frontend`; volume `postgres-data`; volume `agentguild-keys`.

- [ ] **Step 1: Add a failing Compose static test**

Render Compose with non-secret test values and assert with `docker compose config --format json` that only `frontend` has published ports, `backend` waits for successful migration, `frontend` waits for backend health, and PostgreSQL plus backend have no host port bindings.

- [ ] **Step 2: Run it and confirm failure**

Run: `sh deploy/compose_config_test.sh`

Expected: FAIL because the current Compose publishes PostgreSQL and lacks the other services.

- [ ] **Step 3: Implement Compose dependency and health semantics**

Configure:

```text
postgres (healthy) -> migrate (completed successfully) -> backend (healthy) -> frontend (healthy)
```

Set backend `DATABASE_URL` to `postgres://agentguild:${POSTGRES_PASSWORD}@postgres:5432/agentguild?sslmode=disable`, default issuer to `http://agentguild.local`, default audience to `agentguild`, enable web and MCP, and mount `agentguild-keys` at `/var/lib/agentguild`. Publish only `${APP_PORT:-8080}:8080` on frontend.

- [ ] **Step 4: Add safe operator configuration and documentation**

`.env.example` contains descriptive placeholders for the database password, 32-byte cursor/session secrets, and 12-character local password, plus non-secret local tenant defaults. Document `cp .env.example .env`, replacing every placeholder, `docker compose up --build --wait`, login URL, status inspection, logs, and `docker compose down` without volume deletion.

- [ ] **Step 5: Verify rendered configuration**

Run: `sh deploy/compose_config_test.sh`

Expected: PASS with exactly one published port owned by `frontend`.

- [ ] **Step 6: Commit**

```bash
git add docker-compose.yml .env.example deploy/compose_config_test.sh docs/docker-compose.md
git commit -m "build: wire production compose stack"
```

### Task 5: Prove startup, persistence, isolation, and login

**Files:**
- Modify only files found defective by verification.

**Interfaces:**
- Consumes: the complete Compose stack.
- Produces: verified deployable repository state and pushed Git branch.

- [ ] **Step 1: Create untracked verification secrets**

Copy `.env.example` to ignored `.env`, replace placeholders with generated random values, and use a local administrator password used only for this verification. Confirm `git status --short` does not list `.env`.

- [ ] **Step 2: Build and start the complete stack**

Run: `docker compose up --build --wait`

Expected: `postgres`, `backend`, and `frontend` report healthy; `migrate` exits with code 0.

- [ ] **Step 3: Verify host isolation and gateway behavior**

Run `docker compose ps --format json` and inspect published ports. Only frontend may publish `${APP_PORT:-8080}`. Request `/healthz`, a React deep link, and an unauthenticated `/api/v1/agents`; expect 200, SPA HTML, and 401 respectively.

- [ ] **Step 4: Verify local login through Nginx**

Set `VERIFY_PASSWORD` to the same untracked value written to `.env`. POST JSON generated from that variable to `/oauth/local/login` with a cookie jar, expect 200 and an HttpOnly session cookie, then request `/api/v1/agents` with that cookie and expect 200. Do not print the variable.

- [ ] **Step 5: Verify migration idempotency and persistence**

Run `docker compose restart`, wait for health, and confirm the authenticated API remains reachable and `schema_migrations` contains exactly one row per migration version.

- [ ] **Step 6: Run repository verification**

Run:

```bash
cd backend && go build ./... && go test -race ./... -count=1
cd ../frontend && npm run build && npm test -- --run
cd .. && git diff --check
```

Expected: all commands PASS.

- [ ] **Step 7: Stop without deleting persistent data**

Run: `docker compose down`

Expected: containers and network are removed; named volumes remain.

- [ ] **Step 8: Commit verification fixes if any**

Inspect `git diff --name-only`, stage only the implementation files changed during verification with explicit paths, then run `git commit -m "fix: harden compose deployment verification"`. Skip this step when verification required no fixes.

- [ ] **Step 9: Configure and push the requested remote**

If no remote exists, add `origin` as `https://github.com/Hephaestus-Automation/AgentGuild.git`; if `origin` points elsewhere, add this URL as `hephaestus`. Fetch before pushing to detect unrelated remote history, then push the current branch without force.

Expected: the remote contains the verified current branch and all commits; no secrets are tracked.
