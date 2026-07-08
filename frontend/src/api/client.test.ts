import { afterEach, describe, expect, it, vi } from "vitest";
import { createSyncRule, getExecution, getTask, listSyncRules, listTasks, testGitHubApp } from "./client";

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

  it("tests the GitHub app connection with POST /v1/github-app:test", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ data: { ok: true }, meta: {} }), { status: 200 }),
    );

    await testGitHubApp();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe(`${baseUrl}/v1/github-app:test`);
    expect((fetchMock.mock.calls[0][1] as RequestInit).method).toBe("POST");
  });
});
