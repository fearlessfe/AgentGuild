import { useState } from "react";
import { extractExperience } from "./experiences.api";

export function ExperienceReview({ agentId, onSubmitted }: { agentId: string; onSubmitted?: () => void }) {
  const [submissionId, setSubmissionId] = useState("");
  const [evidence, setEvidence] = useState("");
  const [caps, setCaps] = useState("");
  const [status, setStatus] = useState<"idle" | "loading" | "done" | "error">("idle");
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    setStatus("loading");
    setError(null);
    try {
      await extractExperience(agentId, {
        source_submission_id: submissionId,
        evidence_bytes: btoa(evidence),
        applicable_capabilities: caps.split(",").map((s) => s.trim()).filter(Boolean),
      });
      setStatus("done");
      onSubmitted?.();
    } catch (e) {
      setStatus("error");
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  return (
    <section className="experience-review" aria-label="提取经验">
      <h4>从已验收任务提取经验</h4>
      <input
        type="text"
        placeholder="Submission ID"
        value={submissionId}
        onChange={(e) => setSubmissionId(e.target.value)}
      />
      <textarea
        placeholder="证据内容（将被 base64 编码并生成内容哈希）"
        value={evidence}
        onChange={(e) => setEvidence(e.target.value)}
      />
      <input
        type="text"
        placeholder="适用 capabilities，逗号分隔"
        value={caps}
        onChange={(e) => setCaps(e.target.value)}
      />
      <button type="button" onClick={submit} disabled={status === "loading" || !submissionId}>
        提取
      </button>
      {status === "done" ? <p className="success">提取成功</p> : null}
      {error ? <p className="error">{error}</p> : null}
    </section>
  );
}
