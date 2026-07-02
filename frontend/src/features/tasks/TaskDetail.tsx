import { useQuery } from "@tanstack/react-query";
import { getExecution, getTask, pollInterval } from "../../api/client";

export function TaskDetail({ taskId, onClose }: { taskId: string; onClose?: () => void }) {
  const task = useQuery({ queryKey: ["task", taskId], queryFn: () => getTask(taskId), refetchInterval: (q) => pollInterval(q.state.data?.meta.poll_after_seconds) });
  const executionId = task.data?.data.active_execution_id;
  const execution = useQuery({ queryKey: ["execution", executionId], queryFn: () => getExecution(executionId!), enabled: Boolean(executionId), refetchInterval: (q) => pollInterval(q.state.data?.meta.poll_after_seconds) });
  if (task.isPending) return <aside className="task-detail"><div className="loading">加载详情…</div></aside>;
  if (task.isError) return <aside className="task-detail"><div className="error">详情不可用</div></aside>;
  const item = task.data.data, run = execution.data?.data;
  const leaseActive = run && ["leased", "running"].includes(run.status);
  return <aside className="task-detail" aria-label="任务详情">
    <button className="drawer-close" onClick={onClose} aria-label="关闭详情">×</button>
    <div className="detail-heading"><span>{item.id}</span><span className={`detail-status ${item.status}`}>● {item.status === "open" ? "待领取" : item.status}</span></div>
    <h2>{item.title}</h2>
    <section><h3>目标</h3><p>{item.problem}</p></section>
    <section><h3>仓库</h3><p>{item.publisher_agent_version_id} ↗</p></section>
    <section><h3>验收标准</h3><ol>{item.requirements.map((requirement) => <li key={requirement}>{requirement}</li>)}</ol></section>
    <dl><div><dt>状态</dt><dd>{item.status}</dd></div><div><dt>截止时间</dt><dd>{item.deadline.slice(0, 16).replace("T", " ")}</dd></div><div><dt>创建者</dt><dd>◉ 周昊然</dd></div><div><dt>任务版本</dt><dd>v{item.state_version}</dd></div></dl>
    <section className="execution-card"><h3>Execution</h3><div className="execution-line"><b>{run?.agent_version_id ?? "尚未分配"}</b><span>{run?.status ?? "idle"}</span></div>{run && <><div className="detail-progress"><i style={{ width: `${run.progress ?? 0}%` }} /></div><p><span>{run.stage ?? "等待 Agent"}</span>　<span>{run.progress ?? 0}%</span></p><p>Lease: {leaseActive ? "active" : "inactive"}</p><small>Soft {run.lease_soft_expires_at.slice(11, 19)} · Hard {run.lease_hard_expires_at.slice(11, 19)}</small><p>Cost coverage: {run.cost?.coverage ?? "unavailable"}</p><p className="audit">审计　{run.audit_summary ?? "暂无事件"}</p></>}</section>
  </aside>;
}
