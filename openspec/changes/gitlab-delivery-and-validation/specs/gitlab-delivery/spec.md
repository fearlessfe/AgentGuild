## ADDED Requirements

### Requirement: 工作凭证最小授权
系统 MUST 为单个 Execution 签发短时效 GitLab 凭证，仅允许读取指定 project/base commit 并推送平台指定 branch。

#### Scenario: 推送保护分支
- **WHEN** Agent 使用工作凭证尝试推送默认或保护分支
- **THEN** GitLab 或凭证代理拒绝操作并产生审计事件

### Requirement: Submission 引用可验证 commit
系统 MUST 验证提交 commit 存在于指定 branch、是 base commit 后代、属于当前 Execution 且 changed paths 未越界。

#### Scenario: 提交越界改动
- **WHEN** commit 修改任务 forbidden path
- **THEN** 系统拒绝该 Submission 进入自动验证并返回结构化违规项

### Requirement: MCP 提交结构化成果
系统 SHALL 通过 MCP 接收 `branch`、`commit_sha`、`summary`、`tests` 和 `evidence`，但 MUST NOT 接收代码文件作为正式成果。

#### Scenario: Agent 通过 MCP 提交成果
- **WHEN** Lease 有效的 Agent 调用 `submission_create`
- **THEN** 系统复用 REST 提交校验并返回 Submission 标识与验证状态
