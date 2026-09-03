import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { GitPullRequest } from "lucide-react";
import { Navigate, Route, Routes, useLocation, useNavigate, useParams } from "react-router-dom";
import { AgentDetail } from "../features/agents/AgentDetail";
import { AgentList } from "../features/agents/AgentList";
import { AgentRegister } from "../features/agents/AgentRegister";
import { AgentTokenReveal } from "../features/agents/AgentTokenReveal";
import type { RegisterAgentResponse } from "../features/agents/agents.types";
import { LoginPage } from "../features/auth/LoginPage";
import { ReputationPage } from "../features/reputation/ReputationPage";
import { ReviewPage } from "../features/reviews/ReviewPage";
import { listPendingReviews } from "../features/reviews/reviews.api";
import { TaskDetail } from "../features/tasks/TaskDetail";
import { TaskList } from "../features/tasks/TaskList";
import { ExperienceList } from "../features/experiences/ExperienceList";
import { ExperienceReview } from "../features/experiences/ExperienceReview";
import { EvaluationList } from "../features/evaluations/EvaluationList";
import { VersionActions } from "../features/versions/VersionActions";
import { VersionDetail, VersionTree } from "../features/versions/VersionTree";
import type { VersionView } from "../features/versions/versions.types";
import { OnboardingScreen } from "../features/onboarding/OnboardingScreen";
import { GitIntegrationScreen } from "../features/git/GitIntegrationScreen";
import { RepositoryOnboardingScreen } from "../features/repositories/RepositoryOnboardingScreen";
import { SyncRuleScreen } from "../features/sync/SyncRuleScreen";
import { SyncResultScreen } from "../features/sync/SyncResultScreen";
import { ExecutionDetailScreen } from "../features/executions/ExecutionDetailScreen";
import { SubmissionValidationScreen } from "../features/submissions/SubmissionValidationScreen";
import { OutcomeScreen } from "../features/outcome/OutcomeScreen";
import { PageHeader, ButtonLink, Card } from "../ui";
import { Rail } from "./Rail";
import { Topbar } from "./Topbar";
import { useRailCollapsed } from "./useRailCollapsed";
import { HomeRedirect } from "./HomeRedirect";
import { LandingPage } from "../features/landing/LandingPage";
import { ProtocolPage } from "../features/protocol/ProtocolPage";
import { PublicNetworkRouter } from "../features/network/PublicNetwork";
import { DashboardPage } from "../features/dashboard/DashboardPage";
import { DesignGallery } from "../features/design/DesignGallery";

/* Maps the current pathname to the module label shown in the topbar. Ordered
   most-specific first. */
const MODULE_MAP: readonly [string, string][] = [
  ["/console/tasks", "任务中心"],
  ["/console/agents", "Agents"],
  ["/console", "总览"],
  ["/onboarding", "首次引导"],
  ["/git-integration", "Git 接入"],
  ["/repositories", "仓库接入"],
  ["/generation", "任务生成"],
  ["/sync-result", "运行结果"],
  ["/sync", "任务生成"],
  ["/executions", "执行详情"],
  ["/submissions", "提交验证"],
  ["/reviews", "审核工作台"],
  ["/reputation", "声望"],
  ["/outcome", "结果闭环"],
  ["/agents", "Agents"],
  ["/tasks", "任务中心"],
];

function moduleLabel(pathname: string): string {
  const match = MODULE_MAP.find(([prefix]) => pathname.startsWith(prefix));
  return match ? match[1] : "任务中心";
}

export function AppShell() {
  const location = useLocation();
  const { collapsed, toggle } = useRailCollapsed();

  // The login screen renders standalone, without the application chrome.
  if (location.pathname === "/login") {
    return <LoginPage />;
  }

  if (location.pathname === "/") {
    return <LandingPage />;
  }

  if (location.pathname === "/design-preview") {
    return <DesignGallery />;
  }

  if (location.pathname === "/protocol") {
    return <ProtocolPage />;
  }

  if (location.pathname.startsWith("/network")) {
    return <PublicNetworkRouter />;
  }

  // Keep the historical URLs as public browsing aliases. Authenticated
  // governance views live under /console/* and retain their private fields.
  if (location.pathname === "/tasks") {
    return <Navigate to="/network/tasks" replace />;
  }
  if (location.pathname.startsWith("/tasks/")) {
    return <Navigate to={`/network/tasks/${encodeURIComponent(location.pathname.slice("/tasks/".length))}`} replace />;
  }
  if (location.pathname === "/agents") {
    return <Navigate to="/network/agents" replace />;
  }
  const agentAlias = location.pathname.match(/^\/agents\/([^/]+)$/);
  if (agentAlias && agentAlias[1] !== "new") {
    return <Navigate to={`/network/agents/${encodeURIComponent(agentAlias[1])}`} replace />;
  }

  return (
    <div className="app" data-rail={collapsed ? "collapsed" : "expanded"}>
      <Rail collapsed={collapsed} onToggle={toggle} />
      <div className="app-body">
        <Topbar module={moduleLabel(location.pathname)} />
        <main className="page scroll">
          <Routes>
            <Route path="/" element={<LandingPage />} />
            <Route path="/console" element={<DashboardPage />} />
            <Route path="/protocol" element={<ProtocolPage />} />
            <Route path="/login" element={<LoginPage />} />
            <Route path="/onboarding" element={<OnboardingScreen />} />
            <Route path="/git-integration" element={<GitIntegrationScreen />} />
            <Route path="/repositories" element={<RepositoryOnboardingScreen />} />
            <Route path="/generation" element={<SyncRuleScreen />} />
            <Route path="/sync" element={<Navigate to="/generation" replace />} />
            <Route path="/console/tasks" element={<Workbench />} />
            <Route path="/console/tasks/:taskId" element={<Workbench />} />
            <Route path="/executions/:executionId" element={<ExecutionDetailScreen />} />
            <Route path="/submissions/:submissionId" element={<SubmissionValidationScreen />} />
            <Route path="/reviews" element={<ReviewWorkspace />} />
            <Route path="/reviews/:reviewId" element={<ReviewWorkspace />} />
            <Route path="/outcome" element={<OutcomeScreen />} />
            <Route path="/reputation" element={<ReputationWorkspace />} />
            <Route path="/console/agents" element={<AgentsWorkspace />} />
            <Route path="/agents/new" element={<Navigate to="/protocol#registration" replace />} />
            <Route path="/console/agents/:agentId" element={<AgentDetailWorkspace />} />
            <Route path="*" element={<HomeRedirect />} />
          </Routes>
        </main>
      </div>
    </div>
  );
}

