import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes, createMemoryRouter, RouterProvider } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { envelope } from "../../api/fixtures";
import { ReviewWorkspace } from "../../app/AppShell";
import { ReviewPage } from "./ReviewPage";
import type { FileDiff, ReviewView } from "./reviews.types";

function renderWithProviders(
  ui: ReactNode,
  { initialEntries = ["/reviews/rev-1"] }: { initialEntries?: string[] } = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return {
    client,
    ...render(
      <MemoryRouter initialEntries={initialEntries}>
        <QueryClientProvider client={client}>
          <Routes>
            <Route path="/reviews/:reviewId" element={ui} />
          </Routes>
        </QueryClientProvider>
      </MemoryRouter>
    ),
  };
}

const reviewFixture: ReviewView = {
  id: "rev-1",
  submission_id: "sub-1",
  reviewer_id: "reviewer-1",
  status: "pending",
  final_decision: undefined,
  rubric_scores: [
    { dimension: "correctness", score: 85 },
    { dimension: "readability", score: 70 },
  ],
  summary: "整体实现正确，但缺少边界测试。",
  line_comments: [],
};

const diffFixture: FileDiff[] = [
  {
    path: "src/payment.go",
    old_path: "src/payment.go",
    hunks: [
      {
        old_start: 10,
        old_lines: 3,
        new_start: 10,
        new_lines: 5,
        hunk_hash: "h1",
        lines: [
          { type: "context", text: "func Charge(amount int) error {", old_line: 10, new_line: 10 },
          { type: "remove", text: "    return db.Exec(amount)", old_line: 11 },
          { type: "add", text: "    if amount <= 0 {", new_line: 11 },
          { type: "add", text: "        return fmt.Errorf(\"invalid amount\")", new_line: 12 },
          { type: "context", text: "    }", old_line: 12, new_line: 13 },
        ],
      },
    ],
  },
];

afterEach(() => {
  vi.restoreAllMocks();
});

