import { describe, expect, it } from "vitest";
import {
  PI_RELEASE,
  PI_UPSTREAM_COMMIT,
  PROTOCOL_VERSION,
  parseAnalysisJob,
  parseTaskSpecification,
} from "./protocol.js";

const commit = "a".repeat(40);
const hash = "b".repeat(64);

function job() {
  return {
    protocol_version: PROTOCOL_VERSION,
    job_id: "job-1",
    tenant_id: "tenant-1",
    repository: "earendil-works/pi",
    workspace: "/workspace/repository",
    base_commit: commit,
    issue: { number: 1, revision: "revision-1", title: "Bug", body: "Body" },
    agent_version: {
      id: "agent-version-1",
      pi_release: PI_RELEASE,
      pi_commit: PI_UPSTREAM_COMMIT,
      extension_sha256: hash,
      model_provider: "anthropic",
      model_id: "claude-sonnet-4-20250514",
      thinking_level: "medium",
    },
    evidence: [],
    experience_version_ids: [],
  };
}

describe("analysis protocol", () => {
  it("binds every job to an immutable Agent Version and pinned Pi runtime", () => {
    expect(parseAnalysisJob(job())).toEqual(job());
  });

  it("rejects floating or mismatched Pi versions", () => {
    const input = job();
    input.agent_version.pi_release = "latest" as typeof PI_RELEASE;
    expect(() => parseAnalysisJob(input)).toThrow(/pi_release/);
  });

  it("rejects unknown verifier kinds and vague incomplete criteria", () => {
    expect(() =>
      parseTaskSpecification({
        title: "Fix bug",
        diagnosis: "Diagnosis",
        impact: ["API"],
        proposed_solution: "Solution",
        implementation_steps: ["Implement"],
        constraints: [],
        non_goals: [],
        risks: ["Regression"],
        uncertainties: [],
        clarification_questions: [],
        evidence_refs: [
          { id: "e1", commit, path: "src/a.ts", start_line: 1, end_line: 1, content_sha256: hash },
        ],
        acceptance_criteria: [
          {
            id: "ac1",
            statement: "Looks good",
            critical: true,
            verifier_kind: "opinion",
            verifier: "review",
            expected_result: "good",
            timeout_seconds: 60,
            evidence_ref_ids: ["e1"],
          },
        ],
      }),
    ).toThrow(/verifier_kind/);
  });
});
