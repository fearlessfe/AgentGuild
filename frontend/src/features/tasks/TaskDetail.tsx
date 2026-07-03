import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { getExecution, getTask, pollInterval } from "../../api/client";

const statusLabel: Record<string, string> = {
  draft: "草稿",
  open: "待领取",
  claimed: "待审核",
  in_progress: "进行中",
  completed: "已完成",
  cancelled: "已取消",
  expired: "已过期",
};

const executionStatusLabel: Record<string, string> = {
  leased: "Leased",
  running: "Running",
  submitted: "Submitted",
  validating: "Validating",
  reviewing: "Reviewing",
  revision_requested: "Revision requested",
  accepted: "Accepted",
  rejected: "Rejected",
  expired: "Expired",
  cancelled: "Cancelled",
};

function formatDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}`;
}

function formatLeaseDuration(serverTime: string, softExpiresAt: string): string {
  const remaining = new Date(softExpiresAt).getTime() - new Date(serverTime).getTime();
  if (Number.isNaN(remaining)) return "lease unknown";
  if (remaining <= 0) return "expired";
  const totalMinutes = Math.round(remaining / 60_000);
  if (totalMinutes < 1) return "<1m lease";
  if (totalMinutes < 60) return `${totalMinutes}m lease`;
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return minutes > 0 ? `${hours}h ${minutes}m lease` : `${hours}h lease`;
}

function useMobile() {
  const [isMobile, setIsMobile] = useState(false);
  useEffect(() => {
    const media = window.matchMedia("(max-width: 700px)");
    setIsMobile(media.matches);
    const handler = (e: MediaQueryListEvent) => setIsMobile(e.matches);
    media.addEventListener("change", handler);
    return () => media.removeEventListener("change", handler);
  }, []);
  return isMobile;
}

export function TaskDetail({ taskId, onClose }: { taskId: string; onClose?: () => void }) {
  const isMobile = useMobile();
  const task = useQuery({
    queryKey: ["task", taskId],
    queryFn: () => getTask(taskId),
    refetchInterval: (q) => pollInterval(q.state.data?.meta.poll_after_seconds),
  });
  const executionId = task.data?.data.active_execution_id;
  const execution = useQuery({
    queryKey: ["execution", executionId],
    queryFn: () => getExecution(executionId!),
    enabled: Boolean(executionId),
    refetchInterval: (q) => pollInterval(q.state.data?.meta.poll_after_seconds),
  });

  if (task.isPending) return <DetailShell onClose={onClose}><div className="loading">加载详情…</div></DetailShell>;
  if (task.isError) return <DetailShell onClose={onClose}><div className="error">详情不可用</div></DetailShell>;

  return (
    <DetailShell onClose={onClose}>
      <TaskDetailContent task={task.data.data} execution={execution.data?.data} serverTime={task.data.meta.server_time} />
    </DetailShell>
  );
}

function DetailShell({
  children,
  onClose,
}: {
  children: React.ReactNode;
  onClose?: () => void;
}) {
  const isMobile = useMobile();
  return isMobile ? (
    <MobileDrawer onClose={onClose}>{children}</MobileDrawer>
  ) : (
    <aside className="task-detail" aria-label="任务详情">
      <button className="drawer-close" onClick={onClose} aria-label="关闭详情">×</button>
      {children}
    </aside>
  );
}

function MobileDrawer({
  children,
  onClose,
}: {
  children: React.ReactNode;
  onClose?: () => void;
}) {
  const dialogRef = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    if (!dialog.open) dialog.showModal();
    return () => {
      if (dialog.open) dialog.close();
    };
  }, []);

  function handleClose() {
    dialogRef.current?.close();
    onClose?.();
  }

  function handleCancel(e: React.SyntheticEvent<HTMLDialogElement>) {
    e.preventDefault();
    handleClose();
  }

  function handleBackdropClick(e: React.MouseEvent<HTMLDialogElement>) {
    if (e.target === dialogRef.current) {
      handleClose();
    }
  }

  return createPortal(
    <
      dialog
      ref={dialogRef}
      className="task-detail mobile-drawer"
      role="dialog"
      aria-modal="true"
      aria-label="任务详情"
      onCancel={handleCancel}
      onClick={handleBackdropClick}
    >
      <button className="drawer-close" onClick={handleClose} aria-label="关闭详情">×</button>
      {children}
    </dialog>,
    document.body,
  );
}

function TaskDetailContent({
  task,
  execution,
  serverTime,
}: {
  task: import("../../api/client").TaskView;
  execution?: import("../../api/client").ExecutionView;
  serverTime: string;
}) {
  const leaseActive =
    execution && ["leased", "running"].includes(execution.status);

  return (
    <>
      <div className="detail-heading">
        <span>{task.id}</span>
        <span className={`detail-status ${task.status}`}>
          ● {statusLabel[task.status] ?? task.status}
        </span>
      </div>
      <h2>{task.title}</h2>
      <section>
        <h3>目标</h3>
        <p>{task.problem}</p>
      </section>
      <section>
        <h3>仓库</h3>
        <p>{task.publisher_agent_version_id} ↗</p>
      </section>
      <section>
        <h3>验收标准</h3>
        <ol>
          {task.requirements.map((requirement) => (
            <li key={requirement}>{requirement}</li>
          ))}
        </ol>
      </section>
      <dl>
        <div>
          <dt>状态</dt>
          <dd>{task.status}</dd>
        </div>
        <div>
          <dt>类型</dt>
          <dd>{task.type}</dd>
        </div>
        <div>
          <dt>截止时间</dt>
          <dd>{formatDateTime(task.deadline)}</dd>
        </div>
        <div>
          <dt>发布 Agent</dt>
          <dd>{task.publisher_agent_version_id}</dd>
        </div>
        {task.claimed_by && (
          <div>
            <dt>认领 Agent</dt>
            <dd>{task.claimed_by}</dd>
          </div>
        )}
        <div>
          <dt>任务版本</dt>
          <dd>v{task.state_version}</dd>
        </div>
      </dl>

      {execution && (
        <section className="execution-card">
          <h3>Execution</h3>
          <div className="execution-line">
            <b>{execution.agent_version_id}</b>
            <span>{executionStatusLabel[execution.status] ?? execution.status}</span>
          </div>
          {execution.stage && (
            <>
              <div className="detail-progress">
                <i style={{ width: `${execution.progress ?? 0}%` }} />
              </div>
              <p>
                <span>{execution.stage}</span>　
                <span>{execution.progress ?? 0}%</span>
              </p>
            </>
          )}
          <p>{leaseActive ? formatLeaseDuration(serverTime, execution.lease_soft_expires_at) : "expired"}</p>
          <small>
            Soft {formatDateTime(execution.lease_soft_expires_at)} · Hard{" "}
            {formatDateTime(execution.lease_hard_expires_at)}
          </small>
          {execution.cost && (
            <div className="cost-block">
              <p>Cost coverage: {execution.cost.coverage}</p>
              <p>
                Observed cost: ${execution.cost.observed_cost ?? "—"} · Self-reported: $
                {execution.cost.self_reported_cost ?? "—"}
              </p>
              <p>Provider: {execution.cost.provider}</p>
              {execution.cost.observed_at && (
                <p>Observed at: {formatDateTime(execution.cost.observed_at)}</p>
              )}
            </div>
          )}
          {execution.audit_summary && (
            <p className="audit">审计　{execution.audit_summary}</p>
          )}
        </section>
      )}
    </>
  );
}
