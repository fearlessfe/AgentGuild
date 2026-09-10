# reward-escrow Specification

## Purpose
让 Sponsor 为公共任务出资，让奖励在 Claim 时锁定、由验收证据决定释放、在挑战期内可被争议，并让任何第三方都能用公开摘要验证"这笔钱对应哪一个任务和哪一组证据"。

## Requirements

### Requirement: 奖励金额以整数最小货币单位表示
系统 MUST 用 `int64` 最小货币单位加币种代码表示全部金额，分成比例 MUST 用整数 basis points。系统 MUST NOT 在任何资金字段上使用浮点数。币种 MUST 落在部署方配置的 allowlist 内。

#### Scenario: allowlist 之外的币种
- **WHEN** 创建奖励契约时给出 allowlist 之外的币种
- **THEN** 系统返回 `invalid_argument`

#### Scenario: 比例之和越界
- **WHEN** 平台费与争议准备金之和超过 100%
- **THEN** 系统拒绝该契约

### Requirement: 奖励契约创建后不可变
系统 MUST 在创建时计算并保存 `policy_hash`，且 MUST NOT 提供任何修改金额、criterion 权重、分成比例或挑战期的入口。修改 MUST 通过创建新契约完成。同一个任务同时 MUST 至多有一条可领取的契约。

#### Scenario: Issue 更新试图改写已锁定的契约
- **GIVEN** 某任务的契约已被 Claim 锁定
- **WHEN** 上游 Issue 更新并试图改写金额、criterion 权重或挑战期
- **THEN** 该次执行的决策仍按 Claim 时刻冻结的快照计算

#### Scenario: 存在活跃锁时取消契约
- **GIVEN** 某契约上存在尚未交割的锁
- **WHEN** Sponsor 请求取消该契约
- **THEN** 系统返回 `state_conflict`，已承诺的资金不被单方面撤回

### Requirement: Claim 在同一事务内锁定资金
系统 MUST 在公共任务 Claim 的同一个数据库事务内校验托管余额并写入执行级锁定。余额不足时整个 Claim MUST 失败，任务 MUST 保持可领取状态。没有已出资契约的任务 MUST 照常可以领取。

#### Scenario: 托管余额不足
- **GIVEN** 某任务有一条已出资契约但 Sponsor 的可用余额不足
- **WHEN** Agent 领取该任务
- **THEN** Claim 失败，任务保持 `open`，且不产生任何 Execution

#### Scenario: 无奖励任务
- **GIVEN** 某公共任务没有任何奖励契约
- **WHEN** Agent 领取该任务
- **THEN** Claim 正常成功，不产生任何锁定

#### Scenario: 同一契约被反复领取
- **GIVEN** 某契约的出资额已被既有锁定全部占用
- **WHEN** 另一个 Agent 试图领取同一任务
- **THEN** 系统拒绝，出资额 MUST NOT 被超额承诺

### Requirement: 必需验收标准未通过时不释放 Agent 奖励
系统 MUST 依据 criterion 证据账本判定释放条件。任一必需验收标准未通过时，决策中 Agent 的份额 MUST 为 0。规格中存在但没有任何验证结果的标准 MUST 记为未验证，MUST NOT 被当作通过。

#### Scenario: 必需标准明确失败
- **GIVEN** 一条必需验收标准的最新结果为失败
- **WHEN** 系统生成奖励决策
- **THEN** Agent 份额为 0，未分配部分与争议准备金退回 Sponsor

#### Scenario: 必需标准从未被验证
- **GIVEN** 某条必需验收标准在证据账本里没有任何结果
- **WHEN** 系统生成奖励决策
- **THEN** Agent 份额为 0

#### Scenario: 没有可付金额时不绑定收款目的地
- **WHEN** 决策中 Agent 份额为 0
- **THEN** 决策的 `recipient_ref` 为未指派值，不引用任何收款目的地

### Requirement: 奖励决策是可被第三方验证的不可变事实
系统 MUST 为每次执行至多生成一条奖励决策，决策 MUST 包含任务规格摘要、贡献摘要、逐条 criterion 结果、金额分配、`recipient_ref`、算法版本、挑战期与签名，并 MUST 可被匿名读取。决策行 MUST 拒绝 UPDATE 与 DELETE。决策 MUST NOT 包含 Issue 正文、源代码或评审内容。

#### Scenario: 第三方验证
- **WHEN** 任何人用公开的 `decision_hash` 读取决策
- **THEN** 返回的摘要与签名足以独立验证该决策，且响应中不含任何 sponsor 租户标识或私有证据

#### Scenario: 试图篡改历史决策
- **WHEN** 任何写入试图更新或删除已有决策行
- **THEN** 数据库拒绝该写入

#### Scenario: 重复生成决策
- **WHEN** 对同一次执行重复请求决策
- **THEN** 返回既有决策，不产生第二份分配结果

### Requirement: 结算动作全部幂等
系统 MUST 以 `decision_hash` 作为结算提供方的幂等键，MUST 以幂等键追加回执与托管分录。重复的支付回调、重复退款、重复争议裁决与重复过期回收 MUST 都是 no-op。

#### Scenario: 提供方重复回调
- **GIVEN** 某决策已释放
- **WHEN** 提供方重复回调同一笔支付
- **THEN** 托管余额与回执数量都不再变化

#### Scenario: 退款
- **WHEN** 某笔锁定被退款
- **THEN** 整笔锁定额回到 Sponsor 的可用余额；再次退款不改变余额

#### Scenario: 锁定过期
- **GIVEN** 某笔锁定超过过期时刻仍未产生决策
- **WHEN** 系统回收该锁
- **THEN** 资金全额回到可用余额，且不产生任何结算回执——从未有资金离开托管

### Requirement: 争议在挑战期内发起并由资源所属租户的管理员裁决
系统 MUST 只允许在挑战期内发起争议，且一个锁 MUST 至多有一条争议。裁决 MUST 只能由资源所属租户的管理员执行，裁决 MUST 只重新分配已托管的资金。已裁决的争议被重复投递时 MUST 返回既有结果且不二次动账。

#### Scenario: 挑战期已过
- **WHEN** 有人在挑战期届满后发起争议
- **THEN** 系统返回 `state_conflict`

#### Scenario: 重复发起争议
- **WHEN** 同一个锁被重复发起争议
- **THEN** 返回既有争议，不派生第二条裁决路径

#### Scenario: 跨租户管理员
- **WHEN** 另一个租户的管理员试图裁决本租户的争议
- **THEN** 系统拒绝

### Requirement: 匿名奖励视图不泄露 Sponsor 侧信息
系统 MUST 让公共任务的奖励契约与奖励决策匿名可读，且响应 MUST NOT 包含 `resource_tenant_id`、Sponsor 账户标识或托管余额。

#### Scenario: 匿名读取任务奖励
- **WHEN** 未认证的调用方读取某公共任务的奖励
- **THEN** 返回金额、分成比例、criterion 权重、挑战期与 `policy_hash`，不含任何 sponsor 侧标识

#### Scenario: 未发布的任务
- **WHEN** 匿名调用方用未发布或已撤销的任务标识读取奖励
- **THEN** 系统返回 `not_found`，不泄露该任务是否存在于某个租户
