import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  addGitHubAppRepository,
  addPublicRepository,
  apiRequest,
  createSyncRule,
  deleteGitHubApp,
  getExecution,
  getGitHubApp,
  getTask,
  githubInstallUrl,
  listExecutionSubmissions,
  listGitHubAppRepositories,
  listGitHubApps,
  listSyncRules,
  listTasks,
  removeRepository,
  testGitHubApp,
} from "./client";
import type { ValidationJobView } from "../features/reviews/reviews.types";

const baseUrl = "/api";

describe("REST client authorization", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    delete (import.meta.env as Record<string, string | undefined>).VITE_API_TOKEN;
    delete (window as any).AG_TOKEN;
  });

  it("sends Authorization header from VITE_API_TOKEN in development", async () => {
    (import.meta.env as Record<string, string | undefined>).VITE_API_TOKEN = "dev-token";
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ data: { items: [] }, meta: {} }), { status: 200 }),
    );

    await listTasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, init] = fetchMock.mock.calls[0];
    expect((init as RequestInit).headers).toMatchObject({ Authorization: "Bearer dev-token" });
  });

  it("sends Authorization header from window.AG_TOKEN in production", async () => {
    (window as any).AG_TOKEN = "prod-token";
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ data: { items: [] }, meta: {} }), { status: 200 }),
    );

    await listTasks();

    expect((fetchMock.mock.calls[0][1] as RequestInit).headers).toMatchObject({
      Authorization: "Bearer prod-token",
    });
  });

  it("omits Authorization header when no token is configured", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ data: { items: [] }, meta: {} }), { status: 200 }),
    );

    await listTasks();

    const [, init] = fetchMock.mock.calls[0];
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(headers.Authorization).toBeUndefined();
  });

  it("redirects to /login on 401 responses", async () => {
    const originalLocation = window.location;
    Object.defineProperty(window, "location", {
      writable: true,
      value: { ...originalLocation, href: "" },
    });
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ error: { message: "未登录" } }), { status: 401 }),
    );

    await expect(listTasks()).rejects.toThrow("未登录");
    expect(window.location.href).toBe("/login");

    Object.defineProperty(window, "location", {
      writable: true,
      value: originalLocation,
    });
  });

  it("does not send Authorization header in demo mode", async () => {
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
    (import.meta.env as Record<string, string | undefined>).VITE_API_TOKEN = "dev-token";
    const fetchMock = vi.spyOn(globalThis, "fetch");

    await getTask("AG-1");

    expect(fetchMock).not.toHaveBeenCalled();
    delete (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE;
  });
});