describe("ReviewPage", () => {
  it("renders review status, summary, rubric scores and diff", async () => {
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify(envelope(reviewFixture)), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(envelope(diffFixture)), { status: 200 }));

    renderWithProviders(<ReviewPage />);

    expect(await screen.findByText("审核 rev-1")).toBeVisible();
    expect(screen.getByText("状态：")).toBeVisible();
    expect(screen.getByText("待审核")).toBeVisible();
    expect(screen.getByText("整体实现正确，但缺少边界测试。")).toBeVisible();
    expect(screen.getByText(/correctness/)).toBeVisible();
    expect(screen.getByText("85")).toBeVisible();

    expect(await screen.findByLabelText("文件树")).toBeVisible();
    expect(screen.getByLabelText("查看 src/payment.go")).toBeVisible();
    expect((await screen.findAllByText("func Charge(amount int) error {")).length).toBeGreaterThan(0);
  });

  it("switches between split and unified diff modes", async () => {
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify(envelope(reviewFixture)), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(envelope(diffFixture)), { status: 200 }));

    renderWithProviders(<ReviewPage />);

    await screen.findAllByText("func Charge(amount int) error {");

    const unifiedButton = screen.getByRole("button", { name: /unified/i });
    expect(unifiedButton).toHaveAttribute("aria-pressed", "false");

    await userEvent.click(unifiedButton);

    await waitFor(() => expect(unifiedButton).toHaveAttribute("aria-pressed", "true"));
    expect(screen.getByRole("button", { name: /split/i })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByText("func Charge(amount int) error {")).toBeVisible();
  });

  it("does not get stuck loading when the review request fails", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValueOnce(new Error("Network error"));

    renderWithProviders(<ReviewPage />);

    expect(await screen.findByText(/无法加载审核/)).toBeVisible();
    expect(screen.queryByText("正在加载审核详情…")).not.toBeInTheDocument();
  });

  it("does not get stuck loading when the review has no submission_id", async () => {
    const reviewWithoutSubmission = { ...reviewFixture, submission_id: "" };
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify(envelope(reviewWithoutSubmission)), { status: 200 }),
    );

    renderWithProviders(<ReviewPage />);

    await waitFor(() => expect(screen.queryByText("正在加载审核详情…")).not.toBeInTheDocument());
    expect(screen.getByText("审核 rev-1")).toBeVisible();
  });

  it("keeps locally added comments after a review refetch", async () => {
    const reviewWithServerComment = {
      ...reviewFixture,
      line_comments: [
        {
          id: "server-1",
          review_id: reviewFixture.id,
          submission_id: reviewFixture.submission_id,
          file_path: diffFixture[0].path,
          side: "right" as const,
          line_number: 11,
          hunk_hash: "",
          diff_fingerprint: "",
          text: "服务端评论",
          created_at: "2026-07-02T14:00:00Z",
        },
      ],
    };

    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify(envelope(reviewWithServerComment)), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(envelope(diffFixture)), { status: 200 }));

    const { client } = renderWithProviders(<ReviewPage />);

    await screen.findAllByText("func Charge(amount int) error {");
    expect(screen.getByText("服务端评论")).toBeVisible();

    await userEvent.click(screen.getByLabelText("在右侧第 12 行添加评论"));
    await userEvent.type(screen.getByPlaceholderText("输入评论…"), "缺少 nil 检查");
    await userEvent.click(screen.getByRole("button", { name: "添加评论" }));

    expect(await screen.findByText("缺少 nil 检查")).toBeVisible();

    // Simulate a background refetch returning empty comments. Before the fix,
    // this would have overwritten local comments and removed them.
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify(envelope({ ...reviewFixture, line_comments: [] })), { status: 200 }),
    );

    await client.refetchQueries({ queryKey: ["review", "rev-1"] });

    expect(screen.getByText("缺少 nil 检查")).toBeVisible();
    // The server comment seeded on first load should also remain visible.
    expect(screen.getByText("服务端评论")).toBeVisible();
  });

  it("resets seeded comments and selected path when navigating to a different review", async () => {
    // Polyfill around jsdom Request signal validation so React Router navigation works in tests.
    const OriginalRequest = globalThis.Request;
    globalThis.Request = class RequestPolyfill extends OriginalRequest {
      constructor(input: RequestInfo | URL, init?: RequestInit) {
        if (init?.signal) {
          const { signal, ...rest } = init;
          super(input, rest);
        } else {
          super(input, init);
        }
      }
    } as typeof Request;

    const rev1Review = {
      ...reviewFixture,
      id: "rev-1",
      submission_id: "sub-1",
      line_comments: [
        {
          id: "server-rev-1",
          review_id: "rev-1",
          submission_id: "sub-1",
          file_path: diffFixture[0].path,
          side: "right" as const,
          line_number: 11,
          hunk_hash: "",
          diff_fingerprint: "",
          text: "Rev1 server comment",
          created_at: "2026-07-02T14:00:00Z",
        },
      ],
    };

    const rev2Review = {
      ...reviewFixture,
      id: "rev-2",
      submission_id: "sub-2",
      line_comments: [
        {
          id: "server-rev-2",
          review_id: "rev-2",
          submission_id: "sub-2",
          file_path: "src/other.go",
          side: "right" as const,
          line_number: 2,
          hunk_hash: "",
          diff_fingerprint: "",
          text: "Rev2 server comment",
          created_at: "2026-07-02T15:00:00Z",
        },
      ],
    };

    const rev2Diff: FileDiff[] = [
      {
        path: "src/other.go",
        old_path: "src/other.go",
        hunks: [
          {
            old_start: 1,
            old_lines: 1,
            new_start: 1,
            new_lines: 3,
            hunk_hash: "h2",
            lines: [
              { type: "context" as const, text: "package other", old_line: 1, new_line: 1 },
              { type: "add" as const, text: "func Other() {}", new_line: 2 },
            ],
          },
        ],
      },
    ];

    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify(envelope(rev1Review)), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(envelope(diffFixture)), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(envelope(rev2Review)), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(envelope(rev2Diff)), { status: 200 }));

    const client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });

    const router = createMemoryRouter(
      [
        {
          path: "/reviews/:reviewId",
          element: (
            <QueryClientProvider client={client}>
              <ReviewWorkspace />
            </QueryClientProvider>
          ),
        },
      ],
      { initialEntries: ["/reviews/rev-1"] },
    );

    render(<RouterProvider router={router} />);

    try {
      // Wait for rev-1 to load.
      expect(await screen.findByText("审核 rev-1")).toBeVisible();
      expect(screen.getByText("Rev1 server comment")).toBeVisible();
      expect(screen.getByLabelText("查看 src/payment.go")).toHaveAttribute("aria-pressed", "true");

      // Navigate to rev-2 within the same route.
      await router.navigate("/reviews/rev-2");

      // Wait for rev-2 to load and assert new server comments / selected path.
      expect(await screen.findByText("审核 rev-2")).toBeVisible();
      expect(screen.getByText("Rev2 server comment")).toBeVisible();
      expect(screen.queryByText("Rev1 server comment")).not.toBeInTheDocument();
      expect(screen.getByLabelText("查看 src/other.go")).toHaveAttribute("aria-pressed", "true");
    } finally {
      globalThis.Request = OriginalRequest;
    }
  });
});
