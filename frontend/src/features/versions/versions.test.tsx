import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { VersionActions } from "./VersionActions";
import { VersionDetail, VersionTree } from "./VersionTree";
import type { VersionView } from "./versions.types";

const mockVersion: VersionView = {
  id: "v1",
  tenant_id: "t1",
  agent_id: "a1",
  version_number: 2,
  parent_version_id: "v0",
  status: "draft",
  runtime: "go",
  model: "gpt-4",
  capabilities: ["tasks:read"],
  config_fingerprint: "fp",
  content_hash: "hash",
  environment_digest: "env",
  prompt_ref: "sha256:prompt",
  skill_refs: [],
  memory_ref: undefined,
  tool_refs: [],
  created_by: "owner",
  created_at: "2026-07-04T00:00:00Z",
};

vi.mock("@tanstack/react-query", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-query")>("@tanstack/react-query");
  const localMock = {
    id: "v1",
    content_hash: "hash",
    runtime: "go",
    model: "gpt-4",
    status: "draft",
    version_number: 2,
    environment_digest: "env",
    capabilities: ["tasks:read"],
    created_by: "owner",
    created_at: "2026-07-04T00:00:00Z",
  };
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: vi.fn() }),
    useMutation: () => ({ mutate: vi.fn(), isPending: false, error: null }),
    useQuery: ({ queryKey }: { queryKey: unknown[] }) => {
      if (Array.isArray(queryKey) && queryKey[0] === "version") {
        return { isPending: false, isError: false, data: { data: localMock } };
      }
      return { isPending: false, isError: false, data: { data: { items: [localMock] } } };
    },
  };
});

vi.mock("./versions.api", () => ({
  listVersions: vi.fn(),
  getVersion: vi.fn(),
  diffVersion: vi.fn(),
  createDraft: vi.fn(),
  promoteVersion: vi.fn(),
  rollbackVersion: vi.fn(),
  startEvaluation: vi.fn(),
}));

describe("VersionTree", () => {
  it("renders version list", () => {
    render(<VersionTree agentId="a1" />);
    expect(screen.getByText("版本谱系")).toBeInTheDocument();
    expect(screen.getByText(/#2 草稿/)).toBeInTheDocument();
  });
});

describe("VersionDetail", () => {
  it("renders version details", () => {
    render(<VersionDetail agentId="a1" versionId="v1" />);
    expect(screen.getByText("版本 #2")).toBeInTheDocument();
    expect(screen.getByText("go")).toBeInTheDocument();
  });
});

describe("VersionActions", () => {
  it("renders action buttons for draft", () => {
    render(<VersionActions agentId="a1" version={mockVersion} />);
    expect(screen.getByText("基于此版本创建 Draft")).toBeInTheDocument();
    expect(screen.getByText("启动评测")).toBeInTheDocument();
  });
});
