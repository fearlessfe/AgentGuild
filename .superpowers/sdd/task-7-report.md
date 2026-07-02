# Task 7 恢复实现报告

## 状态

DONE_WITH_CONCERNS

Task 7 原提交 `21bf114` 已完成严格审计；修复提交为 `b0eedbc`。未修改 plan、OpenSpec tasks、`.comet.yaml` 或 `subagent-progress.md`。工作树中 `openspec/changes/agent-task-lifecycle/.comet/subagent-progress.md` 的修改为接手前已有，未纳入提交。

## 逐项实现核对

- `TraceCostProvider.Observe`、`CostObservation`：接口保留；覆盖枚举纠正为事实源规定的 `complete | partial | unavailable`，删除把 `complete` 错改为 `full` 的迁移。
- Langfuse Cloud：使用 Basic Auth 和 `/api/public/v2/metrics`；通过 URL 编码的 `query` JSON 查询 `observations`，聚合 `sum(totalCost)`，以 `traceTags all of` 同时过滤 tenant、task、execution、agent version 标签，并解析 v2 的 `sum_totalCost`。
- Langfuse 自托管：支持能力开关与可配置兼容 Metrics API 路径；不支持成本读取时返回 `unavailable`。
- Langfuse 故障：HTTP、读取、JSON、成本解析故障返回 `unavailable` 观测及错误；领域事务不受影响，outbox 先记录 unavailable，再保留事件并指数退避重试。
- 覆盖语义：有成本聚合为 `complete`；无成本数据为 `partial`；能力缺失或 Provider 故障为 `unavailable`。
- outbox 并发：`FOR UPDATE SKIP LOCKED` 领取后先提交 `claimed_until`，再调用外部 Provider，避免持有数据库事务跨越网络调用；成功设置 `published_at`，不删除事实记录。
- outbox 退避与恢复：失败增加 `attempts`，以 `claimed_until` 安排有上限的指数退避；高 attempts 使用无溢出算法；同一 provider/source cursor 依赖主键冲突 `DO NOTHING` 保持幂等。
- 限流：Application Service 依赖可替换 `RateLimiter`；本地实现固定按 tenant+Agent 分桶，并保留“仅进程内、不提供跨实例全局配额”的说明；审计查询也应用限流。
- 限流错误：领域错误保留 retry duration；REST 与 MCP 均输出稳定 `RATE_LIMITED` 和向上取整的 `retry_after_seconds`，REST 同时输出 `Retry-After`。
- 审计查询：要求 `tasks:read`，按 tenant+task 先验证可见性，不可见资源映射为 not found；查询仅返回摘要字段，不返回原始 payload；分页限制为 1..100。

## 变更文件

- `backend/internal/telemetry/{cost.go,langfuse.go,langfuse_test.go}`
- `backend/internal/worker/{outbox.go,outbox_test.go}`
- `backend/internal/ratelimit/{limiter.go,limiter_test.go}`
- `backend/internal/application/{audit_queries.go,audit_queries_test.go}`
- `backend/internal/domain/errors.go`
- `backend/internal/transport/rest/{errors.go,router.go,router_test.go}`
- `backend/internal/transport/mcp/{errors.go,server_test.go}`
- `backend/internal/testdb/postgres.go`
- 删除错误迁移 `backend/migrations/000002_execution_usage_coverage.{up,down}.sql`

## 提交

- 原 Task 7 提交：`21bf1147cb5063726b0e4ca8819159b3edb42b96`
- 恢复审计修复：`b0eedbc` (`fix: harden task telemetry operations`)

## TDD / RED 证据

原提交 `21bf114` 的生产代码与测试位于同一提交，历史中没有可复现的中间测试提交或日志，因此：**历史 RED 不可恢复**。未虚构该提交的 RED。

本次审计修复实际执行并观察到的 RED：

1. `go test ./internal/telemetry ./internal/worker ./internal/ratelimit ./internal/application -run 'Langfuse|Outbox|Rate' -count=1`
   - 编译失败：缺少 `MetricsPath`、`CoverageComplete`、`domain.RetryAfterOf`，证明新增契约测试先于实现。
2. `go test ./internal/ratelimit -run 'KeysByTenantAndAgent' -count=1`
   - 失败：同一 tenant+Agent 的不同 AgentVersion 被错误分到不同桶。
3. `GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/transport/rest ./internal/transport/mcp -run 'ApplicationRateLimit' -count=1`
   - 失败：REST 缺少 `Retry-After`/`retry_after_seconds`，MCP 的 `RetryAfterSeconds` 为 0。
4. `go test ./internal/telemetry ./internal/worker -run 'LangfuseCloudProviderReturnsFullCost|LangfuseCloudHTTPError|LangfuseMalformed|OutboxMarksUnavailable' -count=1`
   - 失败：Metrics v2 query 缺 timestamps；畸形成本未返回错误；Provider 故障事件未保留退避。（首次沙箱内执行因本地端口权限失败，随后经批准在沙箱外重跑并取得上述行为 RED。）
5. `go test ./internal/worker -run 'OutboxBackoffCaps' -count=1`
   - 第一次测试夹具使用非法 JSON，被数据库约束提前拒绝；修正为合法但类型错误的 JSON 后取得有效 RED：高 attempts 计算溢出使 `claimed_until` 落到过去。
6. `go test ./internal/application -run 'ListTaskEventsAppliesApplicationRateLimit' -count=1`
   - 失败：第二次审计查询未返回 `rate_limited`。

## GREEN / 验证

- 当前最终 focused：`gtimeout 90s env GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/telemetry ./internal/worker ./internal/ratelimit ./internal/application ./internal/transport/rest ./internal/transport/mcp -run 'Langfuse|Outbox|Rate|Audit|ListTaskEvents' -count=1`
  - PASS：6 个 package 全部通过。
