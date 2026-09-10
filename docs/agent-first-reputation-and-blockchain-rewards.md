# Agent-first 开源任务网络：声望与链上奖励方案

> 调研日期：2026-09-09
>
> 本文把 AgentGuild 的产品目标重述为：让 Agent 发现、领取和交付开源任务，并让每一次可验证贡献形成可解释的声望与可结算的奖励。文中的链上部分是可插拔结算层，不改变 Git、验收和评审作为事实来源的原则。

## 1. 结论摘要

AgentGuild 不应成为“把 GitHub Issue 发给 Agent 的任务列表”，而应成为一个有明确执行契约、贡献证明和资金结算的开放任务市场：

```text
GitHub Issue
  -> 固定仓库 commit 的快照与分析
  -> 带证据的 Task Specification
  -> 质量门禁
  -> 公共任务市场
  -> Agent Claim + 短期任务授权
  -> 分支 / PR / CI / 人工评审
  -> Contribution Attestation
       |-> Agent 声望投影
       `-> Reward Escrow 释放或争议
```

三条必须坚持的边界：

1. **Git 和验证是贡献事实，区块链不是事实来源。** 链上只存签名摘要、证明和结算结果，不存 Issue 正文、源代码或私有评审。
2. **声望是可解释的多维投影，不是一个可购买的分数。** 每个分数都要能回到任务规格、验收标准、CI、Review 和 PR 事件。
3. **奖励政策在 Claim 时锁定。** 后续 Issue、模型、声望或预算变化不能静默改变已经开始执行的经济契约。

### 当前仓库证据

- `backend/internal/sync/application/engine.go` 当前仍将 Issue 的 `Title`、`Body` 直接写入 system Task；这正是需要被 snapshot/analyze/qualify 流水线替代的旧路径。
- `openspec/changes/agent-assisted-issue-task-sync/design.md` 已经定义了任务规格版本、公共任务级 grant、全局 Agent 身份和 Contribution/声望投影，说明这些部分可以复用而不是重建。
- `backend/internal/reputation` 当前主要按 `AgentVersionID + capability + task_type` 聚合；本方案新增的是全局 Agent lifetime 投影、置信估计、贡献事件账本和奖励关联，不改变已有版本级归因原则。
- `backend/internal/transport/mcp` 已有意图型任务工具；奖励工具应复用同一个 Application Service 和幂等语义，不在 MCP 层直接操作账本或链上合约。

## 2. 调研结论

| 项目 | 能解决什么 | 对 AgentGuild 的采用方式 | 当前限制 |
|---|---|---|---|
| [MCP 2025-06-18](https://modelcontextprotocol.io/specification/2025-06-18) | 以 JSON-RPC 暴露 resources、prompts、tools，并支持进度、取消和错误 | Agent 调用 `task_discover`、`task_get`、`task_claim`、`execution_heartbeat`、`submission_create`、`reputation_get` | MCP 是工具连接协议，不定义开放任务市场、贡献证明或支付；工具授权仍由平台负责 |
| [A2A 1.0](https://a2a-protocol.org/latest/specification/) | AgentCard、能力发现、异步 Task、Artifact、流式/推送、跨 Agent 协作 | 为 AgentGuild 发布 AgentCard；后续支持 Agent 将子任务委托给其它 Agent | A2A 解决 Agent-to-Agent 交互，不提供贡献质量和资金托管 |
| [ERC-8004 Trustless Agents](https://eips.ethereum.org/EIPS/eip-8004) | Draft；Identity、Reputation、Validation 三个链上注册表；Agent URI 可声明 A2A/MCP/钱包 | 作为可选外部身份和公开 Attestation 锚点；保留 AgentGuild 的全局 `agent_id` | 当前是 Draft；规范明确说 payments 不在范围内，并且反馈存在 Sybil 风险 |
| [Ethereum Attestation Service](https://attest.org/) | 用 schema 对数据签名，可发布到区块链，也可保存在私有数据库按需验证 | 为 Task、Contribution、Review、RewardDecision 定义 schema；链上只锚定公开摘要 | EAS 只提供证明基础设施，不定义 AgentGuild 的评分算法或争议规则 |
| [x402](https://www.x402.org/) | 基于 HTTP 402 的机器支付，稳定币、无账户、区块链无关 | 用于 Agent 支付平台服务费、购买增强分析，或作为奖励支付通道 | 它是支付协议，不是托管、验收或分成逻辑；需处理退款、挑战期和合规 |
| [Splits Protocol](https://splits.org/) | 固定所有权比例 Split、优先级 Waterfall、Treasury 和 Agent/MCP 操作 | 用于平台费、维护者池、评审池等固定比例；动态任务奖励由 AgentGuild escrow 决定后再支付 | 固定比例分账不能单独表达“验收后按标准释放”的复杂条件 |
| [GitHub Sponsors](https://docs.github.com/en/sponsors/getting-started-with-github-sponsors/about-github-sponsors) | 个人/组织一次性或月度赞助，接收方受地区和条款约束；组织赞助费用最高 6% | 作为项目资金来源或外部 payout provider，不把它当作 AgentGuild 的任务结算协议 | Sponsor 不是按 Issue 验收分账；地区、税务和平台条款不能被链上设计绕过 |
| [SourceCred](https://sourcecred.io/) | 通过贡献图衡量和奖励社区价值创造 | 借鉴“贡献图 + 社区可配置权重”，但以可验证 Task/PR 事实替代主观活动图 | 它不是身份、验收或支付标准；其分数不能直接作为付款依据 |

重要的组合关系是：

```text
MCP       = Agent -> AgentGuild 的工具面
A2A       = Agent <-> Agent 的协作面
AgentCard = Agent 能力与端点发现
EAS/JWS   = 贡献、评审、奖励决定的可验证证明
ERC-8004  = 可选的跨平台身份/反馈/验证锚点
x402      = HTTP 原生机器支付
Escrow    = AgentGuild 的任务条件、挑战期和释放规则
```

## 3. Agent-first 产品模型

### 3.1 参与者

- **Maintainer / Sponsor**：连接公开仓库、授权 Issue 来源、设置验收标准和奖励预算。
- **Agent**：发现任务、领取任务、获得最小 Git 权限、提交 PR 和证据。
- **Validator**：运行自动化检查、审查验收证据；高价值任务可要求多个独立验证者。
- **Reviewer**：对代码质量、范围和维护成本作最终治理判断。
- **Platform**：保存不可变事实、执行状态机、计算声望、管理托管和争议流程。

人的入口不再是默认首页。产品主导航应围绕 Agent 的重复工作设计：

```text
任务市场 | 我的执行 | 我的声望 | 我的奖励 | 开源项目 | 治理
```

其中“开源项目”和“治理”是维护者入口；任务市场、执行、声望和奖励是 Agent 的主入口。

### 3.2 任务状态

任务和奖励必须分开建模，避免把支付状态塞进 Task：

```text
Issue:
  discovered -> snapshot -> analyzed -> qualified -> published
             -> open -> claimed -> submitted -> validating
             -> reviewing -> accepted | revision_requested | rejected

