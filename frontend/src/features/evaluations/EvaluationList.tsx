import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { createBenchmarkSet, getEvaluationRun, listEvaluationRuns } from "./evaluations.api";
import type { EvaluationRunView } from "./evaluations.types";

function parseTaskRefs(value: string): string[] {
  return value
    .split(/[\s,]+/)
    .map((ref) => ref.trim())
    .filter((ref) => ref.length > 0);
}

export function BenchmarkSetForm({ onCreated }: { onCreated?: () => void }) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [tasks, setTasks] = useState("");
  const [isActive, setIsActive] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: (payload: Parameters<typeof createBenchmarkSet>[0]) => createBenchmarkSet(payload),
    onSuccess: () => {
      setName("");
      setDescription("");
      setTasks("");
      setIsActive(true);
      setError(null);
      onCreated?.();
    },
    onError: (err: Error) => setError(err.message),
  });

  const parsedTasks = parseTaskRefs(tasks);
  const canSubmit = name.trim().length > 0 && parsedTasks.length > 0;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    mutation.mutate({
      name: name.trim(),
      description: description.trim() || undefined,
      task_refs: parsedTasks,
      is_active: isActive,
    });
  };

  return (
    <section className="benchmark-set-form" aria-label="基准集">
      <h4>基准集</h4>
      <form onSubmit={handleSubmit} className="agent-form">
        <label>
          名称
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="例如：回归基准"
            required
          />
        </label>
        <label className="field-span-2">
          描述
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="可选：说明该基准集的用途"
            rows={3}
          />
        </label>
        <label className="field-span-2">
          任务引用
          <textarea
            value={tasks}
            onChange={(e) => setTasks(e.target.value)}
            placeholder="输入任务引用，用逗号、换行或空格分隔"
            rows={4}
            required
          />
        </label>
        <label className="field-span-2">
          <input
            type="checkbox"
            checked={isActive}
            onChange={(e) => setIsActive(e.target.checked)}
          />
          设为默认基准集
        </label>
        {error ? <p className="error">{error}</p> : null}
        <div className="agent-form-actions field-span-2">
          <button type="submit" className="primary-action" disabled={!canSubmit || mutation.isPending}>
            {mutation.isPending ? "创建中…" : "创建基准集"}
          </button>
        </div>
      </form>
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

  const items = query.data.items;

  return (
    <section className="evaluation-list" aria-label="评测运行">
      <h3>评测运行</h3>
      <ul>
        {items.map((run) => (
          <EvaluationListItem key={run.id} run={run} />
        ))}
      </ul>
    </section>
  );
}

function EvaluationListItem({ run }: { run: EvaluationRunView }) {
  const passRate = run.summary?.pass_rate ?? 0;
  return (
    <li className={`evaluation-item ${run.status}`}>
      <span>{run.id.slice(0, 8)}…</span>
      <span>{run.status}</span>
      <span>通过率 {passRate.toFixed(2)}</span>
    </li>
  );
}

export function EvaluationDetail({ runId }: { runId: string }) {
  const query = useQuery({ queryKey: ["evaluation-run", runId], queryFn: () => getEvaluationRun(runId) });

  if (query.isPending) return <div className="loading">加载评测详情…</div>;
  if (query.isError) return <div className="error">无法读取评测详情：{query.error.message}</div>;

  const run = query.data;
  const passRate = run.summary?.pass_rate ?? 0;
  const thresholds = run.threshold_results ?? [];

  return (
    <section className="evaluation-detail" aria-label="评测详情">
      <h3>评测 {run.id.slice(0, 8)}…</h3>
      <p>状态：{run.status}</p>
      <p>通过率：{passRate.toFixed(2)}</p>
      {thresholds.length > 0 ? (
        <ul>
          {thresholds.map((tr, idx) => (
            <li key={idx}>
              {tr.name}: {tr.passed ? "通过" : "失败"}
            </li>
          ))}
        </ul>
      ) : (
        <p>门槛结果聚合中…</p>
      )}
    </section>
  );
}
