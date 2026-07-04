---
comet_change: agent-version-and-experience
role: technical-design
canonical_spec: openspec
---

# Agent Version and Experience 技术设计

## 1. 目标与范围

本设计实现 AgentGuild 的 Agent Version 不可变快照、版本谱系、生命周期、晋级/回滚，以及从已验收任务中提取并治理经验候选的机制。OpenSpec delta specs 是行为要求的事实源；本文描述实现方式和技术取舍。

本 change 不实现：自动修改生产 Agent、跨组织经验市场、在线自学习、复杂推荐模型、经验候选的自动化脱敏 NLP 管线（保留分类接口和策略检查，但初版可由规则/人工完成）。

## 2. 架构

```text
Publisher/Owner ── REST/MCP ──▶ REST Handler / MCP Server
                                    │
                                    ├─ agentversion.Application Service
                                    │    ├─ VersionLifecycleService（创建 Draft、晋级、回滚）
                                    │    ├─ VersionSnapshotService（指纹、内容引用、谱系）
                                    │    └─ PromotionPolicy（门槛、审批、并发控制）
                                    │
                                    ├─ agentversion.Postgres（Agent Version 与内容引用持久化）
                                    │
                                    ├─ agentexperience.Application Service
                                    │    ├─ CandidateExtractionService（从 Accepted Submission 生成候选）
                                    │    ├─ CandidateReviewService（审核、批准、拒绝）
                                    │    └─ SensitivityPolicy（数据分类与策略检查）
                                    │
                                    ├─ agentexperience.Postgres（ExperienceCandidate 持久化）
                                    │
                                    ├─ evaluation.Application Service
                                    │    ├─ BenchmarkSetService（基准集版本管理）
                                    │    └─ EvaluationRunService（冻结版本运行评测）
                                    │
                                    ├─ evaluation.Postgres（BenchmarkSet、EvaluationRun 持久化）
                                    │
                                    └─ transport/rest, transport/mcp（协议适配）

identity module ──▶ Agent（current_version_id）
                       ▲
                       │ 原子切换由 agentversion 服务通过事务化 Store 完成

task / review / git-delivery modules ──▶ 提供 Accepted Submission、Execution、Validation 证据
```

运行时基线：Go 1.26.4、PostgreSQL 18.4，与已有 change 保持一致。

| 模块 | 职责 |
|---|---|
| `agentversion/domain` | `AgentVersion` 领域模型、版本状态机、内容引用、晋级/回滚规则 |
| `agentversion/application` | Draft 创建、评测触发、晋级、回滚、历史查询、权限策略 |
| `agentversion/postgres` | `agent_versions` 表及内容引用表持久化 |
| `agentexperience/domain` | `ExperienceCandidate` 模型、敏感级别、审核状态机 |
| `agentexperience/application` | 从 Submission 提取候选、分类检查、人工审核、与新版本绑定 |
| `agentexperience/postgres` | `experience_candidates` 表持久化 |
| `evaluation/domain` | `BenchmarkSet`、`EvaluationRun`、评分规则与硬门槛 |
| `evaluation/application` | 创建基准集、启动评测运行、判定通过/失败 |
| `evaluation/postgres` | `benchmark_sets`、`evaluation_runs` 表持久化 |
| `transport/rest` | 版本、经验、评测的 REST 路由 |
| `transport/mcp` | `agent_version_*`、`experience_candidate_*`、`evaluation_run_*` 工具 |

Transport 只负责认证上下文、Schema 转换和协议错误映射，不得直接访问 Repository。

## 3. 领域模型

### 3.1 AgentVersion

```text
id, tenant_id, agent_id, version_number
parent_version_id, status
runtime, model, capabilities, config_fingerprint
content_hash, environment_digest
prompt_ref, skill_refs, memory_ref, tool_refs
created_by, created_at, promoted_at, retired_at
```

