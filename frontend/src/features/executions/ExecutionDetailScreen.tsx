import { useQuery } from "@tanstack/react-query";
import { RefreshCcw } from "lucide-react";
import type { ReactNode } from "react";
import { useParams } from "react-router-dom";
import { getExecution, listExecutionSubmissions, pollInterval, type ExecutionStatus, type ExecutionView } from "../../api/client";
import { PageHeader, Card, StatusChip, Timeline, Button, ButtonLink } from "../../ui";
import type { TimelineEvent } from "../../ui";

const STATUS_LABELS: Record<ExecutionStatus, string> = {
  leased: "已领取",
  running: "执行中",
  submitted: "已提交",
  validating: "验证中",
  validation_failed: "验证失败",
  reviewing: "待审核",
  revision_requested: "需修改",
  accepted: "已接受",
  rejected: "已拒绝",
  expired: "已过期",
  cancelled: "已取消",
};

export function ExecutionDetailScreen() {
  const { executionId } = useParams<{ executionId: string }>();
  const query = useQuery({
    queryKey: ["execution", executionId],
    queryFn: () => getExecution(executionId ?? ""),
    enabled: Boolean(executionId),
    refetchInterval: (state) => pollInterval(state.state.data?.meta.poll_after_seconds),
  });
  const submissionsQuery = useQuery({
    queryKey: ["execution-submissions", executionId],
    queryFn: () => listExecutionSubmissions(executionId ?? ""),
    enabled: Boolean(executionId),
  });

  if (!executionId) return <StateCard title="未指定执行" message="请从任务详情进入一次执行。" />;
  if (query.isPending) return <StateCard title="加载执行详情" message="正在读取最新执行状态..." status />;
  if (query.isError) return <StateCard title="无法加载执行" message={query.error.message} retry={() => void query.refetch()} />;

  const execution = query.data.data;
  const events = executionEvents(execution);
  return (
    <div className="stack">
      <PageHeader
        title="执行详情"
        sub={`${execution.task_id} · ${execution.agent_version_id}`}
        actions={<Button icon={<RefreshCcw size={14} />} onClick={() => void query.refetch()} disabled={query.isFetching}>{query.isFetching ? "刷新中..." : "刷新"}</Button>}
      />
      <div className="split-2">
        <div className="col">
          <Card title="执行事实" head={<StatusChip tone={statusTone(execution.status)}>{STATUS_LABELS[execution.status]}</StatusChip>}>
            <div className="fact-grid">
              <Fact label="Execution"><code>{execution.id}</code></Fact>
              <Fact label="Agent Version">{execution.agent_version_id}</Fact>
              <Fact label="阶段">{execution.stage || "尚未上报"}</Fact>
              <Fact label="Lease generation">{execution.lease_generation}</Fact>
              <Fact label="Lease soft expiry">{formatDate(execution.lease_soft_expires_at)}</Fact>
              <Fact label="Lease hard expiry">{formatDate(execution.lease_hard_expires_at)}</Fact>
              <Fact label="最近心跳">{formatDate(execution.last_heartbeat_at)}</Fact>
              <Fact label="观测成本">{formatCost(execution.cost?.observed_cost)}</Fact>
              <Fact label="自报成本">{formatCost(execution.cost?.self_reported_cost)}</Fact>
              <Fact label="成本覆盖率"><StatusChip tone={execution.cost?.coverage === "complete" ? "success" : execution.cost?.coverage === "partial" ? "warning" : "neutral"}>{execution.cost?.coverage ?? "unavailable"}</StatusChip></Fact>
            </div>
            {execution.audit_summary ? <p className="field-hint">{execution.audit_summary}</p> : null}
          </Card>
          <Card title="提交记录" sub={submissionsQuery.data ? `${submissionsQuery.data.data.length} 个提交` : undefined}>
            {submissionsQuery.isPending ? <p className="text-sm muted">正在加载提交...</p> : null}
            {submissionsQuery.isError ? <p className="text-sm error">提交记录不可用：{submissionsQuery.error.message}</p> : null}
            {submissionsQuery.data?.data.length === 0 ? <p className="text-sm muted">当前执行尚未创建提交。</p> : null}
            {submissionsQuery.data?.data.map((submission) => (
              <div className="row-between" key={submission.id}>
                <span><code>{submission.commit_sha.slice(0, 12)}</code> · {submission.status}</span>
                <ButtonLink to={`/submissions/${encodeURIComponent(submission.id)}`}>查看提交</ButtonLink>
              </div>
            ))}
          </Card>
          <div className="row"><ButtonLink to={`/tasks/${encodeURIComponent(execution.task_id)}`}>查看任务</ButtonLink></div>
        </div>
        <div className="col">
          <Card title="生命周期" sub={`${events.length} 个已记录时间点`}>
            {events.length > 0 ? <Timeline events={events} /> : <p className="text-sm muted">该执行暂未记录生命周期时间点。</p>}
          </Card>
        </div>
      </div>
    </div>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return <div className="fact"><span className="ctx-label">{label}</span><span>{children}</span></div>;
}

function StateCard({ title, message, retry, status = false }: { title: string; message: string; retry?: () => void; status?: boolean }) {
  return <Card title={title}><div className="stack-sm"><p className="text-sm" role={status ? "status" : retry ? "alert" : undefined}>{message}</p>{retry ? <div className="row"><Button onClick={retry}>重试</Button></div> : null}</div></Card>;
}

function executionEvents(execution: ExecutionView): TimelineEvent[] {
  const entries: Array<[string | undefined, string, string?]> = [
    [execution.claimed_at, "任务已领取", `Lease generation ${execution.lease_generation}`],
    [execution.started_at, "执行已开始", execution.stage],
    [execution.last_heartbeat_at, "最近心跳", execution.audit_summary],
    [execution.submitted_at, "提交已创建"],
    [execution.expired_at, "执行已过期"],
  ];
  return entries.filter((entry): entry is [string, string, string?] => Boolean(entry[0])).map(([time, title, note]) => ({ time: formatDate(time), title, note }));
}

function formatDate(value?: string): string { return value ? new Date(value).toLocaleString("zh-CN") : "—"; }
function formatCost(value?: string): string { return value ? `$${value}` : "—"; }
function statusTone(status: ExecutionStatus): "success" | "warning" | "danger" | "info" | "neutral" {
  if (status === "accepted") return "success";
  if (status === "validation_failed" || status === "rejected" || status === "expired" || status === "cancelled") return "danger";
  if (status === "revision_requested") return "warning";
  if (["running", "submitted", "validating", "reviewing"].includes(status)) return "info";
  return "neutral";
}
