import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LoginPage } from "./LoginPage";

describe("LoginPage", () => {
  const originalLocation = window.location;

  beforeEach(() => {
    Object.defineProperty(window, "location", {
      writable: true,
      value: { ...originalLocation, href: "" },
    });
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    Object.defineProperty(window, "location", {
      writable: true,
      value: originalLocation,
    });
    vi.unstubAllEnvs();
    vi.restoreAllMocks();
  });

  it("presents local administration as the primary login method", () => {
    render(<LoginPage />);
    expect(screen.getByText(/本地管理登录/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /登录管理后台/i })).toBeInTheDocument();
  });

  it("does not render enterprise OIDC without public runtime configuration", () => {
    render(<LoginPage />);
    expect(screen.queryByRole("button", { name: /使用企业 OIDC 登录/i })).not.toBeInTheDocument();
  });

  it("submits local password and redirects on success", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true, json: async () => ({ data: { authenticated: true } }) });
    render(<LoginPage />);
    await userEvent.type(screen.getByPlaceholderText(/Local admin password/i), "secret");
    await userEvent.click(screen.getByRole("button", { name: /登录管理后台/i }));
    expect(fetch).toHaveBeenCalledWith(
      "/oauth/local/login",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ password: "secret" }),
        credentials: "same-origin",
      })
    );
    await vi.waitFor(() => expect(window.location.href).toBe("/agents"));
  });

  it("shows error when local password is wrong", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: false });
    render(<LoginPage />);
    await userEvent.type(screen.getByPlaceholderText(/Local admin password/i), "wrong");
    await userEvent.click(screen.getByRole("button", { name: /登录管理后台/i }));
    await vi.waitFor(() => expect(screen.getByText(/密码错误或未启用本地登录/i)).toBeInTheDocument());
  });
});
