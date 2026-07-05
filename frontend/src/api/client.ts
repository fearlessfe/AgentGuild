export type Envelope<T> = {
  data: T;
  meta: {
    server_time: string;
    resource_version: number;
    poll_after_seconds?: number;
    next_cursor?: string;
  };
};

export type TaskStatus = "draft" | "open" | "claimed" | "in_progress" | "completed" | "cancelled" | "expired";
export type ExecutionStatus =
  | "leased"
  | "running"
  | "submitted"
  | "validating"
  | "reviewing"
  | "revision_requested"
  | "accepted"
  | "rejected"
  | "expired"
  | "cancelled";

export type CostView = {
  observed_cost?: string;
  self_reported_cost?: string;
  coverage: "complete" | "partial" | "unavailable";
  provider: string;
  observed_at?: string;
};

export type ExecutionView = {
  id: string;
  task_id: string;
  tenant_id: string;
  agent_version_id: string;
  status: ExecutionStatus;
  stage?: string;
  progress?: number;
  lease_generation: number;
  lease_soft_expires_at: string;
  lease_hard_expires_at: string;
  last_heartbeat_at?: string;
  claimed_at?: string;
  started_at?: string;
  submitted_at?: string;
  expired_at?: string;
  cost?: CostView;
  audit_summary?: string;
};

export type TaskPage = { items: TaskView[] };

export type TaskView = {
  id: string;
  tenant_id: string;
  publisher_agent_version_id: string;
  type: string;
  title: string;
  problem: string;
  constraints: string[];
  requirements: string[];
  deadline: string;
  status: TaskStatus;
  claimed_by?: string;
  active_execution_id?: string;
  created_at: string;
  updated_at: string;
  state_version: number;
};

type ApiRequestInit = Omit<RequestInit, "body"> & {
  body?: unknown;
};

type DemoAgent = {
  id: string;
  name: string;
  description?: string;
  status: string;
  team?: string;
  owner_email: string;
  scopes: string[];
  repo_scope?: string[];
  budget_cents?: number;
  budget_currency?: string;
  last_seen_at?: string;
  created_at: string;
  updated_at: string;
};

const base = import.meta.env.VITE_API_BASE_URL ?? "/api";

function apiToken(): string | undefined {
  if (import.meta.env.DEV && import.meta.env.VITE_API_TOKEN) {
    return import.meta.env.VITE_API_TOKEN as string;
  }
  if (typeof window === "undefined") {
    return undefined;
  }
  const globalToken = (window as Window & { AG_TOKEN?: string }).AG_TOKEN;
  if (globalToken) {
    return globalToken;
  }
  const match = document.cookie.match(/(?:^|; )ag_token=([^;]+)/);
  return match ? decodeURIComponent(match[1]) : undefined;
}

export async function apiRequest<T>(path: string, init: ApiRequestInit = {}): Promise<Envelope<T>> {
  if (import.meta.env.VITE_DEMO_MODE === "true") {
    return demo(path, init) as Envelope<T>;
  }

  const headers: Record<string, string> = {};
  if (init.headers instanceof Headers) {
    init.headers.forEach((value, key) => {
      headers[key] = value;
    });
  } else if (Array.isArray(init.headers)) {
    init.headers.forEach(([key, value]) => {
      headers[key] = value;
    });
  } else if (init.headers) {
    Object.assign(headers, init.headers);
  }
  const token = apiToken();
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  let body: BodyInit | undefined;
  if (init.body instanceof FormData) {
    body = init.body;
  } else if (init.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(init.body);
  }

  const response = await fetch(base + path, { ...init, headers, body });
  if (response.status === 401) {
    const pathname = window.location.pathname;
    if (!pathname.startsWith("/login") && !pathname.startsWith("/oauth/oidc/login")) {
      window.location.href = "/login";
    }
    throw new Error("未登录");
  }
  if (!response.ok) {
    let message = `API request failed (${response.status})`;
    try {
      const body = await response.json();
      if (body && typeof body === "object" && "error" in body && body.error && typeof body.error === "object" && "message" in body.error && typeof body.error.message === "string") {
        message = body.error.message;
      }
    } catch {
      // ignore parse errors and fall back to status message
    }
    throw new Error(message);
  }
  return response.json() as Promise<Envelope<T>>;
}

