import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RepositoryOnboardingSummary } from "../api/client";
import { getRepositoryOnboarding } from "../api/client";
import { HomeRedirect, onboardingDestination } from "./HomeRedirect";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return { ...actual, getRepositoryOnboarding: vi.fn() };
});

const mockedGetRepositoryOnboarding = vi.mocked(getRepositoryOnboarding);

function LocationProbe() {
  const location = useLocation();
  return <output aria-label="location">{location.pathname}</output>;
}

function renderRoutes(initialPath: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialPath]}>
        <LocationProbe />
        <Routes>
          <Route path="/" element={<HomeRedirect />} />
          <Route path="/agents" element={<p>Agents page</p>} />
          <Route path="/tasks" element={<p>Tasks page</p>} />
          <Route path="/onboarding" element={<p>Onboarding page</p>} />
          <Route path="*" element={<HomeRedirect />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function summaryFixture(repositories = [{ id: "repo-1" }]): RepositoryOnboardingSummary {
  return {
    github_app: { configured: true, installation_id: 42 },
    app_repositories: { items: [] },
    onboarded_repositories: { items: repositories },
  } as unknown as RepositoryOnboardingSummary;
}

describe("onboardingDestination", () => {
  it.each([
    [{ configured: false, installation_id: 0 }, [{ id: "repo-1" }], "/onboarding"],
    [{ configured: true, installation_id: 0 }, [{ id: "repo-1" }], "/onboarding"],
    [{ configured: true, installation_id: 42 }, [], "/onboarding"],
    [{ configured: true, installation_id: 42 }, [{ id: "repo-1" }], "/tasks"],
  ])("routes readiness to %s", (githubApp, repositories, expected) => {
    const summary = {
      github_app: githubApp,
      app_repositories: { items: [] },
      onboarded_repositories: { items: repositories },
    } as unknown as RepositoryOnboardingSummary;
    expect(onboardingDestination(summary)).toBe(expected);
  });
});

describe("HomeRedirect", () => {
  beforeEach(() => mockedGetRepositoryOnboarding.mockReset());

  it.each(["/", "/unknown"])("routes %s according to onboarding readiness", async (path) => {
    mockedGetRepositoryOnboarding.mockResolvedValue({ data: summaryFixture(), meta: {} } as never);
    renderRoutes(path);
    await waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("/tasks"));
  });

  it("routes request failures to onboarding", async () => {
    mockedGetRepositoryOnboarding.mockRejectedValue(new Error("network failure"));
    renderRoutes("/");
    await waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("/onboarding"));
  });

  it("does not rewrite the explicit agents route", () => {
    renderRoutes("/agents");
    expect(screen.getByLabelText("location")).toHaveTextContent("/agents");
    expect(screen.getByText("Agents page")).toBeInTheDocument();
    expect(mockedGetRepositoryOnboarding).not.toHaveBeenCalled();
  });
});
