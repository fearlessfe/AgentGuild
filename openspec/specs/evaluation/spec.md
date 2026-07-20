# evaluation Specification

## Purpose
定义基准集（BenchmarkSet）版本化管理与评测运行（EvaluationRun）的启动门禁、硬门槛判定与结果对 Agent Version 状态的驱动规则，确保评测证据可审计、执行器身份可溯源。
## Requirements
### Requirement: 评测运行冻结候选版本与基准集
系统 MUST 在启动 EvaluationRun 时冻结 `agent_version_id`、`benchmark_set_id`、`environment_digest` 和 `scoring_rule_version`，确保结果可审计、不可被后续配置变更污染。

#### Scenario: 启动 EvaluationRun
- **WHEN** owner 对某个 Draft 版本请求启动评测
- **THEN** 系统创建 `EvaluationRun` 记录，状态为 `running`，并锁定上述字段

#### Scenario: 仅 Draft 版本可启动评测
- **WHEN** owner 对非 `draft` 状态（如 `evaluating`、`eligible`、`rejected`）的版本请求启动评测
- **THEN** 系统拒绝并返回状态冲突错误

#### Scenario: 评测运行期间版本被修改
- **WHEN** 某版本已关联处于 `running` 状态的 EvaluationRun
- **THEN** 系统拒绝修改该版本的内容引用或配置字段

### Requirement: 硬门槛决定评测结果
系统 MUST 根据预定义硬门槛判定 EvaluationRun 为 `passed` 或 `failed`，并保存各门槛检查结果。

#### Scenario: 所有硬门槛通过
- **WHEN** EvaluationRun 完成且全部 `threshold_results` 为通过
- **THEN** 评测状态变为 `passed`

#### Scenario: 任一硬门槛失败
- **WHEN** EvaluationRun 完成且至少一项硬门槛失败
- **THEN** 评测状态变为 `failed`，并保留失败证据

### Requirement: 评测结果驱动版本状态
系统 SHALL 根据最新通过的 EvaluationRun 将 Agent Version 推进到 `eligible`；评测失败时 MUST 将该版本标记为 `rejected` 并保留失败证据。`rejected` 为终态：该版本不可重跑评测，如需重试 MUST 创建新的 Draft 版本。

#### Scenario: 评测通过晋级 Eligible
- **WHEN** 某版本最新 EvaluationRun 状态为 `passed`
- **THEN** 该 Agent Version 可被推进为 `eligible`

#### Scenario: 评测失败版本被拒绝且不可重跑
- **WHEN** 某版本 EvaluationRun 状态为 `failed`
- **THEN** 系统保留失败结果并将版本标记为 `rejected`，对该版本再次启动评测返回状态冲突错误

### Requirement: 基准集版本化
系统 MUST 对 BenchmarkSet 进行版本化管理，同一租户内 `version_number` 单调递增，且支持标记当前默认使用的 Active 基准集。

#### Scenario: 创建新基准集版本
- **WHEN** owner 创建 BenchmarkSet
- **THEN** 系统分配新的 `version_number`，并可选将其标记为 `is_active`

#### Scenario: 使用 Active 基准集自动评测
- **GIVEN** `EVALUATION_AUTO=true` 且评测执行器已配置
- **WHEN** 租户存在 `is_active=true` 的 BenchmarkSet，且有新 Draft 版本创建成功
- **THEN** 系统自动以建版人身份使用该 Active BenchmarkSet 启动 EvaluationRun；无 Active 基准集或版本状态冲突时跳过，不影响建版

### Requirement: 评测执行器受配置门禁且身份可溯源
系统 MUST 通过 `EVALUATION_EXECUTOR` 显式启用评测执行器：未设置时 `StartEvaluationRun` MUST fail closed 并返回 `evaluation_unavailable` 领域错误；`=fixed` 时启用固定通过的 stub 执行器（仅限开发/演示，启动时输出醒目 warning）；`=platform` 时启用真实执行器，启动评测时为每个基准任务发布一条真实平台任务，run 保持 `running`，由 evaluation harvest worker 收割各任务的验证结果并评分完成；其他取值 MUST 导致启动配置错误。系统 SHALL 在评测运行 summary 的 `executor` 字段记录产生结果的真实执行器身份（stub 执行器记录为 `fixed-stub`，platform 执行器记录为 `platform`），拒绝所有运行的禁用执行器不得出现在 summary 中。