- `status`：`draft` | `evaluating` | `eligible` | `active` | `retired` | `rejected`。
- `parent_version_id` 为空时表示该 Agent 的初始版本（由激活流程创建）。
- `content_hash`：对 `prompt_ref` + `skill_refs` + `memory_ref` + `tool_refs` + `runtime` + `model` 的规范化表示计算 SHA256；任何配置变化都会改变该哈希，从而强制创建新版本。
- `skill_refs` / `tool_refs` 为内容寻址引用数组；`prompt_ref` / `memory_ref` 为单一内容寻址引用。
- `environment_digest`：运行时、工具版本、依赖摘要，用于复现。
- `created_by`：创建者 actor ID；`promoted_at` / `retired_at` 记录状态转换时间。

### 3.2 AgentVersion 状态机

```text
draft ──▶ evaluating ──▶ eligible ──▶ active
                  │           │
                  └──▶ rejected  └──▶ retired

active ──▶ retired（主动下线）
active ──▶ eligible? 否；回滚通过切换 current_version_id 实现，不修改被回滚版本的状态
```

- `draft`：owner 创建的新配置快照，尚未进入评测。
- `evaluating`：已提交评测，至少一次 EvaluationRun 正在进行或已完成。
- `eligible`：最近一次 EvaluationRun 通过硬门槛且策略检查通过，等待 owner 审批后晋级。
- `active`：已晋级为当前生产版本；`agents.current_version_id` 指向该版本。
- `retired`：旧 Active 版本被显式下线，仅用于审计，不能再成为 current。
- `rejected`：评测失败或被拒绝，保留失败证据，不能再晋级。

### 3.3 ContentRefs

```text
prompt_ref  TEXT
skill_refs  TEXT[]
memory_ref  TEXT
tool_refs   TEXT[]
```

- 引用格式：`sha256:<hex>` 或 `cid:<content-id>`；初版统一使用 `sha256:<hex>`。
- 内容实际存储在外部对象存储或数据库大对象表（本 design 不实现对象存储，仅保存引用；对象存储接口后续接入）。
- 不变性约束：任何已创建版本的内容引用字段不可修改；repository 层 UPDATE 排除这些字段。

### 3.4 Agent 生命周期交互

Agent 保留 `CurrentVersionID` 字段。以下行为由 `agentversion` 应用服务在事务中完成：

- **晋级 Promote**：校验目标版本状态为 `eligible` 且属于该 Agent；将版本置为 `active`，并将 `agent.CurrentVersionID` 更新为目标版本 ID。
- **回滚 Rollback**：校验目标版本状态为 `active`/`eligible`/`retired`（仅允许回滚到历史 Active/Eligible 版本），将 `agent.CurrentVersionID` 更新为目标版本 ID；被回滚版本保持 `active` 或 `retired`，不删除。
- **并发控制**：使用 `agents` 行级锁（`SELECT FOR UPDATE`）或乐观锁（`updated_at`）避免并发晋级/回滚。

### 3.5 ExperienceCandidate

```text
id, tenant_id, agent_id
source_task_id, source_submission_id, source_review_id
evidence_ref, content_hash
applicable_capabilities, tenant_scope
sensitivity_class, status
policy_reason, reviewed_by, reviewed_at
created_at
```

- `sensitivity_class`：`public` | `internal` | `restricted` | `forbidden`。
- `status`：`pending_review` | `approved` | `rejected`。
- `tenant_scope`：经验适用范围，默认与来源 Agent 的 `tenant_id` 相同，禁止跨 tenant。
- `applicable_capabilities`：该经验可应用的 capability 标签列表。

### 3.6 ExperienceCandidate 状态机

```text
pending_review ──▶ approved（需 reviewer 审批）
              └──▶ rejected（策略检查失败或人工拒绝）
```

- 从 Accepted Submission 生成时先进入 `pending_review`。
- `SensitivityPolicy.Classify` 对证据进行分类；若分类为 `forbidden`，自动拒绝并记录策略原因。
- 审批通过的经验候选不直接写入 Agent Memory；它作为内容引用被纳入新 Agent Version 的 `memory_ref` 或 `skill_refs`，随版本切换生效或回滚。