export async function listTasks(filters: {
  statuses?: string[];
  type?: string;
  publisherAgentVersionId?: string;
  cursor?: string;
  limit?: number;
} = {}): Promise<Envelope<TaskPage>> {
  const params = new URLSearchParams();
  params.set("limit", String(filters.limit ?? 20));
  filters.statuses?.forEach((status) => params.append("status", status));
  if (filters.type) params.set("type", filters.type);
  if (filters.publisherAgentVersionId) params.set("publisher_agent_version_id", filters.publisherAgentVersionId);
  if (filters.cursor) params.set("cursor", filters.cursor);
  const query = params.toString();
  return apiRequest<TaskPage>(`/v1/tasks${query ? `?${query}` : ""}`);
}

export const getTask = (id: string) => apiRequest<TaskView>(`/v1/tasks/${encodeURIComponent(id)}`);
export const getExecution = (id: string) => apiRequest<ExecutionView>(`/v1/executions/${encodeURIComponent(id)}`);
export const pollInterval = (seconds?: number) => (seconds && seconds > 0 ? seconds * 1000 : false);

const demoTasks: TaskView[] = [
  ["AG-192", "修复批量退款时的余额竞争条件", "open", "billing-service", "TypeScript"],
  ["AG-189", "为 webhook 重试补充幂等保护", "open", "event-gateway", "Go"],
  ["AG-187", "修复分页游标在空结果集下失效", "open", "data-api", "Python"],
  ["AG-186", "优化导出报告的内存占用", "open", "report-service", "TypeScript"],
  ["AG-185", "补充用户注销后数据清理任务", "open", "user-service", "Go"],
  ["AG-188", "修复定时任务重复触发问题", "in_progress", "scheduler", "Go"],
  ["AG-183", "增加支付渠道的限流保护", "in_progress", "payments", "TypeScript"],
  ["AG-181", "重构订单状态机逻辑", "in_progress", "orders", "Go"],
  ["AG-180", "修复优惠券叠加计算错误", "in_progress", "checkout", "Python"],
  ["AG-179", "为文件上传接入病毒扫描", "in_progress", "storage", "Go"],
  ["AG-178", "优化通知发送的吞吐量", "in_progress", "notify", "TypeScript"],
  ["AG-175", "补充日志字段与追踪链路", "in_progress", "platform", "Go"],
  ["AG-174", "修复积分过期计算边界问题", "claimed", "loyalty", "Go"],
  ["AG-171", "增加黑名单用户拦截逻辑", "claimed", "risk", "TypeScript"],
  ["AG-168", "修复退款回调偶发失败", "claimed", "billing", "Go"],
  ["AG-167", "优化数据库索引与查询性能", "claimed", "database", "SQL"],
  ["AG-164", "清理过期 feature flag", "completed", "platform", "Go"],
  ["AG-161", "迁移监控告警规则", "completed", "infra", "YAML"],
].map(([id, title, status, repo, type]) => ({
  id,
  title,
  status: status as TaskStatus,
  publisher_agent_version_id: repo,
  type,
  tenant_id: "billing-platform",
  problem: title,
  constraints: ["保持 API 向后兼容"],
  requirements: ["所有测试通过"],
  deadline: "2026-07-03T10:00:00Z",
  created_at: "2026-07-02T01:15:00Z",
  updated_at: "2026-07-02T02:00:00Z",
  state_version: 2,
  active_execution_id: status === "in_progress" ? `exec-${id}` : undefined,
  claimed_by: status !== "open" ? "Atlas v12" : undefined,
}));

const demoMeta = {
  server_time: "2026-07-02T14:00:00Z",
  resource_version: 12,
  poll_after_seconds: 30,
};

let demoAgentCounter = 4;
let demoAgents: DemoAgent[] = createDemoAgents();

let demoBenchmarkSetCounter = 1;
const demoBenchmarkSets = [
  {
    id: "bs-1",
    tenant_id: "billing-platform",
    version_number: 1,
    name: "默认回归基准",
    description: "包含核心回归任务",
    is_active: true,
    created_by: "owner-1",
    created_at: "2026-07-01T10:00:00Z",
  },
];

const demoEvaluationRuns = [
  {
    id: "run-1",
    tenant_id: "billing-platform",
    agent_version_id: "Atlas v12",
    benchmark_set_id: "bs-1",
    status: "passed",
    environment_digest: "env-abc",
    scoring_rule_version: "v1",
    threshold_results: [{ name: "security", passed: true }],
    summary: { pass_rate: 1.0, avg_latency_ms: 1200, cost_cents: 15, security_passed: true },
    started_at: "2026-07-02T12:00:00Z",
    completed_at: "2026-07-02T12:05:00Z",
  },
  {
    id: "run-2",
    tenant_id: "billing-platform",
    agent_version_id: "Atlas v12",
    benchmark_set_id: "bs-1",
    status: "failed",
    environment_digest: "env-def",
    scoring_rule_version: "v1",
    threshold_results: [
      { name: "security", passed: true },
      { name: "latency", passed: false },
    ],
    summary: { pass_rate: 0.5, avg_latency_ms: 3500, cost_cents: 22, security_passed: true },
    started_at: "2026-07-02T13:00:00Z",
    completed_at: "2026-07-02T13:06:00Z",
  },
];

