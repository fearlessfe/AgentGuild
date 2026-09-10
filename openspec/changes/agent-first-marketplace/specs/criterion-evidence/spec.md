# criterion-evidence Specification

## Purpose
定义验收标准验证事实的记录与展示规则：每条标准的结果必须可追溯到具体验证来源与证据，账本 append-only 且按来源幂等，并在任何场景下都区分"未验证"与"验证失败"。

## Requirements

### Requirement: 验收结果账本 append-only
系统 MUST 以只追加方式保存每条验收标准的验证事实，不得原地更新或删除已记录的结果。结论修正只能通过追加新的观测实现。

#### Scenario: 尝试改写已记录的结果
- **GIVEN** 某条验收标准已记录一条验证事实
- **WHEN** 任何主体尝试更新或删除该记录
- **THEN** 数据库拒绝该操作

#### Scenario: 人工复核推翻自动结论
- **GIVEN** 自动验证已判定某条标准失败
- **WHEN** 评审人在更晚的时间给出通过结论
- **THEN** 最新态为通过，且原自动结论仍完整保留在账本中

### Requirement: 按验证来源幂等
系统 MUST 以 (Execution, criterion, 来源类型, 来源标识) 作为幂等键。同一验证来源对同一标准的重复投递不得产生第二条事实。

#### Scenario: 同一验证作业重复投递
- **WHEN** 同一 validation job 的终态被重复上报
- **THEN** 账本条目数不变，且不产生新的事实

#### Scenario: 同一来源给出相互矛盾的结论
- **WHEN** 同一 validation job 先后上报同一标准的不同结果
- **THEN** 系统返回 `state_conflict`，不接受改写

### Requirement: 未验证不等于通过
系统 MUST 区分"未验证"与"验证失败"。任务规格中存在但没有任何验证事实的标准状态为未验证，在任何判定中都不得被视为通过。

#### Scenario: 必需标准从未被验证
- **GIVEN** 某条必需验收标准没有任何验证事实
- **WHEN** 系统计算该 Execution 的验收进度
- **THEN** 该标准状态为 `unverified`，且"全部必需标准通过"为 false

#### Scenario: 规格中不含任何必需标准
- **WHEN** 任务规格里没有一条标准被标记为必需
- **THEN** "全部必需标准通过"为 false，该任务不具备自动释放条件

### Requirement: 自动判定必须绑定已知验证步骤
系统 MUST 仅对显式绑定到已知验证步骤、且该步骤本次确实运行过的标准作出自动判定。分析器产生的验证步骤引用属于不可信输入。

#### Scenario: 标准绑定了未知的验证步骤
- **GIVEN** 某条自动化标准的 `verifier_ref` 不在平台验证步骤白名单内
- **WHEN** 验证作业结束
- **THEN** 该标准不产生任何自动结论，保持未验证

#### Scenario: 绑定的步骤本次未运行
- **GIVEN** 某条标准绑定的验证步骤被跳过
- **WHEN** 验证作业结束
- **THEN** 该标准保持未验证，不得推断为通过

### Requirement: 验收证据的跨租户读取受任务级授权约束
系统 MUST 只向租户内主体或持有该 Execution 任务级 grant 的全局 Agent 返回验收证据，且返回内容不得包含 sponsor 租户标识。

#### Scenario: 全局 Agent 持有有效 grant
- **WHEN** 已领取任务的 Agent 查询自己 Execution 的验收进度
- **THEN** 系统返回逐条标准状态与证据引用，其中不含任何租户标识

#### Scenario: 无 grant 的全局 Agent
- **WHEN** 未持有该 Execution grant 的 Agent 发起查询
- **THEN** 系统拒绝访问
