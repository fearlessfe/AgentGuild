import { useQuery } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";
import { getRepositoryOnboarding, type RepositoryOnboardingSummary } from "../api/client";

export function onboardingDestination(summary: RepositoryOnboardingSummary): "/tasks" | "/onboarding" {
  const app = summary.github_app;
  return app.configured &&
    typeof app.installation_id === "number" &&
    app.installation_id > 0 &&
    summary.onboarded_repositories.items.length > 0
    ? "/tasks"
    : "/onboarding";
}

export function HomeRedirect() {
  const query = useQuery({
    queryKey: ["repository-onboarding"],
    queryFn: getRepositoryOnboarding,
    retry: false,
  });

  if (query.isPending) return <p role="status">正在检查接入状态...</p>;
  if (query.isError) return <Navigate to="/onboarding" replace />;
  return <Navigate to={onboardingDestination(query.data.data)} replace />;
}
