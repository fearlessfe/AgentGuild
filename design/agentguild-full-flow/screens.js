export const SCREEN_IDS = Object.freeze([
  "journey",
  "api-coverage",
  "login",
  "onboarding",
  "git-integration",
  "repository-sync-rule",
  "sync-result",
  "task-center",
  "execution-detail",
  "submission-validation",
  "review-workspace",
  "outcome",
  "components-states",
  "responsive-rules",
  "gallery",
]);

export const screenRegistry = Object.freeze({
  journey: { title: "全流程总览", kind: "board" },
  "api-coverage": { title: "API 能力覆盖", kind: "board" },
  login: { title: "登录", kind: "screen", exportOrder: "01" },
  onboarding: { title: "首次引导", kind: "screen", exportOrder: "02" },
  "git-integration": { title: "Git 接入", kind: "screen", exportOrder: "03" },
  "repository-sync-rule": {
    title: "仓库同步规则",
    kind: "screen",
    exportOrder: "04",
  },
  "sync-result": { title: "同步结果", kind: "screen", exportOrder: "05" },
  "task-center": { title: "任务中心", kind: "screen", exportOrder: "06" },
  "execution-detail": {
    title: "执行详情",
    kind: "screen",
    exportOrder: "07",
  },
  "submission-validation": {
    title: "提交与验证",
    kind: "screen",
    exportOrder: "08",
  },
  "review-workspace": {
    title: "审核工作台",
    kind: "screen",
    exportOrder: "09",
  },
  outcome: { title: "结果闭环", kind: "screen", exportOrder: "10" },
  "components-states": { title: "组件与状态", kind: "board" },
  "responsive-rules": { title: "响应式规则", kind: "board" },
  gallery: { title: "设计画廊", kind: "index" },
});

export const apiStatus = Object.freeze({
  available: { label: "API · available", description: "当前接口可直接支撑" },
  partial: { label: "API · partial", description: "仅能支撑部分信息或前端尚未接入" },
  planned: { label: "API · planned", description: "需要新增后端能力" },
  "auth-fix": { label: "API · auth fix", description: "接口存在，但认证或授权需修正" },
});

export const apiCapabilities = Object.freeze([
  {
    capability: "登录",
    endpoint:
      "GET /oauth/oidc/login · GET /oauth/oidc/callback · POST /oauth/local/login",
    status: "available",
    note: "本地登录仅在服务端启用时显示；local login 需补入 OpenAPI。",
  },
  {
    capability: "首次引导",
    endpoint: "无 onboarding 状态接口",
    status: "planned",
    note: "完成状态必须由服务端持久化，不能用前端本地状态代替。",
  },
  {
    capability: "Git 接入",
    endpoint: "GET · POST · DELETE /v1/github-app",
    status: "partial",
    note: "REST 已有、前端未接入；检测连接仍需新增接口。",
  },
  {
    capability: "仓库目录",
    endpoint: "无安装仓库列表或仓库启用接口",
    status: "planned",
    note: "搜索、选择、默认分支与权限状态均为规划能力。",
  },
  {
    capability: "同步规则",
    endpoint: "无规则 CRUD、预览、启停接口",
    status: "planned",
    note: "保存、预览和启停不标记为当前可用。",
  },
  {
    capability: "同步运行",
    endpoint: "无 sync run、结果列表、冲突处理接口",
    status: "planned",
    note: "重试、冲突治理和审计记录均为规划能力。",
  },
  {
    capability: "任务中心",
    endpoint: "GET /v1/tasks · GET /v1/tasks/{id}",
    status: "available",
    note: "任务事实可用；Issue 来源、链接与同步批次字段需扩展。",
  },
  {
    capability: "执行详情",
    endpoint: "GET /v1/executions/{id}",
    status: "partial",
    note: "lease、状态、阶段与成本可用；完整事件和 revision 列表待补。",
  },
  {
    capability: "提交摘要",
    endpoint: "GET /v1/submissions/{id} · GET /v1/submissions/{id}/diff",
    status: "partial",
    note: "REST 已有；Submission 详情页和客户端仍需补齐。",
  },
  {
    capability: "验证详情",
    endpoint: "Submission 仅返回 validation_job_id",
    status: "partial",
    note: "检查步骤、attempt、lease 与日志需要新增只读查询。",
  },
  {
    capability: "审核读取",
    endpoint: "GET /v1/reviews/{id} · GET /v1/rubrics/active",
    status: "available",
    note: "可直接支撑审核详情与 Rubric。",
  },
  {
    capability: "创建审核",
    endpoint: "POST /v1/submissions/{id}/reviews",
    status: "partial",
    note: "session-only 语义正确；前端尚无显式入口。",
  },
  {
    capability: "审核评论/结论",
    endpoint: "POST /v1/reviews/{id}/comments · POST /v1/reviews/{id}/decision",
    status: "auth-fix",
    note: "当前 bearer-only；人类 Web 审核应支持 session 身份。",
  },
  {
    capability: "结果闭环",
    endpoint: "GET /v1/reputation · Agent 版本/经验/评测接口",
    status: "partial",
    note: "声望和经验可查；outcome 聚合、Issue 回写与审计时间线待补。",
  },
  {
    capability: "主题偏好",
    endpoint: "无服务端接口",
    status: "partial",
    note: "首期可浏览器持久化；跨设备同步时再新增用户偏好接口。",
  },
]);

export const sharedData = Object.freeze({
  workspace: "平台工程工作区",
  repository: "acme/checkout-service",
  taskId: "TASK-2481",
  executionId: "EXE-7D3A",
  submissionId: "SUB-91C2",
  reviewId: "REV-184",
  commit: "8f4c2d1",
  agentVersion: "release-bot · v3.4",
});
