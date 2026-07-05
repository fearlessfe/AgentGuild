import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { createDraft, promoteVersion, rollbackVersion, startEvaluation } from "./versions.api";
import type { VersionView } from "./versions.types";

export function VersionActions({ agentId, version }: { agentId: string; version: VersionView }) {
  const queryClient = useQueryClient();
  const [benchmarkSetId, setBenchmarkSetId] = useState("");
  const [envDigest, setEnvDigest] = useState("");

  const draftMutation = useMutation({
    mutationFn: () =>
      createDraft(agentId, {
        runtime: version.runtime,
        model: version.model,
        capabilities: version.capabilities,
        environment_digest: version.environment_digest,
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["versions", agentId] }),
  });

  const evalMutation = useMutation({
    mutationFn: () => startEvaluation(agentId, version.id, benchmarkSetId, envDigest || version.environment_digest),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["versions", agentId] }),
  });

  const promoteMutation = useMutation({
    mutationFn: () => promoteVersion(agentId, version.id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["versions", agentId] }),
  });

  const rollbackMutation = useMutation({
    mutationFn: () => rollbackVersion(agentId, version.id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["versions", agentId] }),
  });

  return (
    <section className="version-actions" aria-label="版本操作">
      <h4>操作</h4>
      <button type="button" onClick={() => draftMutation.mutate()} disabled={draftMutation.isPending}>
        基于此版本创建 Draft
      </button>
      {version.status === "draft" ? (
        <div className="eval-form">
          <input
            type="text"
            placeholder="Benchmark Set ID"
            value={benchmarkSetId}
            onChange={(e) => setBenchmarkSetId(e.target.value)}
          />
          <input
            type="text"
            placeholder="Environment Digest（可选）"
            value={envDigest}
            onChange={(e) => setEnvDigest(e.target.value)}
          />
          <button type="button" onClick={() => evalMutation.mutate()} disabled={evalMutation.isPending || !benchmarkSetId}>
            启动评测
          </button>
        </div>
      ) : null}
      {version.status === "eligible" ? (
        <button type="button" onClick={() => promoteMutation.mutate()} disabled={promoteMutation.isPending}>
          晋级为 Active
        </button>
      ) : null}
      {version.status === "active" || version.status === "eligible" || version.status === "retired" ? (
        <button type="button" onClick={() => rollbackMutation.mutate()} disabled={rollbackMutation.isPending}>
          回滚到此版本
        </button>
      ) : null}
      {draftMutation.error ? <p className="error">{draftMutation.error.message}</p> : null}
      {evalMutation.error ? <p className="error">{evalMutation.error.message}</p> : null}
      {promoteMutation.error ? <p className="error">{promoteMutation.error.message}</p> : null}
      {rollbackMutation.error ? <p className="error">{rollbackMutation.error.message}</p> : null}
    </section>
  );
}
