## 1. 技术基线与可验收决策

- [ ] 1.1 建立历史 Issue→merged PR 离线数据集格式，固定 repository、Issue revision、base commit、最终 patch、测试与评审证据
- [ ] 1.2 实现按时间切分并隐藏最终 patch/后续讨论的数据集加载器，加入无未来信息泄漏的单元测试
- [ ] 1.3 定义 localization Recall@K、evidence precision、critical ambiguity rejection、acceptance adequacy、false publish、false accept、成本和延迟指标
- [ ] 1.4 用当前 Issue 直映 Task 行为运行离线基线并保存版本化结果，作为后续 Analyzer 与经验 A/B 的对照
- [ ] 1.5 基于 `earendil-works/pi` SDK 完成 Pi runner spike，验证 in-memory session、只读工具、结构化终止工具、取消、usage 和 Agent Version 绑定
- [ ] 1.6 完成 ArtifactStore spike，确定本地文件、PostgreSQL 小对象和 S3-compatible 生产实现的边界与保留策略
- [ ] 1.7 固化首版 qualification solver 触发策略、公共任务质量等级和人工复核责任矩阵

## 2. 数据库迁移与配置骨架

- [ ] 2.1 新增分析 job、step、run、evidence ref、index manifest 和 artifact metadata 的 up/down 迁移，包含 tenant 复合键、状态约束与幂等唯一键
- [ ] 2.2 新增 Task Specification Version、Acceptance Criterion、Task Quality Report 及 Task/Execution 规格绑定字段的 up/down 迁移
- [ ] 2.3 新增 repository experience candidate/version/evidence/conflict 与 public/private visibility 字段的 up/down 迁移
- [ ] 2.4 新增 public task projection、task participation grant 和跨租户 grant audit 的 up/down 迁移
- [ ] 2.5 在 `internal/testdb/postgres.go` 注册全部新迁移，并覆盖连续升级、完整回滚和约束失败测试
- [ ] 2.6 扩展配置加载，加入 analysis worker、lease/attempt、artifact store、indexer、Pi runner image/digest/model endpoint、shadow mode、quality policy、experience 和 public distribution 开关
- [ ] 2.7 为新增配置编写默认值、非法组合、生产 fail-closed 和敏感值不回显测试

## 3. 分析领域模型与持久化

- [ ] 3.1 创建模块化 `analysis/domain`，实现 AnalysisJob 状态机、step 状态、lease、fencing generation、attempt 和终态规则
- [ ] 3.2 实现不可变 TaskSpecificationVersion、AnalysisEvidenceRef、AcceptanceCriterion、CriticFinding 和 TaskQualityReport 领域模型及校验
- [ ] 3.3 定义 `analysis/application` 的 Store、Repository、ArtifactStore、Indexer、Retriever、Provider、Verifier 与 TaskPublisher ports
- [ ] 3.4 实现 PostgreSQL AnalysisJob repository 的幂等创建、claim、续租、过期回收、fenced step 更新和终态查询
- [ ] 3.5 实现 Task Specification、criteria、evidence 和 quality report repository，保证版本内容不可原地修改
- [ ] 3.6 为分析存储实现短事务 Store，并覆盖并发 claim、旧 worker 写入、重复发现和 tenant 隔离测试

## 4. 固定仓库快照与代码索引

- [ ] 4.1 扩展 repository resolver，为已接入 GitHub App 仓库和允许的公开仓库解析 canonical identity 与不可变 base commit
- [ ] 4.2 实现分析用只读 Git workspace，在受控容器中检出精确 commit，禁止分支漂移、credential 持久化和非允许网络
- [ ] 4.3 实现内容寻址 IndexManifest，按 repository、base commit 和 indexer version 复用已完成索引并处理失败索引
- [ ] 4.4 实现文件清单、语言、构建清单、测试映射和 Git history 的基础索引器
- [ ] 4.5 集成 Tree-sitter 符号/import 解析并为 Go、TypeScript/JavaScript 建立 fixture 与降级测试
- [ ] 4.6 定义 SCIP adapter 并接入至少一种现有 indexer，缺少 SCIP 时明确降级到语法图和文本检索
- [ ] 4.7 实现 exact path/symbol、代码图邻居和 PostgreSQL 全文的混合检索与 token budget 裁剪
- [ ] 4.8 实现证据引用生成和解析校验，验证 commit、path、line span 与 content hash 一致
- [ ] 4.9 实现本地 ArtifactStore 与生产接口契约，覆盖内容 hash、大小限制、加密/访问策略和清理测试

