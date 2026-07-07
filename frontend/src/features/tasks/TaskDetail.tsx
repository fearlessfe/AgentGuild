import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { getExecution, getTask, pollInterval } from "../../api/client";
import { Card, StatusChip } from "../../ui";

type ChipTone = "action" | "success" | "warning" | "danger" | "info" | "neutral";

const statusLabel: Record<string, string> = {
  draft: "草稿",
  open: "待领取",
  claimed: "待审核",
  in_progress: "进行中",
  completed: "已完成",
  cancelled: "已取消",
  expired: "已过期",
};

const statusTone: Record<string, ChipTone> = {
  draft: "neutral",
  open: "neutral",
  claimed: "warning",
  in_progress: "info",
  completed: "success",
  cancelled: "danger",
  expired: "danger",
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

  if (task.isPending)
    return (
      <DetailShell onClose={onClose}>
        <div className="loading">加载详情…</div>
      </DetailShell>
    );
  if (task.isError)
    return (
      <DetailShell onClose={onClose}>
        <div className="error">详情不可用</div>
      </DetailShell>
    );

  return (
    <DetailShell onClose={onClose}>
      <TaskDetailContent
        task={task.data.data}
        execution={execution.data?.data}
        serverTime={task.data.meta.server_time}
      />
    </DetailShell>
  );
}

TaskDetail.Empty = function TaskDetailEmpty() {
  return (
    <Card>
      <div className="empty-state">
        <span className="es-icon" aria-hidden="true">
          ◱
        </span>
        <div className="es-title">选择左侧任务查看详情</div>
      </div>
    </Card>
  );
};

function DetailShell({ children, onClose }: { children: React.ReactNode; onClose?: () => void }) {
  const isMobile = useMobile();
  return isMobile ? (
    <MobileDrawer onClose={onClose}>{children}</MobileDrawer>
  ) : (
    <Card>
      <div aria-label="任务详情">{children}</div>
    </Card>
  );
}

function MobileDrawer({ children, onClose }: { children: React.ReactNode; onClose?: () => void }) {
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
    <dialog
      ref={dialogRef}
      className="card task-detail-drawer"
      role="dialog"
      aria-modal="true"
      aria-label="任务详情"
      onCancel={handleCancel}
      onClick={handleBackdropClick}
    >
      <div className="card-pad">
        <button className="icon-btn drawer-close" onClick={handleClose} aria-label="关闭详情">
          ×
        </button>
        {children}
      </div>
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
  const leaseActive = execution && ["leased", "running"].includes(execution.status);

  return (
    <div className="stack">
      <div className="row-between">
        <strong>{task.id}</strong>
        <StatusChip tone={statusTone[task.status] ?? "neutral"}>
          {statusLabel[task.status] ?? task.status}
        </StatusChip>
      </div>

      <div className="detail-block">
        <span className="ctx-label">标题</span>
        <div className="text-sm">{task.title}</div>
      </div>
      <div className="detail-block">
        <span className="ctx-label">目标</span>
        <div className="text-sm muted">{task.problem}</div>
      </div>
      <div className="detail-block">
        <span className="ctx-label">仓库</span>
        <div className="text-sm">{task.publisher_agent_version_id} ↗</div>
      </div>
      <div className="detail-block">
        <span className="ctx-label">验收标准</span>
        <ul className="perm-list">
          {task.requirements.map((requirement) => (
            <li key={requirement}>{requirement}</li>
          ))}
        </ul>
      </div>

      <div className="fact-grid">
        <div className="fact">
          <span className="ctx-label">状态</span>
          <span>{task.status}</span>
        </div>
        <div className="fact">
          <span className="ctx-label">类型</span>
          <span>{task.type}</span>
        </div>
        <div className="fact">
          <span className="ctx-label">截止时间</span>
          <span>{formatDateTime(task.deadline)}</span>
        </div>
        <div className="fact">
          <span className="ctx-label">发布 Agent</span>
          <span>{task.publisher_agent_version_id}</span>
        </div>
        {task.claimed_by ? (
          <div className="fact">
            <span className="ctx-label">认领 Agent</span>
            <span>{task.claimed_by}</span>
          </div>
        ) : null}
        <div className="fact">
          <span className="ctx-label">任务版本</span>
          <span>v{task.state_version}</span>
        </div>
      </div>

      {execution ? (
        <Card title="Execution">
          <div className="stack-sm">
            <div className="row-between">
              <b>{execution.agent_version_id}</b>
              <span>{executionStatusLabel[execution.status] ?? execution.status}</span>
            </div>
            {execution.stage ? (
              <p className="text-sm muted">
                <span>{execution.stage}</span>　<span>{execution.progress ?? 0}%</span>
              </p>
            ) : null}
            <p className="text-sm">{leaseActive ? formatLeaseDuration(serverTime, execution.lease_soft_expires_at) : "expired"}</p>
            <small className="faint">
              Soft {formatDateTime(execution.lease_soft_expires_at)} · Hard {formatDateTime(execution.lease_hard_expires_at)}
            </small>
            {execution.cost ? (
              <div className="text-sm muted">
                <p>Cost coverage: {execution.cost.coverage}</p>
                <p>
                  Observed cost: ${execution.cost.observed_cost ?? "—"} · Self-reported: $
                  {execution.cost.self_reported_cost ?? "—"}
                </p>
                <p>Provider: {execution.cost.provider}</p>
                {execution.cost.observed_at ? <p>Observed at: {formatDateTime(execution.cost.observed_at)}</p> : null}
              </div>
            ) : null}
            {execution.audit_summary ? <p className="text-sm faint">审计　{execution.audit_summary}</p> : null}
          </div>
        </Card>
      ) : null}
    </div>
  );
}
