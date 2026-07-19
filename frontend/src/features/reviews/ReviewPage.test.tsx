import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes, createMemoryRouter, RouterProvider } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { envelope, taskViewFixture } from "../../api/fixtures";
import type { SubmissionView, TaskView } from "../../api/client";
import { ReviewWorkspace } from "../../app/AppShell";
import { ReviewPage } from "./ReviewPage";
import type { FileDiff, ReviewView, RubricView, ValidationJobView } from "./reviews.types";

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
  rubric_version_id: "rubric-1",
  capability: "code-review",
  status: "pending",
  final_decision: undefined,
  rubric_scores: [
    { dimension: "correctness", score: 85 },
    { dimension: "readability", score: 70 },
  ],
  summary: "整体实现正确，但缺少边界测试。",
  line_comments: [],
};

const rubricFixture: RubricView = {
  id: "rubric-1",
  tenant_id: "tenant-1",
  version_number: 1,
  name: "默认代码审核评分表",
  dimensions: [
    { id: "correctness", name: "正确性" },
    { id: "readability", name: "可读性" },
    { id: "testing", name: "测试覆盖" },
  ],
  weights: { correctness: 0.5, readability: 0.3, testing: 0.2 },
  algorithm_version: "v1",
  is_active: true,
  created_at: "2026-07-01T00:00:00Z",
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

const submissionFixture: SubmissionView = {
  id: "sub-1",
  tenant_id: "tenant-1",
  task_id: "task-1",
  execution_id: "exec-1",
  repo: "acme/billing-service",
  branch: "agent/task-1",
  commit_sha: "8f3a1c2d9e7b6a5f4c3d2e1a0b9c8d7e6f5a4b3c",
  base_commit_sha: "1a2b3c4d5e6f78901234567890abcdef12345678",
  summary: "修复重复触发并补充回归测试",
  diff_fingerprint: "sha256:sub-1",
  status: "validated",
  created_at: "2026-07-02T13:12:05Z",
  updated_at: "2026-07-02T13:14:20Z",
};

const revisionsFixture: SubmissionView[] = [
  submissionFixture,
  {
    ...submissionFixture,
    id: "sub-2",
    commit_sha: "bbbb1c2d9e7b6a5f4c3d2e1a0b9c8d7e6f5a4b3c",
    summary: "第二版修订",
    diff_fingerprint: "sha256:sub-2",
    status: "pending_verification",
    created_at: "2026-07-02T15:00:00Z",
    updated_at: "2026-07-02T15:00:00Z",
  },
];

const taskFixture: TaskView = taskViewFixture({
  id: "task-1",
  title: "修复定时任务重复触发问题",
  requirements: ["所有测试通过", "补充回归测试"],
});

const validationFixture: ValidationJobView = {
  id: "validation-1",
  tenant_id: "tenant-1",
  submission_id: "sub-1",
  status: "succeeded",
  attempt: 1,
  config_version: "validation-config@v3",
  steps: [
    {
      step: "build",
      status: "succeeded",
      hard_gate: true,
      log_summary: "编译通过，无告警",
      resource_usage: { elapsed_ms: 4200, cpu_seconds: 3.1, peak_memory_mb: 256 },
      started_at: "2026-07-02T13:12:06Z",
      finished_at: "2026-07-02T13:12:10Z",
    },
    {
      step: "public_tests",
      status: "succeeded",
      hard_gate: true,
      log_summary: "48 个公开测试全部通过",
      resource_usage: { elapsed_ms: 12500, cpu_seconds: 9.8, peak_memory_mb: 512 },
      started_at: "2026-07-02T13:12:10Z",
      finished_at: "2026-07-02T13:12:23Z",
    },
    {
      step: "security_scan",
      status: "failed",
      hard_gate: false,
      log_summary: "1 个低危告警",
      resource_usage: { elapsed_ms: 3100 },
      started_at: "2026-07-02T13:12:41Z",
      finished_at: "2026-07-02T13:12:44Z",
    },
  ],
  created_at: "2026-07-02T13:12:05Z",
  updated_at: "2026-07-02T13:14:20Z",
};

const validationFixtureSub2: ValidationJobView = {
  ...validationFixture,
  id: "validation-2",
  submission_id: "sub-2",
  status: "pending",
  attempt: 0,
  steps: [{ step: "build", status: "pending", hard_gate: true, log_summary: "等待执行" }],
};

type FetchState = {
  review: ReviewView;
  diff: FileDiff[];
  rubric: RubricView;
  submission: SubmissionView;
  revisions: SubmissionView[];
  task: TaskView;
  validations: Record<string, ValidationJobView>;
};

type MockFetchOverrides = {
  onCommentPost?: (input: {
    reviewId: string;
    body: Record<string, unknown>;
  }) => Promise<Response> | Response;
  onDecisionPost?: (input: {
    reviewId: string;
    body: Record<string, unknown>;
  }) => Promise<Response> | Response;
};

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function mockFetch(
  {
    review = reviewFixture,
    diff = diffFixture,
    rubric = rubricFixture,
    submission = submissionFixture,
    revisions = revisionsFixture,
    task = taskFixture,
    validations = { "sub-1": validationFixture, "sub-2": validationFixtureSub2 },
  }: Partial<FetchState> = {},
  overrides: MockFetchOverrides = {},
) {
  const state: FetchState = { review, diff, rubric, submission, revisions, task, validations };
  const spy = vi.spyOn(globalThis, "fetch").mockImplementation((input, init) => {
    const url = new URL(typeof input === "string" ? input : input.toString(), "http://localhost");
    const method = (init as RequestInit | undefined)?.method ?? "GET";

    if (url.pathname.startsWith("/api/v1/reviews/") && !url.pathname.includes("/comments") && !url.pathname.endsWith("/decision")) {
      return Promise.resolve(new Response(JSON.stringify(envelope(state.review)), { status: 200 }));
    }

    if (url.pathname === "/api/v1/rubrics/active") {
      return Promise.resolve(new Response(JSON.stringify(envelope(state.rubric)), { status: 200 }));
    }

    if (url.pathname.startsWith("/api/v1/submissions/") && url.pathname.endsWith("/diff")) {
      return Promise.resolve(new Response(JSON.stringify(envelope(state.diff)), { status: 200 }));
    }

    const validationMatch = url.pathname.match(/^\/api\/v1\/submissions\/([^/]+)\/validation$/);
    if (validationMatch && method === "GET") {
      const id = decodeURIComponent(validationMatch[1]);
      const job = state.validations[id] ?? Object.values(state.validations)[0];
      return Promise.resolve(new Response(JSON.stringify(envelope(job)), { status: 200 }));
    }

    const submissionMatch = url.pathname.match(/^\/api\/v1\/submissions\/([^/]+)$/);
    if (submissionMatch && method === "GET") {
      return Promise.resolve(new Response(JSON.stringify(envelope(state.submission)), { status: 200 }));
    }

    const executionSubmissionsMatch = url.pathname.match(/^\/api\/v1\/executions\/([^/]+)\/submissions$/);
    if (executionSubmissionsMatch && method === "GET") {
      return Promise.resolve(new Response(JSON.stringify(envelope(state.revisions)), { status: 200 }));
    }

    const taskMatch = url.pathname.match(/^\/api\/v1\/tasks\/([^/]+)$/);
    if (taskMatch && method === "GET") {
      return Promise.resolve(new Response(JSON.stringify(envelope(state.task)), { status: 200 }));
    }

    const commentMatch = url.pathname.match(/^\/api\/v1\/reviews\/([^/]+)\/comments$/);
    if (commentMatch && method === "POST") {
      const body = JSON.parse((init as RequestInit).body as string);
      if (overrides.onCommentPost) {
        return Promise.resolve(
          overrides.onCommentPost({
            reviewId: decodeURIComponent(commentMatch[1]),
            body,
          }),
        );
      }
      return Promise.resolve(
        new Response(
          JSON.stringify(
            envelope({
              id: `comment-${Date.now()}`,
              tenant_id: "tenant-1",
              review_id: decodeURIComponent(commentMatch[1]),
              ...body,
              created_at: "2026-07-02T14:00:00Z",
            })
          ),
          { status: 201 },
        )
      );
    }

    const decisionMatch = url.pathname.match(/^\/api\/v1\/reviews\/([^/]+)\/decision$/);
    if (decisionMatch && method === "POST") {
      const body = JSON.parse((init as RequestInit).body as string);
      if (overrides.onDecisionPost) {
        return Promise.resolve(
          overrides.onDecisionPost({
            reviewId: decodeURIComponent(decisionMatch[1]),
            body,
          }),
        );
      }
      return Promise.resolve(
        new Response(
          JSON.stringify(
            envelope({
              ...state.review,
              status: "submitted",
              final_decision: body.decision,
              rubric_scores: body.scores,
              summary: body.summary,
            })
          ),
          { status: 200 },
        )
      );
    }

    return Promise.reject(new Error(`Unexpected fetch: ${url.pathname}`));
  });
  return { spy, state };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("ReviewPage", () => {
  it("renders review status, summary, rubric scores and diff", async () => {
    mockFetch();
    renderWithProviders(<ReviewPage />);

    expect(await screen.findByText("审核 rev-1")).toBeVisible();
    const header = screen.getByText("审核 rev-1").closest(".review-header") as HTMLElement;
    expect(within(header).getByText("状态：")).toBeVisible();
    expect(screen.getByText("待审核")).toBeVisible();
    expect(screen.getByText("整体实现正确，但缺少边界测试。")).toBeVisible();
    expect(await screen.findByTestId("rubric-form")).toBeVisible();
    expect(screen.getByLabelText("正确性 分数")).toHaveValue(85);

    expect(await screen.findByLabelText("文件树")).toBeVisible();
    expect(screen.getByLabelText("查看 src/payment.go")).toBeVisible();
    expect((await screen.findAllByText("func Charge(amount int) error {")).length).toBeGreaterThan(0);
  });

  it("loads and renders the active rubric form", async () => {
    mockFetch();
    renderWithProviders(<ReviewPage />);

    await screen.findByText("审核 rev-1");
    expect(await screen.findByTestId("rubric-form")).toBeVisible();
    expect(screen.getByLabelText("正确性 分数")).toBeVisible();
    expect(screen.getByLabelText("可读性 分数")).toBeVisible();
    expect(screen.getByLabelText("测试覆盖 分数")).toBeVisible();
    expect(screen.getByTestId("rubric-total")).toHaveTextContent("总分：");
  });

  it("renders the task acceptance requirements", async () => {
    mockFetch();
    renderWithProviders(<ReviewPage />);

    await screen.findByText("审核 rev-1");
    expect(await screen.findByText("任务验收条件")).toBeVisible();
    expect(screen.getByText("修复定时任务重复触发问题")).toBeVisible();
    expect(screen.getByText("所有测试通过")).toBeVisible();
    expect(screen.getByText("补充回归测试")).toBeVisible();
  });

  it("renders validation evidence with hard gates, logs and resource usage", async () => {
    mockFetch();
    renderWithProviders(<ReviewPage />);

    await screen.findByText("审核 rev-1");
    const panel = await screen.findByTestId("validation-panel");
    expect(within(panel).getByText("配置版本：validation-config@v3")).toBeVisible();
    expect(within(panel).getByText("尝试次数：1")).toBeVisible();
    expect(within(panel).getByText("build")).toBeVisible();
    expect(within(panel).getByText("public_tests")).toBeVisible();
    expect(within(panel).getByText("security_scan")).toBeVisible();
    // hard_gate 步骤带硬门槛标识，非 hard_gate 步骤不带。
    expect(within(panel).getAllByText("硬门槛")).toHaveLength(2);
    expect(within(panel).getByText("编译通过，无告警")).toBeVisible();
    expect(within(panel).getByText("48 个公开测试全部通过")).toBeVisible();
    expect(within(panel).getByText("1 个低危告警")).toBeVisible();
    // 资源消耗以可读形式展示。
    expect(within(panel).getByText("资源：耗时 4.2s · CPU 3.1s · 峰值内存 256MB")).toBeVisible();
    expect(within(panel).getByText("资源：耗时 3.1s")).toBeVisible();
  });

  it("lists revisions and loads the selected revision's diff and validation", async () => {
    const { spy } = mockFetch();
    renderWithProviders(<ReviewPage />);

    await screen.findByText("审核 rev-1");
    expect((await screen.findAllByText("func Charge(amount int) error {")).length).toBeGreaterThan(0);
    expect(await screen.findByText("48 个公开测试全部通过")).toBeVisible();

    const selector = screen.getByLabelText("Revision");
    const options = within(selector as HTMLElement).getAllByRole("option");
    expect(options).toHaveLength(2);
    expect(options[0]).toHaveTextContent("R1 · 8f3a1c2 · 已验证");
    expect(options[1]).toHaveTextContent("R2 · bbbb1c2 · 待验证");

    await userEvent.selectOptions(selector, "sub-2");

    await waitFor(() => {
      const requested = spy.mock.calls.map(([input]) => String(input));
      expect(requested.some((url) => url.includes("/v1/submissions/sub-2/diff"))).toBe(true);
      expect(requested.some((url) => url.includes("/v1/submissions/sub-2/validation"))).toBe(true);
    });

    // 验证面板切换为所选修订的 Job。
    const panel = await screen.findByTestId("validation-panel");
    expect(await within(panel).findByText("等待执行")).toBeVisible();
    expect(within(panel).queryByText("48 个公开测试全部通过")).not.toBeInTheDocument();
  });

  it("switches between split and unified diff modes", async () => {
    mockFetch();
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
    mockFetch({ review: reviewWithoutSubmission });

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

    const { state } = mockFetch({ review: reviewWithServerComment });
    const { client } = renderWithProviders(<ReviewPage />);

    await screen.findAllByText("func Charge(amount int) error {");
    expect(screen.getByText("服务端评论")).toBeVisible();

    await userEvent.click(screen.getByLabelText("在右侧第 12 行添加评论"));
    await userEvent.type(screen.getByPlaceholderText("输入评论…"), "缺少 nil 检查");
    await userEvent.click(screen.getByRole("button", { name: "添加评论" }));

    expect(await screen.findByText("缺少 nil 检查")).toBeVisible();

    // Simulate a background refetch returning empty comments. Before the fix,
    // this would have overwritten local comments and removed them.
    state.review = { ...reviewFixture, line_comments: [] };
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

    const { state } = mockFetch({ review: rev1Review, diff: diffFixture });

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
      state.review = rev2Review;
      state.diff = rev2Diff;
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

  it("submits a decision with scores and summary", async () => {
    const { spy } = mockFetch();
    renderWithProviders(<ReviewPage />);

    await screen.findByText("审核 rev-1");
    await screen.findByTestId("rubric-form");

    await userEvent.type(screen.getByPlaceholderText("输入审核总结…"), " 符合要求");

    await userEvent.click(screen.getByRole("button", { name: "通过" }));

    await waitFor(() => expect(spy).toHaveBeenCalledWith(
      expect.stringContaining("/v1/reviews/rev-1/decision"),
      expect.objectContaining({ method: "POST" }),
    ));

    const [, init] = spy.mock.calls.find(
      ([url]) => typeof url === "string" && url.includes("/v1/reviews/rev-1/decision")
    )!;
    const body = JSON.parse((init as RequestInit).body as string);
    expect(body.decision).toBe("accepted");
    expect(body.summary).toContain("符合要求");
    expect(body.scores).toEqual(expect.arrayContaining([
      { dimension: "correctness", score: expect.any(Number) },
    ]));
  });

  it("shows a pending label while a decision is being submitted", async () => {
    const submitDecisionResponse = deferred<Response>();
    mockFetch({}, { onDecisionPost: () => submitDecisionResponse.promise });
    renderWithProviders(<ReviewPage />);

    await screen.findByText("审核 rev-1");
    await screen.findByTestId("rubric-form");

    await userEvent.click(screen.getByRole("button", { name: "通过" }));

    const pendingButtons = await screen.findAllByRole("button", { name: "提交中…" });
    expect(pendingButtons.length).toBeGreaterThan(0);
    expect(pendingButtons[0]).toBeDisabled();

    submitDecisionResponse.resolve(
      new Response(JSON.stringify(envelope({ ...reviewFixture, status: "submitted", final_decision: "accepted" })), { status: 200 }),
    );

    await waitFor(() => expect(screen.queryByRole("button", { name: "提交中…" })).not.toBeInTheDocument());
  });

  it("persists a line comment via POST /v1/reviews/:id/comments", async () => {
    const { spy } = mockFetch();
    renderWithProviders(<ReviewPage />);

    await screen.findAllByText("func Charge(amount int) error {");

    await userEvent.click(screen.getByLabelText("在右侧第 12 行添加评论"));
    await userEvent.type(screen.getByPlaceholderText("输入评论…"), "边界情况未处理");
    await userEvent.click(screen.getByRole("button", { name: "添加评论" }));

    await waitFor(() => expect(spy).toHaveBeenCalledWith(
      expect.stringContaining("/v1/reviews/rev-1/comments"),
      expect.objectContaining({ method: "POST" }),
    ));

    const [, init] = spy.mock.calls.find(
      ([url]) => typeof url === "string" && url.includes("/v1/reviews/rev-1/comments")
    )!;
    const body = JSON.parse((init as RequestInit).body as string);
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(body.file_path).toBe("src/payment.go");
    expect(body.line_number).toBe(12);
    expect(body.text).toBe("边界情况未处理");
    expect(body.request_id).toBeUndefined();
    expect(headers["Idempotency-Key"]).toBeTruthy();
  });

  it("shows a local alert and keeps the draft when comment submission fails", async () => {
    mockFetch({}, { onCommentPost: () => Promise.reject(new Error("评论提交失败，请重试")) });
    renderWithProviders(<ReviewPage />);

    await screen.findAllByText("func Charge(amount int) error {");

    await userEvent.click(screen.getByLabelText("在右侧第 12 行添加评论"));
    const textarea = screen.getByPlaceholderText("输入评论…");
    await userEvent.type(textarea, "边界情况未处理");
    await userEvent.click(screen.getByRole("button", { name: "添加评论" }));

    const form = textarea.closest(".comment-form");
    expect(form).not.toBeNull();
    expect(await within(form as HTMLElement).findByRole("alert")).toHaveTextContent("评论提交失败，请重试");
    expect(textarea).toHaveValue("边界情况未处理");
  });

  it("displays hard-gate error when accepting a submission with failed hard gates", async () => {
    const { spy } = mockFetch();
    renderWithProviders(<ReviewPage />);

    await screen.findByText("审核 rev-1");
    await screen.findByTestId("rubric-form");

    spy.mockImplementation((input) => {
      const url = new URL(typeof input === "string" ? input : input.toString(), "http://localhost");
      if (url.pathname.endsWith("/decision")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({ error: { code: "hard_gates_failed", message: "硬门槛未通过：测试覆盖率不足" } }),
            { status: 400 },
          ),
        );
      }
      return Promise.resolve(new Response(JSON.stringify(envelope(reviewFixture)), { status: 200 }));
    });

    await userEvent.click(screen.getByRole("button", { name: "通过" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("硬门槛未通过：测试覆盖率不足");
  });
});
