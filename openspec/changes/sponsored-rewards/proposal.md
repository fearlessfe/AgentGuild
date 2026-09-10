## Why

`docs/agent-first-reputation-and-blockchain-rewards.md` §5 要求 AgentGuild 是一个**有资金结算的**开放任务市场：Sponsor 出资、奖励政策在 Claim 时锁定、验收标准决定释放、挑战期内可争议。

当前仓库里这条链路完全不存在——`grep reward|escrow|wallet|settlement` 在 `backend/internal` 下零命中。任务能被发现、领取、交付、评审、计入声望，但"这次交付值多少钱、谁付、什么条件下付"没有任何地方安放。

Stage 1 的 `execution_criterion_results` 与 Stage 2 的声望 v2 已经把"哪条验收标准通过了"和"这个 Agent 有多可靠"变成可查询的事实。本变更把它们接到资金上：奖励释放由 criterion 证据决定，而不是由人临时判断。

本变更**不写合约**（§7 明确第一阶段不需要），只保留 settlement provider 接口与 fake 实现。

## What Changes

- 新增 sponsor 托管账本：`sponsor_escrow_accounts` + append-only 的 `sponsor_escrow_entries`。余额非负由数据库 CHECK 强制，每条分录带幂等键——重复回调、重复支付、重复退款的共同兜底。
- **文档 §3.2 的单条 Reward 状态链拆成两条**：`RewardPolicy`（任务级出资：`unfunded → funded → exhausted|cancelled`）与 `RewardLock`（执行级锁定：`locked → releasable → released|refunded|disputed|expired`）。原文把任务级出资和执行级锁定混在一条链上，无法表达"一个任务被多次 claim / 退款"。
- **分成比例一律用整数 basis points，不用小数**。浮点数无法产生稳定的 `policy_hash`，也无法让第三方跨语言复算 `RewardDecision`。
- `RewardPolicy` 创建后不可变；Claim 时把整份快照连同 `policy_hash` 冻结在 `RewardLock` 上。此后 Issue 如何更新都不改变已经开始执行的经济契约。
- 与现有 Claim 事务的集成用 `ClaimParticipant` 接口：`publictask` 的 `Claim()` 在已持锁的事务里回调参与者，reward 在同一事务内校验余额并写入锁。**余额不足则整个 claim 失败、任务保持 `open`**——不允许无资金背书的 claim（文档未定义此情形，此为本变更的默认决策）。
- `RewardDecision` 是 append-only 的可验证事实：`task_spec_hash`、`contribution_hash`、逐条 criterion 结果、金额分配、`recipient_ref = sha256(chain|address)`、算法版本、挑战期与签名。**任一 required criterion 未通过 ⇒ Agent 份额为 0**；规格中存在但无任何结果的 criterion 记为未验证，绝不当作通过。
- 收款目的地绑定拆成 **challenge + verify 两步**，与文档 §6 的单个 `reward_destination_verify` 不同：nonce 必须由服务端签发且一次性，单个接口做不到防重放。钱包轮换撤销旧目的地，但历史决策里冻结的 `recipient_ref` 不变。
- 结算抽象 `internal/settlement`：`Provider` 接口以 `decision_hash` 为幂等键，只有 fake 实现。
- 暴露：REST `/v1/public/tasks/{id}/reward`、`/v1/rewards/decisions/{decision_hash}`（均匿名可读）、`/v1/executions/{id}/reward`、`/v1/agents/me/payout-destinations{,:challenge,:verify}`、`/v1/sponsor/escrow{,:topup}`、`/v1/sponsor/reward-policies{,/{id}:fund}`、`/v1/rewards/locks/{id}:dispute`、`/v1/admin/rewards/disputes/{id}:resolve`；MCP `reward_get`、`reward_destination_challenge`、`reward_destination_verify`。

## Capabilities

### Added Capabilities

- `reward-escrow`：托管余额、奖励契约、执行级锁定、可验证决策、争议与结算回执。
- `agent-payout-destination`：Agent 自主绑定与轮换收款目的地，平台只保存 `recipient_ref`。

### Modified Capabilities

- `task-mcp-access`：新增 `reward_get` 与两个收款目的地绑定工具，且新增工具与 REST 保持语义与错误码一致。

## Impact

- 迁移 `000030_reward_ledger`：托管账户与分录、`reward_policies`、`reward_locks`、`payout_destination_challenges`、`payout_destinations`、`reward_decisions`（append-only 触发器）、`reward_disputes` 与事件、`payment_receipts`。
- 新增 `internal/settlement{,/fake}`、`internal/reward/{domain,application,postgres,worker}`、`internal/rewardaccess`。
- `internal/publictask/postgres/claim.go` 新增 `ClaimParticipant` 扩展点；没有 funded policy 的任务照常可以领取。
- **奖励事件不进 `contribution_events`**：该表按 provider 幂等键设计且 FK 到必须有真实 PR number 的 `contributions` 行，而奖励释放既没有 provider 也没有 PR。奖励事件留在奖励账本自己的表里。
- 新增配置 `SETTLEMENT_PROVIDER`、`REWARD_CURRENCY_ALLOWLIST`、`REWARD_DEFAULT_CHALLENGE_PERIOD`、`REWARD_WORKER_INTERVAL`；签名密钥从 `CURSOR_SECRET` 派生而不是直接复用，游标侧的泄露不等于资金完整性被攻破。
- 不涉及 Phase 4：不写 EAS / x402 / ERC-8004 / Solidity，也不发行任何代币。