### 3.7 BenchmarkSet

```text
id, tenant_id, version_number, name, description
is_active, created_by, created_at
```

- `version_number` 租户内单调递增。
- `is_active` 标识当前默认用于自动评测的基准集。
- 任务引用单独存储在 `benchmark_set_tasks(benchmark_set_id, task_ref, ordering)`。

### 3.8 EvaluationRun

```text
id, tenant_id, agent_version_id, benchmark_set_id
status, environment_digest, scoring_rule_version
threshold_results, summary
started_at, completed_at
```

- `status`：`running` | `passed` | `failed`。
- `threshold_results`：各硬门槛检查结果（JSONB）。
- `summary`：通过率、平均延迟、成本等聚合指标（JSONB）。
- 运行结果明细存储在 `evaluation_run_results(evaluation_run_id, task_ref, score, passed, details)`。

## 4. 关键流程

### 4.1 创建 Draft 版本

1. Owner 提交新的模型 / Prompt / Skill / Memory / 工具配置。
2. `VersionSnapshotService` 计算配置指纹 `config_fingerprint` 和内容哈希 `content_hash`。
3. 若指纹与当前 Active 版本相同，返回「无变化」错误，不创建新版本。
4. 查询当前最新版本号，生成 `version_number = latest + 1`。
5. 创建 `AgentVersion{status: draft, parent_version_id: current_active_id}`。
6. 持久化版本及内容引用。

### 4.2 启动评测

1. Owner 或系统选择 Draft 版本和 BenchmarkSet。
2. `EvaluationRunService` 创建 `EvaluationRun{status: running}`，冻结 `agent_version_id`、`benchmark_set_id`、`environment_digest`、`scoring_rule_version`。
3. 异步或同步执行基准任务（本 design 先实现同步 EvaluationRun 记录 + 模拟/可注入的执行器）。
4. 执行完成后计算 `threshold_results` 和 `summary`。
5. 若全部硬门槛通过，`EvaluationRun.status = passed`；否则 `failed`。
6. `VersionLifecycleService` 根据最新 EvaluationRun 结果更新 AgentVersion 状态：
   - passed → `eligible`
   - failed → `rejected`（或保留 `evaluating` 允许重跑，由策略配置决定）

### 4.3 晋级到 Active

1. Owner 请求将 Eligible 版本晋级。
2. `PromotionPolicy` 校验：
   - 版本状态为 `eligible`；
   - 最新 EvaluationRun 为 `passed`；
   - 所有经验候选（若该版本包含经验）已通过审批；
   - 当前无其他并发晋级事务。
3. 事务内：
   - 将旧 Active 版本（如有）置为 `retired`；
   - 将目标版本置为 `active`；
   - 更新 `agents.current_version_id`。
4. 记录审计事件。

### 4.4 回滚

1. Owner 选择历史 Active/Eligible 版本执行回滚。
2. 校验目标版本属于该 Agent 且状态允许回滚（`active`、`eligible`、`retired`）。
3. 事务内更新 `agents.current_version_id` 为目标版本 ID。
4. 被回滚的当前版本保持原状态；新任务立即使用回滚后的版本，旧任务仍绑定原版本。

### 4.5 从 Accepted Submission 提取经验候选

1. `git-delivery-and-validation` 与 `code-review-and-reputation` 完成后，Submission 状态为 `accepted`。
2. `CandidateExtractionService` 读取 Submission 的 Execution（含 `agent_version_id`、任务类型、capabilities）。
3. 生成经验证据摘要并计算 `content_hash`；创建 `ExperienceCandidate{status: pending_review}`。
4. `SensitivityPolicy.Classify` 对内容进行分类：
   - `forbidden`：自动拒绝，记录策略原因；
   - 其他级别：保留待人工审核。
5. 持久化候选。

### 4.6 经验候选审批并绑定到新版本