## 5. Pi Analyzer、Critic 与安全边界

- [ ] 5.1 定义版本化 Analyzer/Critic 输入输出 JSON Schema，覆盖诊断、影响、方案、步骤、约束、非目标、风险、不确定性和验收标准
- [x] 5.2 创建独立 TypeScript Pi runner workspace，精确固定 `@earendil-works/pi-coding-agent`、`pi-agent-core`、`pi-ai` 和 lockfile integrity
- [ ] 5.3 构建固定 upstream tag/commit 与 OCI digest 的 Pi runner 镜像，加入 Node engine、SBOM、漏洞扫描和依赖升级检查
- [ ] 5.4 定义 Go↔Pi runner 版本化 JSON job/result 协议，实现 TaskAnalysisProvider 容器 adapter、取消、timeout 和错误映射
- [ ] 5.5 使用 `createAgentSession`、`SessionManager.inMemory` 和自定义 ResourceLoader 创建无宿主配置发现的一次性 Pi session
- [ ] 5.6 实现 localization context builder，只向 Pi Analyzer 提供固定快照、可追溯证据和命中的经验版本
- [ ] 5.7 实现 TypeBox `submit_task_specification` 终止工具，拒绝自由文本、重复提交、缺失字段、越界长度和未知 verifier kind
- [ ] 5.8 实现独立 Pi TaskCritiqueProvider session，检查错误引用、遗漏影响面、关键歧义、验收不可判定和实现过度限定
- [ ] 5.9 保证 Pi Analyzer 与 Critic 使用分离 session、Agent Version、Prompt、上下文和 trace，且 Critic 不接收 Analyzer hidden reasoning
- [ ] 5.10 仅启用 read/grep/find/ls 与受控 evidence/experience tools，并使用 beforeToolCall 阻断写入、bash、越界路径和非允许网络
- [ ] 5.11 使用 Pi ModelRuntime 的内存 credential store 或短期 runtime credential，禁止挂载宿主 `.pi`、auth 和 session 目录
- [ ] 5.12 过滤并持久化 Pi tool events、公开文本、usage 和结构化结果，不保存 thinking delta 或把 transcript 晋升为经验
- [ ] 5.13 实现不可信 DATA block、Prompt Injection、secret、PII 和敏感性检查，覆盖代码注释、Issue、文档和经验污染样例
- [ ] 5.14 为 Pi provider timeout、rate limit、malformed output、未调用终止工具、上下文取消和成本覆盖状态编写测试

## 6. 分析 Worker 与 Issue 同步接线

- [ ] 6.1 实现 AnalysisWorker 的短事务 claim 和事务外 snapshot/index/provider/verification 执行循环
- [ ] 6.2 实现每阶段持久化、lease 续期、fencing 写入、最大尝试、指数退避和安全错误摘要
- [ ] 6.3 修改 sync engine，使匹配 Issue 幂等入队 analysis job，成功入队后再推进同步水位
- [ ] 6.4 保留 legacy direct-sync 开关，并实现 shadow mode 只保存分析与指标、不改变生产 Task
- [ ] 6.5 处理 Issue 新 revision、默认分支新 commit、关闭 Issue 和重复手动同步的重新分析策略
- [ ] 6.6 在 API main 组装 analysis repositories、Pi provider、container executor、worker 和优雅关闭，并加入 worker interval/lease 配置
- [ ] 6.7 新增分析运行查询、重新分析、补充澄清和人工复核 REST 应用服务与路由
- [ ] 6.8 更新 OpenAPI，加入分析 job/run/specification/quality DTO、状态和稳定错误码
- [ ] 6.9 覆盖多规则隔离、单 job 失败不阻塞、进程重启恢复、旧 lease 写入和幂等入队集成测试