function createDemoAgents(): DemoAgent[] {
  return [
    {
      id: "agent-active",
      name: "Atlas v12",
      description: "Primary merge queue worker",
      status: "active",
      team: "Platform",
      owner_email: "atlas@example.com",
      scopes: ["tasks:publish", "tasks:claim", "tasks:execute", "tasks:read"],
      repo_scope: ["billing-service", "event-gateway"],
      budget_cents: 8000,
      budget_currency: "USD",
      last_seen_at: "2026-07-02T13:54:00Z",
      created_at: "2026-07-01T09:00:00Z",
      updated_at: "2026-07-02T13:54:00Z",
    },
    {
      id: "agent-suspended",
      name: "Suspended Worker",
      description: "Budget guardrail exceeded",
      status: "suspended",
      team: "Ops",
      owner_email: "ops@example.com",
      scopes: ["tasks:read"],
      repo_scope: ["infra"],
      budget_cents: 3000,
      budget_currency: "USD",
      last_seen_at: "2026-07-01T23:40:00Z",
      created_at: "2026-06-30T08:00:00Z",
      updated_at: "2026-07-01T23:40:00Z",
    },
    {
      id: "agent-revoked",
      name: "Legacy Runner",
      description: "Credential rotation pending",
      status: "revoked",
      team: "Security",
      owner_email: "security@example.com",
      scopes: ["tasks:read", "tasks:execute"],
      repo_scope: ["legacy-monolith"],
      budget_cents: 0,
      budget_currency: "USD",
      created_at: "2026-06-25T08:00:00Z",
      updated_at: "2026-07-01T20:10:00Z",
    },
    {
      id: "agent-pending",
      name: "Review Draft",
      description: "Waiting for activation",
      status: "pending_activation",
      team: "Code Quality",
      owner_email: "review@example.com",
      scopes: ["tasks:read", "tasks:execute"],
      repo_scope: ["frontend", "api"],
      budget_cents: 2000,
      budget_currency: "USD",
      created_at: "2026-07-02T10:00:00Z",
      updated_at: "2026-07-02T10:00:00Z",
    },
  ];
}

