## Context

当前同步引擎按规则拉取 GitHub Issue 后，直接把 `title` 和 `body` 写入 system Task，并在同一次同步中记录 Issue→Task 映射。Task 内容虽然已有 problem、constraints、requirements 等字段，但同步链路没有仓库定位、方案分析、验收标准生成、独立验证或发布质量门禁。现有 `agentexperience` 以 tenant 和执行 Agent 为作用域，保存人工审核的经验候选，不能表达公共仓库知识、commit 适用范围、冲突与过期。

项目已经具备可复用基础：tenant-scoped repository onboarding 与 Git resolver、不可变 Agent Version、PostgreSQL 事务和 worker lease、受控 Git workspace/validation runner、Submission 自动验证、逐条代码评审、outbox、Langfuse 成本观测以及人类治理界面。新能力必须保持 Git 为代码事实来源、多租户默认隔离、任务领取时契约不可变，并把 Issue、代码、注释、文档和历史经验全部视为不可信输入。

调研给出四点约束：SWE-bench Verified 说明任务可解性需要固定环境与人工验证；Agentless 说明定位、生成、验证的显式阶段比不透明的全自主循环更易解释和控制；Aider repo map、Tree-sitter 与 SCIP 说明代码图和 token-budget 检索应作为仓库理解骨架；ExpeL、Reflexion 与 Voyager 说明成功和失败反馈可以沉淀经验，但生产系统还需要证据、版本、过期、冲突和撤销治理。

## Goals / Non-Goals

**Goals:**

- 将 Issue 同步改造成持久化、可重试的“快照→索引→分析→批判→资格验证→发布”流水线。
- 只发布具备可解析证据、明确不确定性和可判定验收标准的任务；关键歧义导致 fail closed。
- 让执行 Agent 领取不可变 Task Specification Version，后续 Issue 或经验变化不得静默修改执行契约。
- 使用已验证的公共仓库经验提高后续分析质量，同时支持租户私有策略覆盖、commit 适用范围和撤销。
- 逐条关联验收标准、自动验证、人工评审与最终结论，使任务质量和经验收益可度量。
- 在现有 Go/PostgreSQL/worker 架构内落地，并保持模型提供者、代码索引器和对象存储可替换。

**Non-Goals:**

- 不承诺模型分析永远正确；平台承诺的是证据化流程、质量门禁和可审计的不确定性。
- 不自动合并第三方提交，不替代仓库维护者的最终治理权。
- 首版不训练或微调基础模型，不以模型参数保存仓库知识。
- 首版不采用完整 GraphRAG 作为代码主索引，也不要求所有语言立即具备精确语义索引。
- 不把原始 Issue、模型自我反思或未验证执行轨迹直接晋升为可信经验。
- 不将私有仓库内容、租户策略或秘密发布到公共经验层。

## Decisions

### D1: 使用持久化分阶段分析流水线，而非单个长生命周期自主 Agent

每个唯一的 `(tenant, repository, issue_number, issue_revision, base_commit, analyzer_agent_version)` 创建一个 `issue_analysis_job`。Job 按以下步骤推进：

```text
discovered -> snapshot -> index -> localize -> analyze -> critique
           -> qualify -> draft_ready -> published | needs_clarification | rejected
```

worker 复用 validation job 的短事务 claim、lease、attempt 和退避模式，但模型调用、Git 操作与容器执行 MUST 在事务外运行。每次保存步骤结果时使用 lease owner 和 fencing generation 防止过期 worker 覆盖新结果。同步事务只负责更新 Issue revision 并幂等写入 job/outbox；成功入队后才推进同步水位。

选择显式阶段是因为各阶段可以独立重试、限时、计费和审核，也可以分别评测代码定位、任务生成与质量判定。备选的单 Agent 自主循环实现更快，但难以区分检索失败、推理失败和验证失败，也更容易被不可信内容影响工具调用，因此不采用。

