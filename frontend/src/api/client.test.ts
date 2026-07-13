import { afterEach, describe, expect, it, vi } from "vitest";
import {
  addGitHubAppRepository,
  createSyncRule,
  deleteGitHubApp,
  getExecution,
  getTask,
  githubInstallUrl,
  listGitHubAppRepositories,
  listGitHubApps,
  listSyncRules,
  listTasks,
  testGitHubApp,
} from "./client";

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
    await deleteGitHubApp("gha/a");
    await listGitHubAppRepositories("gha/a");
    await addGitHubAppRepository("gha/a", "acme/service");

    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      `${baseUrl}/v1/github-apps/gha%2Fa:test`,
      `${baseUrl}/v1/github-apps/gha%2Fa`,
      `${baseUrl}/v1/github-apps/gha%2Fa/repositories`,
      `${baseUrl}/v1/repositories/github-app`,
    ]);
    expect((fetchMock.mock.calls[0][1] as RequestInit).method).toBe("POST");
    expect((fetchMock.mock.calls[1][1] as RequestInit).method).toBe("DELETE");
    expect((fetchMock.mock.calls[3][1] as RequestInit).body).toBe(
      JSON.stringify({ github_app_id: "gha/a", repo: "acme/service" }),
    );
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
