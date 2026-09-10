## 1. 结算抽象

- [x] 1.1 `internal/settlement/provider.go`：`Provider` 接口（`Pay` / `Refund` / `Status`），全部以 `decision_hash` 为幂等键；`Request.Validate` 做提供方无关的校验
- [x] 1.2 `internal/settlement/fake/provider.go`：内存实现，支持重复回调 no-op、退款、以及"先失败再重试成功"的失败注入
- [x] 1.3 配置 `SETTLEMENT_PROVIDER`（默认 `fake`，未知值启动报错）、`REWARD_CURRENCY_ALLOWLIST`、`REWARD_DEFAULT_CHALLENGE_PERIOD`、`REWARD_WORKER_INTERVAL`，并同步 `AGENTS.md`

## 2. 迁移与账本

- [x] 2.1 `000030_reward_ledger`：`sponsor_escrow_accounts` + append-only `sponsor_escrow_entries`（余额非负 CHECK、分录幂等键）
- [x] 2.2 `reward_policies`：整数 bps 的分成比例、`policy_hash` 唯一、`(tenant, task)` 上只允许一条可领取的契约
- [x] 2.3 `reward_locks`：Claim 时刻的 policy 快照与 `policy_hash`、六态状态机、终态必须带时间戳、一个 Execution 至多一条锁
- [x] 2.4 `reward_decisions`：append-only 触发器拒绝 UPDATE / DELETE
- [x] 2.5 `reward_disputes` + `reward_dispute_events`：一个锁至多一条争议
- [x] 2.6 `payout_destination_challenges` + `payout_destinations`：一个 Agent 至多一个 verified 目的地
- [x] 2.7 `payment_receipts`：以 `(provider, reference, state)` 幂等追加
- [x] 2.8 在 `internal/testdb/postgres.go` 三处调用点注册迁移

## 3. 领域层

- [x] 3.1 `domain/money.go`：`AmountMinor int64` + currency allowlist，禁止浮点
- [x] 3.2 `domain/policy.go`：`policy_hash` 用 canonicaljson + sha256；创建后不可变；状态机在领域层
- [x] 3.3 `domain/lock.go`：执行级状态机与终态判定
- [x] 3.4 `domain/decision.go`：分配算法 `net = gross − platform_fee − dispute_reserve`；**任一 required criterion 未通过 ⇒ Agent 份额为 0**；`recipient_ref = sha256(chain|address)`
- [x] 3.5 `domain/decision.go`：`Signer` 与 `HMACSigner`，签名密钥从部署密钥**派生**而非直接复用
- [x] 3.6 `domain/dispute.go`、`domain/receipt.go`、`domain/destination.go`、`domain/escrow.go`、`domain/errors.go`
- [x] 3.7 领域层表驱动单测：状态机、分配、签名确定性

## 4. 应用层、存储与 worker

- [x] 4.1 `application/{service,decision,dispute,destination}.go`：托管、契约、决策、结算、争议、收款目的地
- [x] 4.2 `application/ports.go`：`Store` / `Tx` 与七个仓储端口；`EvidenceSource` 只回传可验证摘要，不回传 Issue、diff 或评审内容
- [x] 4.3 `application/views.go`：视图白名单，**绝不含 `resource_tenant_id`、sponsor 账户或余额**
- [x] 4.4 `postgres/*`：仓储实现与 `Store.WithTx`
- [x] 4.5 `postgres/claim_participant.go`：在 publictask 的 Claim 事务里锁定资金，余额不足则整个 claim 回滚
- [x] 4.6 `worker/worker.go`：决策 → releasable → 挑战期届满释放 → 过期回收

## 5. 暴露面

- [x] 5.1 `internal/rewardaccess`：全局标识到租户的翻译与跨租户 grant 授权，REST 与 MCP 共享同一份判断
- [x] 5.2 REST `reward_router.go` 与路由注册；全部 mutation 挂 `mutationIdempotency(op)`
- [x] 5.3 MCP `reward_tools.go`：`reward_get`、`reward_destination_challenge`、`reward_destination_verify`（后两个走 `idempotentMutation`）
- [x] 5.4 两个 transport 注册 `insufficient_escrow` 与 `policy_immutable` 错误码
- [x] 5.5 `openapi.yaml` 同步：12 条路径与全部 `Reward*` / `Payout*` / `Escrow*` schema
- [x] 5.6 `cmd/agentguild-api/main.go` 装配服务、Claim 参与者与 worker

## 6. 验收

- [x] 6.1 `internal/transport/rest/public_leak_test.go`：表驱动，打全部匿名端点并断言原始 JSON 不含租户 ID / 组织名 / sponsor 账户 ID
- [x] 6.2 `internal/transport/mcp/reward_leak_test.go`：MCP 侧同一批毒串，且跨租户 Agent 拿不到别人的执行级奖励
- [x] 6.3 `internal/transport/contract/reward_equivalence_test.go`：REST/MCP 对同一领域结果与错误码等价
- [x] 6.4 完整闭环：充值 → 建契约并出资 → claim 锁定快照 → 全部标准通过 → 释放 → 重复回调 no-op
- [x] 6.5 doc §10-1 重复投递不重复计入奖励
- [x] 6.6 doc §10-2 声望重算不改变任何资金状态
- [x] 6.7 doc §10-3 新 Agent Version 不改写历史锁的归属
- [x] 6.8 doc §10-4 Claim 后契约不可变，直接改库也改不动锁上的快照
- [x] 6.9 doc §10-5 required criterion 失败与从未验证两种情形都释放 0
- [x] 6.10 doc §10-6 决策可用公开 hash 与签名验证且不泄露私有内容
- [x] 6.11 doc §10-7 重复支付、退款、争议裁决与过期全部幂等
- [x] 6.12 doc §10-8 钱包轮换不改变历史归属，旧钱包不能二次领取
