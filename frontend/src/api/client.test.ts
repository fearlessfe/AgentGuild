import { afterEach, describe, expect, it, vi } from "vitest";
import { getExecution, getTask, listTasks } from "./client";

const baseUrl = "http://localhost/api";

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
