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
