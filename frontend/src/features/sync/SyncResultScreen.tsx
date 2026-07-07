import { PageHeader, Card, Button, StatusChip, DenseTable, MetricGrid, ApiNote } from "../../ui";
import type { DenseRow, Metric } from "../../ui";
import { sharedData } from "../shared/mockData";

const METRICS: Metric[] = [
  { label: "新增", value: "8", positive: true },
  { label: "更新", value: "3" },
  { label: "忽略", value: "5" },
  { label: "冲突", value: "2" },
];

const RESULT_ROWS: DenseRow[] = [
  {
    cells: [
      <code>#412</code>,
      <code>{sharedData.taskId}</code>,
      <StatusChip tone="success">新增</StatusChip>,
      "标签匹配 agent-ready",
      "20s 前",
      <Button variant="ghost">查看</Button>,
    ],
  },
  {
    cells: [
      <code>#398</code>,
      <code>AG-188</code>,
      <StatusChip tone="info">更新</StatusChip>,
      "标题与验收条件变更",
      "20s 前",
      <Button variant="ghost">查看</Button>,
    ],
  },
  {
    cells: [
      <code>#377</code>,
      "—",
      <StatusChip tone="neutral">忽略</StatusChip>,
      "命中排除标签 needs-product",
      "20s 前",
      <Button variant="ghost">查看</Button>,
    ],
  },
  {
    cells: [
      <code>#365</code>,
      <code>AG-171</code>,
      <StatusChip tone="warning">冲突</StatusChip>,
      "Task 已被人工修改",
      "21s 前",
      <span className="row">
        <Button variant="ghost">重试</Button>
        <Button variant="ghost">忽略</Button>
        <Button variant="ghost">暂停规则</Button>
      </span>,
    ],
  },
  {
    cells: [
      <code>#359</code>,
      "—",
      <StatusChip tone="danger">失败</StatusChip>,
      "GitHub 速率限制 · 可重试",
      "21s 前",
      <Button variant="ghost">重试</Button>,
    ],
  },
];

export function SyncResultScreen() {
  return (
    <div className="stack">
      <PageHeader title="同步结果" sub="规则 billing-service · agent-ready 的最近一次运行。" />
      <MetricGrid metrics={METRICS} />
      <div className="row">
        <span className="faint text-sm">另有</span>
        <StatusChip tone="danger">失败 1</StatusChip>
        <span className="faint text-sm">为平台可重试错误</span>
      </div>
      <Card title="运行明细" sub="19 条" pad={false}>
        <DenseTable
          columns={["Issue", "Task", "结果", "原因", "更新时间", "操作"]}
          rows={RESULT_ROWS}
          caption="同步运行明细"
        />
      </Card>
      <ApiNote status="planned">
        同步运行、结果列表、冲突处理与重试/审计均为规划能力；冲突行不提供直接编辑正式 Task 的入口。
      </ApiNote>
    </div>
  );
}
