import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { getExecution, getTask } from "../../api/client";
import { ButtonLink, Card, PageHeader, StatusChip } from "../../ui";

export function OutcomeScreen() {
  const [params] = useSearchParams();
  const taskId = params.get("task_id") ?? "";
  const task = useQuery({ queryKey: ["task", taskId], queryFn: () => getTask(taskId), enabled: !!taskId });
  const executionId = task.data?.data.active_execution_id ?? "";
  const execution = useQuery({
    queryKey: ["execution", executionId],
    queryFn: () => getExecution(executionId),
    enabled: !!executionId,
  });

  if (!taskId) {
    return (
      <div className="stack">
        <PageHeader title="结果闭环" sub="查看任务与执行的最终治理状态" />
        <Card title="选择任务">
          <p className="text-sm muted">从任务详情进入结果页，平台将展示真实 Task 与 Execution 状态。</p>
          <ButtonLink to="/tasks">查看任务</ButtonLink>
        </Card>
      </div>
    );
  }

  if (task.isLoading || execution.isLoading) return <Card title="结果闭环">加载中...</Card>;
  if (task.isError || execution.isError || !task.data) return <Card title="结果闭环">结果加载失败。</Card>;

  const taskView = task.data.data;
  const executionView = execution.data?.data;
  const accepted = taskView.status === "completed" && executionView?.status === "accepted";
  return (
    <div className="stack">
      <PageHeader title="结果闭环" sub={taskView.id} actions={<ButtonLink to={`/tasks/${encodeURIComponent(taskView.id)}`}>返回任务</ButtonLink>} />
      <Card
        title={accepted ? "已接受" : "处理中"}
        head={<StatusChip tone={accepted ? "success" : "warning"}>{taskView.status}</StatusChip>}
      >
        <div className="fact-grid">
          <div className="fact"><span className="ctx-label">Task</span><b>{taskView.status}</b></div>
          <div className="fact"><span className="ctx-label">Execution</span><b>{executionView?.status ?? "无"}</b></div>
          <div className="fact"><span className="ctx-label">Agent Version</span><span>{executionView?.agent_version_id ?? "-"}</span></div>
          <div className="fact"><span className="ctx-label">状态版本</span><span>v{taskView.state_version}</span></div>
        </div>
      </Card>
    </div>
  );
}
