import { useInfiniteQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { listTasks, pollInterval, type TaskStatus, type TaskView } from "../../api/client";
import { StatusChip } from "../../ui";

const statusLabel: Record<TaskStatus, string> = {
  draft: "草稿",
  open: "待领取",
  claimed: "待审核",
  in_progress: "进行中",
  completed: "已完成",
  cancelled: "已取消",
  expired: "已过期",
};

type ChipTone = "action" | "success" | "warning" | "danger" | "info" | "neutral";

const statusTone: Record<TaskStatus, ChipTone> = {
  draft: "neutral",
  open: "neutral",
  claimed: "warning",
  in_progress: "info",
  completed: "success",
  cancelled: "danger",
  expired: "danger",
};

const tabs: { status?: TaskStatus; label: string }[] = [
  { label: "全部" },
  { status: "open", label: "待领取" },
  { status: "in_progress", label: "进行中" },
  { status: "claimed", label: "待审核" },
  { status: "completed", label: "已完成" },
  { status: "cancelled", label: "已取消" },
  { status: "expired", label: "已过期" },
  { status: "draft", label: "草稿" },
];

const typeOptions = ["", "Go", "TypeScript", "Python", "Rust", "SQL", "YAML"];

function formatDeadline(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}`;
}

export function TaskList() {
  const [searchParams, setSearchParams] = useSearchParams();
  const statusParam = searchParams.get("status");
  const statuses = statusParam ? [statusParam] : undefined;
  const type = searchParams.get("type") ?? undefined;
  const publisher = searchParams.get("publisher_agent_version_id") ?? undefined;

  const query = useInfiniteQuery({
    queryKey: ["tasks", { statuses, type, publisher }],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      listTasks({
        statuses,
        type,
        publisherAgentVersionId: publisher,
        cursor: pageParam || undefined,
      }),
    getNextPageParam: (last) => last.meta.next_cursor || undefined,
    refetchInterval: (state) => pollInterval(state.state.data?.pages.at(-1)?.meta.poll_after_seconds),
  });

  function updateSearchParam(key: string, value: string | undefined) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (value) next.set(key, value);
      else next.delete(key);
      return next;
    });
  }

  function setStatusFilter(status?: TaskStatus) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (status) next.set("status", status);
      else next.delete("status");
      return next;
    });
  }

  const toolbar = (
    <div className="stack-sm">
      <div className="tabs" role="tablist" aria-label="任务状态筛选">
        {tabs.map((tab) => {
          const active = tab.status ? statusParam === tab.status : !statusParam;
          return (
            <button
              key={tab.status ?? "all"}
              type="button"
              role="tab"
              aria-selected={active}
              className="tab"
              data-active={active}
              onClick={() => setStatusFilter(tab.status)}
            >
              {tab.label}
            </button>
          );
        })}
      </div>
      <div className="filter-bar">
        <label className="filter">
          类型：
          <select
            aria-label="类型"
            value={type ?? ""}
            onChange={(e) => updateSearchParam("type", e.target.value || undefined)}
          >
            {typeOptions.map((opt) => (
              <option key={opt} value={opt}>
                {opt || "全部"}
              </option>
            ))}
          </select>
        </label>
        <label className="filter">
          发布 Agent：
          <input
            type="text"
            aria-label="发布 Agent"
            value={publisher ?? ""}
            placeholder="publisher_agent_version_id"
            onChange={(e) => updateSearchParam("publisher_agent_version_id", e.target.value || undefined)}
          />
        </label>
      </div>
    </div>
  );

  if (query.isPending) {
    return (
      <div className="stack">
        {toolbar}
        <div className="loading">正在同步任务…</div>
      </div>
    );
  }
  if (query.isError) {
    return (
      <div className="stack">
        {toolbar}
        <div className="error">无法读取任务：{query.error.message}</div>
      </div>
    );
  }

  const tasks = query.data.pages.flatMap((page) => page.data.items);
  const groups = tabs
    .filter((t) => t.status)
    .map((t) => ({
      status: t.status!,
      label: statusLabel[t.status!],
      items: tasks.filter((task) => task.status === t.status),
    }))
    .filter((group) => group.items.length > 0);

  return (
    <div className="stack">
      {toolbar}
      <section className="card" aria-label="任务列表">
        <table className="dense-table">
          <thead>
            <tr>
              <th scope="col">Task ID</th>
              <th scope="col">标题</th>
              <th scope="col">仓库</th>
              <th scope="col">类型</th>
              <th scope="col">状态</th>
              <th scope="col">截止时间</th>
            </tr>
          </thead>
          <tbody>
            {groups.map((group) => (
              <TaskGroup key={group.status} label={group.label} count={group.items.length} items={group.items} />
            ))}
          </tbody>
        </table>
      </section>
      <div className="task-mobile-list" aria-label="移动任务列表">
        {groups.flatMap((group) =>
          group.items.map((task) => (
            <Link className="task-mobile-card" key={task.id} to={`/console/tasks/${task.id}`} aria-label={`查看 ${task.id}`}>
              <div className="row-between">
                <code>{task.id}</code>
                <StatusChip tone={statusTone[task.status]}>{statusLabel[task.status]}</StatusChip>
              </div>
              <strong>{task.title}</strong>
              <span className="muted text-sm">{task.publisher_agent_version_id}</span>
              <span className="faint text-xs">{formatDeadline(task.deadline)}</span>
            </Link>
          )),
        )}
      </div>
      {query.hasNextPage && (
        <button
          type="button"
          className="btn"
          onClick={() => query.fetchNextPage()}
          disabled={query.isFetchingNextPage}
        >
          加载更多
        </button>
      )}
    </div>
  );
}

function TaskGroup({ label, count, items }: { label: string; count: number; items: TaskView[] }) {
  return (
    <>
      <tr className="group-row">
        <td colSpan={6}>
          {label} · {count}
        </td>
      </tr>
      {items.map((task) => (
        <TaskRow task={task} key={task.id} />
      ))}
    </>
  );
}

function TaskRow({ task }: { task: TaskView }) {
  return (
    <tr>
      <td>
        <Link to={`/console/tasks/${task.id}`} aria-label={`查看 ${task.id}`}>
          <code>{task.id}</code>
        </Link>
      </td>
      <td className="task-title" title={task.title}>
        <div>
          {task.title}
          {task.source?.kind === "issue" && (
            <div className="text-sm muted" style={{ marginTop: "0.25rem" }}>
              <a
                href={task.source.issue_url}
                target="_blank"
                rel="noopener noreferrer"
                style={{ textDecoration: "none", color: "inherit" }}
              >
                Issue #{task.source.issue_number} @ {task.source.repo}
              </a>
            </div>
          )}
        </div>
      </td>
      <td>{task.publisher_agent_version_id}</td>
      <td className="language">{task.type}</td>
      <td>
        <StatusChip tone={statusTone[task.status]}>{statusLabel[task.status]}</StatusChip>
      </td>
      <td>{formatDeadline(task.deadline)}</td>
    </tr>
  );
}
