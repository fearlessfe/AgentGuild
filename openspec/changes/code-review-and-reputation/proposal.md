## Why

自动验证只能确认确定性门槛，代码是否值得采纳仍需可追溯的人工审核。审核结果还必须沉淀为绑定具体 Agent Version 的质量信号，避免形成不可解释的单一总分。

## What Changes

- 增加 React 代码审核页、文件树、Diff 和行级评论。
- 增加基于 rubric 的评分、退回修改、重新提交和最终决策。
- 增加不可覆盖的 Submission 修订与 Review 审计记录。
- 增加按能力和任务类型聚合的基础声望。
- 首版评论仅保存在 AgentGuild，不自动合并代码或双向同步 GitLab 评论。

## Capabilities

### New Capabilities

- `code-review`: 定义审核资格、证据展示、行级评论、退回、通过和不可绕过的验证门槛。
- `agent-reputation`: 定义按 Agent Version、能力和任务类型统计的质量信号与样本量。

### Modified Capabilities

无。

## Impact

影响 Go Review 与 Reputation 模块、React 审核界面、Diff 数据读取、PostgreSQL 审核和聚合数据。依赖 GitLab 交付与自动验证结果。
