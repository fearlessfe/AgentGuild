import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ExperienceList } from "./ExperienceList";
import { ExperienceReview } from "./ExperienceReview";
import type { ExperienceCandidateView } from "./experiences.types";

const mockXp: ExperienceCandidateView = {
  id: "xp1",
  tenant_id: "t1",
  agent_id: "a1",
  source_submission_id: "sub1",
  evidence_ref: "sha256:abc",
  content_hash: "hash",
  applicable_capabilities: ["tasks:read"],
  tenant_scope: "t1",
  sensitivity_class: "internal",
  status: "pending_review",
  created_at: "2026-07-04T00:00:00Z",
};

vi.mock("@tanstack/react-query", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-query")>("@tanstack/react-query");
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: vi.fn() }),
    useMutation: () => ({ mutate: vi.fn(), isPending: false, error: null }),
    useQuery: () => ({ isPending: false, isError: false, data: { data: { items: [mockXp] } } }),
  };
});

vi.mock("./experiences.api", () => ({
  listExperiences: vi.fn(),
  reviewExperience: vi.fn(),
  extractExperience: vi.fn(),
}));

describe("ExperienceList", () => {
  it("renders pending candidate", () => {
    render(<ExperienceList agentId="a1" />);
    expect(screen.getByText("经验候选")).toBeInTheDocument();
    expect(screen.getByText("待审核")).toBeInTheDocument();
    expect(screen.getByText("通过")).toBeInTheDocument();
  });
});

describe("ExperienceReview", () => {
  it("renders extract form", () => {
    render(<ExperienceReview agentId="a1" />);
    expect(screen.getByPlaceholderText("Submission ID")).toBeInTheDocument();
    expect(screen.getByText("提取")).toBeInTheDocument();
  });
});