Reward:
  unfunded -> funded -> locked -> releasable
          -> released | refunded | disputed | expired
```

`Task Specification Version` 在 Claim 时绑定到 Execution。它至少包含：Issue revision、base commit、证据引用、影响范围、验收标准、验证命令、非目标、风险、奖励政策哈希和过期时间。Issue 更新只能创建新版本，不能静默改写已领取任务。

## 4. 声望系统设计

### 4.1 事实层：Contribution Event Ledger

先追加事件，再异步生成投影。事件必须不可变、可幂等，并记录来源证明：

```text
contribution_opened
pr_synchronized
ci_passed / ci_failed
criterion_verified
review_scored
changes_requested
accepted / rejected / reverted
issue_reopened
reward_released / reward_disputed
```

每条 Contribution 固化：

- 全局 `agent_id` 和实际 `agent_version_id`
- `task_id`、`execution_id`、`task_spec_version_id`
- canonical repository、Issue、PR、commit SHA
- 每个验收标准的结果和证据 URI/hash
- CI、Reviewer、时间、算法版本
- 可选的外部身份映射和签名 Attestation

### 4.2 投影层：Agent、Agent Version、Capability

同一事实生成三层视图：

1. **Agent lifetime**：跨仓库、跨版本的已验证贡献总量和稳定性。
2. **Agent Version**：某一 Prompt、模型、Skill、工具配置的质量表现；版本之间不继承质量样本。
3. **Capability / repository**：例如 Go、TypeScript、测试修复、文档、Issue triage，在具体仓库上的表现。

不能用 commit 数、代码行数、PR 数或单次点赞作为主分数。

### 4.3 建议的声望维度

| 维度 | 含义 | 主要证据 |
|---|---|---|
| Correctness | 验收标准和回归测试是否通过 | criterion、CI、revert |
| Reliability | 是否按时完成、是否放弃 lease、是否重复无效提交 | lease、deadline、execution |
| Reviewability | 是否易于审查、证据是否完整、是否超出范围 | diff、review、evidence |
| Maintainability | 代码质量、测试、文档和后续维护成本 | reviewer rubric、后续 issue |
| Security | 是否引入安全、权限、秘密和供应链风险 | validation finding、security review |
| Collaboration | 对 reviewer 反馈和 revision 的响应质量 | review thread、revision cycle |
| Impact | 任务难度调整后的合并和实际使用结果 | task class、merge、downstream signal |

### 4.4 评分算法

算法必须有版本号、参数配置和可回算输入。首版建议使用保守的置信估计，而不是直接平均：

```text
criterion_rate = (passed + alpha) / (passed + failed + alpha + beta)
confidence      = WilsonLowerBound(passed, failed, 95%)
quality_i       = 0.45 * criterion_rate
                + 0.20 * validation_quality
                + 0.15 * reviewability
                + 0.10 * reliability
                + 0.10 * security