## 7. 质量门禁与资格验证

- [ ] 7.1 实现 evidence resolution、critical ambiguity、schema、安全扫描和 verifier policy 的 hard gate evaluator
- [ ] 7.2 实现 evidence coverage、localization confidence、acceptance determinism、方案一致性、风险覆盖和 experience support 软维度
- [ ] 7.3 保证任一 hard gate 失败不可被软评分抵消，并持久化逐项 evidence、policy version 和决定原因
- [ ] 7.4 实现 command、static、behavioral 和 manual Acceptance Criterion verifier 契约与 allowlist
- [ ] 7.5 在固定 base commit 的受控容器中运行 verifier qualification，记录 baseline observation、环境、命令、超时和结果
- [ ] 7.6 对新增缺陷 probe 实现 baseline fail/缺陷可观察检查，拒绝 baseline 已通过或无法启动的错误验收
- [ ] 7.7 实现公共自动发布策略：全部 critical criteria 自动可判定，否则必须获得人类复核 provenance
- [ ] 7.8 使用独立可写 copy-on-write workspace 的 Pi session 实现 QualificationSolver，限制 bash/edit 工具并确保 shadow patch 不对领取 Agent 暴露
- [ ] 7.9 将每阶段 Langfuse 成本、耗时和 complete/partial/unavailable 覆盖状态写入质量指标
- [ ] 7.10 为高分但 hard gate 失败、模糊验收、错误证据、危险命令和影子求解失败场景编写领域与集成测试

## 8. Task 与 Execution 不可变规格契约

- [ ] 8.1 扩展 Task 领域模型，支持 Issue 来源 Draft、current specification version、source changed 和质量门禁发布意图
- [ ] 8.2 扩展 system Task 发布应用服务，只允许绑定合格 quality report 的 Draft Issue Task 进入 Open
- [ ] 8.3 修改 Claim 事务，原子绑定 current specification version 到 Execution，并拒绝无可发布规格的 Task
- [ ] 8.4 修改 Task/Execution/Submission views 和 repositories，返回 current 与 bound specification version ID
- [ ] 8.5 实现未领取 Task 显式切换新规格版本，并保留旧版本和切换审计
- [ ] 8.6 实现活动 Execution 遇到 Issue 更新时的 source_changed 标记、继续/取消/替代策略，禁止静默改写
- [ ] 8.7 让任务列表仅把通过门禁的 Open Task 作为可领取资源，Draft/待澄清保留治理可见性
- [ ] 8.8 覆盖 Draft 越权发布、并发 Claim、Claim 版本绑定、来源更新和旧规格提交测试

## 9. 验收驱动的 Submission 与 Review

- [ ] 9.1 扩展 validation job 创建，从 Execution 绑定规格生成逐 criterion steps，而不是读取 Task 当前版本
- [ ] 9.2 新增 criterion evidence 持久化，绑定 Task、Execution、Submission revision、commit、spec version、环境和时间
- [ ] 9.3 保留既有 commit/branch/base/integrity hard gates，并把 criterion 未满足纳入 validation terminal 结果
- [ ] 9.4 扩展 Review domain 和 API，要求对每条 criterion 记录状态、说明和 evidence refs
- [ ] 9.5 阻止 critical criterion failed、missing、needs_evidence 或未经治理的 not_applicable Submission 被 Accepted
- [ ] 9.6 在新 Submission revision 上重新运行验收，禁止静默复用旧 revision evidence
- [ ] 9.7 修改 Review 查询，聚合绑定规格、Issue/base commit、经验版本、Diff、验证证据、成本和修订历史
- [ ] 9.8 在终态 Review 事务/outbox 中幂等发出 repository experience extraction 事件
- [ ] 9.9 覆盖使用 Task 新版本绕过旧执行契约、错误接受、返工 evidence 漂移和经验事件重复测试

