import { PageHeader, Card, StatusChip, MetricGrid, ApiNote } from "../../ui";
import type { Metric } from "../../ui";
import { sharedData } from "../shared/mockData";

const OUTCOME_METRICS: Metric[] = [
  { label: "审核评分", value: sharedData.reviewScore, positive: true },
  { label: "声望", value: sharedData.reputationDelta, positive: true },
  { label: "Issue 回写", value: "成功", positive: true },
  { label: "经验候选", value: "已创建" },
];

export function OutcomeScreen() {
  return (
    <div className="stack">
      <PageHeader title="结果闭环" sub={`${sharedData.taskId} · 已接受路径为主状态`} />
      <Card title="已接受" head={<StatusChip tone="success">已完成</StatusChip>}>
        <div className="stack">
          <MetricGrid metrics={OUTCOME_METRICS} />
          <ul className="perm-list">
            <li>Task 已完成，Execution 已接受</li>
            <li>GitHub Issue 回写成功（#412 已关闭并评论）</li>
            <li>经验候选已创建，等待评测</li>
          </ul>
        </div>
      </Card>
      <div className="split-2">
        <div className="col">
          <Card title="返工分支">
            <div className="stack-sm">
              <div className="row-between">
                <strong>返工请求（并列状态）</strong>
                <StatusChip tone="warning">待返工</StatusChip>
              </div>
              <ul className="perm-list">
                <li>2 条未解决审核评论</li>
                <li>Agent 正在准备下一版修订</li>
                <li>尚未提交新的 revision</li>
              </ul>
            </div>
          </Card>
        </div>
        <div className="col">
          <ApiNote status="partial">GET /v1/reputation 与 Agent 经验/评测接口可查声望与经验。</ApiNote>
          <ApiNote status="planned">outcome 聚合、Issue 回写状态与审计时间线待补。</ApiNote>
        </div>
      </div>
    </div>
  );
}
