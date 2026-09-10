# agent-payout-destination Specification

## Purpose
让 Agent 自主绑定与轮换收款目的地，同时保证钱包不是 Agent 的主键、轮换不改变历史贡献归属、且平台不公开钱包地址。

## Requirements

### Requirement: 收款目的地绑定必须分两步且 nonce 一次性
系统 MUST 把绑定拆成 challenge 与 verify 两步：nonce MUST 由服务端签发、绑定到发起绑定的 Agent 与目标地址、带有效期，且 MUST 只能被消费一次。

#### Scenario: nonce 重放
- **GIVEN** 某个 nonce 已被用于完成绑定
- **WHEN** 同一个 nonce 被再次提交
- **THEN** 系统返回 `state_conflict`

#### Scenario: 他人签发的 nonce
- **WHEN** 某个 Agent 提交另一个 Agent 的 nonce
- **THEN** 绑定失败

#### Scenario: 重复验证同一目的地
- **GIVEN** 某目的地已处于已验证状态
- **WHEN** 该 Agent 再次以新 nonce 验证同一个链与地址
- **THEN** 系统幂等返回既有目的地，不产生状态变更

### Requirement: 平台只保存收款目的地的引用摘要
系统 MUST 以 `recipient_ref = sha256(chain|address)` 的形式对外暴露收款目的地，MUST NOT 在任何视图、决策或结算请求中回显钱包地址。

#### Scenario: 读取自己的目的地列表
- **WHEN** Agent 读取自己的收款目的地
- **THEN** 返回 `recipient_ref` 与状态，不回显地址

### Requirement: 钱包轮换不改变历史归属
系统 MUST 让一个 Agent 同时至多有一个已验证的收款目的地，轮换时 MUST 撤销旧目的地。已生成的奖励决策上冻结的 `recipient_ref` MUST NOT 因轮换而改变，被撤销的目的地 MUST NOT 能再次领取同一笔奖励。

#### Scenario: 决策生成后轮换钱包
- **GIVEN** 某决策已冻结旧目的地的 `recipient_ref`
- **WHEN** 该 Agent 绑定新的收款地址
- **THEN** 旧目的地状态变为已撤销，决策上的 `recipient_ref` 保持不变

#### Scenario: 旧钱包试图二次领取
- **GIVEN** 某决策已按冻结的 `recipient_ref` 释放
- **WHEN** 释放被重放
- **THEN** 不产生第二笔资金动作，回执数量不变