## 10. 仓库经验领域与治理

- [ ] 10.1 创建 `repositoryexperience/domain`，实现经验类型、visibility、candidate/active/stale/conflicted/revoked 状态机和不可变版本
- [ ] 10.2 定义 repository experience Store、evidence resolver、extractor、classifier、retriever 和 governance ports
- [ ] 10.3 实现 candidate/version/evidence/conflict PostgreSQL repositories，覆盖 canonical repo、commit 范围和 tenant/public 隔离
- [ ] 10.4 实现 accepted 与 rejected Submission 的幂等候选提取，拒绝没有 terminal validation/review 的来源
- [ ] 10.5 实现 provenance、执行证据、独立评审和高风险人工批准的晋升策略
- [ ] 10.6 实现成功模式、失败方案、架构事实、变更模式、验证配方、约定和已知风险结构化 schema
- [ ] 10.7 实现 commit ancestry、path/symbol content hash 和仓库变化驱动的 stale 检测 worker
- [ ] 10.8 实现同范围经验矛盾检测、conflict group、支持/反对证据和冲突期间禁止确定性使用
- [ ] 10.9 实现 public base 与 tenant overlay 的检索硬过滤和证据/相关性/新鲜度排序，确保数量本身不提升置信度
- [ ] 10.10 在 analysis run 固化使用的 experience version IDs、检索分数和后续执行结果反馈
- [ ] 10.11 新增经验候选列表、批准、拒绝、撤销、冲突处理和版本查询 REST API
- [ ] 10.12 覆盖私有证据公共泄漏、重复非独立证据、过期经验、负面经验、撤销回滚和 tenant 隔离测试

## 11. 公共任务与跨租户 Grant

- [ ] 11.1 实现 public task projection 领域与 repository，只投影通过公共门禁且通过敏感性/可见性检查的字段
- [ ] 11.2 实现 task participation grant 领域、expiry、scope、revocation 和完整审计 repository
- [ ] 11.3 新增独立公共任务发现 REST API，支持匿名脱敏读取、认证 Agent 分页和与 tenant cursor 隔离
- [ ] 11.4 实现跨租户 Claim 事务，验证公共投影、Agent home tenant 状态和资格并原子创建 Execution 与 grant
- [ ] 11.5 扩展 Principal/authorizer，显式区分 home tenant 与 resource tenant，禁止 grant 扩大普通 tenant 列表权限
- [ ] 11.6 将 grant 校验接入 Task、Execution、heartbeat、Submission、Review 结果和所有相关读取路径
- [ ] 11.7 将 grant 校验接入 Git credential 与 proxy，限制 repository、branch、base commit、Execution 和过期时间
- [ ] 11.8 实现 Agent revoked、Task cancelled、Execution terminal、grant expiry 和滥用策略触发的撤销/失效流程
- [ ] 11.9 保证 sponsor tenant 持有唯一 Task/Execution/Submission/Review 事实，不向 Agent home tenant 复制业务记录
- [ ] 11.10 覆盖本地/外部 Agent 并发 Claim、grant replay、跨任务访问、列表泄漏、过期 credential 和错误资源 tenant 测试

## 12. MCP 契约扩展

- [ ] 12.1 扩展 task list/get/claim MCP DTO，返回结构化 Task Specification Version、criteria 和公开 evidence refs
- [ ] 12.2 新增公共任务发现 MCP tool，与 tenant 私有列表使用独立 cursor 签名和授权语义
- [ ] 12.3 在跨租户 MCP 写操作中同时校验 OAuth Principal、server-side grant、Execution 和 lease generation
- [ ] 12.4 保持 grant/lease secret 不进入模型工具参数，并返回稳定的 revoked、expired、forbidden 和 state conflict 错误
- [ ] 12.5 更新 `skill.md`，说明任务规格、验收证据、公共 Claim、最小权限和不可信任务内容规则
- [ ] 12.6 扩展 REST/MCP contract equivalence 测试，覆盖本 tenant 与公共任务发现、领取、读取和提交

