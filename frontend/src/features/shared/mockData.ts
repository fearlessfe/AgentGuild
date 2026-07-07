/* Shared placeholder facts mirrored from the approved full-flow visual spec.
   These back the static flow screens (onboarding, git integration, sync rule,
   sync result, submission validation, outcome) until their APIs land. */

export const sharedData = {
  workspace: "Billing Platform",
  repository: "billing-service",
  branch: "agentguild/ag-192",
  baseCommit: "a18d220",
  commit: "a1b2c3d",
  taskId: "AG-192",
  agentVersion: "Atlas v12",
  reviewScore: "89/100",
  reputationDelta: "+12",
} as const;
