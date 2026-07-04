# Subagent Progress

Change: agent-onboarding-and-identity
Plan: docs/superpowers/plans/2026-07-03-agent-onboarding-and-identity.md
Review mode: thorough
TDD mode: tdd

## Current Task

Plan task: Task 8: 组装服务、配置与端到端验收
OpenSpec task: 3.3 完成凭证泄露、重放、越权和审计验收测试
Stage: done
Review/fix round: 3

## Implementation

Implementer: 019f28f0-c180-7f71-b665-8e1c1fda5d86 (Chandrasekhar)
Base commit: 23b2f2f
Brief: .superpowers/sdd/task-8-brief.md
Report: .superpowers/sdd/task-8-report.md
Changed files: pending
Commit: b9c261d test: verify agent onboarding and identity end to end
Changed files: backend/cmd/agentguild-api/main.go; backend/cmd/agentguild-api/main_test.go; backend/internal/config/config.go; backend/internal/config/config_test.go; backend/internal/acceptance/env.go; backend/internal/acceptance/identity_test.go; .superpowers/sdd/task-8-report.md
RED evidence: recorded in .superpowers/sdd/task-8-report.md (missing identity/OIDC/session/RSA config and acceptance harness)
GREEN evidence: recorded in .superpowers/sdd/task-8-report.md (target identity runtime/config/acceptance tests, identity/rest packages, backend build, diff check passed; full make verify blocked by sandbox)

## Review

Batch/thorough review: 019f2902-72ac-7aa2-b9f0-f7e8f587e648 (Linnaeus) requested changes
Open feedback: Important - tenant isolation acceptance test must create real cross-tenant data/request; Minor - credential leakage assertion should decode response and inspect persisted credential shape; Minor - report targeted GREEN should not imply cmd tests ran when filter reports no tests.
Fixer: 019f290b-af00-7770-b003-aa261f164b9c (Averroes)
Fix commit: 9b18437 fix: strengthen identity onboarding acceptance coverage
Review package: .superpowers/sdd/review-23b2f2f..9b18437.diff
Re-review: 019f2917-3182-74f1-966c-0b214759acba (Confucius) found no Critical/Important/Minor issues; diagnostics service unavailable with 503.
Coordinator verification: acceptance focus, identity/auth/rest/acceptance regression, original targeted command, backend race suite, frontend Vitest, make build, and git diff check passed locally. Playwright E2E blocked by sandbox local listener EPERM; escalated Playwright and Docker/PostgreSQL attempts rejected by approval service 503.

## Final Review Remediation

Final reviewer: 019f2928-5026-7361-aff9-c8213bc29550 (Russell)
Verdict: Not ready to archive
Open feedback:
- Critical: task/execution protected APIs only trust JWT scopes and do not perform real-time Agent status checks; Suspended/Revoked Agents can keep using unexpired Access Tokens for task operations.
- Important: revoked-token paths in identity refresh/heartbeat return STATE_CONFLICT instead of TOKEN_REVOKED, and acceptance tests currently expect the wrong error.
- Important: React registration form exposes owner_email even though the backend binds owner from the OIDC session and ignores owner_email.
- Important: React default scopes use tasks:read/tasks:write even though backend task operations use tasks:publish/tasks:claim/tasks:execute/tasks:cancel.
Fixer: 019f2ac2-c0a9-7660-8a00-586195687756 (Singer)
Fix commit: dc0ae47 fix: enforce agent status and align registration contract
Review package: .superpowers/sdd/review-7b5de74..dc0ae47.diff
Re-review: 019f2ad3-04af-7381-8241-b2d74e112c5c (Volta) found Critical remaining TOCTOU window in live-agent status checks.
Extra fixer: 019f2adb-9435-7c62-bef9-2a12879e4c3d (Bernoulli)
Extra fix commit: e28f147 fix: close agent status transaction window
Extra review package: .superpowers/sdd/review-dc0ae47..e28f147.diff
Extra re-review: 019f2b00-54e0-78d0-8986-d87290e73448 (Banach) found no Critical or Important issues; Ready to archive: Yes.
Coordinator verification: backend application/auth/identity/rest/mcp/acceptance tests passed; frontend Vitest passed; git diff check passed; `make build` hit sandbox Go cache permission, escalated retry was rejected by approval service 503, and `make build GOCACHE=/private/tmp/agentguild-go-build-cache` passed.
