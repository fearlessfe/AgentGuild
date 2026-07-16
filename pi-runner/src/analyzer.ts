import { InMemoryCredentialStore } from "@earendil-works/pi-ai";
import {
  createAgentSession,
  DefaultResourceLoader,
  defineTool,
  ModelRuntime,
  SessionManager,
  SettingsManager,
} from "@earendil-works/pi-coding-agent";
import type { AgentSessionEvent } from "@earendil-works/pi-coding-agent";
import { resolve } from "node:path";
import { Type } from "typebox";
import { ANALYZER_SYSTEM_PROMPT, buildAnalyzerPrompt } from "./prompt.js";
import {
  PROTOCOL_VERSION,
  TaskSpecificationSchema,
  emptyUsage,
  parseTaskSpecification,
  type AnalysisJob,
  type AnalysisResult,
  type AuditEvent,
  type TaskSpecification,
  type Usage,
} from "./protocol.js";

export interface AnalyzerOptions {
  signal?: AbortSignal;
  runtimeApiKey?: string;
}

function safeError(error: unknown): string {
  const message = error instanceof Error ? error.message : String(error);
  return message.replace(/(?:sk|key|token)-[A-Za-z0-9._-]+/gi, "[REDACTED]").slice(0, 2000);
}

function addUsage(target: Usage, usage: unknown): void {
  if (!usage || typeof usage !== "object") return;
  const value = usage as Record<string, unknown>;
  target.input_tokens += typeof value.input === "number" ? value.input : 0;
  target.output_tokens += typeof value.output === "number" ? value.output : 0;
  target.cache_read_tokens += typeof value.cacheRead === "number" ? value.cacheRead : 0;
  target.cache_write_tokens += typeof value.cacheWrite === "number" ? value.cacheWrite : 0;
  const cost = value.cost;
  const total = cost && typeof cost === "object" ? (cost as Record<string, unknown>).total : undefined;
  if (typeof total === "number") {
    target.cost_usd += total;
  }
}

function collectEvent(event: AgentSessionEvent, auditEvents: AuditEvent[], usage: Usage): void {
  switch (event.type) {
    case "tool_execution_start":
      auditEvents.push({ type: "tool_start", tool_name: event.toolName });
      break;
    case "tool_execution_end":
      auditEvents.push({ type: "tool_end", tool_name: event.toolName, is_error: event.isError });
      break;
    case "message_update":
      if (event.assistantMessageEvent.type === "text_delta") {
        auditEvents.push({ type: "public_text", text: event.assistantMessageEvent.delta.slice(0, 8000) });
      }
      break;
    case "turn_end":
      if (event.message.role === "assistant") addUsage(usage, event.message.usage);
      break;
  }
}

export async function runAnalyzer(job: AnalysisJob, options: AnalyzerOptions = {}): Promise<AnalysisResult> {
  const auditEvents: AuditEvent[] = [];
  const usage = emptyUsage();
  let specification: TaskSpecification | undefined;
  let submitCount = 0;
  const workspace = resolve(job.workspace);

  if (options.signal?.aborted) {
    return result(job, "cancelled", usage, auditEvents, undefined, "CANCELLED", "analysis was cancelled");
  }

  const submitTool = defineTool({
    name: "submit_task_specification",
    label: "Submit task specification",
    description: "Submit the single final AgentGuild task specification.",
    promptSnippet: "Submit exactly one final task specification as the last action.",
    promptGuidelines: ["Call this tool exactly once and do not emit a final prose answer."],
    parameters: TaskSpecificationSchema,
    async execute(_toolCallId, params) {
      submitCount += 1;
      if (submitCount !== 1) throw new Error("task specification was submitted more than once");
      specification = parseTaskSpecification(params);
      return {
        content: [{ type: "text" as const, text: "Task specification accepted." }],
        details: { accepted: true },
        terminate: true,
      };
    },
  });

  const settingsManager = SettingsManager.inMemory({
    compaction: { enabled: false },
    retry: { enabled: true, maxRetries: 2 },
  });
  const resourceLoader = new DefaultResourceLoader({
    cwd: workspace,
    agentDir: "/nonexistent/agentguild-pi-agent-dir",
    settingsManager,
    noExtensions: true,
    noSkills: true,
    noPromptTemplates: true,
    noThemes: true,
    noContextFiles: true,
    extensionFactories: [],
    systemPrompt: ANALYZER_SYSTEM_PROMPT,
  });
  await resourceLoader.reload();

  const credentials = new InMemoryCredentialStore();
  const modelRuntime = await ModelRuntime.create({
    credentials,
    modelsPath: null,
    allowModelNetwork: false,
  });
  if (options.runtimeApiKey) {
    await modelRuntime.setRuntimeApiKey(job.agent_version.model_provider, options.runtimeApiKey);
  }
  const model = modelRuntime.getModel(job.agent_version.model_provider, job.agent_version.model_id);
  if (!model) {
    return result(job, "invalid_request", usage, auditEvents, undefined, "MODEL_NOT_FOUND", "configured model not found");
  }

  const { session } = await createAgentSession({
    cwd: workspace,
    agentDir: "/nonexistent/agentguild-pi-agent-dir",
    model,
    thinkingLevel: job.agent_version.thinking_level,
    modelRuntime,
    tools: ["read", "grep", "find", "ls"],
    customTools: [submitTool],
    resourceLoader,
    sessionManager: SessionManager.inMemory(),
    settingsManager,
  });
  const unsubscribe = session.subscribe((event) => collectEvent(event, auditEvents, usage));
  const onAbort = () => void session.abort();
  options.signal?.addEventListener("abort", onAbort, { once: true });

  try {
    await session.prompt(buildAnalyzerPrompt(job));
    if (options.signal?.aborted) {
      return result(job, "cancelled", usage, auditEvents, undefined, "CANCELLED", "analysis was cancelled");
    }
    if (submitCount !== 1 || !specification) {
      return result(
        job,
        "provider_failure",
        usage,
        auditEvents,
        undefined,
        "STRUCTURED_RESULT_MISSING",
        "Pi session ended without one structured task specification",
      );
    }
    return result(job, "succeeded", usage, auditEvents, specification);
  } catch (error) {
    auditEvents.push({ type: "provider_error", text: safeError(error) });
    const cancelled = options.signal?.aborted ?? false;
    return result(
      job,
      cancelled ? "cancelled" : "provider_failure",
      usage,
      auditEvents,
      undefined,
      cancelled ? "CANCELLED" : "PROVIDER_ERROR",
      cancelled ? "analysis was cancelled" : safeError(error),
    );
  } finally {
    options.signal?.removeEventListener("abort", onAbort);
    unsubscribe();
    session.dispose();
  }
}

function result(
  job: AnalysisJob,
  status: AnalysisResult["status"],
  usage: Usage,
  auditEvents: AuditEvent[],
  specification?: TaskSpecification,
  errorCode?: string,
  errorMessage?: string,
): AnalysisResult {
  return {
    protocol_version: PROTOCOL_VERSION,
    job_id: job.job_id,
    agent_version_id: job.agent_version.id,
    status,
    usage,
    audit_events: auditEvents.slice(0, 10000),
    ...(specification ? { specification } : {}),
    ...(errorCode ? { error_code: errorCode } : {}),
    ...(errorMessage ? { error_message: errorMessage } : {}),
  };
}

export const analyzerToolNames = ["read", "grep", "find", "ls", "submit_task_specification"] as const;

export const AnalyzerToolPolicySchema = Type.Object({
  allowed_tools: Type.Tuple(analyzerToolNames.map((name) => Type.Literal(name))),
});
