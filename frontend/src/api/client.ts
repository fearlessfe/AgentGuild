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

const base = import.meta.env.VITE_API_BASE_URL ?? "/api";

async function request<T>(path: string): Promise<Envelope<T>> {
  if (import.meta.env.VITE_DEMO_MODE === "true") return demo(path) as Envelope<T>;
  const response = await fetch(base + path);
  if (!response.ok) throw new Error(`API request failed (${response.status})`);
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
  return request<TaskPage>(`/v1/tasks${query ? `?${query}` : ""}`);
}

export const getTask = (id: string) => request<TaskView>(`/v1/tasks/${encodeURIComponent(id)}`);
export const getExecution = (id: string) => request<ExecutionView>(`/v1/executions/${encodeURIComponent(id)}`);
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

function demo(path: string): Envelope<unknown> {
  const meta = { server_time: "2026-07-02T14:00:00Z", resource_version: 12, poll_after_seconds: 30 };
  if (path.startsWith("/v1/tasks?")) return { data: { items: demoTasks }, meta };
  if (path.startsWith("/v1/executions/")) {
    return {
      data: {
        id: path.split("/").pop(),
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
      meta,
    };
  }
  const id = decodeURIComponent(path.split("/").pop() ?? "AG-192");
  return { data: demoTasks.find((task) => task.id === id) ?? demoTasks[0], meta };
}
