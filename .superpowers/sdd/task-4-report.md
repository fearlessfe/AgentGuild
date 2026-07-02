# Task 4 Report — Claim、Lease、Heartbeat、Reaper

## 状态

实现完成；真实 PostgreSQL 18.4 focused、full 与 race runtime gates 均已通过。

## 实现

- `Service.ClaimTask`：单事务条件 Claim，创建 generation=1 的 Leased Execution，10m soft lease、30s grace，保存随机 lease secret 的 SHA-256，幂等响应、审计和 outbox 同事务。
- `Service.StartExecution`：校验 tenant、holder、current generation、hard expiry 和 Task deadline，原子推进 Execution→running 与 Task→in_progress。
- `Service.HeartbeatExecution`：校验 holder、current generation、active status、hard expiry 和 Task deadline，成功后 generation+1 并从数据库事务时间重算 soft/hard expiry。
- `Service.GetExecution`：tenant 与 execution holder 隔离。
- PostgreSQL fencing 更新包含 owner、expected state version、current generation、active status 和 hard expiry 条件。
- `Reaper.RunBatch`：事务内缓存数据库时间，`FOR UPDATE OF e SKIP LOCKED`，支持多实例；hard-expired 且未到 deadline 时 Execution→expired/Task→open；deadline 等值或已过时 Execution+Task→expired；审计和 outbox 同事务；终态不再命中，重复运行幂等。
- 未引入最大运行时或续租次数上限。

## TDD 证据

1. Application RED：新增 Claim/Start/Heartbeat/GetExecution 测试后，因 API 不存在编译失败。
2. Application GREEN：实现最小 API 与 repository fencing 后 focused 测试通过。
3. 跨聚合 RED：Start 未推进 Task、deadline 等值 heartbeat 未拒绝，两项断言失败。
4. 跨聚合 GREEN：加入 Task deadline 校验和 Start 原子 Task 更新后 focused 测试通过。
5. Reaper RED：新增 Reaper 集成测试后，因 `postgres.NewReaper` 不存在编译失败。
6. Reaper 首次 runtime RED：3 个测试在 fixture seed 稳定失败，`ERROR: cannot insert multiple commands into a prepared statement (SQLSTATE 42601)`。
7. 根因调查一：`seedReaperExecution` 将 3 条带参数 SQL 放入一次 `pgxpool.Exec`，而 pgx extended/prepared protocol 不允许一个 prepared statement 包含多条命令；仓库工作的 seed pattern 均为一条 SQL 一次 `Exec`。拆分为 3 次 `Exec`。
8. Reaper 第二次 runtime RED：3 个测试进入下一层稳定失败，`column lease_soft_expires_at is timestamptz but expression is interval (SQLSTATE 42804)`。
9. 根因调查二：prepared SQL 的未定型参数 `$3 - interval '30 seconds'` 受减法重载影响被推断为 interval；工作的 repository insert 直接绑定独立 Go `time.Time`。fixture 改为在 Go 中计算 soft expiry，并分别绑定 soft/hard `time.Time`。
10. Reaper runtime GREEN：协调器在现有 PostgreSQL 18.4 容器上运行 focused gate，Claim、generation 与全部 Reaper 测试通过。

## 已执行验证

- `GOCACHE=/tmp/agentguild-go-cache go test ./internal/application -count=1` — PASS
- `GOCACHE=/tmp/agentguild-go-cache go test ./internal/application ./internal/domain -count=1` — PASS
- `GOCACHE=/tmp/agentguild-go-cache go test -race ./internal/application ./internal/domain -count=1` — PASS
- `GOCACHE=/tmp/agentguild-go-cache go test ./internal/postgres -run TestCanonicalHash -count=1` — PASS
- `go test ./internal/postgres -run 'ConcurrentClaim|ClaimRetry|HeartbeatGeneration|Reaper' -count=1` — PASS（协调器代跑，PostgreSQL 18.4，`ok ... 0.966s`）
- `go test ./internal/postgres -count=1` — PASS（协调器代跑，PostgreSQL 18.4，`ok ... 1.483s`）
- `go test -race ./internal/postgres ./internal/application ./internal/domain -count=1` — PASS（协调器代跑；分别 `3.188s / 1.856s / 2.116s`）
- `GOCACHE=/tmp/agentguild-go-cache go test ./... -run '^$' -count=1` — PASS（全仓编译）
- `GOCACHE=/tmp/agentguild-go-cache go test -race ./internal/postgres -run '^$' -count=1` — PASS（race 构建，无 runtime tests）
- `GOCACHE=/tmp/agentguild-go-cache go vet ./...` — PASS
- `git diff --check` — PASS

环境说明：本 Agent 的默认 Go cache `~/Library/Caches/go-build` 被 sandbox 拒绝，故本地命令使用 `/tmp/agentguild-go-cache`。PostgreSQL runtime gates 由协调器连接现有 `127.0.0.1:55432` PostgreSQL 18.4 容器代跑并回传结果。