### D2: 所有分析绑定固定 Git commit，并使用确定性代码图作为检索骨架

创建分析 job 时通过已接入仓库解析默认分支当前 commit，并将其固定为 `base_commit`。分析 workspace 只读检出该 commit，不跟随分支漂移。索引按 `(repository_id, base_commit, indexer_version)` 内容寻址并复用。

首版索引包含文件清单、语言、Tree-sitter 符号、imports、测试文件映射、构建清单和 Git history。可用 SCIP indexer 的语言增加 definition/reference/implementation 边；不支持的语言仍可使用语法图与文本检索。检索顺序为精确路径/符号、代码图邻居、全文相关性，最后才是可选 embedding 相似度。每个返回片段必须携带 commit、path、line span 和 content hash。

PostgreSQL 保存索引 manifest、可查询元数据和全文索引；较大的压缩图、日志与模型原始响应通过 `ArtifactStore` 接口保存，开发环境可使用本地文件实现，生产使用 S3-compatible 实现。首版不引入 GraphRAG，原因是代码已有确定结构且 GraphRAG 索引成本高；它可在后续用于大量非结构化设计文档。

### D3: 使用固定版本的 Pi Agent Harness 执行 Analyzer 与 Critic

任务生成运行时采用 Pi Agent Harness 的 `@earendil-works/pi-coding-agent` SDK 和 `@earendil-works/pi-agent-core`，仓库为 `earendil-works/pi`（原 `badlogic/pi-mono`）。调研时基线版本为 `v0.80.8`；实现必须在 lockfile 中固定精确 npm 版本，在构建记录中固定 upstream tag/commit 和 OCI image digest，不得运行浮动 `main` 或未固定 `latest`。Pi 升级必须创建新的 Agent Version 并通过离线/shadow 回归后才能 promote。

Go `AnalysisWorker` 通过模型无关的 `TaskAnalysisProvider`/`TaskCritiqueProvider` port 调用独立 Node Pi runner，不把 TypeScript runtime 嵌入 Go 进程。runner 使用 `createAgentSession()`、`SessionManager.inMemory()` 和自定义 ResourceLoader，每个 job 创建一次性 session；输入 job manifest 和输出 artifact 使用版本化 JSON 协议。Pi session 不作为业务状态来源，Go/PostgreSQL 继续负责 lease、重试、Task Specification、质量报告和状态迁移。

```text
Go AnalysisWorker
  -> pinned OCI Pi runner (Node >= 22.19)
       -> in-memory Analyzer session
       -> read/grep/find/ls + evidence tools
       -> submit_task_specification(TypeBox schema, terminate=true)
  <- structured result + filtered audit events + usage
```

Analyzer 只启用 `read`、`grep`、`find`、`ls` 和 AgentGuild 自定义 evidence/experience tools，默认禁用 `write`、`edit` 和 `bash`。Pi extension 注册 `submit_task_specification` 终止型工具，以 TypeBox schema 验证最终任务规格；未调用该工具、只输出自然语言或输出多份结果均视为 provider failure。`beforeToolCall` 再执行路径、参数和工具 allowlist，不能把 Pi 自带工具视为权限边界。

分析分为 localization 与 specification synthesis。Critic 由新的 Pi session 执行，独立读取 Issue、证据集合和生成的规格，检查错误引用、遗漏影响面、不可验证验收、过度限定实现与关键歧义；它不读取 Analyzer session、隐藏推理或未经筛选的 transcript。Qualification Solver 也使用独立 Pi session，但运行在单独可写的 copy-on-write workspace，仅开放受控 `bash` 与编辑工具，生成 patch 永不返回给公共领取 Agent。

每次运行绑定已 promote 的不可变 Agent Version，并记录 Pi repo/tag/commit、npm package/shrinkwrap integrity、container digest、extension hash、模型、thinking level、Prompt、Skill、工具策略和 token/cost。持久化 Pi 的工具调用、可公开文本、错误和最终结构化结果，但 MUST NOT 将 provider hidden reasoning 或 thinking delta 作为仓库经验或传给 Critic。

