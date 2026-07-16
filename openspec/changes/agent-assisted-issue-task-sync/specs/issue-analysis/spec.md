## ADDED Requirements

### Requirement: Issue 分析任务幂等且可恢复
系统 SHALL 为每个 tenant、仓库、Issue revision、base commit 和分析 Agent Version 的唯一组合创建至多一个持久化分析任务，并以 lease、fencing generation、最大尝试次数和退避策略执行各分析阶段。

#### Scenario: 同一 Issue revision 被重复发现
- **WHEN** 轮询、手动触发或重试重复发现相同 Issue revision、base commit 和分析 Agent Version
- **THEN** 系统复用既有分析任务或稳定结果，不得创建重复 Task Specification

#### Scenario: 分析 worker 在 lease 后完成旧请求
- **WHEN** 原 worker 的 lease 已失效且新 worker 已取得更高 fencing generation
- **THEN** 系统拒绝原 worker 写入步骤结果，保留新 worker 的执行权

#### Scenario: 单个分析任务失败
- **WHEN** 某 Issue 的索引、模型或资格验证阶段失败
- **THEN** 系统记录结构化失败原因并按策略重试，且不得阻塞其它 Issue 分析任务

### Requirement: 分析绑定不可变仓库快照
系统 MUST 在分析开始前解析并固定 canonical repository、Issue revision 和 base commit；所有代码证据、索引、经验检索和资格验证 MUST 以该快照为准。

#### Scenario: 默认分支在分析期间推进
- **WHEN** 分析任务已固定 base commit 后默认分支出现新 commit
- **THEN** 当前分析继续使用原 base commit，新 commit 仅可触发新的分析任务

#### Scenario: base commit 无法读取
- **WHEN** 系统无法验证或检出分析任务记录的 base commit
- **THEN** 分析进入失败或待澄清状态且不得生成可发布任务

### Requirement: 仓库上下文使用可追溯的混合检索
系统 SHALL 从固定快照构建文件、符号、引用、依赖、测试和 Git 历史上下文，并 MUST 为提供给分析 Agent 的每条代码证据记录 commit、path、line span 与 content hash。

#### Scenario: 分析引用相关代码
- **WHEN** 分析结果声称某符号、文件或测试与 Issue 有关
- **THEN** 结果包含可在固定 base commit 解析到相同内容的证据引用

#### Scenario: 语言缺少精确语义 indexer
- **WHEN** 目标语言没有可用的 SCIP 或等价语义索引
- **THEN** 系统降级到语法图与文本检索，并在质量报告中降低相应证据覆盖状态，不得伪造精确引用关系

### Requirement: 分析结果是版本化结构化任务规格
系统 SHALL 生成不可变 Task Specification Version，至少包含问题诊断、影响范围、证据、建议方案、实施步骤、约束、非目标、风险、待澄清问题、验收标准、base commit、Issue revision 和所用经验版本。

#### Scenario: 分析成功生成规格
- **WHEN** 分析 Agent 完成 Issue 分析且输出通过 schema 校验
- **THEN** 系统持久化新的不可变规格版本及其完整 provenance，不得只保存自由文本摘要

#### Scenario: 必填结构缺失
- **WHEN** 模型输出缺少问题诊断、关键证据或验收标准等必填部分
- **THEN** 系统拒绝该输出进入质量门禁并记录可重试的 schema 失败

### Requirement: Analyzer 与 Critic 独立执行
系统 MUST 使用分别版本化的 Analyzer 和 Critic 配置执行任务生成与独立批判；Critic MUST 检查错误引用、遗漏影响、关键歧义、不可验证验收和方案过度限定。

#### Scenario: Critic 发现关键冲突
- **WHEN** Critic 对问题诊断、影响范围或关键验收提出 unresolved critical finding
- **THEN** 系统保留分析草稿和 finding，并阻止其自动发布

#### Scenario: Analyzer 与 Critic 使用同一基础模型
- **WHEN** 部署方为两阶段选择同一模型
- **THEN** 系统仍使用不同 Agent Version、Prompt、上下文和运行记录，且不得把 Analyzer 隐藏推理传给 Critic

