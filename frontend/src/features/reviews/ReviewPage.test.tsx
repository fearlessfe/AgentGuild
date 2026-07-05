import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { envelope } from "../../api/fixtures";
import { ReviewPage } from "./ReviewPage";
import type { FileDiff, ReviewView } from "./reviews.types";

function renderWithProviders(
  ui: ReactNode,
  { initialEntries = ["/reviews/rev-1"] }: { initialEntries?: string[] } = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <QueryClientProvider client={client}>
        <Routes>
          <Route path="/reviews/:reviewId" element={ui} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  );
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

  it("adds a line comment on the right side", async () => {
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify(envelope(reviewFixture)), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(envelope(diffFixture)), { status: 200 }));

    renderWithProviders(<ReviewPage />);

    await screen.findAllByText("func Charge(amount int) error {");

    await userEvent.click(screen.getByLabelText("在右侧第 12 行添加评论"));

    await userEvent.type(screen.getByPlaceholderText("输入评论…"), "缺少 nil 检查");
    await userEvent.click(screen.getByRole("button", { name: "添加评论" }));

    expect(await screen.findByText("缺少 nil 检查")).toBeVisible();
    expect(screen.getByText(/第 12 行/)).toBeVisible();
  });
});