function Workbench() {
  const { taskId } = useParams();
  const navigate = useNavigate();
  return (
    <div className="stack">
      <PageHeader title="任务中心" sub="查看任务规格、执行和验收状态；人工仅负责治理与审核。" />
      <div className="split-2 split-2--wide">
        <div className="col col--fill">
          <TaskList />
        </div>
        <div className="col">
          {taskId ? (
            <TaskDetail key={taskId} taskId={taskId} onClose={() => navigate("/console/tasks")} />
          ) : (
            <TaskDetail.Empty />
          )}
        </div>
      </div>
    </div>
  );
}

function AgentsWorkspace() {
  return (
    <div className="stack">
      <PageHeader
        title="Agents"
        sub="注册、启用并管理执行 Agent 的身份与状态"
        actions={
          <ButtonLink to="/protocol#registration" variant="primary">
            Agent 自助接入
          </ButtonLink>
        }
      />
      <AgentList />
    </div>
  );
}

function AgentRegistrationWorkspace() {
  const [tokenView, setTokenView] = useState<RegisterAgentResponse | null>(null);

  return (
    <div className="stack">
      <PageHeader
        title="注册 Agent"
        sub="创建新 Agent，并在注册完成后一次性领取 activation token"
        actions={<ButtonLink to="/console/agents">返回列表</ButtonLink>}
      />
      <div className="split-2">
        <div className="col">
          <AgentRegister onRegistered={setTokenView} />
        </div>
        <div className="col">
          {tokenView ? <AgentTokenReveal tokenView={tokenView} onDismiss={() => setTokenView(null)} /> : null}
        </div>
      </div>
    </div>
  );
}

function AgentDetailWorkspace() {
  const { agentId } = useParams();
  const [selectedVersion, setSelectedVersion] = useState<VersionView | null>(null);

  if (!agentId) {
    return <Navigate to="/console/agents" replace />;
  }

  return (
    <div className="stack">
      <PageHeader
        title="Agent 详情"
        sub="查看单个 Agent 的状态、版本谱系、经验候选与评测"
        actions={<ButtonLink to="/console/agents">返回列表</ButtonLink>}
      />
      <AgentDetail agentId={agentId} />
      <div className="split-2">
        <div className="col">
          <VersionTree agentId={agentId} onSelect={setSelectedVersion} />
          {selectedVersion ? <VersionActions agentId={agentId} version={selectedVersion} /> : null}
        </div>
        <div className="col">
          {selectedVersion ? <VersionDetail agentId={agentId} versionId={selectedVersion.id} /> : null}
          <ExperienceList agentId={agentId} />
          <ExperienceReview agentId={agentId} />
          <EvaluationList />
        </div>
      </div>
    </div>
  );
}

function PendingReviewList() {
  const reviews = useQuery({ queryKey: ["reviews", "pending"], queryFn: listPendingReviews });
  if (reviews.isLoading) return <Card title="待审核">加载中...</Card>;
  if (reviews.isError) return <Card title="待审核">加载失败，请稍后重试。</Card>;
  const items = reviews.data?.data ?? [];
  if (items.length === 0) {
    return (
      <div className="card">
        <div className="empty-state">
          <span className="es-icon" aria-hidden="true"><GitPullRequest size={18} strokeWidth={1.8} /></span>
          <div className="es-title">暂无待审核提交</div>
        <ButtonLink to="/console/tasks">查看任务与执行</ButtonLink>
        </div>
      </div>
    );
  }
  return (
    <Card title={`待审核 (${items.length})`}>
      <div className="stack-sm">
        {items.map((review) => (
          <div className="row-between" key={review.id}>
            <div>
              <b>{review.submission_id}</b>
              <p className="text-sm muted">Reviewer {review.reviewer_id}</p>
            </div>
            <ButtonLink to={`/reviews/${encodeURIComponent(review.id)}`}>开始审核</ButtonLink>
          </div>
        ))}
      </div>
    </Card>
  );
}

export function ReviewWorkspace() {
  const { reviewId } = useParams();

  return (
    <div className="stack">
      <PageHeader title="审核工作台" sub="查看提交 Diff、评分并给出审核结论" />
      {reviewId ? (
        <ReviewPage key={reviewId} />
      ) : (
        <PendingReviewList />
      )}
    </div>
  );
}

function ReputationWorkspace() {
  return (
    <div className="stack">
      <PageHeader title="声望" sub="按 Agent 版本、能力与任务类型查看评审声誉投影" />
      <ReputationPage />
    </div>
  );
}
