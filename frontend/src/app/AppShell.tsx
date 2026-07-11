import { useMemo, useState } from "react";
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
import { PageHeader, ButtonLink } from "../ui";
import { Rail } from "./Rail";
import { Topbar } from "./Topbar";
import { useRailCollapsed } from "./useRailCollapsed";
import { HomeRedirect } from "./HomeRedirect";

/* Maps the current pathname to the module label shown in the topbar. Ordered
   most-specific first. */
const MODULE_MAP: readonly [string, string][] = [
  ["/onboarding", "首次引导"],
  ["/git-integration", "Git 接入"],
  ["/repositories", "仓库接入"],
  ["/sync-result", "同步结果"],
  ["/sync", "同步规则"],
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

  return (
    <div className="app" data-rail={collapsed ? "collapsed" : "expanded"}>
      <Rail collapsed={collapsed} onToggle={toggle} />
      <div className="app-body">
        <Topbar module={moduleLabel(location.pathname)} />
        <main className="page scroll">
          <Routes>
            <Route path="/" element={<HomeRedirect />} />
            <Route path="/login" element={<LoginPage />} />
            <Route path="/onboarding" element={<OnboardingScreen />} />
            <Route path="/git-integration" element={<GitIntegrationScreen />} />
            <Route path="/repositories" element={<RepositoryOnboardingScreen />} />
            <Route path="/sync" element={<SyncRuleScreen />} />
            <Route path="/tasks" element={<Workbench />} />
            <Route path="/tasks/:taskId" element={<Workbench />} />
            <Route path="/executions/:executionId" element={<ExecutionDetailScreen />} />
            <Route path="/submissions/:submissionId" element={<SubmissionValidationScreen />} />
            <Route path="/reviews" element={<ReviewWorkspace />} />
            <Route path="/reviews/:reviewId" element={<ReviewWorkspace />} />
            <Route path="/outcome" element={<OutcomeScreen />} />
            <Route path="/reputation" element={<ReputationWorkspace />} />
            <Route path="/agents" element={<AgentsWorkspace />} />
            <Route path="/agents/new" element={<AgentRegistrationWorkspace />} />
            <Route path="/agents/:agentId" element={<AgentDetailWorkspace />} />
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
      <PageHeader title="任务中心" sub="任务由同步规则生成；人工不领取、不提交，仅治理与审核。" />
      <div className="split-2 split-2--wide">
        <div className="col col--fill">
          <TaskList />
        </div>
        <div className="col">
          {taskId ? (
            <TaskDetail key={taskId} taskId={taskId} onClose={() => navigate("/tasks")} />
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
          <ButtonLink to="/agents/new" variant="primary">
            新增 Agent
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
        actions={<ButtonLink to="/agents">返回列表</ButtonLink>}
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
    return <Navigate to="/agents" replace />;
  }

  return (
    <div className="stack">
      <PageHeader
        title="Agent 详情"
        sub="查看单个 Agent 的状态、版本谱系、经验候选与评测"
        actions={<ButtonLink to="/agents">返回列表</ButtonLink>}
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

export function ReviewWorkspace() {
  const { reviewId } = useParams();

  return (
    <div className="stack">
      <PageHeader title="审核工作台" sub="查看提交 Diff、评分并给出审核结论" />
      {reviewId ? (
        <ReviewPage key={reviewId} />
      ) : (
        <div className="card">
          <div className="empty-state">
            <span className="es-icon" aria-hidden="true">
              <GitPullRequest size={18} strokeWidth={1.8} />
            </span>
            <div className="es-title">选择一次审核查看详情</div>
          </div>
        </div>
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
