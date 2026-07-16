import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { SubmissionValidationScreen } from "./SubmissionValidationScreen";

describe("SubmissionValidationScreen", () => {
  beforeEach(() => {
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
  });

  it("loads a submission and enters the created review", async () => {
    const user = userEvent.setup();
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/submissions/sub-1"]}>
          <Routes>
            <Route path="/submissions/:submissionId" element={<SubmissionValidationScreen />} />
            <Route path="/reviews/:reviewId" element={<p>已进入审核工作台</p>} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect((await screen.findAllByText("验证通过"))[0]).toBeVisible();
    expect(screen.getByText("acme/billing-service")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "创建审核" }));
    expect(await screen.findByText("已进入审核工作台")).toBeVisible();
  });
});
