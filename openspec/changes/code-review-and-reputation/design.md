## Context

审核人需要在 AgentGuild 内同时看到任务目标、GitLab Diff、自动验证和修订历史。声望应来源于不可变审核事实，而不是可被覆盖的 Agent 当前配置。

## Goals / Non-Goals

**Goals:**

- 提供可追溯的 Diff 审核、行级评论、退回与评分。
- 将质量信号绑定 Submission、Execution 和 Agent Version。
- 按能力与任务类型聚合声望并展示样本量。

**Non-Goals:**

- 自动合并、GitLab 评论双向同步、单一全局总分和复杂推荐模型。

## Decisions

1. Review 绑定具体 Submission revision；新提交创建新 revision，不覆盖旧 Diff、评论或决策。
2. 自动验证硬门槛在应用服务层再次检查，前端不能通过隐藏按钮绕过。
3. React Diff 页面读取服务端规范化 diff；行评论使用文件路径、side、line 和 diff fingerprint 定位。
4. Rubric 版本化，评分记录各维度、总分、审核耗时和最终决策。
5. Reputation 使用异步可重算投影，按 Agent Version、capability、task type 聚合通过率、返工率、人工成本和样本量。

## Risks / Trade-offs

- [Diff 更新导致评论漂移] → 评论绑定 revision 与 diff fingerprint，跨 revision 只做显式映射。
- [少样本声望误导] → 展示样本量与置信提示，不输出虚假的精确排名。
- [聚合逻辑变更] → 保留原始审核事实和算法版本，允许重算。

## Migration Plan

先上线只读 Diff 与验证证据，再启用评论、决策和评分，最后启用声望投影；聚合层可停用而不影响审核事实。

## Open Questions

- React Diff 组件和初版置信区间算法在深度设计中选定。
