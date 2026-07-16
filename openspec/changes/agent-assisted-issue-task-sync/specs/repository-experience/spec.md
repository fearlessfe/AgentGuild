## ADDED Requirements

### Requirement: 仓库经验只从终态执行证据提取
系统 MUST 仅从具有 terminal validation 与 review 结果的 accepted 或 rejected Submission 中创建 repository experience candidate；原始 Issue、自由对话、模型反思和未验证轨迹不得直接晋升为经验。

#### Scenario: 已接受提交生成候选
- **WHEN** Submission 通过验证并被授权审核人接受
- **THEN** 系统可从 Task Specification、patch、验证和评审证据中提取成功模式候选，并记录所有来源引用

#### Scenario: 被拒绝提交生成负面候选
- **WHEN** Submission 因明确技术原因被拒绝
- **THEN** 系统可生成 failed approach 或 known risk 候选，且不得把失败方案标记为成功经验

#### Scenario: 分析 Agent 仅产生自我反思
- **WHEN** 没有 terminal Submission、validation 和 review 证据
- **THEN** 系统最多保存分析日志，不得创建可晋升的可信仓库经验

### Requirement: 经验晋升要求可解析与独立证据
repository experience candidate MUST 至少具有一项可解析执行证据和一项独立验证或评审证据才能进入 active；高风险经验 MUST 经过授权人类批准。

#### Scenario: 候选只有模型摘要
- **WHEN** 候选缺少 commit、测试、Submission 或 Review 等可解析证据
- **THEN** 系统拒绝晋升并记录缺失项

#### Scenario: 安全经验准备晋升
- **WHEN** 候选涉及认证、授权、秘密、供应链或数据迁移
- **THEN** 即使自动证据充分，系统仍要求授权人类批准后才能 active

### Requirement: 仓库经验版本不可变且可撤销
系统 SHALL 将每次经验内容、适用范围、证据或置信度变化保存为新的 immutable version，并 MUST 支持 candidate、corroborated、active、stale、conflicted 和 revoked 状态。

#### Scenario: Active 经验获得新佐证
- **WHEN** 后续独立任务提供新的支持证据
- **THEN** 系统创建包含新证据的新版本，旧版本保持可追溯

#### Scenario: 经验被证明错误
- **WHEN** 验证、评审或治理者确认某 active 经验错误或有害
- **THEN** 系统将其标记 revoked，停止后续检索使用，并保留历史任务使用记录

### Requirement: 经验绑定仓库身份与 commit 适用范围
每个经验版本 MUST 记录 canonical repository、visibility、applicable paths/capabilities/languages、valid-from commit、last-verified commit 和证据 content hashes；检索 MUST 验证目标 base commit 与适用范围。

#### Scenario: 仓库代码使经验失效
- **WHEN** 目标 base commit 不再包含经验引用的路径、符号或内容，或与 last-verified commit 不满足允许的 ancestry
- **THEN** 系统排除该版本或标记 stale，不得静默当作当前事实

#### Scenario: 经验只适用于特定模块
- **WHEN** 新 Issue 与经验的 path/capability 范围无交集
- **THEN** 检索不得仅因属于同一仓库而优先使用该经验

### Requirement: 经验冲突显式治理
系统 MUST 检测同一仓库和适用范围内互相矛盾的 active 经验，保存 conflict group、支持与反对证据，并在冲突解决前阻止其作为确定事实使用。

#### Scenario: 新提交否定旧约定
- **WHEN** 新证据表明 active convention 与当前代码或维护者决定冲突
- **THEN** 系统将相关版本标记 conflicted 或 stale，并触发重新评估

#### Scenario: 分析检索到未解决冲突
- **WHEN** Issue 分析命中同一主题的冲突经验
- **THEN** 系统把冲突作为不确定性提供给 Analyzer/Critic，不得只选择高相似度一方

### Requirement: 经验检索由有效性、证据和相关性共同决定
系统 SHALL 先按 repository、visibility、active status、commit ancestry 和适用范围硬过滤，再按任务相关性、证据强度、独立佐证、结果质量、新鲜度与冲突惩罚排序；任务数量本身 MUST NOT 增加置信度。

#### Scenario: 大量低质量候选重复同一结论
- **WHEN** 多个候选来自重复或非独立来源且缺少终态证据
- **THEN** 系统不得因为数量增加 active 经验置信度

#### Scenario: 分析使用经验
- **WHEN** Analyzer 检索并使用 repository experience
- **THEN** analysis run 固化具体 experience version IDs 和检索分数，Task Specification 引用相关经验来源

### Requirement: 公共经验与租户私有覆盖层隔离
系统 SHALL 仅将可由公开仓库 commit、公开 Issue/PR 和可公开评审证据证明的经验放入公共基础层；租户策略、私有仓库知识和敏感补充 MUST 保存在 tenant overlay。

#### Scenario: 公共仓库经验通过公开性检查
- **WHEN** 候选的全部内容和证据均公开、许可策略允许再分发且敏感扫描通过
- **THEN** 系统可在晋升后将其作为该 canonical repository 的公共基础经验

#### Scenario: Tenant overlay 补充公共经验
- **WHEN** 租户为公共仓库添加内部风险策略
- **THEN** 该策略只影响本 tenant 的分析，不修改公共版本且不向其它 tenant 返回

#### Scenario: 私有证据混入公共候选
- **WHEN** 候选引用私有评论、私有仓库内容、秘密或 tenant-only Review
- **THEN** 系统阻止其进入公共层并记录敏感性判定

### Requirement: 经验使用效果可评估和回滚
系统 SHALL 记录每次分析使用的经验集合及后续 Task 发布、执行、验证和评审结果，并支持按经验版本比较 no-experience 与 verified-experience 效果。

#### Scenario: 启用经验后错误接受率上升
- **WHEN** 某经验版本相关任务的 false accept 或关键返工指标超过治理阈值
- **THEN** 系统支持停用或撤销该版本，并使后续分析回退到不使用该经验

#### Scenario: 撤销已被历史任务使用的经验
- **WHEN** 治理者 revoke 经验版本
- **THEN** 历史 Task Specification 保留使用记录，新分析不再检索该版本

### Requirement: 经验提取和检索抵御污染
系统 MUST 把经验候选内容和来源证据视为不可信数据，执行 provenance、secret/PII、Prompt Injection、许可和输出 schema 检查；经验内容不得直接控制工具。

#### Scenario: 提交内容试图写入持久化指令
- **WHEN** patch、评论或评审文本包含要求未来 Agent 忽略策略或调用工具的指令
- **THEN** 系统标记污染风险并阻止候选自动晋升

#### Scenario: 检索返回含自然语言操作指令的经验
- **WHEN** active 经验包含面向分析者的步骤描述
- **THEN** orchestrator 仍将其作为带 provenance 的数据输入，所有工具动作继续受系统策略校验