describe("GitHub issue sync API client", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    delete (import.meta.env as Record<string, string | undefined>).VITE_API_TOKEN;
    delete (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE;
  });

  it("lists sync rules with GET /v1/sync-rules", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ data: { items: [] }, meta: {} }), { status: 200 }),
    );

    await listSyncRules();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe(`${baseUrl}/v1/sync-rules`);
    expect((fetchMock.mock.calls[0][1] as RequestInit).method).toBeUndefined();
  });

  it("creates sync rules with a POST JSON body", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ data: { id: "rule-1" }, meta: {} }), { status: 200 }),
    );
    const input = {
      repo: "acme/billing-service",
      include_labels: ["bug", "agent-task"],
      exclude_labels: ["wontfix"],
      issue_state: "open",
      task_type: "github_issue",
      default_priority: "normal",
      dedupe_strategy: "repo_issue",
    };

    await createSyncRule(input);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe(`${baseUrl}/v1/sync-rules`);
    expect((init as RequestInit).method).toBe("POST");
    expect((init as RequestInit).body).toBe(JSON.stringify(input));
    expect((init as RequestInit).headers).toMatchObject({ "Content-Type": "application/json" });
  });

  it("lists GitHub Apps with GET /v1/github-apps", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ data: { items: [] }, meta: {} }), { status: 200 }),
    );

    await listGitHubApps();

    expect(fetchMock.mock.calls[0][0]).toBe(`${baseUrl}/v1/github-apps`);
  });

  it("uses the selected App id for test, delete, and repository requests", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async () =>
      new Response(JSON.stringify({ data: {}, meta: {} }), { status: 200 }),
    );

    await testGitHubApp("gha/a");
    await deleteGitHubApp("gha/a", { idempotencyKey: "delete-key" });
    await listGitHubAppRepositories("gha/a");
    await addGitHubAppRepository("gha/a", "acme/service", { idempotencyKey: "add-key" });
    await addPublicRepository("octo/public", { idempotencyKey: "public-key" });
    await removeRepository("repo/a", { idempotencyKey: "remove-key" });

    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      `${baseUrl}/v1/github-apps/gha%2Fa:test`,
      `${baseUrl}/v1/github-apps/gha%2Fa`,
      `${baseUrl}/v1/github-apps/gha%2Fa/repositories`,
      `${baseUrl}/v1/repositories/github-app`,
      `${baseUrl}/v1/repositories/public`,
      `${baseUrl}/v1/repositories/repo%2Fa`,
    ]);
    expect((fetchMock.mock.calls[0][1] as RequestInit).method).toBe("POST");
    expect((fetchMock.mock.calls[1][1] as RequestInit).method).toBe("DELETE");
    expect((fetchMock.mock.calls[3][1] as RequestInit).body).toBe(
      JSON.stringify({ github_app_id: "gha/a", repo: "acme/service" }),
    );
    expect((fetchMock.mock.calls[1][1] as RequestInit).headers).toMatchObject({ "Idempotency-Key": "delete-key" });
    expect((fetchMock.mock.calls[3][1] as RequestInit).headers).toMatchObject({ "Idempotency-Key": "add-key" });
    expect((fetchMock.mock.calls[4][1] as RequestInit).headers).toMatchObject({ "Idempotency-Key": "public-key" });
    expect((fetchMock.mock.calls[5][1] as RequestInit).headers).toMatchObject({ "Idempotency-Key": "remove-key" });
  });

  it("preserves HTTP status and domain code on API errors", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ error: { code: "STATE_CONFLICT", message: "already bound" } }), { status: 409 }),
    );

    const error = await addPublicRepository("acme/api", { idempotencyKey: "conflict-key" }).catch((value) => value);

    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ status: 409, code: "STATE_CONFLICT", message: "already bound" });
  });

  it("includes the encoded App id in the install URL", () => {
    expect(githubInstallUrl("gha/a & b")).toBe(
      `${baseUrl}/oauth/github/app/install?github_app_id=gha%2Fa+%26+b`,
    );
  });

  it("provides two demo Apps with disjoint App-scoped repositories and a binding conflict", async () => {
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";

    const apps = await listGitHubApps();
    expect(apps.data.items).toHaveLength(2);
    const [alpha, beta] = apps.data.items;
    const alphaRepositories = await listGitHubAppRepositories(alpha.id);
    const betaRepositories = await listGitHubAppRepositories(beta.id);
    expect(alphaRepositories.data.items.map((repo) => repo.full_name)).not.toEqual(
      betaRepositories.data.items.map((repo) => repo.full_name),
    );
    expect(new Set([...alphaRepositories.data.items, ...betaRepositories.data.items].map((repo) => repo.full_name)).size)
      .toBe(alphaRepositories.data.items.length + betaRepositories.data.items.length);
    await expect(deleteGitHubApp(alpha.id)).rejects.toThrow(/绑定.*仓库/);
  });

  it("rejects demo duplicates tenant-wide regardless of source and case", async () => {
    vi.resetModules();
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
    const demoClient = await import("./client");

    await expect(demoClient.addPublicRepository("ACME/BILLING-SERVICE", { idempotencyKey: "demo-unique" }))
      .rejects.toMatchObject({ status: 409, code: "STATE_CONFLICT" });
  });

  it("replays demo mutations and rejects a reused key with a different payload", async () => {
    vi.resetModules();
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
    const demoClient = await import("./client");

    const first = await demoClient.addPublicRepository("octo/new-repo", { idempotencyKey: "demo-replay" });
    const replay = await demoClient.addPublicRepository("octo/new-repo", { idempotencyKey: "demo-replay" });
    expect(replay).toEqual(first);
    await expect(demoClient.addPublicRepository("octo/other-repo", { idempotencyKey: "demo-replay" }))
      .rejects.toMatchObject({ status: 409, code: "IDEMPOTENCY_MISMATCH" });
  });

  it("replays canonical-equivalent demo JSON bodies", async () => {
    vi.resetModules();
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
    const demoClient = await import("./client");
    const headers = { "Idempotency-Key": "canonical-demo" };

    const first = await demoClient.apiRequest("/v1/repositories/github-app", {
      method: "POST",
      headers,
      body: { github_app_id: "gha-beta", repo: "acme/frontend" },
    });
    const replay = await demoClient.apiRequest("/v1/repositories/github-app", {
      method: "POST",
      headers,
      body: { repo: "acme/frontend", github_app_id: "gha-beta" },
    });

    expect(replay).toEqual(first);
  });

  it("scopes the same demo key by operation", async () => {
    vi.resetModules();
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
    const demoClient = await import("./client");

    const added = await demoClient.addPublicRepository("octo/operation-scope", { idempotencyKey: "shared-operation-key" });
    await expect(demoClient.removeRepository(added.data.id!, { idempotencyKey: "shared-operation-key" }))
      .resolves.toMatchObject({ data: { deleted: true } });
  });

  it("isolates demo idempotency state by reloading the module without a production reset export", async () => {
    vi.resetModules();
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
    const firstClient = await import("./client");

    await firstClient.addPublicRepository("octo/reset-one", { idempotencyKey: "reset-key" });
    expect("resetDemoIdempotencyForTests" in firstClient).toBe(false);

    vi.resetModules();
    const reloadedClient = await import("./client");

    await expect(reloadedClient.addPublicRepository("octo/reset-two", { idempotencyKey: "reset-key" })).resolves.toBeDefined();
  });

  it("promotes the default after plural demo deletion and hides the deleted App repositories", async () => {
    vi.resetModules();
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
    const demoClient = await import("./client");

    const initialApps = await demoClient.listGitHubApps();
    const alpha = initialApps.data.items.find((app) => app.id === "gha-alpha");
    expect(alpha).toMatchObject({ is_default: true });
    await expect(demoClient.deleteGitHubApp(alpha!.id)).rejects.toThrow(/绑定.*仓库/);

    await demoClient.removeRepository("repo-inv-1");
    await demoClient.deleteGitHubApp(alpha!.id);

    const remainingApps = await demoClient.listGitHubApps();
    expect(remainingApps.data.items).toEqual([
      expect.objectContaining({ id: "gha-beta", is_default: true }),
    ]);
    const repositories = await demoClient.listRepositories();
    expect(repositories.data.items.map((repo) => repo.full_name)).toEqual([
      "acme/frontend",
      "acme/data-api",
    ]);
    await expect(demoClient.listGitHubAppRepositories(alpha!.id)).rejects.toThrow(/not found/);
  });

  it("makes singular demo deletion remove the default App and promote the earliest remaining App", async () => {
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";

    const initialDefault = await getGitHubApp();
    expect(initialDefault.data.id).toBe("gha-alpha");
    await expect(deleteGitHubApp()).rejects.toThrow(/绑定.*仓库/);

    await removeRepository("repo-inv-1");
    await deleteGitHubApp();

    const currentDefault = await getGitHubApp();
    expect(currentDefault.data).toMatchObject({ id: "gha-beta", is_default: true });
    const apps = await listGitHubApps();
    expect(apps.data.items).toEqual([expect.objectContaining({ id: "gha-beta", is_default: true })]);
    const testResult = await testGitHubApp();
    expect(testResult.data).toMatchObject({ ok: true, repo_count: 2 });
  });

  it("returns an unconfigured GitHub App in onboarding after deleting every demo App", async () => {
    vi.resetModules();
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
    const demoClient = await import("./client");

    await demoClient.removeRepository("repo-inv-1");
    await demoClient.deleteGitHubApp("gha-beta");
    await demoClient.deleteGitHubApp("gha-alpha");

    const summary = await demoClient.getRepositoryOnboarding();
    expect(summary.data.github_app).toEqual({ configured: false });
  });

  it("keeps the legacy singular connection test helper compatible", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ data: { ok: true }, meta: {} }), { status: 200 }),
    );

    await testGitHubApp();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe(`${baseUrl}/v1/github-app:test`);
    expect((fetchMock.mock.calls[0][1] as RequestInit).method).toBe("POST");
  });
});

