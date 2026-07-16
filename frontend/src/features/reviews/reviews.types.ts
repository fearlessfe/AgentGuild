export type ReviewStatus = "pending" | "submitted";

export type Decision = "accepted" | "rejected" | "revision_requested";

export type RubricScore = {
  dimension: string;
  score: number;
};

export type RubricDimension = {
  id: string;
  name: string;
};

export type RubricView = {
  id: string;
  tenant_id: string;
  version_number: number;
  name: string;
  dimensions: RubricDimension[];
  weights: Record<string, number>;
  algorithm_version: string;
  is_active: boolean;
  created_at: string;
};

export type Revision = {
  id: string;
  label: string;
  created_at?: string;
};

export type DiffLineType = "context" | "add" | "remove";

export type DiffLine = {
  type: DiffLineType;
  text: string;
  old_line?: number;
  new_line?: number;
};

export type Hunk = {
  old_start: number;
  old_lines: number;
  new_start: number;
  new_lines: number;
  hunk_hash: string;
  lines: DiffLine[];
};

export type FileDiff = {
  path: string;
  old_path?: string;
  hunks: Hunk[];
};

export type LineComment = {
  id: string;
  review_id: string;
  submission_id: string;
  file_path: string;
  side: "left" | "right";
  line_number: number;
  hunk_hash: string;
  diff_fingerprint: string;
  text: string;
  created_at: string;
};

export type ReviewView = {
  id: string;
  submission_id: string;
  reviewer_id: string;
  rubric_version_id: string;
  capability: string;
  status: ReviewStatus;
  final_decision?: Decision;
  rubric_scores: RubricScore[];
  summary?: string;
  line_comments: LineComment[];
};

export type Review = ReviewView;

export type ReviewDiff = {
  files: FileDiff[];
};
