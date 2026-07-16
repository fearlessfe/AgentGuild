import { Value } from "typebox/value";
import { Type, type Static, type TSchema } from "typebox";

export const PROTOCOL_VERSION = "agentguild.pi-analysis.v1" as const;
export const PI_RELEASE = "v0.80.8" as const;
export const PI_UPSTREAM_COMMIT = "fae7176cb9f7c4725a40d9d481d8d70b80f18086" as const;

const NonEmptyString = Type.String({ minLength: 1 });
const Sha256 = Type.String({ pattern: "^[a-f0-9]{64}$" });

export const EvidenceRefSchema = Type.Object(
  {
    id: NonEmptyString,
    commit: Type.String({ pattern: "^[a-f0-9]{40}$" }),
    path: NonEmptyString,
    start_line: Type.Integer({ minimum: 1 }),
    end_line: Type.Integer({ minimum: 1 }),
    content_sha256: Sha256,
  },
  { additionalProperties: false },
);

export const AcceptanceCriterionSchema = Type.Object(
  {
    id: NonEmptyString,
    statement: Type.String({ minLength: 1, maxLength: 2000 }),
    critical: Type.Boolean(),
    verifier_kind: Type.Union([
      Type.Literal("command"),
      Type.Literal("static"),
      Type.Literal("behavioral"),
      Type.Literal("manual"),
    ]),
    verifier: Type.String({ minLength: 1, maxLength: 4000 }),
    expected_result: Type.String({ minLength: 1, maxLength: 2000 }),
    timeout_seconds: Type.Integer({ minimum: 1, maximum: 1800 }),
    evidence_ref_ids: Type.Array(NonEmptyString, { minItems: 1, uniqueItems: true }),
  },
  { additionalProperties: false },
);

export const TaskSpecificationSchema = Type.Object(
  {
    title: Type.String({ minLength: 1, maxLength: 300 }),
    diagnosis: Type.String({ minLength: 1, maxLength: 12000 }),
    impact: Type.Array(NonEmptyString, { minItems: 1, maxItems: 50 }),
    proposed_solution: Type.String({ minLength: 1, maxLength: 12000 }),
    implementation_steps: Type.Array(NonEmptyString, { minItems: 1, maxItems: 100 }),
    constraints: Type.Array(NonEmptyString, { maxItems: 50 }),
    non_goals: Type.Array(NonEmptyString, { maxItems: 50 }),
    risks: Type.Array(NonEmptyString, { minItems: 1, maxItems: 50 }),
    uncertainties: Type.Array(NonEmptyString, { maxItems: 50 }),
    clarification_questions: Type.Array(NonEmptyString, { maxItems: 50 }),
    evidence_refs: Type.Array(EvidenceRefSchema, { minItems: 1, maxItems: 200 }),
    acceptance_criteria: Type.Array(AcceptanceCriterionSchema, { minItems: 1, maxItems: 100 }),
  },
  { additionalProperties: false },
);

export const AnalysisJobSchema = Type.Object(
  {
    protocol_version: Type.Literal(PROTOCOL_VERSION),
    job_id: NonEmptyString,
    tenant_id: NonEmptyString,
    repository: NonEmptyString,
    workspace: NonEmptyString,
    base_commit: Type.String({ pattern: "^[a-f0-9]{40}$" }),
    issue: Type.Object(
      {
        number: Type.Integer({ minimum: 1 }),
        revision: NonEmptyString,
        title: Type.String({ minLength: 1, maxLength: 1000 }),
        body: Type.String({ maxLength: 100000 }),
      },
      { additionalProperties: false },
    ),
    agent_version: Type.Object(
      {
        id: NonEmptyString,
        pi_release: Type.Literal(PI_RELEASE),
        pi_commit: Type.Literal(PI_UPSTREAM_COMMIT),
        extension_sha256: Sha256,
        model_provider: NonEmptyString,
        model_id: NonEmptyString,
        thinking_level: Type.Union([
          Type.Literal("off"),
          Type.Literal("minimal"),
          Type.Literal("low"),
          Type.Literal("medium"),
          Type.Literal("high"),
          Type.Literal("xhigh"),
          Type.Literal("max"),
        ]),
      },
      { additionalProperties: false },
    ),
    evidence: Type.Array(EvidenceRefSchema, { maxItems: 500 }),
    experience_version_ids: Type.Array(NonEmptyString, { uniqueItems: true, maxItems: 100 }),
  },
  { additionalProperties: false },
);

export const UsageSchema = Type.Object(
  {
    input_tokens: Type.Integer({ minimum: 0 }),
    output_tokens: Type.Integer({ minimum: 0 }),
    cache_read_tokens: Type.Integer({ minimum: 0 }),
    cache_write_tokens: Type.Integer({ minimum: 0 }),
    cost_usd: Type.Number({ minimum: 0 }),
  },
  { additionalProperties: false },
);

export const AuditEventSchema = Type.Object(
  {
    type: Type.Union([
      Type.Literal("tool_start"),
      Type.Literal("tool_end"),
      Type.Literal("public_text"),
      Type.Literal("provider_error"),
    ]),
    tool_name: Type.Optional(NonEmptyString),
    is_error: Type.Optional(Type.Boolean()),
    text: Type.Optional(Type.String({ maxLength: 8000 })),
  },
  { additionalProperties: false },
);

export const AnalysisResultSchema = Type.Object(
  {
    protocol_version: Type.Literal(PROTOCOL_VERSION),
    job_id: NonEmptyString,
    agent_version_id: NonEmptyString,
    status: Type.Union([
      Type.Literal("succeeded"),
      Type.Literal("provider_failure"),
      Type.Literal("cancelled"),
      Type.Literal("invalid_request"),
    ]),
    specification: Type.Optional(TaskSpecificationSchema),
    usage: UsageSchema,
    audit_events: Type.Array(AuditEventSchema, { maxItems: 10000 }),
    error_code: Type.Optional(NonEmptyString),
    error_message: Type.Optional(Type.String({ minLength: 1, maxLength: 2000 })),
  },
  { additionalProperties: false },
);

export type AnalysisJob = Static<typeof AnalysisJobSchema>;
export type TaskSpecification = Static<typeof TaskSpecificationSchema>;
export type AnalysisResult = Static<typeof AnalysisResultSchema>;
export type Usage = Static<typeof UsageSchema>;
export type AuditEvent = Static<typeof AuditEventSchema>;

function validationErrors(schema: TSchema, input: unknown): string[] {
  return [...Value.Errors(schema, input)].map((error) => {
    const location = "instancePath" in error && typeof error.instancePath === "string" ? error.instancePath : "/";
    return `${location}: ${error.message}`;
  });
}

export function parseAnalysisJob(input: unknown): AnalysisJob {
  if (!Value.Check(AnalysisJobSchema, input)) {
    const errors = validationErrors(AnalysisJobSchema, input);
    throw new Error(`invalid analysis job: ${errors.join("; ")}`);
  }
  return Value.Clone(input);
}

export function parseTaskSpecification(input: unknown): TaskSpecification {
  if (!Value.Check(TaskSpecificationSchema, input)) {
    const errors = validationErrors(TaskSpecificationSchema, input);
    throw new Error(`invalid task specification: ${errors.join("; ")}`);
  }
  return Value.Clone(input);
}

export function emptyUsage(): Usage {
  return {
    input_tokens: 0,
    output_tokens: 0,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    cost_usd: 0,
  };
}