### Requirement: 任务生成由固定版本 Pi Agent 执行
生产 Analyzer 与 Critic MUST 使用 Pi Agent Harness（`earendil-works/pi`）的程序化 SDK 在隔离 runner 中执行，并 MUST 将精确 Pi release/commit、依赖完整性、extension hash 和容器 digest 绑定到 Agent Version；不得使用浮动版本或解析交互式 CLI 文本作为任务规格。

#### Scenario: Analyzer 生成结构化任务规格
- **WHEN** Pi Analyzer 完成仓库分析
- **THEN** 它通过 AgentGuild 注册的终止型结构化工具提交唯一 Task Specification，系统校验 schema 后才接受结果

#### Scenario: Pi 只返回自然语言
- **WHEN** Pi session 结束但未调用结构化提交工具、调用多次或结果不符合 schema
- **THEN** 系统将运行标记为 provider failure，不得从 stdout 或 transcript 猜测正式任务字段

#### Scenario: Pi runtime 版本未固定
- **WHEN** 分析配置使用浮动 npm range、未固定 upstream source 或未固定容器 digest
- **THEN** 生产配置校验失败且 Analysis Worker 不得启动该任务

#### Scenario: Pi runtime 升级
- **WHEN** 管理员变更 Pi release、commit、SDK package、extension 或 image
- **THEN** 系统要求创建新 Agent Version，并保留旧版本运行可复现信息

### Requirement: Pi Agent 工具受 AgentGuild 沙箱约束
系统 MUST 在整个 Pi 进程外实施文件系统、进程、网络、资源和 credential 边界；Analyzer/Critic 默认只允许只读代码与证据工具，并 MUST 禁用 write、edit 和 unrestricted bash。

#### Scenario: Pi 请求未授权工具
- **WHEN** Pi Agent 请求写文件、执行非 allowlist 命令、访问 workspace 外路径或连接非模型 endpoint
- **THEN** 沙箱或 tool preflight 拒绝动作并记录安全审计，分析不得因此获得额外权限

#### Scenario: Pi 读取宿主配置
- **WHEN** Pi Agent 尝试访问宿主 `~/.pi`、provider auth、session、环境秘密或其它 tenant workspace
- **THEN** 资源在容器中不可见，且系统不得把不存在误判为允许访问

#### Scenario: Critic 运行
- **WHEN** Analyzer 已生成候选 Task Specification
- **THEN** 系统启动新的隔离 Pi session，仅传入候选规格和允许证据，不复用 Analyzer session 或 hidden reasoning

### Requirement: 分析明确表达不确定性
系统 MUST 区分已证实事实、基于证据的推断和未知项，并 SHALL 将缺失信息与待澄清问题持久化；不得用未经证实的内容填充正式事实字段。

#### Scenario: Issue 无法稳定复现
- **WHEN** 分析无法从固定快照和允许环境确认 Issue 描述的行为
- **THEN** 系统将复现状态和缺失条件写入待澄清问题，并禁止任务自动发布

#### Scenario: 人类补充澄清后重新分析
- **WHEN** 授权治理者补充缺失条件并触发重新分析
- **THEN** 系统创建新分析运行和新规格版本，保留此前运行及失败原因

### Requirement: 不可信输入不能控制分析工具
系统 MUST 将 Issue、代码、注释、文档、Git history 和仓库经验视为不可信数据，在无租户秘密、默认无外网、只读仓库和工具 allowlist 的隔离环境中执行分析。

#### Scenario: 代码注释包含工具调用指令
- **WHEN** 检索到的代码或文档要求模型泄露秘密、访问网络或修改仓库
- **THEN** 系统将其作为数据保留或标记风险，且拒绝超出分析策略的工具动作

#### Scenario: 分析输出疑似包含秘密
- **WHEN** 输出验证检测到凭证、私有数据或禁止公开的内容
- **THEN** 系统阻止持久化到公共任务规格并记录安全审计事件
