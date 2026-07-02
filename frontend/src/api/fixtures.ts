import type { Envelope, ExecutionView, TaskPage, TaskView } from "./client";

export const defaultMeta = {
  server_time: "2026-07-02T02:00:00Z",
  resource_version: 1,
  poll_after_seconds: 30,
};

export function envelope<T>(data: T, meta?: Partial<Envelope<T>["meta"]>): Envelope<T> {
  return { data, meta: { ...defaultMeta, ...meta } };
}

export function taskViewFixture(overrides?: Partial<TaskView>): TaskView {
  return {
    id: "task-1",
    tenant_id: "tenant-1",
    publisher_agent_version_id: "billing-agent",
    type: "code",
    title: "修复并发退款余额竞争",
    problem: "避免批量退款时余额被重复扣减",
    constraints: ["保持 API 向后兼容"],
    requirements: ["并发测试通过"],
    deadline: "2026-07-03T10:00:00Z",
    status: "open",
    created_at: "2026-07-02T01:00:00Z",
    updated_at: "2026-07-02T01:00:00Z",
    state_version: 1,
    ...overrides,
  };
}

export function executionViewFixture(overrides?: Partial<ExecutionView>): ExecutionView {
  return {
    id: "exec-1",
    task_id: "task-1",
    tenant_id: "tenant-1",
    agent_version_id: "Atlas v12",
    status: "running",
    stage: "运行测试",
    progress: 65,
    lease_generation: 3,
    lease_soft_expires_at: "2026-07-02T02:10:00Z",
    lease_hard_expires_at: "2026-07-02T02:10:30Z",
    last_heartbeat_at: "2026-07-02T02:00:00Z",
    claimed_at: "2026-07-02T01:05:00Z",
    started_at: "2026-07-02T01:06:00Z",
    cost: {
      observed_cost: "0.067",
      self_reported_cost: "0.070",
      coverage: "partial",
      provider: "langfuse",
      observed_at: "2026-07-02T02:00:00Z",
    },
    audit_summary: "agent heartbeat · running",
    ...overrides,
  };
}

export function taskPageFixture(items: TaskView[]): TaskPage {
  return { items };
}