#### Scenario: 未配置执行器时评测失败关闭
- **GIVEN** `EVALUATION_EXECUTOR` 未设置
- **WHEN** owner 请求启动 EvaluationRun
- **THEN** 系统返回 `evaluation_unavailable` 且不创建运行记录

#### Scenario: 评测运行证据包含执行器身份
- **GIVEN** `EVALUATION_EXECUTOR=fixed`
- **WHEN** EvaluationRun 完成
- **THEN** 运行 summary 的 `executor` 字段为 `fixed-stub`，评审方可据此判断证据来源

#### Scenario: platform 执行器发布真实任务并收割评分
- **GIVEN** `EVALUATION_EXECUTOR=platform`
- **WHEN** owner 请求启动 EvaluationRun
- **THEN** 系统为每个基准任务发布一条真实平台任务，run 保持 `running`；harvest worker 收割全部任务验证结果后评分完成运行，summary 的 `executor` 字段为 `platform`

### Requirement: 评测运行详情接口返回聚合结果
系统 SHALL 在 `/v1/evaluations/{id}` 返回包含 `threshold_results` 和 `summary` 的完整 EvaluationRun 详情，字段名为 snake_case。

#### Scenario: 查看评测运行详情
- **WHEN** 调用者请求 `/v1/evaluations/{id}` 或通过 MCP `evaluation_run_get` 查询
- **THEN** 响应包含 `id`、`agent_version_id`、`benchmark_set_id`、`status`、`environment_digest`、`scoring_rule_version`、`threshold_results`、`summary`（含 `pass_rate`、`avg_latency_ms`、`cost_cents`、`security_passed`）及时间戳

### Requirement: 评测运行列表返回完整详情
系统 SHALL 在 `/v1/evaluations` 返回包含 `threshold_results` 和 `summary` 的 EvaluationRun 详情列表，字段名为 snake_case，并包装在 `Envelope<EvaluationRunPage>` 中。

#### Scenario: 查看评测运行列表
- **WHEN** 调用者请求 `/v1/evaluations?agent_version_id={id}`
- **THEN** 响应为 `{data: {items: [...]}, meta: {...}}`，其中 `items` 每项包含完整 `EvaluationRunView` 字段

### Requirement: 基准集与评测运行 REST/MCP 视图字段使用 snake_case
系统 SHALL 保证基准集和评测运行接口返回的 JSON 字段名为 snake_case，以与人类控制台前端类型一致。

#### Scenario: 基准集视图字段为 snake_case
- **WHEN** 调用者请求任一基准集或评测运行接口
- **THEN** 响应 JSON 字段名为 `id`、`tenant_id`、`version_number`、`is_active`、`threshold_results` 等 snake_case 形式

### Requirement: 基准集与评测运行 REST 响应使用 Envelope<T> 信封
系统 SHALL 将基准集和评测运行的查询端点响应包装在 `{data, meta}` 信封中，与任务生命周期、Identity 模块保持一致。

#### Scenario: 列出基准集
- **WHEN** 调用者请求 `/v1/benchmarks`
- **THEN** 响应体为 `{ data: { items: [...] }, meta: { server_time } }`

#### Scenario: 创建基准集
- **WHEN** 调用者请求 `POST /v1/benchmarks`
- **THEN** 响应体为 `{ data: { benchmark_set_id, version_number }, meta: { server_time } }`

#### Scenario: 查看评测运行详情（信封形式）
- **WHEN** 调用者请求 `/v1/evaluations/{id}`
- **THEN** 响应体为 `{ data: EvaluationRunDetail, meta: { server_time } }`
