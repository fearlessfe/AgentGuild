## Context

初始 Agent Version 在激活阶段创建。本 change 补齐后续版本谱系、经验候选、基准回归、晋级和回滚，确保任务评分与运行配置可重现。

## Goals / Non-Goals

**Goals:**

- 对模型、Prompt、Skill、Memory 和工具配置生成不可变版本。
- 从已验收任务提取可审计的经验候选。
- 通过基准回归和明确审批完成晋级或回滚。

**Non-Goals:**

- 自动修改生产 Agent、跨组织经验市场和不可解释的在线自学习。

## Decisions

1. 任何配置指纹变化创建新 Agent Version；已有版本内容不可原地修改。
2. 版本采用父子谱系和状态机：Draft、Evaluating、Eligible、Active、Retired、Rejected。
3. ExperienceCandidate 保存来源任务、证据、适用范围、敏感级别和内容哈希，不直接写入 Agent Memory。
4. EvaluationRun 冻结候选版本、基准集版本、环境与评分规则；晋级必须满足硬门槛并记录审批人。
5. 回滚仅切换 Agent 的 `current_version_id`，不删除失败版本或历史任务关系。

## Risks / Trade-offs

- [配置无法完整复现] → 保存内容寻址引用、哈希、工具版本和环境摘要。
- [经验泄露敏感数据] → 提取前分类、脱敏和人工审批，禁止跨 tenant 使用。
- [基准过拟合] → 使用版本化公开与保留基准，并监控真实任务表现。

## Migration Plan

将现有初始版本标记为 Active，随后启用候选版本、评测和晋级；出现问题时切回上一个 Active 版本并保留完整审计。

## Open Questions

- 初版基准集构建、人工审批角色和各能力门槛在深度设计中确定。
