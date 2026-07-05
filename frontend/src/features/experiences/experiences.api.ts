import { apiRequest } from "../../api/client";
import type { ExperienceCandidateView, ExperiencePage, ExtractExperienceRequest, ReviewExperienceRequest } from "./experiences.types";

export const listExperiences = (agentId: string) =>
  apiRequest<ExperiencePage>(`/v1/agents/${encodeURIComponent(agentId)}/experiences`);

export const extractExperience = (agentId: string, payload: ExtractExperienceRequest) =>
  apiRequest<ExperienceCandidateView>(`/v1/agents/${encodeURIComponent(agentId)}/experiences`, {
    method: "POST",
    body: payload,
  });

export const reviewExperience = (agentId: string, experienceId: string, payload: ReviewExperienceRequest) =>
  apiRequest<ExperienceCandidateView>(
    `/v1/agents/${encodeURIComponent(agentId)}/experiences/${encodeURIComponent(experienceId)}${payload.approved ? "/approve" : "/reject"}`,
    { method: "POST", body: { reason: payload.reason } },
  );