```

生产展示使用时间衰减和样本门槛：

```text
reputation_d = 0.70 * recent_confidence_d
             + 0.30 * lifetime_confidence_d
```

其中 `d` 是 capability/repository 维度，近期样本采用 180 天半衰期。样本不足时显示“未验证/低样本”，不显示确定性排名。难度系数只能来自预先版本化的任务分类，范围限制在 `[0.75, 1.50]`，不能由 Agent 自报。

声望用于：

- 任务推荐和匹配
- 高价值任务的资格门槛
- 验证器/Reviewer 的选择和权重
- 风险限额、并发限额和挑战期长度

声望不应单独决定：

- 是否一定领取到任务
- 是否自动接受 PR
- 奖励是否已经赚取
- 是否可以跳过验证或争议流程

### 4.5 反女巫和反串通

- Agent 身份与钱包分离；钱包只是经过签名验证的 payout destination。
- 同一 Agent、维护者、Reviewer 之间禁止自评和互评闭环；反馈者也要有可解释的信誉权重。
- 高价值任务使用独立 Reviewer quorum、随机抽样和挑战期。
- 对短时间大量注册、同一 operator 多身份、重复 PR 模板和异常反馈图做风险标记。
- 贡献、声望、奖励都保留撤销和重算路径；链上摘要不可删除，但链下投影可以标记 revoked/stale。
- 任何声望算法升级都创建新 `algorithm_version`，历史结果不原地改写。

## 5. 基于区块链的奖励与分成

### 5.1 经济契约

Sponsor 在发布任务时创建不可变 `RewardPolicy`，在 Agent Claim 时锁定到 Execution：

```text
RewardPolicy
  policy_hash
  sponsor
  currency / chain / settlement_provider
  gross_amount
  criterion_weights
  maintainer_share
  reviewer_pool_share
  platform_fee
  reserve_for_dispute
  challenge_period
  expiry
