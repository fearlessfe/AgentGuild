import { useInfiniteQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { listTasks, pollInterval, type TaskStatus, type TaskView } from "../../api/client";

const statusLabel: Record<TaskStatus, string> = {
  draft: "草稿",
  open: "待领取",
  claimed: "待审核",
  in_progress: "进行中",
  completed: "已完成",
  cancelled: "已取消",
  expired: "已过期",
};

const statusDotLabel: Record<TaskStatus, string> = {
  draft: "草稿",
  open: "待领取",
  claimed: "待审核",
  in_progress: "运行中",
  completed: "完成",
  cancelled: "取消",
  expired: "过期",
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

  if (query.isPending) return <div className="loading">正在同步任务…</div>;
  if (query.isError) return <div className="error">无法读取任务：{query.error.message}</div>;

  const tasks = query.data.pages.flatMap((page) => page.data.items);
  const groups = tabs.filter((t) => t.status).map((t) => ({
    status: t.status!,
    label: statusLabel[t.status!],
    items: tasks.filter((task) => task.status === t.status),
  }));

  return (
    <>
      <nav className="tabs" role="tablist" aria-label="任务状态筛选">
        {tabs.map((tab) => {
          const active = tab.status ? statusParam === tab.status : !statusParam;
          return (
            <button
              key={tab.status ?? "all"}
              role="tab"
              aria-selected={active}
              className={active ? "active" : undefined}
              onClick={() => setStatusFilter(tab.status)}
            >
              {tab.label}
            </button>
          );
        })}
      </nav>
      <div className="filters">
        <label htmlFor="type-filter">
          类型
          <select
            id="type-filter"
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
        <label htmlFor="publisher-filter">
          发布 Agent
          <input
            id="publisher-filter"
            type="text"
            value={publisher ?? ""}
            placeholder="publisher_agent_version_id"
            onChange={(e) => updateSearchParam("publisher_agent_version_id", e.target.value || undefined)}
          />
        </label>
      </div>
      <div className="task-table" aria-label="任务列表">
        {groups.map((group) => (
          <section className={`task-group group-${group.status}`} key={group.status}>
            <header className="group-header">
              <span>⌄</span>
              <strong>{group.label} · {group.items.length}</strong>
              <span className="group-meta">仓库　　　　　　类型　　　 截止时间</span>
            </header>
            {group.items.map((task) => (
              <TaskRow task={task} key={task.id} />
            ))}
          </section>
        ))}
        {query.hasNextPage && (
          <button
            className="load-more"
            onClick={() => query.fetchNextPage()}
            disabled={query.isFetchingNextPage}
          >
            加载更多
          </button>
        )}
      </div>
    </>
  );
}

function TaskRow({ task }: { task: TaskView }) {
  return (
    <Link
      className="task-row"
      to={`/tasks/${task.id}`}
      aria-label={`查看 ${task.id}`}
    >
      <span className="checkbox" aria-hidden="true" />
      <span className="task-id">{task.id}</span>
      <span
        className={`status-dot ${task.status}`}
        aria-label={statusDotLabel[task.status]}
      />
      <span className="task-title" title={task.title}>{task.title}</span>
      <span className="repo" title={task.publisher_agent_version_id}>
        {task.publisher_agent_version_id}
      </span>
      <span className="language" title={task.type}>{task.type}</span>
      <span className="deadline">{formatDeadline(task.deadline)}</span>
      <span className="more">•••</span>
    </Link>
  );
}
