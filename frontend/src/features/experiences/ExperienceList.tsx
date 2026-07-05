import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { listExperiences, reviewExperience } from "./experiences.api";
import type { ExperienceCandidateView } from "./experiences.types";

function formatStatus(status: string) {
  const map: Record<string, string> = {
    pending_review: "待审核",
    approved: "已通过",
    rejected: "已拒绝",
  };
  return map[status] ?? status;
}

export function ExperienceList({ agentId }: { agentId: string }) {
  const query = useQuery({ queryKey: ["experiences", agentId], queryFn: () => listExperiences(agentId) });

  if (query.isPending) return <div className="loading">加载经验候选…</div>;
  if (query.isError) return <div className="error">无法读取经验候选：{query.error.message}</div>;

  const items = query.data.data.items;

  return (
    <section className="experience-list" aria-label="经验候选">
      <h3>经验候选</h3>
      <ul>
        {items.map((xp) => (
          <li key={xp.id} className={`experience-item ${xp.status}`}>
            <ExperienceRow agentId={agentId} xp={xp} />
          </li>
        ))}
      </ul>
    </section>
  );
}

function ExperienceRow({ agentId, xp }: { agentId: string; xp: ExperienceCandidateView }) {
  const queryClient = useQueryClient();
  const reviewMutation = useMutation({
    mutationFn: (approved: boolean) => reviewExperience(agentId, xp.id, { approved }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["experiences", agentId] }),
  });

  return (
    <div className="experience-row">
      <span>{xp.evidence_ref.slice(0, 24)}…</span>
      <span>{formatStatus(xp.status)}</span>
      <span>敏感级：{xp.sensitivity_class}</span>
      {xp.status === "pending_review" ? (
        <>
          <button type="button" onClick={() => reviewMutation.mutate(true)} disabled={reviewMutation.isPending}>通过</button>
          <button type="button" onClick={() => reviewMutation.mutate(false)} disabled={reviewMutation.isPending}>拒绝</button>
        </>
      ) : null}
      {reviewMutation.error ? <span className="error">{reviewMutation.error.message}</span> : null}
    </div>
  );
}