- Backend full：`GOCACHE=/private/tmp/agentguild-go-cache go test ./... -count=1`
  - PASS：所有 backend packages 通过。
- Focused race：`GOCACHE=/private/tmp/agentguild-go-cache go test -race ./internal/worker ./internal/ratelimit ./internal/application -run 'Outbox|Rate|Audit|Langfuse' -count=1`
  - PASS：3 个 package 通过，无 race 报告。
- 当前最终 vet：`gtimeout 90s env GOCACHE=/private/tmp/agentguild-go-cache go vet ./...`
  - PASS，无输出。
- `git diff --check`
  - PASS，无输出。

## 顾虑

- 历史 RED 不可恢复，只能确认本次修复的 RED/GREEN 与最终回归结果。
- Langfuse 协议由官方 Metrics API v2 契约和 `httptest` 验证，未使用真实 Cloud/自托管实例做在线集成测试；不同自托管版本仍需通过 `MetricsPath` 与 `SupportsCost` 配置匹配其能力。

## Review Round 1 Fix

### 修复内容

- outbox 不再依赖 Langfuse Metrics v2 的可选 cursor：按 tenant/execution/provider 使用稳定的 `execution-snapshot:<execution_id>` 逻辑快照键，并用 coverage 等级受约束 UPSERT；`unavailable -> partial -> complete` 可升级，较差 coverage 不得覆盖 complete。
- `claimed_until` 的精确值作为 claim fencing token。usage、成功发布、失败退避全部先验证 tenant+event+token 所有权，0 rows 明确视为 lease lost；Provider 网络调用仍位于事务外。
- 成功/失败事务分别重新读取 `clock_timestamp()`，用 fresh PostgreSQL time 写 `observed_at`、`published_at` 和退避时间。
- 缺 `task_id`/`execution_id` 的毒事件返回处理错误并增加 attempts/退避，不再静默循环。
- Langfuse 仅在配置明确的 `CompleteCoverageTag` 且查询命中成本时判定 complete；无该外部覆盖证据时有成本也保持 partial，partial/unavailable 保留重试机会。
- MCP HTTP middleware 的 429 JSON body 现包含 `RATE_LIMITED` 与 `retry_after_seconds`。
- 审计摘要及 PostgreSQL 查询彻底移除自由文本 Reason；补充多页游标测试，并修复 fake repository 未回填摘要 ID 的夹具错误。
- complete 零成本以数值 0 存储；本地令牌桶构造器拒绝非正 Rate/Burst；Start/Heartbeat 在消耗配额前检查 `tasks:execute`。

### 本轮 TDD / RED 证据

1. `go test ./internal/telemetry ./internal/worker -run 'LangfuseCostWithout|EmptyCursor|PartialRemains|StaleClaimant|PoisonEvent|CompleteZero' -count=1`
   - RED：`CompleteCoverageTag` 尚不存在；恢复仍为 unavailable；partial 被发布；stale claimant 返回 nil；毒事件 attempts 为 0；complete 零成本为 NULL。
2. `go test ./internal/ratelimit -run 'RejectsInvalid' -count=1`
   - RED：构造器只返回一个值，无法报告非法 Rate/Burst。
3. `go test ./internal/transport/mcp ./internal/application -run 'MCPRateLimiter|TenantScopedAndSanitized' -count=1`
   - RED：MCP middleware body 的 `RetryAfterSeconds` 为 0；修正测试 import 后审计 JSON 仍包含 `reason:"wanted"`。
4. `env GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/application -run 'AuthorizeBeforeConsuming' -count=1`
   - RED：两个未授权 Start/Heartbeat 请求消耗了 2 次限流配额。
5. `go test ./internal/worker -run UsesFreshDatabaseTime -count=1`（临时恢复旧 claim-time 实现以验证测试）
   - RED：成功处理的 `observed_at` 早于 Provider 完成时的 PostgreSQL 时间。

### 本轮 GREEN / 验证证据

- 沙箱外 focused（实现完成后的首轮）：`env GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/telemetry ./internal/worker ./internal/ratelimit ./internal/application ./internal/transport/mcp -run 'Langfuse|Outbox|Rate|Audit|AuthorizeBefore|TenantScoped' -count=1`
  - PASS：5 个 package 全部通过，包含恢复升级、stale claimant、毒事件、零成本、coverage 依据、MCP 与审计契约。
- 最新纯单元 focused：`env GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/ratelimit ./internal/application ./internal/transport/mcp -run 'Rate|Audit|AuthorizeBefore|ListTaskEvents|MCPApplicationRate' -count=1`
  - PASS：3 个 package 全部通过，包含新增多页审计测试。
- 最新 focused race：`env GOCACHE=/private/tmp/agentguild-go-cache go test -race ./internal/ratelimit ./internal/application -run 'Rate|Audit|AuthorizeBefore|ListTaskEvents' -count=1`
  - PASS：2 个 package，无 race 报告。
- `env GOCACHE=/private/tmp/agentguild-go-cache go vet ./...`
  - PASS，无输出。
- `git diff --check`
  - PASS，无输出。

### 本轮验证限制

- 最新 worker/telemetry 聚焦重跑在沙箱内被 loopback/PostgreSQL 权限阻止；再次申请沙箱外执行时被账户工具额度拒绝。新增的最后两项 worker fencing/降级测试已通过 `go vet ./...` 编译，但未取得当前工作树上的运行证据。
- 因同一额度限制，本轮未重新执行 backend full 与包含 worker 的 race；不沿用上一轮结果冒充当前结果。
- 仍未连接真实 Langfuse Cloud/自托管实例；complete 依赖执行侧正确写入配置的覆盖证明 tag。