Pi 官方明确不提供文件系统、进程、网络或凭证权限系统，因此整个 Pi 进程必须运行在 AgentGuild 控制的容器/沙箱内。workspace 只读挂载固定 base commit，默认无外网，仅允许访问配置的模型 endpoint；不挂载宿主 `~/.pi`、`auth.json` 或 session 目录。ModelRuntime 使用内存 credential store 或短期 runtime credential，凭证不写入 artifact、Prompt、工具结果或 session transcript。

Issue、代码、评论、文档和经验作为带 provenance 的 DATA block 输入。备选方案包括抓取 Pi CLI stdout、使用持久 Pi 交互 session 或直接使用通用模型 API；前两者缺乏稳定结构契约并扩大记忆污染面，后者不满足指定 Pi Agent runtime 的要求，因此不采用。provider port 仍保留，以便测试使用 deterministic fake，而生产 Analyzer/Critic 必须配置 Pi 实现。

### D4: Task Specification Version 是领取和验证的不可变执行契约

新增 `issue_analysis_runs`、`task_specification_versions`、`acceptance_criteria`、`analysis_evidence_refs` 和 `task_quality_reports`。Task Specification 至少包含：

- 问题诊断、影响范围与证据引用；
- 建议方案、实施步骤、约束、非目标、风险和待澄清问题；
- base commit、Issue revision、分析/批判 Agent Version；
- 排序稳定的验收标准与验证方式；
- 使用的 repository experience version IDs。

Issue 来源 Task 首先以 draft 创建。质量门禁通过后由 system actor 发布为 open。Claim 原子地把当前 `task_specification_version_id` 写入 Execution；Submission、validation job 和 Review 沿用该版本。Issue 更新时生成新分析和新规格：draft/open 且未领取的 Task 可显式替换版本；存在活动 Execution 时旧版本保持不变，重大变化标记 `source_changed`，由策略决定继续、取消或创建替代 Task。

### D5: 发布门禁由不可绕过的硬条件和可解释软评分组成

质量报告包含 hard gates、soft dimensions、证据和失败原因。以下任一条件失败，Task MUST 保持 draft：

- base commit、Issue revision 或关键证据不可读取；
- 引用 path/line/content hash 无法在固定 commit 解析；
- 存在未解决的 critical ambiguity 或 analyzer/critic 关键冲突；
- 任一 critical acceptance criterion 缺少允许的 verifier 或明确人工判定规则；
- 验证命令超出 allowlist、依赖未固定或无法在受控环境启动；
- 输出 schema、长度、敏感信息或 Prompt Injection 检查失败。

软评分覆盖 evidence coverage、localization confidence、acceptance determinism、solution/criteria consistency、risk coverage 和 experience support。分数用于排序、告警和决定是否需要人工复核，不得抵消 hard gate 失败，也不得对外表述为“正确率保证”。公共自动分发要求所有 critical criteria 可自动判定；含关键人工标准的任务必须先由授权人复核。

### D6: 验收标准是可执行的一等对象，并支持高保证影子求解

每条 criterion 包含稳定 ID、statement、criticality、verifier kind、command/query、expected result、baseline observation、evidence refs 和 timeout。verifier kind 首版支持 `command`、`static`、`behavioral` 与 `manual`。资格验证阶段在 base commit 上确认命令可启动、静态引用存在，并在适用时验证缺陷观察或新增 probe 在 baseline 上失败。

Submission validation 从 Execution 绑定的规格版本生成 steps，逐条保存结果，不依赖任务当前最新版本。隐藏验证材料与公开任务说明分离，且验收标准描述行为而不是泄露唯一实现。高风险或高奖励任务可启用 qualification solver：由第三个隔离 Agent 根据公开规格尝试影子求解；其失败不会证明任务不可解，但会触发人工复核，其成功 patch 永不提供给领取 Agent。