## 顾虑

- 无已知功能阻塞；focused、full、race、vet 与 diff gates 均通过。
- 未修改 OpenSpec、plan、REST、MCP、frontend，也未勾选 Task 4。

## Review Round 1 修复

### 实现

- PostgreSQL adapter 仅将 constraint `executions_one_active_per_task` 的 `23505` 映射为 domain `state_conflict`；Application 未引入 `pgx`/`pgconn`/PostgreSQL 依赖。
- Execution mutation 新增显式 `GetExecutionForUpdate`。Start/Heartbeat 先锁 Execution；Start 先写 Execution、再写 Task，和 Reaper 的 Execution→Task 顺序一致。
- Start 的 Task 更新失败由同一事务回滚已写入的 Execution、幂等记录、事件与 outbox；新增 fake transaction 回滚断言。
- Reaper 状态断言现在直接验证 `tasks.active_execution_id` 被清空。
- 新增真实 PostgreSQL 确定性竞态测试：advisory-lock trigger 负责暂停指定写入，`pg_locks` 的 advisory/transactionid waiter 与数据库时钟 predicate 负责确认阶段；没有用固定 sleep 猜测事务是否到位。
  - Start-vs-Reaper：Start 在 lease 到期前取得事务时间并持有 Execution 锁，Task trigger 暂停后等待数据库时钟越过 hard expiry；Reaper 必须通过 `SKIP LOCKED` 返回 0，随后 Start 完整提交 running/in_progress，不出现 `40P01` 或 partial state。
  - heartbeat-vs-Reaper：Reaper 持有 Execution 锁并暂停，确认 heartbeat 正在等待 transactionid lock 后释放；Reaper 完整提交 expired/open/active_execution_id=NULL，heartbeat 读取已提交终态并返回 `state_conflict`，不出现 partial state。

### TDD 证据

1. Adapter RED：先新增 `TestMapExecutionInsertErrorMapsActivePerTaskUniqueViolation`，运行时稳定编译失败：`undefined: mapExecutionInsertError`。
2. Adapter GREEN：实现指定 SQLSTATE+constraint 的最小映射后，内部 focused 单测通过。
3. PostgreSQL RED 覆盖：新增真实 unique-index defense、Start-vs-Reaper、heartbeat-vs-Reaper 测试；本 Agent 无 PG runtime 权限，因此未虚报本地 RED，按 brief 将精确命令交协调器运行。
4. Lock-order GREEN：加入 `GetExecutionForUpdate` 并将 Start 写序改为 Execution→Task；协调器代跑全部 4 个 focused PostgreSQL 测试通过。
5. Rollback GREEN：`TestStartRollsBackExecutionWhenTaskUpdateFails` 验证 Task 失败后 Execution 仍为 leased，Task 仍为 claimed，且无事件、outbox 或幂等 partial state。

### Review Round 1 验证

- `GOCACHE=/tmp/agentguild-go-cache go test ./internal/postgres -run 'TestClaimMapsActiveExecutionUniqueViolationToStateConflict|TestStartAndReaperUseExecutionThenTaskLockOrder|TestHeartbeatWaitsForReaperExecutionLockAndObservesExpiredState|TestReaperReopensHardExpiredTaskAndIsIdempotent' -count=1 -v` — PASS（协调器代跑，PostgreSQL 18.4，4/4，package `1.301s`）
- `GOCACHE=/tmp/agentguild-go-cache go test ./internal/postgres -count=1` — PASS（协调器代跑，PostgreSQL 18.4，package `2.249s`）
- `GOCACHE=/tmp/agentguild-go-cache go test -race ./internal/postgres -count=1` — PASS（协调器代跑，PostgreSQL 18.4，package `3.899s`）
- `GOCACHE=/tmp/agentguild-go-cache go test ./internal/application -run 'Start|Heartbeat' -count=1` — PASS
- `GOCACHE=/tmp/agentguild-go-cache go test ./internal/application ./internal/domain -count=1` — PASS
- `GOCACHE=/tmp/agentguild-go-cache go test -race ./internal/application ./internal/domain -count=1` — PASS
- `GOCACHE=/tmp/agentguild-go-cache go test ./... -run '^$' -count=1` — PASS
- `GOCACHE=/tmp/agentguild-go-cache go test -race ./internal/postgres -run '^$' -count=1` — PASS
- `GOCACHE=/tmp/agentguild-go-cache go vet ./...` — PASS
- `git diff --check` — PASS

### Review Round 1 顾虑

- 无已知功能阻塞；真实 PG focused/full/race、application/domain full/race、vet 与 diff gates 均通过。
- 本 Agent 的 PG runtime 命令受执行环境本机网络权限/额度拒绝；所有真实 PostgreSQL 18.4 runtime 结果均由协调器代跑并回传。
