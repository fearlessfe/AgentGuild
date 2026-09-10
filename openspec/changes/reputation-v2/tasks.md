## 1. 版本化的投影与参数

- [x] 1.1 新增迁移 `000029_reputation_v2`：`agent_reputation_projections`（主键含 `algorithm_version` 与 `scope`）、`agent_reputation_dimension_scores`（CASCADE 到 header）
- [x] 1.2 `reputation_algorithm_params`：维度权重、半衰期、recent 权重、Wilson z、先验、最小样本，全部作为版本化数据落库，并预置首版参数行
- [x] 1.3 `task_difficulty_classes`：`multiplier` CHECK 在 `[0.75, 1.50]`，预置 trivial / standard / substantial / complex
- [x] 1.4 header 上的 CHECK：scope 与细分键的组合合法、未验证投影不得带总分、维度行 `observed = (sample_size > 0)`
- [x] 1.5 在 `internal/testdb/postgres.go` 三处调用点注册迁移

## 2. 纯函数算法层

- [x] 2.1 `domain/wilson.go`：接受非整数伪计数的 Wilson 置信下界
- [x] 2.2 `domain/decay.go`：`2^(-age/halfLife)`，未来时间戳不放大权重
- [x] 2.3 `domain/dimension.go`：七个维度、统一的 `Observation`、单维度评分与稳定分组
- [x] 2.4 `domain/score.go`：`Params` 校验、`ScoreKey` 合法性、零观测维度剔除并重新归一化、样本门槛与 `SampleSizeHint`
- [x] 2.5 全部表驱动单测，覆盖零观测、样本门槛、衰减与输入顺序无关性

## 3. 事实映射与重算

- [x] 3.1 `application/facts.go`：把 criterion 结果、CI/merge/revert 事件、lease 与截止时间、评审循环、安全步骤、难度系数降解为统一观测流
- [x] 3.2 `application/scorer.go`：三层投影分组，样本量按已验证贡献数而非观测条数计
- [x] 3.3 `application/rebuilder.go`：`FactSource` / `ParamsRepository` / `ScoreCardRepository` 三个端口与 `Rebuild(ctx, algorithmVersion, evaluatedAt)`
- [x] 3.4 `postgres/fact_source.go`：从 contribution 账本、criterion 账本、验证步骤与难度分级读取确定性事实
- [x] 3.5 `postgres/score_card_repository.go`：`ReplaceAlgorithm` 在单事务内只删该算法版本
- [x] 3.6 `postgres/params_repository.go`：读取版本化参数；`EnsureVersion` 只在参数行缺失时创建
- [x] 3.7 `worker/rebuild_worker.go`：按事实层水位增量触发，无新事实时是 no-op

## 4. 暴露面

- [x] 4.1 `application/query_v2.go`：三层视图的只读服务，视图不含任何 sponsor 租户标识
- [x] 4.2 REST `GET /v1/public/agents/{id}/reputation`、`GET /v1/agents/me/reputation`
- [x] 4.3 REST `POST /v1/admin/reputation:rebuild`（管理员 + 幂等键，可显式指定 `evaluated_at`）
- [x] 4.4 MCP `agent_reputation_get`（不复用 v1 的 `reputation_get`）
- [x] 4.5 openapi.yaml 同步：路径与 `AgentReputation*` / `ReputationRebuildResult` schema
- [x] 4.6 REST/MCP 契约等价测试与匿名不泄露租户的断言
- [x] 4.7 配置三项并同步 `AGENTS.md`；在 `cmd/agentguild-api/main.go` 装配服务与 worker

## 5. 验收

- [x] 5.1 删光投影后在固定 `evaluatedAt` 下重算，逐字节复现
- [x] 5.2 同一事实重复投递不改变任何分数
- [x] 5.3 新建 Agent Version 样本归零且无总分，agent lifetime 保留全部历史样本
- [x] 5.4 规格中存在但无结果的安全标准不产生观测，security 维度保持零样本
- [x] 5.5 重算某个算法版本不影响其他版本的投影；v1 的 `reputation_projections` 不被写入
