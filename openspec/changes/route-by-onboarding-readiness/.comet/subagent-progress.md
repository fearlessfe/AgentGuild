# Subagent Progress

- Change: `route-by-onboarding-readiness`
- Review mode: `standard`
- TDD mode: `tdd`

## Current task

- Plan task: `Task 2: 证明 manifest 地址只来自显式配置并更新部署文档`
- OpenSpec task: `1.2 补充 manifest 测试，证明 callback/setup URL 只使用配置值且不受请求头影响，并同步更新部署文档中的必填变量说明。`
- Stage: `done`
- Review/fix round: `0/1`
- Commit: `5f5524d66d9caf31d1e31a3cde57c748bafc3146`
- Changed files: `backend/internal/git/application/manifest_test.go`, `docs/local-dev-github-issue-sync.md`, `AGENTS.md`
- RED evidence: not applicable unless production behavior changes; this task adds regression coverage for an existing structural boundary.
- GREEN evidence: `go test ./internal/git/application -run TestManifestBuildContainsPermissionsAndCallbacks -count=1` and `go test ./internal/config ./internal/git/application -count=1` passed.
- Final review: pending
