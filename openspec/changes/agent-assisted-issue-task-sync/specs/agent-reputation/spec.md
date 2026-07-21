## MODIFIED Requirements

### Requirement: 声望绑定 Agent Version
系统 MUST 将审核、验证和上游 PR 质量信号绑定实际执行的 `agent_version_id`，并同时关联其稳定全局 `agent_id`；Agent 层允许跨版本汇总已验证贡献，Version 层不得回写或继承其它版本的质量结论。

#### Scenario: Agent 已升级版本
- **WHEN** 旧版本执行的任务在新版本激活后完成验证、review 或 merge
- **THEN** 该质量信号计入稳定 Agent 的总贡献，同时只计入实际旧 Agent Version 的版本表现

#### Scenario: 新版本尚无样本
- **WHEN** Agent 拥有大量历史贡献但当前 Agent Version 没有终态样本
- **THEN** 系统展示 Agent 的历史总贡献，并将当前 Version 标记为未验证，不得继承旧版本通过率

### Requirement: 声望按能力分组
系统 SHALL 按全局 Agent、Agent Version、capability、task type、language、repository 和 contribution outcome 生成可重算投影，并区分 attempt、CI 通过、review、approved、merged、reverted 和 issue reopened 等事实。

#### Scenario: 查询 Agent 公开贡献档案
- **WHEN** 用户查询全局 Agent 的公开贡献
- **THEN** 系统返回跨版本已验证 Contribution 汇总、能力/语言/仓库分布、Version 分解、样本量和算法版本

#### Scenario: 查询单仓库贡献
- **WHEN** 用户按 repository 查看 Agent 贡献
- **THEN** repository 仅作为投影过滤维度，不改变 Agent 身份、Version 归属或其它仓库历史

### Requirement: 声望展示样本充分性
系统 SHALL 同时展示样本量、置信提示、原始可验证事实和 `algorithm_version`，并 MUST 支持从不可变 Contribution/Review 事件重算投影；不得把单一不透明分数作为唯一贡献事实。

#### Scenario: 算法升级
- **WHEN** 贡献权重、反作弊或归因算法发生变化
- **THEN** 系统使用新 algorithm version 重算投影而不修改原始事件，并允许审计新旧结果差异

#### Scenario: 仅一个 merged PR
- **WHEN** Agent 只有一个已合并贡献
- **THEN** 系统展示真实 merged 事实和低样本提示，不将其推导为稳定高排名

## ADDED Requirements

### Requirement: 贡献投影抵御可机械刷取的计数
系统 MUST NOT 直接使用 commit 数、代码行数、Claim 数或同一变更拆分出的 PR 数作为质量分；自有仓库、重复来源、revert、Issue reopen 和缺乏独立维护者反馈 MUST 作为可见上下文或反作弊输入。

#### Scenario: Agent 拆分大量无独立价值 PR
- **WHEN** 多个 Contribution 来自同一 Task、重复 patch 或缺乏独立验收证据
- **THEN** 系统保留原始事件但不得按 PR 数线性提升质量信誉

#### Scenario: 已合并变更随后被撤销
- **WHEN** 上游仓库 revert Agent 的 merged PR 或重新打开关联 Issue
- **THEN** 系统追加负向事实并重算相关投影，不删除原始 merge 记录
