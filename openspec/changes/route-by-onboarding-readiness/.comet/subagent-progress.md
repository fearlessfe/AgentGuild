# Subagent Progress

- Change: `route-by-onboarding-readiness`
- Review mode: `standard`
- TDD mode: `tdd`

## Current task

- Plan task: `Task 1: 严格校验并规范化 GitHub App 公网 origin`
- OpenSpec task: `1.1 先补充配置单元测试，覆盖 Web 模式缺失、非法及合法 GITHUB_APP_PUBLIC_BASE_URL，确认测试按预期失败后实现严格解析与规范化。`
- Stage: `done`
- Review/fix round: `0/1`
- Commit: `b75dc980b52cae11a4e3c6353c5506dd19388692`
- Changed files: `backend/internal/config/config.go`, `backend/internal/config/config_test.go`
- RED evidence: `go test ./internal/config -run 'TestLoad(Requires|Validates|Normalizes)GitHubAppPublicBaseURL' -count=1` failed because missing/invalid values were accepted and the trailing slash was not normalized.
- GREEN evidence: `GOCACHE=/tmp/agentguild-go-cache go test ./internal/config -count=1` passed.
- Final review: pending
