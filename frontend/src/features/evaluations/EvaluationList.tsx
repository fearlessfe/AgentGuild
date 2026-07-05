import { useQuery } from "@tanstack/react-query";
import { getEvaluationRun, listEvaluationRuns } from "./evaluations.api";

export function BenchmarkSetForm({ onCreated }: { onCreated?: () => void }) {
  return (
    <section className="benchmark-set-form" aria-label="基准集">
      <h4>基准集</h4>
      <p>请在控制台通过 API 创建基准集（MVP 表单占位）。</p>
      <button type="button" onClick={onCreated}>刷新列表</button>
    </section>
  );
}

export function EvaluationList({ agentVersionId }: { agentVersionId?: string }) {
  const query = useQuery({
    queryKey: ["evaluation-runs", agentVersionId],
    queryFn: () => listEvaluationRuns(agentVersionId),
  });

  if (query.isPending) return <div className="loading">加载评测…</div>;
  if (query.isError) return <div className="error">无法读取评测：{query.error.message}</div>;

  const items = query.data.data.items;

  return (
    <section className="evaluation-list" aria-label="评测运行">
      <h3>评测运行</h3>
      <ul>
        {items.map((run) => (
          <li key={run.id} className={`evaluation-item ${run.status}`}>
            <span>{run.id.slice(0, 8)}…</span>
            <span>{run.status}</span>
            <span>通过率 {run.summary.pass_rate.toFixed(2)}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function EvaluationDetail({ runId }: { runId: string }) {
  const query = useQuery({ queryKey: ["evaluation-run", runId], queryFn: () => getEvaluationRun(runId) });

  if (query.isPending) return <div className="loading">加载评测详情…</div>;
  if (query.isError) return <div className="error">无法读取评测详情：{query.error.message}</div>;

  const run = query.data.data;

  return (
    <section className="evaluation-detail" aria-label="评测详情">
      <h3>评测 {run.id.slice(0, 8)}…</h3>
      <p>状态：{run.status}</p>
      <p>通过率：{run.summary.pass_rate.toFixed(2)}</p>
      <ul>
        {run.threshold_results.map((tr, idx) => (
          <li key={idx}>
            {tr.name}: {tr.passed ? "通过" : "失败"}
          </li>
        ))}
      </ul>
    </section>
  );
}
