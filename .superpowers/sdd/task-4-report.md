# Task 4 Report — Claim、Lease、Heartbeat、Reaper

## 状态

实现完成；真实 PostgreSQL 18.4 runtime gate 因当前执行环境无法获准启动 Docker，待主 Agent/用户侧代跑。

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
6. Reaper GREEN（编译级）：实现 Reaper 后 PostgreSQL 包完整编译通过；runtime 受环境阻塞，未虚报为 PASS。

## 已执行验证

- `GOCACHE=/tmp/agentguild-go-cache go test ./internal/application -count=1` — PASS
- `GOCACHE=/tmp/agentguild-go-cache go test ./internal/application ./internal/domain -count=1` — PASS
- `GOCACHE=/tmp/agentguild-go-cache go test -race ./internal/application ./internal/domain -count=1` — PASS
- `GOCACHE=/tmp/agentguild-go-cache go test ./internal/postgres -run TestCanonicalHash -count=1` — PASS
- `GOCACHE=/tmp/agentguild-go-cache go test ./... -run '^$' -count=1` — PASS（全仓编译）
- `GOCACHE=/tmp/agentguild-go-cache go test -race ./internal/postgres -run '^$' -count=1` — PASS（race 构建，无 runtime tests）
- `GOCACHE=/tmp/agentguild-go-cache go vet ./...` — PASS
- `git diff --check` — PASS

## 尚待执行

```bash
cd backend
GOCACHE=/tmp/agentguild-go-cache go test ./internal/postgres -run 'ConcurrentClaim|ClaimRetry|HeartbeatGeneration|Reaper' -count=1
GOCACHE=/tmp/agentguild-go-cache go test ./internal/postgres ./internal/application ./internal/domain -count=1
GOCACHE=/tmp/agentguild-go-cache go test -race ./internal/postgres ./internal/application ./internal/domain -count=1
```

环境说明：默认 Go cache `~/Library/Caches/go-build` 被 sandbox 拒绝，已改用 `/tmp/agentguild-go-cache`。本机 `127.0.0.1:55432` 无可用 PostgreSQL；`testdb` 需要 Docker 启动 `postgres:18.4`，但 `require_escalated` 因平台用量额度被自动拒绝。

## 顾虑

- PostgreSQL 集成测试尚未实际运行，因此 100 并发 Claim、真实 SQL generation fencing、多 Reaper SKIP LOCKED 与 deadline runtime 行为仍需上述 gate 确认。
- 未修改 OpenSpec、plan、REST、MCP、frontend，也未勾选 Task 4。