describe("demo mode review decision context", () => {
  afterEach(() => {
    delete (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE;
  });

  it("returns the persisted validation job for a submission", async () => {
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";

    const response = await apiRequest<ValidationJobView>("/v1/submissions/sub-1/validation");

    expect(response.data.submission_id).toBe("sub-1");
    expect(response.data.status).toBe("succeeded");
    expect(response.data.config_version).toBeTruthy();
    expect(response.data.steps.length).toBeGreaterThan(0);
    expect(response.data.steps.some((step) => step.hard_gate)).toBe(true);
    expect(response.data.steps[0].log_summary).toBeTruthy();
    expect(response.data.steps[0].resource_usage?.elapsed_ms).toBeGreaterThan(0);
  });

  it("marks the earlier revision's validation job as failed", async () => {
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";

    const response = await apiRequest<ValidationJobView>("/v1/submissions/sub-0/validation");

    expect(response.data.status).toBe("failed");
    expect(response.data.steps.find((step) => step.step === "public_tests")?.status).toBe("failed");
  });

  it("lists every revision of the demo execution", async () => {
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";

    const response = await listExecutionSubmissions("exec-AG-188");

    expect(response.data.map((item) => item.id)).toEqual(["sub-0", "sub-1"]);
  });

  it("falls back to a known demo task for arbitrary execution ids", async () => {
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";

    const response = await getExecution("exec-route-99");

    expect(response.data.task_id).toBe("AG-188");
  });
});
