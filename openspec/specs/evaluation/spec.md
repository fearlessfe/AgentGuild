# evaluation Specification

## Purpose
TBD - created by archiving change agent-version-and-experience. Update Purpose after archive.
## Requirements
### Requirement: 评测运行冻结候选版本与基准集
系统 MUST 在启动 EvaluationRun 时冻结 `agent_version_id`、`benchmark_set_id`、`environment_digest` 和 `scoring_rule_version`，确保结果可审计、不可被后续配置变更污染。

#### Scenario: 启动 EvaluationRun
- **WHEN** owner 对某个 Draft/Eligible 版本请求启动评测
- **THEN** 系统创建 `EvaluationRun` 记录，状态为 `running`，并锁定上述字段

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
系统 SHALL 根据最新通过的 EvaluationRun 将 Agent Version 推进到 `eligible`；未通过时允许重跑或标记为 `rejected`。

#### Scenario: 评测通过晋级 Eligible
- **WHEN** 某版本最新 EvaluationRun 状态为 `passed`
- **THEN** 该 Agent Version 可被推进为 `eligible`

#### Scenario: 评测失败保留证据
- **WHEN** 某版本 EvaluationRun 状态为 `failed`
- **THEN** 系统保留失败结果，版本可重跑评测或标记为 `rejected`

### Requirement: 基准集版本化
系统 MUST 对 BenchmarkSet 进行版本化管理，同一租户内 `version_number` 单调递增，且支持标记当前默认使用的 Active 基准集。

#### Scenario: 创建新基准集版本
- **WHEN** owner 创建 BenchmarkSet
- **THEN** 系统分配新的 `version_number`，并可选将其标记为 `is_active`

#### Scenario: 使用 Active 基准集自动评测
- **WHEN** 系统配置为自动评测且存在 `is_active=true` 的 BenchmarkSet
- **THEN** 新 Draft 版本创建后可自动使用 Active BenchmarkSet 启动 EvaluationRun