1. Owner/Reviewer 查看 `pending_review` 候选。
2. 审批通过 → `approved`；拒绝 → `rejected` 并记录原因。
3. 创建新 Agent Version 时，owner 可选择纳入一组 `approved` 经验候选；这些候选的 `evidence_ref` 被合并进新版本的 `memory_ref` 或 `skill_refs`。
4. 若该版本随后被回滚，这些经验随内容引用一起失效。

## 5. 数据模型与迁移

扩展现有表：

1. `agent_versions`
   - 新增：`parent_version_id`, `status`, `content_hash`, `environment_digest`, `prompt_ref`, `skill_refs`, `memory_ref`, `tool_refs`, `created_by`, `promoted_at`, `retired_at`
   - 新增 CHECK：`status IN ('draft','evaluating','eligible','active','retired','rejected')`
   - 新增外键：`(tenant_id, parent_version_id) -> agent_versions(tenant_id, id)`

新增表：

2. `agent_version_content_refs`（可选，若选择将 refs 存在独立表而非 `agent_versions` 列中；本 design 推荐直接扩展 `agent_versions` 列，减少 JOIN）
3. `experience_candidates`
4. `benchmark_sets`
5. `benchmark_set_tasks`
6. `evaluation_runs`
7. `evaluation_run_results`

迁移脚本 `000003_agent_version_and_experience.up.sql` 同时负责将现有初始版本标记为 `active`、设置 `parent_version_id = NULL`、填充默认 `status`。

## 6. 外部依赖

### 6.1 identity 模块需提供的接口

- `GetAgent(ctx, tenantID, agentID) (*Agent, error)`：用于校验 owner 权限和当前版本。
- `UpdateCurrentVersion(ctx, tenantID, agentID, versionID) error`：事务内更新 `current_version_id`；由 `agentversion` 服务通过共享 Store 接口或领域事件调用。
- 初版实现方式：`agentversion` 应用服务持有 `identity.application.Store` 接口，通过事务直接操作 `AgentRepository` 和 `VersionRepository`。

### 6.2 task / review / git-delivery 模块

- `GetAcceptedSubmission(ctx, submissionID) (*Submission, error)`：提供来源任务和证据。
- `GetExecution(ctx, executionID) (*Execution, error)`：提供 `agent_version_id`、任务类型、capabilities。

### 6.3 对象存储（未来）

- 内容寻址引用目前仅保存哈希字符串；真实内容可存在文件系统/对象存储。本 design 预留 `ContentStore` 接口：`Get(ref string) ([]byte, error)`、`Put(content []byte) (string, error)`，初版可用内存或本地文件 stub。

## 7. REST 契约（关键端点）

版本管理：

- `GET /v1/agents/:id/versions`：列出该 Agent 的版本谱系。
- `POST /v1/agents/:id/versions`：从当前版本创建 Draft（需新配置内容）。
- `GET /v1/agents/:id/versions/:version_id`：版本详情（含内容引用、父版本、状态）。
- `POST /v1/agents/:id/versions/:version_id/diff`：与父版本或当前版本对比。
- `POST /v1/agents/:id/versions/:version_id/evaluations`：对该版本启动 EvaluationRun。
- `POST /v1/agents/:id/versions/:version_id/promote`：将 Eligible 版本晋级为 Active。
- `POST /v1/agents/:id/versions/:version_id/rollback`：回滚到该版本。

经验治理：

- `GET /v1/agents/:id/experiences`：列出经验候选。
- `POST /v1/agents/:id/experiences`：从指定 Accepted Submission 提取候选。
- `POST /v1/agents/:id/experiences/:experience_id/approve`：审批通过。
- `POST /v1/agents/:id/experiences/:experience_id/reject`：拒绝并记录原因。

评测：