function parseDemoBody(body: unknown): Record<string, unknown> {
  if (typeof body !== "string") {
    return {};
  }
  try {
    return JSON.parse(body) as Record<string, unknown>;
  } catch {
    return {};
  }
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function demo(path: string, init: ApiRequestInit = {}): Envelope<unknown> {
  const url = new URL(path, "http://demo.local");
  const method = (init.method ?? "GET").toUpperCase();

  if (url.pathname === "/v1/tasks" && method === "GET") {
    const statuses = url.searchParams.getAll("status");
    const type = url.searchParams.get("type");
    const publisher = url.searchParams.get("publisher_agent_version_id");
    let items = demoTasks;
    if (statuses.length > 0) {
      items = items.filter((task) => statuses.includes(task.status));
    }
    if (type) {
      items = items.filter((task) => task.type === type);
    }
    if (publisher) {
      items = items.filter((task) => task.publisher_agent_version_id.includes(publisher));
    }
    return clone({ data: { items }, meta: demoMeta });
  }

  if (url.pathname.startsWith("/v1/executions/") && method === "GET") {
    return clone({
      data: {
        id: url.pathname.split("/").pop(),
        task_id: "AG-188",
        tenant_id: "billing-platform",
        agent_version_id: "Atlas v12",
        status: "running",
        stage: "运行测试",
        progress: 65,
        lease_generation: 3,
        lease_soft_expires_at: "2026-07-02T14:10:00Z",
        lease_hard_expires_at: "2026-07-02T14:10:30Z",
        cost: { observed_cost: "0.067", self_reported_cost: "0.070", coverage: "partial", provider: "langfuse" },
        audit_summary: "Atlas v12 heartbeat · running",
      },
      meta: demoMeta,
    });
  }

  if (url.pathname.startsWith("/v1/tasks/") && method === "GET") {
    const id = decodeURIComponent(url.pathname.split("/").pop() ?? "AG-192");
    return clone({ data: demoTasks.find((task) => task.id === id) ?? demoTasks[0], meta: demoMeta });
  }

  if (url.pathname.startsWith("/v1/reviews/") && method === "GET") {
    const id = decodeURIComponent(url.pathname.split("/").pop() ?? "rev-1");
    return clone({
      data: {
        id,
        submission_id: "sub-1",
        reviewer_id: "reviewer-1",
        status: "pending",
        final_decision: undefined,
        rubric_scores: [
          { dimension: "correctness", score: 85 },
          { dimension: "readability", score: 70 },
        ],
        summary: "整体实现正确，但缺少边界测试。",
        line_comments: [],
      },
      meta: demoMeta,
    });
  }

  if (url.pathname.startsWith("/v1/submissions/") && url.pathname.endsWith("/diff") && method === "GET") {
    return clone({
      data: [
        {
          path: "src/payment.go",
          old_path: "src/payment.go",
          hunks: [
            {
              old_start: 10,
              old_lines: 3,
              new_start: 10,
              new_lines: 5,
              hunk_hash: "h1",
              lines: [
                { type: "context", text: "func Charge(amount int) error {", old_line: 10, new_line: 10 },
                { type: "remove", text: "    return db.Exec(amount)", old_line: 11 },
                { type: "add", text: "    if amount <= 0 {", new_line: 11 },
                { type: "add", text: "        return fmt.Errorf(\"invalid amount\")", new_line: 12 },
                { type: "context", text: "    }", old_line: 12, new_line: 13 },
              ],
            },
          ],
        },
      ],
      meta: demoMeta,
    });
  }

  if (url.pathname === "/v1/rubrics/active" && method === "GET") {
    return clone({
      data: {
        id: "rubric-1",
        tenant_id: "tenant-1",
        version_number: 1,
        name: "默认代码审核评分表",
        dimensions: [
          { id: "correctness", name: "正确性" },
          { id: "readability", name: "可读性" },
          { id: "testing", name: "测试覆盖" },
        ],
        weights: { correctness: 0.5, readability: 0.3, testing: 0.2 },
        algorithm_version: "v1",
        is_active: true,
        created_at: "2026-07-01T00:00:00Z",
      },
      meta: demoMeta,
    });
  }

  const reviewCommentMatch = url.pathname.match(/^\/v1\/reviews\/([^/:]+)\/comments$/);
  if (reviewCommentMatch && method === "POST") {
    const body = parseDemoBody(init.body);
    return clone({
      data: {
        id: `comment-${Date.now()}`,
        tenant_id: "tenant-1",
        review_id: decodeURIComponent(reviewCommentMatch[1]),
        submission_id: body.submission_id ?? "sub-1",
        file_path: body.file_path ?? "src/payment.go",
        side: body.side ?? "right",
        line_number: typeof body.line_number === "number" ? body.line_number : 1,
        hunk_hash: body.hunk_hash ?? "",
        diff_fingerprint: body.diff_fingerprint ?? "",
        text: body.text ?? "",
        created_at: "2026-07-02T14:00:00Z",
      },
      meta: demoMeta,
    });
  }

  const reviewDecisionMatch = url.pathname.match(/^\/v1\/reviews\/([^/:]+)\/decision$/);
  if (reviewDecisionMatch && method === "POST") {
    const body = parseDemoBody(init.body);
    return clone({
      data: {
        id: decodeURIComponent(reviewDecisionMatch[1]),
        submission_id: "sub-1",
        reviewer_id: "reviewer-1",
        rubric_version_id: "rubric-1",
        status: "submitted",
        final_decision: body.decision ?? "accepted",
        rubric_scores: Array.isArray(body.scores) ? body.scores : [],
        summary: typeof body.summary === "string" ? body.summary : "",
        line_comments: [],
      },
      meta: demoMeta,
    });
  }

  if (url.pathname === "/v1/agents" && method === "GET") {
    return clone({ data: { items: demoAgents }, meta: demoMeta });
  }

  if (url.pathname === "/v1/agents" && method === "POST") {
    const body = parseDemoBody(init.body);
    demoAgentCounter += 1;
    const now = "2026-07-02T14:00:00Z";
    const agent = {
      id: `agent-${demoAgentCounter}`,
      name: String(body.name ?? `Agent ${demoAgentCounter}`),
      description: typeof body.description === "string" && body.description.trim() ? body.description : undefined,
      status: "pending_activation",
      team: typeof body.team === "string" && body.team.trim() ? body.team : undefined,
      owner_email: "owner@example.com",
      scopes: Array.isArray(body.scopes) && body.scopes.length > 0 ? body.scopes : ["tasks:read", "tasks:execute"],
      repo_scope: Array.isArray(body.repo_scope) ? body.repo_scope : [],
      budget_cents: typeof body.budget_cents === "number" ? body.budget_cents : undefined,
      budget_currency: typeof body.budget_currency === "string" ? body.budget_currency : undefined,
      created_at: now,
      updated_at: now,
    };
    demoAgents = [agent, ...demoAgents];
    return clone({
      data: {
        agent,
        activation_token: `agtok_${agent.id}_once`,
        activation_expires_at: "2026-07-09T14:00:00Z",
      },
      meta: demoMeta,
    });
  }

  if (url.pathname.startsWith("/v1/agents/") && method === "GET") {
    const id = decodeURIComponent(url.pathname.split("/").pop() ?? "");
    const agent = demoAgents.find((item) => item.id === id);
    if (!agent) {
      throw new Error(`Demo agent not found: ${id}`);
    }
    return clone({ data: agent, meta: demoMeta });
  }

  const agentActionMatch = url.pathname.match(/^\/v1\/agents\/([^/:]+):(suspend|resume|revoke)$/);
  if (agentActionMatch && method === "POST") {
    const [, rawId, action] = agentActionMatch;
    const id = decodeURIComponent(rawId);
    let updatedAgent: (typeof demoAgents)[number] | undefined;
    demoAgents = demoAgents.map((agent) => {
      if (agent.id !== id) return agent;
      updatedAgent = {
        ...agent,
        status:
          action === "suspend" ? "suspended" : action === "resume" ? "active" : "revoked",
        updated_at: "2026-07-02T14:00:00Z",
      };
      return updatedAgent;
    });
    if (!updatedAgent) {
      throw new Error(`Demo agent not found: ${id}`);
    }
    return clone({ data: updatedAgent, meta: demoMeta });
  }

  if (url.pathname === "/v1/reputation" && method === "GET") {
    const agentVersionId = url.searchParams.get("agent_version_id") ?? "agent-v12";
    const capability = url.searchParams.get("capability") ?? "code-review";
    const taskType = url.searchParams.get("task_type") ?? "typescript";
    return clone({
      data: {
        agent_version_id: agentVersionId,
        capability,
        task_type: taskType,
        total_reviews: 12,
        pass_rate: 0.75,
        rework_rate: 0.17,
        avg_review_cost_cents: 120,
        avg_review_latency_ms: 3450,
        sample_size_hint: "medium",
      },
      meta: demoMeta,
    });
  }

  if (url.pathname === "/v1/benchmarks" && method === "GET") {
    return clone({ data: { items: demoBenchmarkSets }, meta: demoMeta });
  }

  if (url.pathname === "/v1/benchmarks" && method === "POST") {
    const body = parseDemoBody(init.body);
    demoBenchmarkSetCounter += 1;
    const benchmarkSet = {
      id: `bs-${demoBenchmarkSetCounter}`,
      tenant_id: "billing-platform",
      version_number: demoBenchmarkSetCounter,
      name: String(body.name ?? `Benchmark ${demoBenchmarkSetCounter}`),
      description: typeof body.description === "string" ? body.description : "",
      is_active: Boolean(body.is_active),
      created_by: "owner-1",
      created_at: "2026-07-02T14:00:00Z",
    };
    demoBenchmarkSets.push(benchmarkSet);
    return clone({
      data: { benchmark_set_id: benchmarkSet.id, version_number: benchmarkSet.version_number },
      meta: demoMeta,
    });
  }

  if (url.pathname.startsWith("/v1/benchmarks/") && method === "GET") {
    const id = decodeURIComponent(url.pathname.split("/").pop() ?? "");
    const benchmarkSet = demoBenchmarkSets.find((item) => item.id === id);
    if (!benchmarkSet) {
      throw new Error(`Demo benchmark set not found: ${id}`);
    }
    return clone({ data: benchmarkSet, meta: demoMeta });
  }

  if (url.pathname === "/v1/evaluations" && method === "GET") {
    const agentVersionId = url.searchParams.get("agent_version_id");
    let items = demoEvaluationRuns;
    if (agentVersionId) {
      items = items.filter((run) => run.agent_version_id === agentVersionId);
    }
    return clone({ data: { items }, meta: demoMeta });
  }

  if (url.pathname.startsWith("/v1/evaluations/") && method === "GET") {
    const id = decodeURIComponent(url.pathname.split("/").pop() ?? "");
    const run = demoEvaluationRuns.find((item) => item.id === id);
    if (!run) {
      throw new Error(`Demo evaluation run not found: ${id}`);
    }
    return clone({ data: run, meta: demoMeta });
  }

  throw new Error(`Unsupported demo route: ${method} ${url.pathname}`);
}
