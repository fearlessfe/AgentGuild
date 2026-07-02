import { useInfiniteQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { listTasks, pollInterval, type TaskStatus, type TaskView } from "../../api/client";

const groups: { status: TaskStatus; label: string }[] = [
  { status: "open", label: "待领取" }, { status: "in_progress", label: "进行中" }, { status: "claimed", label: "待审核" }, { status: "completed", label: "已完成" },
];
const statusLabel: Record<string, string> = { open: "待领取", in_progress: "运行中", claimed: "待审核", completed: "完成", cancelled: "取消", expired: "过期" };

export function TaskList() {
  const query = useInfiniteQuery({
    queryKey: ["tasks"], initialPageParam: "",
    queryFn: ({ pageParam }) => listTasks({ cursor: pageParam || undefined }),
    getNextPageParam: (last) => last.meta.next_cursor || undefined,
    refetchInterval: (state) => pollInterval(state.state.data?.pages.at(-1)?.meta.poll_after_seconds),
  });
  if (query.isPending) return <div className="loading">正在同步任务…</div>;
  if (query.isError) return <div className="error">无法读取任务：{query.error.message}</div>;
  const tasks = query.data.pages.flatMap((page) => page.data.items);
  return <div className="task-table" aria-label="任务列表">
    {groups.map((group) => {
      const items = tasks.filter((task) => task.status === group.status);
      return <section className={`task-group group-${group.status}`} key={group.status}>
        <header className="group-header"><span>⌄</span><strong>{group.label} · {items.length}</strong><span className="group-meta">{group.status === "in_progress" ? "Agent　　进度　　　　耗时　　 当前活动　　 消耗 (USD)" : "仓库　　　　　　语言　　　 截止时间"}</span></header>
        {items.map((task, index) => <TaskRow task={task} index={index} key={task.id} />)}
      </section>;
    })}
    {query.hasNextPage && <button className="load-more" onClick={() => query.fetchNextPage()} disabled={query.isFetchingNextPage}>加载更多</button>}
  </div>;
}

function TaskRow({ task, index }: { task: TaskView; index: number }) {
  const progress = task.status === "in_progress" ? [65, 30, 80, 55, 20, 0, 90][index % 7] : undefined;
  return <Link className="task-row" to={`/tasks/${task.id}`} aria-label={`查看 ${task.id}`}>
    <span className="checkbox" aria-hidden="true" />
    <span className="task-id">{task.id}</span><span className={`status-dot ${task.status}`} aria-label={statusLabel[task.status]} />
    <span className="task-title">{task.title}</span>
    {progress !== undefined ? <><span className="agent">▣　{task.claimed_by ?? "Atlas v12"}</span><span className="progress"><i style={{ width: `${progress}%` }} /></span><span className="percent">{progress ? `${progress}%` : "等待响应"}</span><span className="elapsed">{progress ? `${16 + index * 9}m` : "—"}</span><span className="stage">{progress ? ["运行测试", "修改代码", "静态分析"][index % 3] : "等待 Agent"}</span><span className="cost">${progress ? (progress * .00103).toFixed(3) : "0.000"}</span></> : <><span className="repo">{task.publisher_agent_version_id}</span><span className="language">{task.type}</span><span className="deadline">今天 18:00</span></>}
    <span className="more">•••</span>
  </Link>;
}