- `GET /v1/benchmarks`：列出 BenchmarkSet。
- `POST /v1/benchmarks`：创建 BenchmarkSet。
- `GET /v1/benchmarks/:id`：获取 BenchmarkSet 及任务列表。
- `GET /v1/evaluations`：列出 EvaluationRun。
- `GET /v1/evaluations/:id`：获取 EvaluationRun 结果。

## 8. MCP 工具

- `agent_version_list`：列出版本。
- `agent_version_create`：创建 Draft。
- `agent_version_promote`：晋级版本。
- `agent_version_rollback`：回滚版本。
- `experience_candidate_list`：列出经验候选。
- `experience_candidate_review`：审批/拒绝候选。
- `evaluation_run_start`：启动评测。
- `evaluation_run_get`：查询评测结果。

## 9. 权限策略

- 版本管理、经验审核、评测配置仅限 Agent 的 `owner` 或 `admin`。
- `agentversion/application/policy.go` 统一实现 `RequireOwnerOrAdmin`。
- REST/MCP 处理器统一调用 Policy，不自行判断角色。

## 10. 测试策略

- **单元测试**：
  - `AgentVersion` 状态机迁移规则；
  - `content_hash` 计算与不变性约束；
  - `ExperienceCandidate` 分类与审批状态机；
  - `BenchmarkSet` 评分规则与硬门槛判定。
- **集成测试**：
  - Draft → EvaluationRun passed → Eligible → Active 完整链路；
  - 回滚后 `agents.current_version_id` 切换，旧任务版本绑定不变；
  - 从 Accepted Submission 提取候选并审批；
  - 包含敏感数据的候选被自动拒绝。
- **验收测试**：
  - 评分跨版本隔离：不同 `agent_version_id` 的任务结果不合并；
  - 未经验证自动修改生产配置被阻止；
  - 回滚保留历史：旧版本、评测、经验候选均可查询；
  - 跨 tenant 隔离：经验候选和版本查询均受 tenant 边界限制。

## 11. 主要风险

| 风险 | 缓解 |
|---|---|
| 配置无法完整复现 | 保存内容寻址引用 + SHA256 + environment_digest；对象存储接口可后续接入 |
| 经验泄露敏感数据 | 分类策略自动拒绝 `forbidden` 级别；审批通过后才纳入新版本 |
| 基准过拟合 | BenchmarkSet 版本化；保留公开与私有基准；监控真实任务表现 |
| 并发晋级/回滚导致状态不一致 | 使用事务 + 行级锁/乐观锁；状态迁移在领域模型中强制校验 |
| identity 与 agentversion 模块循环依赖 | agentversion 依赖 identity 的 Store 接口；identity 不反向依赖 agentversion |
| 前端版本谱系展示复杂 | 提供 `/versions` 列表 + 父版本 ID，前端递归构建树 |

## 12. 迁移计划

1. 扩展 `agent_versions` 表并迁移现有数据为 `active` 状态。
2. 实现 `agentversion` 领域模型、repository、应用服务和 REST/MCP 接口。
3. 实现 `evaluation` 的 BenchmarkSet、EvaluationRun 和硬门槛判定。
4. 实现 `agentexperience` 的候选提取、分类、审核和绑定。
5. 实现 React 版本谱系、候选经验、评测、晋级/回滚界面。
6. 补充单元、集成、验收测试。

## 13. Spec Patch 说明

本 design 已回写以下 OpenSpec delta spec：

- `specs/agent-versioning/spec.md`：
  - 明确版本状态机与状态迁移规则。
  - 补充 current version 原子切换、并发控制、不可变内容引用和回滚保留历史的验收场景。
- `specs/agent-experience/spec.md`：
  - 明确经验候选敏感级别、数据分类策略、审批人字段。
  - 补充经验候选与新版本绑定、随版本回滚失效的验收场景。
- 新增 `specs/evaluation/spec.md`：
  - 将 EvaluationRun、BenchmarkSet 和硬门槛作为独立 capability 补充，支撑版本晋级要求。

OpenSpec delta spec 仍是行为要求的事实源；本 Design Doc 只描述实现方式。
