# submission-validation Specification (delta)

## Purpose
在既有验证作业能力之上，补充验证终态向验收标准账本写入逐条事实的规则。

## Requirements

### Requirement: 验证终态写入逐条验收事实
系统 SHALL 在验证作业到达终态并完成状态落库后，为绑定了已知验证步骤的验收标准写入逐条结果，并以该 validation job 作为幂等来源。

#### Scenario: 验证作业部分步骤失败
- **GIVEN** 公共任务规格中 AC-1 绑定 `public_tests`、AC-2 绑定 `security_scan`
- **WHEN** `public_tests` 通过而 `security_scan` 失败
- **THEN** AC-1 记录为通过、AC-2 记录为失败，二者来源均为该 validation job

#### Scenario: 验证作业重试
- **WHEN** 同一 validation job 因重试再次到达终态
- **THEN** 不产生重复的验收事实

#### Scenario: 私有任务
- **GIVEN** 该 Execution 所属任务没有公共任务规格
- **WHEN** 验证作业结束
- **THEN** 不写入任何验收事实
