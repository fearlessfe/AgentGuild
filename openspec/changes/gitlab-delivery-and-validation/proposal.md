## Why

Coding MVP 必须把 GitLab commit 作为代码成果的事实来源，并在人工审核前验证提交来源、改动边界和自动检查结果。客户端 Agent 需要通过 MCP 提交 commit 引用与结构化证据，而不是直接上传代码包。

## What Changes

- 增加任务级短期 GitLab 凭证和受限工作分支。
- 增加 `branch + commit SHA` 成果提交及 GitLab 真实性校验。
- 增加 Diff、Pipeline 和 commit metadata 同步。
- 增加独立 Worker 进程执行构建、测试、静态分析和安全检查。
- 增加 MCP 成果提交与验证状态查询工具，复用 REST 状态机和权限。
- 禁止直接上传代码文件、通用成果包及绕过验证硬门槛。

## Capabilities

### New Capabilities

- `gitlab-delivery`: 定义短期 GitLab 凭证、受限分支、commit 提交和不可变交付引用。
- `submission-validation`: 定义提交校验、异步验证流水线、结果可见性和硬门槛。

### Modified Capabilities

无。

## Impact

影响 Go GitLab 集成与 Submission 模块、验证 Worker、PostgreSQL 作业和结果数据、MCP 工具及 GitLab CI 对接。依赖身份和任务生命周期 changes。
