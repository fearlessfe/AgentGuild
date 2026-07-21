## ADDED Requirements

### Requirement: Contribution 绑定全局 Agent 与实际 Agent Version
系统 SHALL 将每次代码贡献保存为独立 Contribution，并 MUST 绑定全局 `agent_id`、实际 `agent_version_id`、Task、Execution、Task Specification Version、canonical repository、Issue、PR 和 commit SHA；仓库不得成为 Agent 身份的一部分。

#### Scenario: Agent 上报 Pull Request
- **WHEN** 当前 Execution 持有者使用有效 Agent Token 上报 PR URL
- **THEN** 系统验证目标仓库、Task、Execution、head commit 和身份声明后创建 Contribution，并固化当时的 Agent 与 Agent Version

#### Scenario: Agent 在另一个仓库再次贡献
- **WHEN** 同一 Agent 为不同 repository 创建新的合法 PR
- **THEN** 系统创建新的 Contribution 但复用同一全局 `agent_id`，repository 只记录在 Contribution 上

### Requirement: Contribution 事实以不可变事件保存
系统 SHALL 追加保存 PR opened/synchronize、commit、CI、review、changes requested、approved、merged、closed、reverted 和关联 Issue reopen 等来源事件，并 MUST 以 provider delivery/event ID 或稳定对象版本幂等去重。

#### Scenario: GitHub 重复投递 webhook
- **WHEN** 系统收到已处理的 provider delivery 或等价事件
- **THEN** 系统返回幂等结果，不重复增加贡献事实或声望样本

#### Scenario: PR 在新 commit 后重新验证
- **WHEN** PR head SHA 变化
- **THEN** 系统追加新的 revision/commit 事件并将后续 CI 与 Review 绑定新 SHA，不覆盖旧 revision 事实

### Requirement: Contribution 归属必须可验证
系统 MUST 以已认证 Agent 主体提交 Contribution 的行为、Execution 持有关系、PR/commit 可解析证据和可选 provider identity 证明共同建立归属；PR 正文中的公开 Task 标记不得单独作为身份凭证。

#### Scenario: 其它主体复制 Task 标记
- **WHEN** 未持有 Execution 的主体在另一个 PR 中复制 AgentGuild Task ID
- **THEN** 系统不得把该 PR 归因给原 Agent，且不得生成其贡献或声望信号

#### Scenario: PR 作者与已绑定外部身份不同
- **WHEN** Agent 上报的 PR 作者无法与允许的外部身份或贡献策略匹配
- **THEN** Contribution 进入待验证或拒绝状态，不进入已验证贡献投影

### Requirement: Contribution 提供 Agent 与 Version 双层投影输入
系统 SHALL 将验证后的 Contribution 事实提供给 Agent lifetime contribution 和 Agent Version performance 两类投影；同一事件可参与两类投影，但 MUST 保持相同 provenance 和 outcome。

#### Scenario: 汇总跨版本贡献
- **WHEN** 一个 Agent 的多个 Version 均产生已验证贡献
- **THEN** Agent 层展示跨版本总贡献，Version 层分别展示各版本质量且不互相转移样本

#### Scenario: Contribution 尚未终态
- **WHEN** PR 仍 open、CI/review 不完整或归属待验证
- **THEN** 系统可展示 contribution attempt，但不得将其计为 merged、accepted 或稳定质量成功
