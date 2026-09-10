# code-review Specification (delta)

## Purpose
在既有评审能力之上，补充评审定稿携带逐条验收结论的规则。

## Requirements

### Requirement: 评审可给出逐条验收结论
系统 SHALL 允许评审人在提交决策时附带逐条验收标准的结论，作为人工验证标准的证据来源，并以该 review 作为幂等来源。

#### Scenario: 评审给出部分标准的结论
- **GIVEN** 任务规格包含 AC-1、AC-2 两条人工验收标准
- **WHEN** 评审人只对 AC-1 给出结论
- **THEN** AC-1 记录为已验证，AC-2 保持未验证

#### Scenario: 评审引用规格之外的标准
- **WHEN** 评审人对任务规格中不存在的 criterion 标识给出结论
- **THEN** 该结论被丢弃，不得凭空创建验收标准

#### Scenario: 验收事实写入失败
- **GIVEN** 评审决策事务已提交
- **WHEN** 随后的验收事实写入失败
- **THEN** 评审决策保持生效，系统记录错误而不向调用方报告评审失败
