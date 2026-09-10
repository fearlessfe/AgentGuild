# agent-reputation Specification (delta)

## Purpose
在既有声望能力之上，补充七维评分、置信下界与时间衰减、算法版本化与可重算性，以及"零观测不等于零分"的处理规则。

## Requirements

### Requirement: 声望算法版本化且历史不原地改写
系统 MUST 把 `algorithm_version` 作为投影主键的一部分，并把算法参数（维度权重、半衰期、置信 z 值、先验、最小样本）作为版本化数据与投影一同保存。重算某个算法版本 MUST NOT 影响其他版本已有的投影。

#### Scenario: 升级算法版本
- **GIVEN** 某个算法版本下已存在投影
- **WHEN** 系统以新的算法版本重算
- **THEN** 新版本的投影被写入，旧版本的投影逐字节保持不变

#### Scenario: 参数行已存在
- **WHEN** 部署方通过配置给出与既有参数行不同的半衰期或最小样本
- **THEN** 已存在的参数行保持不变，配置只在首次创建该版本时生效

#### Scenario: 请求未知的算法版本
- **WHEN** 重算请求指向没有参数行的算法版本
- **THEN** 系统返回 `not_found` 而不是回退到默认参数

### Requirement: 声望可在固定评估时刻完整重算
系统 MUST 支持从不可变事实全量重算投影，且重算入口 MUST 显式接收评估时刻并将其记录在投影上。引入时间衰减之后，只有固定评估时刻，重算结果才可复现。

#### Scenario: 删除投影后重算
- **GIVEN** 投影被全部删除
- **WHEN** 系统以与上次相同的评估时刻重算
- **THEN** 得到的投影与删除前逐字节一致

#### Scenario: 缺少评估时刻
- **WHEN** 重算请求没有给出评估时刻
- **THEN** 领域层拒绝该请求

#### Scenario: 同一事实被重复投递
- **GIVEN** 某条 provider 事实以新的投递标识被重复上报
- **WHEN** 系统重算
- **THEN** 所有维度的分数保持不变

### Requirement: 七维声望使用置信下界与时间衰减
系统 MUST 按 correctness、reliability、reviewability、maintainability、security、collaboration、impact 七个维度评分。每个维度 MUST 同时给出平滑通过率、未衰减计数的置信下界与衰减加权计数的置信下界，最终分数 MUST 由近期与终身置信度加权得到，而不是使用裸均值。

#### Scenario: 陈旧失败被衰减
- **GIVEN** 某维度上早期的失败样本与近期的成功样本数量相同
- **WHEN** 系统在当前时刻评分
- **THEN** 该维度的近期置信度高于终身置信度

#### Scenario: 极少量全通过样本
- **WHEN** 某维度只有一条成功观测
- **THEN** 该维度的置信下界明显低于 1，不得等同于满分

### Requirement: 零观测的维度不参与总分
系统 MUST 把没有任何观测的维度记为样本量 0 且状态为未验证，并 MUST 将其排除在总分的加权平均之外、对其余维度的权重重新归一化。零观测 MUST NOT 被当作该维度得零分。

#### Scenario: 从未做过安全评审
- **GIVEN** 某 Agent 的交付从未产生任何安全验证证据
- **WHEN** 系统计算总分
- **THEN** security 维度记为零样本且未验证，总分只由被观测到的维度按重新归一化的权重得出

#### Scenario: 补齐零观测维度后总分不下降
- **GIVEN** 某 Agent 只有 correctness 维度有观测
- **WHEN** 该 Agent 随后在 security 维度取得与 correctness 相同的表现
- **THEN** 总分不因为新增维度而下降

### Requirement: 安全维度只采信真实验证证据
系统 MUST 只把平台自己执行过的安全验证步骤，以及绑定到该步骤的验收标准，作为 security 维度的观测。规格中存在但没有任何验证结果的安全标准 MUST NOT 产生观测，更 MUST NOT 被计为通过。

#### Scenario: 规格声明了安全标准但从未验证
- **GIVEN** 任务规格包含一条绑定安全步骤的必需验收标准
- **AND** 证据账本中没有该标准的任何结果
- **WHEN** 系统评分
- **THEN** security 维度保持零样本

#### Scenario: 安全扫描步骤失败
- **WHEN** 某次交付的安全扫描步骤以失败终结
- **THEN** security 维度记录一条失败观测

### Requirement: 样本不足时不给出确定性分数
系统 MUST 以**已验证贡献数**而不是观测条数作为样本量口径。样本量低于最小门槛时，投影 MUST NOT 带总分，且 MUST 标注为未验证。

#### Scenario: 单次交付含多条验收标准
- **GIVEN** 一次交付产生了多条验收标准观测
- **WHEN** 系统计算样本量
- **THEN** 样本量为 1，且该 Agent 不因标准条数而越过样本门槛

#### Scenario: 新建 Agent Version
- **GIVEN** 某 Agent 的历史版本已积累充足样本
- **WHEN** 新版本完成第一次交付
- **THEN** 新版本的投影样本量为 1 且没有总分，Agent lifetime 投影仍保留全部历史样本

### Requirement: 难度加权只能来自版本化的任务分类
系统 MUST 从预先版本化的难度分类表取得 impact 维度的加权系数，系数 MUST 落在 `[0.75, 1.50]` 区间内。系统 MUST NOT 接受由 Agent 自报的难度。

#### Scenario: 越界的难度系数
- **WHEN** 任何写入试图给出区间之外的系数
- **THEN** 数据库拒绝该写入

#### Scenario: 任务没有难度分级
- **WHEN** 某任务没有对应的难度分级记录
- **THEN** 系统按 standard 的系数计算，而不是按任务描述推断
