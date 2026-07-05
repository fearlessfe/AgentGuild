export type ExperienceStatus = "pending_review" | "approved" | "rejected";
export type SensitivityClass = "public" | "internal" | "restricted" | "forbidden";

export type ExperienceCandidateView = {
  id: string;
  tenant_id: string;
  agent_id: string;
  source_task_id?: string;
  source_submission_id?: string;
  source_review_id?: string;
  evidence_ref: string;
  content_hash: string;
  applicable_capabilities: string[];
  tenant_scope: string;
  sensitivity_class: SensitivityClass;
  status: ExperienceStatus;
  policy_reason?: string;
  reviewed_by?: string;
  reviewed_at?: string;
  created_at: string;
};

export type ExperiencePage = { items: ExperienceCandidateView[] };

export type ExtractExperienceRequest = {
  source_submission_id: string;
  evidence_bytes: string;
  applicable_capabilities?: string[];
};

export type ReviewExperienceRequest = {
  approved: boolean;
  reason?: string;
};