## 13. 前端治理与公共体验

- [ ] 13.1 扩展同步规则页，配置 analyze-before-publish、shadow mode、质量策略、人工复核和公共分发开关
- [ ] 13.2 新增分析运行治理视图，展示阶段、base commit、Analyzer/Critic Version、失败原因、澄清和重新分析操作
- [ ] 13.3 扩展 Task 详情，展示问题诊断、影响范围、方案、约束、风险、证据、经验版本和逐条验收标准
- [ ] 13.4 明确区分 Draft、待澄清、质量失败、待复核、Open、source changed 和替代任务状态
- [ ] 13.5 扩展 Submission/Review 页面，按绑定规格逐条展示 verifier evidence 并提交 criterion 判定
- [ ] 13.6 新增 repository experience 治理页，支持候选审核、版本历史、适用范围、冲突、过期和撤销
- [ ] 13.7 新增公共任务目录与脱敏详情，认证 Agent 操作引导不得暴露 sponsor tenant 私有字段
- [ ] 13.8 为所有新页面补充 loading、empty、partial failure、forbidden 和 retry 状态及键盘可访问性
- [ ] 13.9 编写前端单元测试，覆盖真实 API 数据、规格版本固定、criterion evidence、经验隔离和公共字段脱敏
- [ ] 13.10 编写 Playwright 流程，覆盖 Issue→分析→门禁→公共任务→外部 Claim→提交→验收→经验候选

## 14. 离线评测、影子发布与可观测性

- [ ] 14.1 实现离线 evaluator，运行 Analyzer/Critic/quality policy 并与隐藏 merged PR、测试和评审证据比较
- [ ] 14.2 实现 Analyzer/Critic/quality policy 版本之间的可重复对比报告和 promote 前回归检查
- [ ] 14.3 在 shadow mode 记录 false publish/accept proxy、人工复核一致性、成本、延迟和失败分类
- [ ] 14.4 实现 repository experience no-experience/verified-experience A/B 分桶与结果归因
- [ ] 14.5 增加 worker backlog、lease expiry、retry exhaustion、provider error、gate failure、经验冲突和 grant denial 指标
- [ ] 14.6 定义 allowlist rollout 与自动回退阈值，确保错误接受率恶化时停用新版本或经验检索

## 15. 安全、文档与端到端验收

- [ ] 15.1 建立包含 Issue、代码注释、README、经验和工具输出注入的安全测试语料并纳入回归
- [ ] 15.2 对分析 sandbox、ArtifactStore、公开投影、跨租户 grant 和 Git proxy 执行威胁建模与安全评审
- [ ] 15.3 更新 OpenAPI、Pi runner 构建/升级、部署环境变量、worker 运维、数据保留、回滚和公共经验 attribution 文档
- [ ] 15.4 扩展 acceptance harness，使用真实 PostgreSQL 组装分析、质量、经验、公共授权、validation 和 review 全服务
- [ ] 15.5 运行并通过后端 `go build ./...`、`go test -race ./... -count=1` 和新增迁移 up/down 验证
- [ ] 15.6 运行并通过前端 `npm run build`、`npm test -- --run` 和新增完整 Playwright E2E
- [ ] 15.7 在至少一个 allowlisted 开源仓库完成 shadow 分析冒烟，核对证据、验收、成本和不发布行为
- [ ] 15.8 在测试 tenant 完成 fail-closed Draft→Open→Claim→Submission→逐条验收→Review→经验候选闭环
- [ ] 15.9 在两个隔离 tenant 完成公共任务 Claim 与越权负例验收，确认无 tenant 列表、私有证据或经验泄漏
- [ ] 15.10 完成发布前治理评审，记录剩余开放问题、质量阈值、试点仓库和逐阶段回滚负责人
