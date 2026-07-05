import { apiRequest } from "../../api/client";
import type { FileDiff, ReviewView } from "./reviews.types";

export const getReview = (id: string) =>
  apiRequest<ReviewView>(`/v1/reviews/${encodeURIComponent(id)}`);

export const getSubmissionDiff = (submissionId: string) =>
  apiRequest<FileDiff[]>(`/v1/submissions/${encodeURIComponent(submissionId)}/diff`);
