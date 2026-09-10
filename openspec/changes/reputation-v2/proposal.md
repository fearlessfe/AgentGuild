## Why

`docs/agent-first-reputation-and-blockchain-rewards.md` §4 要求声望是**可解释、可重算、不原地改写**的：七个维度、置信下界而不是裸均值、180 天时间衰减、样本不足时明确标注"未验证"。

当前的 v1 `reputation_projections` 三条都不满足：它按 `(agent_version_id, capability, task_type)` 聚合 review 结论，用裸 `pass_rate`，没有 Wilson 置信下界，也没有时间衰减。更严重的是 `algorithm_version` **不在主键里**——升级算法会原地改写历史，直接违反 §4.5。

同时，Stage 1 落地的 `execution_criterion_results` 证据账本至今没有下游消费者：逐条验收标准是否通过这件事，还没有进入任何 Agent 的声望。

本变更并行引入声望 v2，不迁移也不改写 v1。

## What Changes

- 新增三层投影 `agent_reputation_projections`（`agent_lifetime` / `agent_version` / `capability`），主键含 `algorithm_version` 与 `scope`。算法升级只写入新版本行，历史投影原样保留。
- 新增 `agent_reputation_dimension_scores`，每个维度一行，`ON DELETE CASCADE` 到 header。同时保留 `raw_rate`、`lifetime_confidence`、`recent_confidence` 与最终 `score`，让"为什么是这个分数"可以逐项追问。
- 算法参数进 `reputation_algorithm_params` 表——**权重是版本化数据，不是代码常量**。历史行必须能用当时的参数复算。
- 难度系数进 `task_difficulty_classes`，`multiplier` 由 CHECK 约束在 `[0.75, 1.50]`。难度只能来自这张预先版本化的表，绝不接受 Agent 自报。
- 纯函数算法层：`raw_rate = (passed+α)/(passed+failed+α+β)`；`lifetime_confidence = Wilson(未衰减计数)`；`recent_confidence = Wilson(衰减加权计数)`，`DecayWeight = 2^(-age/halfLife)`；`score = 0.70*recent + 0.30*lifetime`。Wilson 接受非整数伪计数，衰减后的有效样本直接代入。
- **零观测的维度是零样本，不是零分**：它被排除在总分的加权平均之外并重新归一化。否则"从没做过安全评审"会被当成"安全表现差"，静默拉低每个 Agent。
- **security 维度零行 = 零样本**，只采信平台自己跑过的 `security_scan` 步骤与绑定到该步骤的验收标准。
- `Rebuild(ctx, algorithmVersion, evaluatedAt)` 从不可变事实全量重算，`evaluatedAt` 必须显式给出；`ReplaceAlgorithm` 在单个事务内删除该版本全部行再插入。
- 暴露：`GET /v1/public/agents/{id}/reputation`、`GET /v1/agents/me/reputation`、`POST /v1/admin/reputation:rebuild`，以及 MCP `agent_reputation_get`。

## Capabilities

### Modified Capabilities

- `agent-reputation`：在既有的"绑定 Agent Version / 按能力分组 / 展示样本充分性"之上，补充七维评分、置信下界与时间衰减、算法版本化与可重算性、零观测维度的处理规则。

## Impact

- 迁移 `000029_reputation_v2`：四张新表 + 首版参数行 + 四个难度分级。
- 新增 `internal/reputation/{domain,application,postgres,worker}` 的 v2 文件族；v1 的 `domain/projection.go`、`application/projector.go`、`postgres/projection_repository.go`、`worker/worker.go` 与 `reputation_projections` 表、`GET /v1/reputation`、MCP `reputation_get` **全部原样保留并继续服务**。
- MCP 新工具名为 `agent_reputation_get` 而非复用 `reputation_get`：复用会静默改变已接入客户端拿到的响应结构。
- 新增配置 `REPUTATION_V2_ALGORITHM_VERSION`、`REPUTATION_RECENT_HALF_LIFE`、`REPUTATION_MIN_SAMPLE`。
- 消费 Stage 1 的 `execution_criterion_results`，为 Stage 3 的 `quality_multiplier` 提供可解释的输入。