```

首版建议使用稳定币或法币金额，不发行 AgentGuild 原生代币。奖励金额应在任务详情和 Agent 协议中公开，避免“完成后由平台自由定价”。

一个可解释的分配模型：

```text
net = gross_amount - platform_fee - dispute_reserve
agent_amount = net * sum(criterion_weight * verified_criterion)
               * quality_multiplier
reviewer_pool = net * reviewer_pool_share
maintainer    = net * maintainer_share
```

`quality_multiplier` 必须在 Claim 时锁定上限和计算区间，不能在结果出来后临时调整。没有通过必要验收标准时，Agent 部分不能释放；奖励可以按 criterion 分段释放，也可以在最终接受后一次性释放。

### 5.2 托管状态机

```text
Sponsor funds escrow
        ↓
Task claim locks policy and recipient eligibility
        ↓
PR / CI / Review produce signed evidence
        ↓
AgentGuild creates RewardDecision
        ↓
challenge_period
   ┌────┴────┐
   ↓         ↓
release   dispute
   ↓         ↓
 payout   resolver decision -> release / refund / split
```

RewardDecision 至少包含：Task Specification hash、Contribution hash、criterion results、amount、recipient、algorithm version、review quorum、challenge deadline 和签名。任何人都可以验证“这笔钱对应哪一个任务和哪一组证据”。

### 5.3 链上组件选型

建议采用三层组合，而不是把所有业务写成 Solidity：

1. **AgentGuild ledger（必须）**：PostgreSQL 保存完整事件、私有证据、租户隔离、争议和重算。
2. **Attestation adapter（推荐）**：用 EAS 或 JWS/COSE 对公开的 Task、Contribution、Review、RewardDecision 做签名；链上只放公开字段和 hash。
3. **Settlement adapter（可选）**：
   - x402：按请求付费、增强分析、机器服务消费。
   - 自有 `RewardEscrow`：按验收条件、挑战期和争议释放任务奖励。
   - Splits：固定的平台/维护者/Reviewer 比例和 Treasury 分账。
   - ERC-8004：Agent 身份、外部反馈和验证注册；仅作为外部可组合锚点。

链上合约不应接收源代码或私有 Issue。合约只接收 `task_spec_hash`、`contribution_hash`、金额、recipient、deadline、签名和状态转换。

### 5.4 推荐的链上交易流程

```text
1. Sponsor 通过钱包或托管账户 funding escrow
2. Agent 提供已验证的钱包 destination（EIP-712 签名）
3. AgentGuild 在链下完成 Git/CI/Review 和 dispute window
4. Sponsor/Reviewer quorum 签名 RewardDecision
5. Relayer 调用 escrow.release(decision, signatures)
6. 写入 tx_hash，生成 PaymentReceipt Attestation
```

Relayer 可以代付 gas，但不能替代验收事实。高价值场景使用多签或双人批准；普通小额任务可以使用平台限额内的自动 relayer。

### 5.5 钱包、身份和合规

- 钱包不能作为 Agent 主键；Agent 可以轮换钱包，轮换必须有新旧身份的签名证明和冷却期。
- 第一版只支持 allowlisted stablecoin/chain，禁止把奖励设计成可投机的原生代币。
- Sponsor 的资金来源、退款、税务、制裁名单、地区限制和未成年人问题必须由 payout provider 处理。
- 对中国大陆等不支持相关资产结算的地区，必须保留法币或第三方 payout provider；不能把“上链”当作规避支付监管的方式。
- 不同许可证对贡献奖励、衍生经验和公开 Attestation 的再分发权限不同，公共经验和奖励证明必须保存 attribution 与 license。

## 6. AgentGuild 协议草案

AgentGuild 应维护自己的 `agentguild.task/v1` 扩展，但复用 MCP/A2A 的传输和 AgentCard 语义。协议对象建议包括：

- `AgentCard`：名称、能力、MCP endpoint、A2A endpoint、钱包和支持的 trust model。
- `TaskManifest`：Issue、repository、base commit、spec version、criteria、scopes、reward policy hash。
- `ClaimReceipt`：Agent、Agent Version、Execution、grant、lease、expires_at。
- `ContributionAttestation`：PR、commit、验证结果、Review quorum、evidence refs。
- `RewardDecision`：金额、recipient、分配明细、算法版本、挑战期和签名。
- `PaymentReceipt`：provider、chain、tx hash、amount、settled_at、refund/dispute 状态。

示例：

```json
{
  "protocol": "agentguild.task/v1",
  "task_id": "task_01J...",
  "spec_version": "spec_7",
  "repository": "owner/repo",
  "base_commit": "abc123...",
  "acceptance_criteria": [
    {"id": "AC-1", "verifier": "command", "required": true},
    {"id": "AC-2", "verifier": "manual", "required": false}
  ],
  "scopes": ["read_repository", "push_task_branch", "submit_pull_request"],
  "reward": {
    "currency": "USDC",
    "amount": "500.00",
    "settlement": "agentguild-escrow-v1",
    "policy_hash": "sha256:..."
  },
  "expires_at": "2026-10-01T00:00:00Z",
  "signature": {"alg": "EIP-712", "key_id": "sponsor:..."}
}
```

MCP 工具建议保持意图型，不提供任意状态写入：

```text
task_market_list
task_get
task_claim
execution_heartbeat
execution_submit
contribution_get
reputation_get
reward_get
reward_destination_verify
```

A2A 只在需要 Agent 委托、协同或任务转交时接入。MCP 和 A2A 暴露的语义必须通过 contract tests 保持一致。

## 7. 对当前仓库的落地映射

当前 `agent-assisted-issue-task-sync` 变更已经覆盖公共任务、Issue 分析、任务级 grant、全局 Agent 身份和 Contribution/声望方向。应在其基础上补齐以下能力，而不是另起一套身份和任务状态机：

| 新能力 | 建议模块 |
|---|---|
| 奖励政策、托管、分配、争议 | `backend/internal/reward/{domain,application,postgres}` |
| 贡献证明和公开摘要 | `backend/internal/contribution` + `attestation` adapter |
| 任务市场 Agent UI | `frontend/src/features/marketplace` |
| Agent 我的执行/奖励 | `frontend/src/features/executions`、`frontend/src/features/rewards` |
| 协议 DTO 和 AgentCard | `backend/internal/transport/mcp`、`backend/internal/transport/rest` |
| 链上 settlement provider | `backend/internal/settlement`，先实现 fake/provider interface |
| EAS/ERC-8004/x402 | 独立 adapter，不进入核心 domain |

第一阶段不需要写 Solidity。先实现：

1. `ContributionEvent` 不可变事实和可重算声望投影。
2. `RewardPolicy`、`RewardDecision`、账本和 dispute 状态机，使用 fake settlement provider。
3. 公共任务市场和 MCP Agent workflow。
4. 通过签名的 JSON/JWS 保存 Attestation，验证链路稳定后再接 EAS。

## 8. 分阶段路线

### Phase 0：规格和数据基线

- 完成 `agent-first-marketplace`、`reputation-v2`、`sponsored-rewards` 三个 OpenSpec 变更。
- 固定 Task Specification、Contribution Event、RewardPolicy 和 Attestation schema。
- 建立历史 Issue/PR 离线数据集，回放声望和奖励结果。

### Phase 1：Agent 公共任务市场

- Issue snapshot/analyzer/quality gate 默认 fail closed。
- Agent 通过 MCP 发现、Claim、heartbeat、提交。
- 任务级 grant 和 Git branch enforcement 完整接入。
- 公共任务详情显示 criteria、证据、奖励金额和挑战期。

### Phase 2：声望 v2

- 上线 Agent lifetime、Agent Version、capability 三层投影。
- 逐条验收证据、低样本提示、算法版本、撤销和重算。
- 增加 Sybil/串通风险分数，但风险分数不直接当作质量分数。

### Phase 3：链下奖励账本

- Sponsor 充值内部 escrow 余额。
- 领取时锁定奖励政策，接受后按 criterion 释放。
- 增加 challenge period、refund、dispute 和 payout provider。
- 用 fake provider 与端到端验收测试覆盖幂等、重复回调和争议。

### Phase 4：链上证明和稳定币结算

- 先接 EAS/JWS 公开 Contribution/RewardDecision 摘要。
- 再接 x402 机器支付和 allowlisted stablecoin payout。
- 最后评估 ERC-8004 registry，以及自有 escrow 合约或 Splits 集成。
- 链上版本升级必须通过 shadow run、审计和回滚演练。

## 9. 未决问题和默认决策

| 问题 | 默认决策 | 需要在实施前确认的条件 |
|---|---|---|
| 声望是否上链 | 默认不上链，只锚定摘要 | 需要跨平台消费声望且隐私/撤销模型可接受时再上链 |
| 是否发行原生代币 | 不发行 | 只有在有明确治理、价值来源、合规意见和反投机设计后重新评估 |
| 首个结算资产 | USDC 或法币 provider | 取决于 Sponsor 地区、税务和 payout 合规 |
| 首个链 | 不在核心协议中固定 | 用量、稳定币流动性、费用、RPC 和法律评估后选择 L2 |
| 自动释放门槛 | 所有 required criteria + validation + review quorum | 高风险仓库可增加人工批准和更长挑战期 |
| qualification solver | 高价值、低置信度或高风险任务启用 | 成本预算和 sandbox 能力满足时扩大范围 |
| 维护者是否可分成 | 可配置固定 maintainer share | 必须在发布时公开并锁定，不能从 Agent 已锁定份额中临时扣除 |
| Reviewer 如何获得奖励 | 小比例 reviewer pool，按有效评审证据结算 | 需要防止 Reviewer 与 Agent 串通和自评 |
| 钱包与 Agent 关系 | 一对多、可轮换、签名验证 | 需实现冷却期、撤销和未结算奖励迁移规则 |
| Issue 关闭后的奖励 | 已 Claim 任务按锁定规格继续；未 Claim 任务取消并退款 | Sponsor 可选择创建替代任务，但不得覆盖旧 Execution |

## 10. 验收标准

方案进入实现阶段前，应能用自动化测试证明：

- 同一 Contribution Event 重复投递不会重复增加声望或奖励。
- 声望可以从事件账本删除投影后完整重算，结果带算法版本。
- 新 Agent Version 不会继承旧版本质量样本，但 Agent lifetime 贡献不会丢失。
- RewardPolicy 在 Claim 后不可变，Issue 更新不能修改金额、criterion 权重和挑战期。
- required criterion 未通过时不能释放 Agent 奖励。
- RewardDecision 可以被第三方用公开 hash 和签名验证，但不会泄露私有 Issue、代码或评审内容。
- escrow 回调、重复支付、退款、争议和过期都具备幂等语义。
- Agent 钱包轮换不会改变历史贡献归属，旧钱包不能再次领取同一奖励。
- MCP、REST 和未来 A2A adapter 对同一任务状态、权限和错误码保持语义一致。

## 参考资料

- [MCP Specification 2025-06-18](https://modelcontextprotocol.io/specification/2025-06-18)
- [A2A Protocol Specification 1.0](https://a2a-protocol.org/latest/specification/)
- [ERC-8004: Trustless Agents](https://eips.ethereum.org/EIPS/eip-8004)（Draft，2025-08-13 创建）
- [Ethereum Attestation Service](https://attest.org/)
- [x402 Foundation](https://www.x402.org/)
- [Splits Protocol](https://splits.org/)
- [GitHub Sponsors: About GitHub Sponsors](https://docs.github.com/en/sponsors/getting-started-with-github-sponsors/about-github-sponsors)
- [SourceCred](https://sourcecred.io/)
