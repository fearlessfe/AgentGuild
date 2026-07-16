import { apiRequest, createIdempotencyKey } from "../../api/client";
import type { Decision, FileDiff, LineComment, ReviewView, RubricScore, RubricView } from "./reviews.types";

function generateRequestId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export const getReview = (id: string) =>
  apiRequest<ReviewView>(`/v1/reviews/${encodeURIComponent(id)}`);

export const listPendingReviews = () => apiRequest<ReviewView[]>("/v1/reviews?status=pending");

export const getSubmissionDiff = (submissionId: string) =>
  apiRequest<FileDiff[]>(`/v1/submissions/${encodeURIComponent(submissionId)}/diff`);

export const getActiveRubric = () => apiRequest<RubricView>("/v1/rubrics/active");

export const createReview = (submissionId: string, capabilities: string[] = [], idempotencyKey = createIdempotencyKey()) =>
  apiRequest<ReviewView>(`/v1/submissions/${encodeURIComponent(submissionId)}/reviews`, {
    method: "POST",
    headers: { "Idempotency-Key": idempotencyKey },
    body: { capabilities },
  });

export const addComment = (
  reviewId: string,
  payload: Omit<LineComment, "id" | "tenant_id" | "review_id" | "created_at">,
) =>
  apiRequest<LineComment>(`/v1/reviews/${encodeURIComponent(reviewId)}/comments`, {
    method: "POST",
    headers: { "Idempotency-Key": generateRequestId() },
    body: payload,
  });

export const submitDecision = (
  reviewId: string,
  payload: {
    decision: Decision;
    scores: RubricScore[];
    summary: string;
  },
) =>
  apiRequest<ReviewView>(`/v1/reviews/${encodeURIComponent(reviewId)}/decision`, {
    method: "POST",
    headers: { "Idempotency-Key": generateRequestId() },
    body: {
      decision: payload.decision,
      scores: payload.scores,
      summary: payload.summary,
    },
  });
