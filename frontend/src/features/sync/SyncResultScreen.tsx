import { PageHeader, Card, Button, MetricGrid } from "../../ui";
import type { Metric } from "../../ui";
import type { SyncResult } from "../../api/client";

type SyncResultScreenProps = {
  result: SyncResult;
  onClose: () => void;
};

export function SyncResultScreen({ result, onClose }: SyncResultScreenProps) {
  const metrics: Metric[] = [
    { label: "新增", value: String(result.created), positive: result.created > 0 },
    { label: "更新", value: String(result.updated) },
    { label: "跳过", value: String(result.skipped) },
    { label: "取消", value: String(result.cancelled) },
    { label: "待处理", value: String(result.flagged) },
    { label: "失败", value: String(result.failed), positive: result.failed === 0 },
  ];

  const total = result.created + result.updated + result.skipped + result.cancelled + result.flagged + result.failed;

  return (
    <div className="stack">
      <PageHeader
        title="运行结果"
        sub={`生成策略运行完成，共处理 ${total} 项`}
        actions={<Button onClick={onClose}>返回</Button>}
      />
      <MetricGrid metrics={metrics} />
      <Card title="摘要">
        <div className="stack">
          <p className="text-sm">
            本次运行创建了 <strong>{result.created}</strong> 个新任务，
            更新了 <strong>{result.updated}</strong> 个现有任务，
            跳过了 <strong>{result.skipped}</strong> 个不符合条件的 Issue。
          </p>
          {result.cancelled > 0 && (
            <p className="text-sm muted">
              有 <strong>{result.cancelled}</strong> 个操作被取消。
            </p>
          )}
          {result.flagged > 0 && (
            <p className="text-sm muted">
              有 <strong>{result.flagged}</strong> 个已领取任务对应的 Issue 已关闭，任务状态未被强制改变。
            </p>
          )}
          {result.failed > 0 && (
            <p className="text-sm" style={{ color: "var(--danger)" }}>
              有 <strong>{result.failed}</strong> 个操作失败，请检查日志。
            </p>
          )}
        </div>
      </Card>
    </div>
  );
}