### D7: 仓库经验采用“候选→验证→版本化知识”模型

新增 `repository_experience_candidates`、`repository_experience_versions`、`repository_experience_evidence` 和 `repository_experience_conflicts`。经验类型首版包括 architecture fact、change pattern、validation recipe、convention、known risk、successful approach 和 failed approach。每条版本记录 canonical repository、visibility、summary、structured content、applicable paths/capabilities/languages、valid-from commit、last-verified commit、evidence refs、confidence dimensions、sensitivity class、status、extractor Agent Version 和 reviewer。

经验生命周期为：

```text
candidate -> corroborated -> active -> stale | conflicted | revoked
```

只有带 terminal validation/review 的 accepted 或 rejected Submission 才能生成候选。原始 Issue 和模型反思只能作为线索，不能作为晋升证据。晋升至少需要一项可解析执行证据和一项独立验证/评审证据；安全、权限、数据迁移等高风险经验要求人工批准。失败提交生成负面经验，避免后续分析重复无效方案。

检索先按 canonical repo、visibility、commit ancestry、path/capability 和状态硬过滤，再按任务相关性、证据强度、独立佐证、结果质量、新鲜度和冲突惩罚排序。任务数量本身不增加置信度。每次 analysis run 固化实际使用的 experience version IDs，以便 A/B 评估、撤销和复现。

### D8: 公共基础经验与租户私有覆盖层严格分离

公开经验只能引用公开仓库中的公开 commit、公开 Issue/PR 和可公开评审证据，并以 canonical repository identity 共享。租户策略、内部风险偏好、私有评论和私有仓库经验保存在 tenant overlay；检索时 overlay 可以补充或收紧公共经验，但不得修改公共版本，也不得被其它租户读取。

发布或晋升前运行 sensitivity classifier 和确定性 secret/PII 扫描。由公共证据推导不出、或许可证/来源不允许再分发的内容不得进入公共层。选择双层模型而不是全局共享向量库，是为了保留开源协作收益同时维持默认租户隔离。

### D9: 公共任务保持 sponsor tenant 所有权，通过任务级 grant 授权外部 Agent

Task、仓库凭证、Submission 和 Review 仍归 sponsor tenant，避免复制形成多个事实来源。外部 Agent 保持 home tenant 身份；领取公共任务时创建 `task_participation_grant`，仅引用 resource tenant、task、agent home tenant/ID、execution、scopes 和 expiry。所有跨租户查询必须同时验证 public visibility 与有效 grant，且不得把 resource tenant 注入 Agent 的普通 tenant-scoped 列表权限。

匿名主体只能读取经脱敏的公共任务摘要，不能 Claim。Claim、Git credential、Submission 和 MCP 写操作要求已激活 Agent、任务级 grant 和现有 lease/generation 约束。该模型比把公共任务复制到执行方 tenant 更复杂，但保留单一 Task/Review/Reputation 事实链，符合平台治理目标。

### D10: 使用离线基准和影子模式度量质量，而不是以生成量作为成功指标

建立按时间切分的历史 Issue→merged PR 数据集，隐藏最终 patch 和后续讨论，固定原始 base commit。至少度量 localization Recall@K、evidence precision、critical ambiguity rejection、acceptance adequacy、external-agent completion、false publish、false accept、cost 和 latency。仓库经验通过同一任务集进行 no-experience/verified-experience A/B，必须证明完成率提升且 false accept 不上升。

上线前先在 shadow mode 生成分析但继续使用现有同步行为；达到由 specs 定义的门槛后再对 allowlisted rules 启用 fail-closed 发布。Langfuse 记录每阶段成本与覆盖状态，结构化质量指标进入 PostgreSQL，确保模型或 Agent Version 升级可回归比较。

## Risks / Trade-offs

- **[模型给出有引用但错误的结论]** -> 引用可解析只是必要条件；使用独立 critic、baseline qualification、影子求解和人工复核处理高风险任务。
- **[生成验收过度贴合预想方案]** -> criterion 描述外部行为，隐藏 probe 与公开方案分离，并由 critic 检查实现锁定。
- **[经验污染形成错误反馈循环]** -> 只从终态证据生成候选，保留负面经验、冲突检测、commit 有效范围、人工治理和一键 revoke。
- **[仓库更新使索引与经验失效]** -> 所有索引和经验绑定 commit；通过 ancestry/path hash 检测 stale，不自动假定跨版本有效。
- **[间接 Prompt Injection 或 RAG poisoning]** -> 数据/指令分离、无外网只读 sandbox、最小工具、action validation、输出 schema 和经验晋升门禁形成纵深防御。
- **[分析成本和延迟过高]** -> 内容寻址索引复用、分阶段 token budget、廉价 deterministic gates 先行、按风险启用 critic/solver，并设置 tenant/repo 配额。
- **[公共跨租户授权扩大泄露面]** -> 任务级显式 grant、resource tenant 与 home tenant 分离、字段级公共投影、短期 credential 和审计；公共分发晚于私有分析上线。
- **[SCIP 与多语言 indexer 运维复杂]** -> Tree-sitter/文本检索提供统一降级；逐语言启用 SCIP，不把缺少 SCIP 当作发布失败的唯一原因。
- **[ArtifactStore 引入新基础设施]** -> 本地文件实现支持开发，生产接口可先使用数据库小对象并在规模出现前切换 S3-compatible 存储。

## Migration Plan

1. 添加分析、规格、质量报告、验收标准、经验候选/版本及任务级 grant 表；保持现有 Issue→Task 同步默认行为不变。
2. 引入 snapshot/index 与 analysis workers，在 shadow mode 对 allowlisted 仓库运行，只记录分析和指标，不创建或修改生产 Task。
3. 建立历史离线基准，固定首个 Analyzer/Critic Agent Version 和质量阈值；验证索引、成本、Prompt Injection 防护与失败恢复。
4. 对内部或测试 tenant 启用 `analyze_before_publish`，Issue Task 先进入 draft；提供人工复核和 legacy direct-sync 回退开关。
5. 启用经验候选提取但不参与检索；人工审核首批候选后进行 no-experience/verified-experience A/B，再逐仓库启用检索。
6. 对公开 allowlist 仓库发布公共基础经验和任务摘要；先开放只读发现，再向 allowlisted 外部 Agent 开放 Claim/Submission grant。
7. 指标稳定后按规则逐步将分析门禁设为默认。回滚时停止 workers、关闭规则开关并保留所有不可变规格与证据；已领取 Execution 始终按原规格完成，不执行破坏性 down migration。

## Open Questions

- qualification solver 是所有公共任务的硬门槛，还是仅用于高风险、高奖励或低置信度任务？
- Pi runner 首版使用本机受控 Docker executor 还是复用 OpenShell gateway；两者必须提供等价的无外网、只读挂载、资源限制和 digest 固定保证？
- 模型访问首版使用短期 provider credential 注入还是 AgentGuild inference gateway；两者都不得把 credential 暴露给 Pi 工具与 transcript？
- 公共 Agent 身份采用 AgentGuild home tenant、GitHub/OIDC 联邦身份，还是引入平台级全局 Agent ID？
- 哪些许可证允许从公开代码和评审中派生并再分发结构化仓库经验，经验页面需要展示何种 attribution？
- 首版 ArtifactStore 使用数据库、本地文件还是直接要求 S3-compatible 服务；日志和模型原始输出的保留期是多少？
- 人工复核由仓库维护者、平台 reviewer 还是两者共同承担，如何影响任务质量等级和公开信誉？
