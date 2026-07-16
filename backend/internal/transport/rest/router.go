package rest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	agentexperienceapp "agentguild.dev/agentguild/backend/internal/agentexperience/application"
	agentversionapp "agentguild.dev/agentguild/backend/internal/agentversion/application"
	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/config"
	"agentguild.dev/agentguild/backend/internal/domain"
	evaluationapp "agentguild.dev/agentguild/backend/internal/evaluation/application"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	syncapp "agentguild.dev/agentguild/backend/internal/sync/application"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// applicationService 是 REST 层消费的任务应用服务边界；*application.Service 天然满足此接口。
type applicationService interface {
	PublishTask(ctx context.Context, principal auth.Principal, command application.PublishTask) (application.Envelope[application.TaskView], error)
	ListTasks(ctx context.Context, principal auth.Principal, query application.ListTasks) (application.Envelope[application.TaskPage], error)
	GetTask(ctx context.Context, principal auth.Principal, query application.GetTask) (application.Envelope[application.TaskView], error)
	CancelTask(ctx context.Context, principal auth.Principal, command application.CancelTask) (application.Envelope[application.TaskView], error)
	ClaimTask(ctx context.Context, principal auth.Principal, command application.ClaimTask) (application.Envelope[application.ExecutionView], error)
	StartExecution(ctx context.Context, principal auth.Principal, command application.StartExecution) (application.Envelope[application.ExecutionView], error)
	HeartbeatExecution(ctx context.Context, principal auth.Principal, command application.HeartbeatExecution) (application.Envelope[application.ExecutionView], error)
	GetExecution(ctx context.Context, principal auth.Principal, query application.GetExecution) (application.Envelope[application.ExecutionView], error)
}

type submissionService interface {
	CreateSubmission(ctx context.Context, principal gitapp.Principal, command gitapp.CreateSubmission) (gitapp.Envelope[gitapp.SubmissionView], error)
	GetSubmission(ctx context.Context, principal gitapp.Principal, query gitapp.GetSubmission) (gitapp.Envelope[gitapp.SubmissionView], error)
	ListSubmissions(ctx context.Context, principal gitapp.Principal, query gitapp.ListSubmissions) (gitapp.Envelope[[]gitapp.SubmissionView], error)
}

type credentialService interface {
	IssueCredential(ctx context.Context, principal gitapp.Principal, cmd gitapp.IssueCredential) (gitapp.Envelope[gitapp.IssueCredentialResponse], error)
	GetCredential(ctx context.Context, principal gitapp.Principal, query gitapp.GetCredential) (gitapp.Envelope[gitapp.CredentialView], error)
	RevokeCredential(ctx context.Context, principal gitapp.Principal, cmd gitapp.RevokeCredential) (gitapp.Envelope[gitapp.CredentialView], error)
}

type identityService interface {
	RegisterAgent(context.Context, identityapp.Principal, identityapp.RegisterAgent) (identityapp.Envelope[identityapp.RegisterAgentResponse], error)
	ListAgents(context.Context, identityapp.Principal, identityapp.ListAgents) (identityapp.Envelope[identityapp.AgentPage], error)
	GetAgent(context.Context, identityapp.Principal, identityapp.GetAgent) (identityapp.Envelope[identityapp.AgentView], error)
	SuspendAgent(context.Context, identityapp.Principal, identityapp.SuspendAgent) (identityapp.Envelope[identityapp.AgentView], error)
	ResumeAgent(context.Context, identityapp.Principal, identityapp.ResumeAgent) (identityapp.Envelope[identityapp.AgentView], error)
	RevokeAgent(context.Context, identityapp.Principal, identityapp.RevokeAgent) (identityapp.Envelope[identityapp.AgentView], error)
	GetActivationStatus(context.Context, identityapp.Principal, identityapp.GetActivationStatus) (identityapp.Envelope[identityapp.ActivationStatusView], error)
	ActivateAgent(context.Context, identityapp.ActivateAgent) (identityapp.Envelope[identityapp.AccessTokenView], error)
	IssueAccessToken(context.Context, identityapp.Principal, identityapp.IssueAccessToken) (identityapp.Envelope[identityapp.AccessTokenView], error)
	AgentHeartbeat(context.Context, identityapp.Principal, identityapp.AgentHeartbeat) (identityapp.Envelope[identityapp.AgentView], error)
}

type versionService interface {
	ListVersions(ctx context.Context, tenantID, agentID string) ([]agentversionapp.VersionSummary, error)
	GetVersion(ctx context.Context, tenantID, agentID, versionID string) (*agentversionapp.VersionDetail, error)
	GetVersionDiff(ctx context.Context, tenantID, agentID, versionID, baseVersionID string) (*agentversionapp.VersionDiff, error)
	CreateDraft(ctx context.Context, cmd agentversionapp.CreateDraft) (*agentversionapp.CreateDraftResponse, error)
	StartEvaluation(ctx context.Context, principal identityapp.Principal, cmd agentversionapp.StartEvaluation) error
	Promote(ctx context.Context, cmd agentversionapp.Promote) error
	Rollback(ctx context.Context, cmd agentversionapp.Rollback) error
}

type evaluationService interface {
	ListBenchmarkSetSummaries(ctx context.Context, principal identityapp.Principal, tenantID string) ([]evaluationapp.BenchmarkSetSummary, error)
	GetBenchmarkSetSummary(ctx context.Context, principal identityapp.Principal, tenantID, id string) (*evaluationapp.BenchmarkSetSummary, error)
	CreateBenchmarkSet(ctx context.Context, cmd evaluationapp.CreateBenchmarkSet) (*evaluationapp.CreateBenchmarkSetResponse, error)
	ListEvaluationRunDetails(ctx context.Context, principal identityapp.Principal, tenantID, agentVersionID string) ([]evaluationapp.EvaluationRunDetail, error)
	GetEvaluationRunDetail(ctx context.Context, principal identityapp.Principal, tenantID, id string) (*evaluationapp.EvaluationRunDetail, error)
	StartEvaluationRun(ctx context.Context, cmd evaluationapp.StartEvaluationRun) (*evaluationapp.StartEvaluationRunResponse, error)
}

type experienceService interface {
	ListCandidates(ctx context.Context, tenantID, agentID, status string) ([]agentexperienceapp.CandidateSummary, error)
	GetCandidate(ctx context.Context, tenantID, agentID, candidateID string) (*agentexperienceapp.CandidateSummary, error)
	ExtractCandidate(ctx context.Context, cmd agentexperienceapp.ExtractCandidate) (*agentexperienceapp.ExtractCandidateResponse, error)
	ReviewCandidate(ctx context.Context, cmd agentexperienceapp.ReviewCandidate) error
}

type repositoryOnboardingService interface {
	Summary(ctx context.Context, principal gitapp.Principal) (gitapp.RepositoryOnboardingSummary, error)
	ListGitHubAppRepositories(ctx context.Context, principal gitapp.Principal, appID string) ([]gitapp.RepositoryCandidateView, error)
	AddGitHubAppRepository(ctx context.Context, principal gitapp.Principal, appID, fullName string) (gitapp.OnboardedRepositoryView, error)
	AddPublicRepository(ctx context.Context, principal gitapp.Principal, input string) (gitapp.OnboardedRepositoryView, error)
	Remove(ctx context.Context, principal gitapp.Principal, id string) error
}

type oidcProvider interface {
	BeginAuthURL(state string) string
	Exchange(context.Context, string) (*auth.Session, error)
}

type socketRemoteAddrContextKey struct{}

type healthChecker interface {
	Ping(context.Context) error
}

// Server 暴露任务生命周期、Submission、代码评审、版本管理与经验治理的 REST API。
type Server struct {
	svc                  applicationService
	submissions          submissionService
	credentials          credentialService
	identity             identityService
	reviewSvc            ReviewService
	rubricSvc            RubricService
	reputationSvc        ReputationService
	versions             versionService
	evaluations          evaluationService
	experiences          experienceService
	verifier             auth.TokenVerifier
	limiter              RateLimiter
	sessionSecret        string
	sessionSecure        bool
	oidc                 oidcProvider
	localAdmin           *localAdmin
	gitHubAppManager     gitapp.GitHubAppManager
	manifest             *gitapp.ManifestService
	syncRules            *syncapp.RuleService
	syncEngine           SyncEngine
	repositoryOnboarding repositoryOnboardingService
	idempotencyStore     mutationIdempotencyStore
	idempotencyHeartbeat time.Duration
	idempotencyIOTimeout time.Duration
	healthChecker        healthChecker
	gitProxy             http.Handler
}

// WithLocalAdmin 挂载本地管理员 fallback 登录接口。
func WithLocalAdmin(cfg config.Config) Option {
	return func(s *Server) { s.localAdmin = newLocalAdmin(cfg) }
}

// WithGitHubAppManager 挂载 tenant 级 GitHub App 管理。
func WithGitHubAppManager(m gitapp.GitHubAppManager) Option {
	return func(s *Server) { s.gitHubAppManager = m }
}

// WithGitHubManifest 挂载 GitHub App manifest 安装流程（manifest/callback/installed）。
func WithGitHubManifest(m *gitapp.ManifestService) Option {
	return func(s *Server) { s.manifest = m }
}

// WithSyncRuleService 挂载 GitHub Issue 到任务的同步规则管理接口。
func WithSyncRuleService(svc *syncapp.RuleService) Option {
	return func(s *Server) { s.syncRules = svc }
}

// WithSyncEngine 挂载同步引擎，用于手动触发单条同步规则。
func WithSyncEngine(engine SyncEngine) Option {
	return func(s *Server) { s.syncEngine = engine }
}

// WithRepositoryOnboardingService 挂载仓库 onboarding REST API。
func WithRepositoryOnboardingService(svc repositoryOnboardingService) Option {
	return func(s *Server) { s.repositoryOnboarding = svc }
}

// WithIdempotencyStore enables durable replay protection for human mutation routes.
func WithIdempotencyStore(store mutationIdempotencyStore) Option {
	return func(s *Server) { s.idempotencyStore = store }
}

// WithMutationIdempotencyTimings overrides heartbeat and bounded I/O timings.
// It is primarily useful for deterministic concurrency tests.
func WithMutationIdempotencyTimings(heartbeat, ioTimeout time.Duration) Option {
	return func(s *Server) {
		if heartbeat > 0 {
			s.idempotencyHeartbeat = heartbeat
		}
		if ioTimeout > 0 {
			s.idempotencyIOTimeout = ioTimeout
		}
	}
}

// Option 配置 Server。
type Option func(*Server)

// WithRateLimiter 替换默认的无限流实现。
func WithRateLimiter(l RateLimiter) Option {
	return func(s *Server) { s.limiter = l }
}

// WithHealthChecker 挂载无需认证的数据库就绪检查。
func WithHealthChecker(checker healthChecker) Option {
	return func(s *Server) { s.healthChecker = checker }
}

// WithIdentityService 挂载 Agent 身份管理与自服务 REST API。
func WithIdentityService(identity identityService) Option {
	return func(s *Server) { s.identity = identity }
}

// WithSubmissionService 挂载 Submission 创建与查询接口。
func WithSubmissionService(submissions submissionService) Option {
	return func(s *Server) { s.submissions = submissions }
}

// WithCredentialService 挂载 Git 凭证签发/查询/撤销接口。
func WithCredentialService(credentials credentialService) Option {
	return func(s *Server) { s.credentials = credentials }
}

// WithGitProxy mounts the branch-enforcing Git smart HTTP gateway.
func WithGitProxy(proxy http.Handler) Option {
	return func(s *Server) { s.gitProxy = proxy }
}

// WithSession 配置人类管理端的签名 session cookie。
func WithSession(secret string, secure bool) Option {
	return func(s *Server) {
		s.sessionSecret = secret
		s.sessionSecure = secure
	}
}

// WithOIDCProvider 挂载 OIDC login/callback 路由。
func WithOIDCProvider(provider oidcProvider) Option {
	return func(s *Server) { s.oidc = provider }
}

// WithVersionService 挂载 Agent 版本管理 REST API。
func WithVersionService(svc versionService) Option {
	return func(s *Server) { s.versions = svc }
}

// WithEvaluationService 挂载评测基准集与运行 REST API。
func WithEvaluationService(svc evaluationService) Option {
	return func(s *Server) { s.evaluations = svc }
}

// WithExperienceService 挂载经验候选治理 REST API。
func WithExperienceService(svc experienceService) Option {
	return func(s *Server) { s.experiences = svc }
}

// NewServer 创建 REST server；svc 通常是 *application.Service。
func NewServer(svc applicationService, verifier auth.TokenVerifier, opts ...Option) *Server {
	s := &Server{
		svc:                  svc,
		verifier:             verifier,
		limiter:              noopRateLimiter{},
		idempotencyHeartbeat: mutationHeartbeatInterval,
		idempotencyIOTimeout: mutationIdempotencyIOTimeout,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Router 返回 chi 路由；调用方负责监听与优雅关闭。
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(captureSocketRemoteAddr)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(jsonResponse)
	r.Get("/healthz", s.healthz)
	if s.gitProxy != nil {
		r.Handle("/git/*", s.gitProxy)
	}

	if s.oidc != nil {
		r.Get("/oauth/oidc/login", s.oidcLogin)
		r.Get("/oauth/oidc/callback", s.oidcCallback)
	}
	if s.manifest != nil {
		r.With(s.requireSession).Get("/oauth/github/app/manifest", s.githubManifest)
		r.With(s.requireSession).Get("/oauth/github/app/install", s.githubAppInstall)
		r.With(s.requireSession).Get("/oauth/github/app/callback", s.githubManifestCallback)
		r.With(s.requireSession).Get("/oauth/github/app/installed", s.githubAppInstalled)
	}
	if s.localAdmin != nil {
		r.Post("/oauth/local/login", s.localAdmin.login)
	}
	r.Get("/.well-known/agentguild", s.getAgentWellKnown)
	r.Get("/openapi.yaml", serveOpenAPI)
	r.Get("/skill.md", serveAgentSkill)

	r.Route("/v1", func(r chi.Router) {
		if s.identity != nil {
			// Agent self-service routes (bearer token only)
			r.Post("/agents/me:activate", s.rateLimit(http.HandlerFunc(s.activateAgent)).ServeHTTP)
			r.With(s.authenticate, s.rateLimit).Post("/agents/me:refresh", s.refreshAgentToken)
			r.With(s.authenticate, s.rateLimit).Post("/agents/me:heartbeat", s.agentHeartbeat)
			r.With(s.authenticate, s.rateLimit).Get("/agents/me", s.getSelfAgent)

			// Human management routes (session only)
			// Registration reveals a one-time activation token. It must never pass
			// through the generic response-persisting idempotency middleware.
			r.With(s.requireSession, s.rateLimit).Post("/agents", s.registerAgent)
			r.With(s.requireSession, s.rateLimit).Get("/agents", s.listAgents)
			r.With(s.requireSession, s.rateLimit).Get("/agents/{id}", s.getAgent)
			r.With(s.requireSession, s.rateLimit).Post("/agents/{id}:suspend", s.suspendAgent)
			r.With(s.requireSession, s.rateLimit).Post("/agents/{id}:resume", s.resumeAgent)
			r.With(s.requireSession, s.rateLimit).Post("/agents/{id}:revoke", s.revokeAgent)
			r.With(s.requireSession, s.rateLimit).Get("/agents/{id}:token", s.getAgentToken)
		}

		// Agent-only write routes (bearer token only)
		r.With(s.authenticate, s.rateLimit).Post("/tasks", s.publishTask)
		r.With(s.authenticate, s.rateLimit).Post("/tasks/{id}:claim", s.claimTask)
		r.With(s.authenticate, s.rateLimit).Post("/tasks/{id}:cancel", s.cancelTask)

		// Shared read-only routes (session or bearer)
		r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/tasks", s.listTasks)
		r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/tasks/{id}", s.getTask)

		// Agent-only write routes (bearer token only)
		r.With(s.authenticate, s.rateLimit).Post("/executions/{id}:start", s.startExecution)
		r.With(s.authenticate, s.rateLimit).Post("/executions/{id}:heartbeat", s.heartbeatExecution)

		// Shared read-only routes (session or bearer)
		r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/executions/{id}", s.getExecution)

		if s.submissions != nil {
			// Agent-only write routes (bearer token only)
			r.With(s.authenticate, s.rateLimit).Post("/executions/{id}/submissions", s.createSubmission)

			// Shared read-only routes (session or bearer)
			r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/submissions/{id}", s.getSubmission)
			r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/executions/{id}/submissions", s.listSubmissions)
		}

		// Human-only write routes (session only)
		r.With(s.requireSession, s.rateLimit).Post("/submissions/{id}/reviews", s.createReview)

		// Shared read-only routes (session or bearer)
		r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/submissions/{id}/diff", s.getSubmissionDiff)

		// Review mutations are human governance actions and require a browser
		// session. Application policy additionally enforces reviewer assignment.
		r.With(s.requireSession, s.rateLimit).Post("/reviews/{id}/decision", s.submitDecision)
		r.With(s.requireSession, s.rateLimit).Post("/reviews/{id}/comments", s.addComment)

		// Shared read-only routes (session or bearer)
		r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/reviews/{id}", s.getReview)
		r.With(s.requireSession, s.rateLimit).Get("/reviews", s.listReviews)
		r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/rubrics/active", s.getActiveRubric)
		r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/reputation", s.getReputation)

		// Agent-only write routes (bearer token only)
		r.With(s.authenticate, s.rateLimit).Post("/executions/{id}:submit_for_review", s.submitForReview)

		if s.versions != nil {
			// Shared read-only routes (session or bearer)
			r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/agents/{id}/versions", s.listAgentVersions)

			// Human-only write routes (session only)
			r.With(s.requireSession, s.rateLimit).Post("/agents/{id}/versions", s.createAgentVersion)

			// Shared read-only routes (session or bearer)
			r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/agents/{id}/versions/{version_id}", s.getAgentVersion)

			// Human-only write routes (session only)
			r.With(s.requireSession, s.rateLimit).Post("/agents/{id}/versions/{version_id}/diff", s.diffAgentVersion)
			r.With(s.requireSession, s.rateLimit).Post("/agents/{id}/versions/{version_id}/evaluations", s.startAgentVersionEvaluation)
			r.With(s.requireSession, s.rateLimit).Post("/agents/{id}/versions/{version_id}/promote", s.promoteAgentVersion)
			r.With(s.requireSession, s.rateLimit).Post("/agents/{id}/versions/{version_id}/rollback", s.rollbackAgentVersion)
		}

		if s.experiences != nil {
			// Shared read-only routes (session or bearer)
			r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/agents/{id}/experiences", s.listAgentExperiences)

			// Human-only write routes (session only)
			r.With(s.requireSession, s.rateLimit).Post("/agents/{id}/experiences", s.createAgentExperience)
			r.With(s.requireSession, s.rateLimit).Post("/agents/{id}/experiences/{experience_id}/approve", s.approveAgentExperience)
			r.With(s.requireSession, s.rateLimit).Post("/agents/{id}/experiences/{experience_id}/reject", s.rejectAgentExperience)
		}

		if s.evaluations != nil {
			// Shared read-only routes (session or bearer)
			r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/benchmarks", s.listBenchmarks)
			r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/benchmarks/{id}", s.getBenchmark)
			r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/evaluations", s.listEvaluations)
			r.With(s.authenticateHumanOrAgent, s.rateLimit).Get("/evaluations/{id}", s.getEvaluation)

			// Human-only write routes (session only)
			r.With(s.requireSession, s.rateLimit).Post("/benchmarks", s.createBenchmark)
		}

		// Agent-only write routes (bearer token only)
		if s.credentials != nil {
			r.With(s.authenticate, s.rateLimit).Post("/executions/{id}/credentials", s.issueCredential)
			r.With(s.authenticate, s.rateLimit).Get("/executions/{id}/credentials", s.getCredential)
			r.With(s.authenticate, s.rateLimit).Delete("/executions/{id}/credentials", s.revokeCredential)
		}

		// Tenant-level GitHub App configuration (human-only)
		if s.gitHubAppManager != nil {
			r.With(s.requireSession, s.rateLimit).Get("/repositories", s.listRepositories)
			r.With(s.requireSession, s.rateLimit).Get("/github-app", s.getGitHubApp)
			r.With(s.requireSession, s.rateLimit).Post("/github-app", s.upsertGitHubApp)
			r.With(s.requireSession, s.rateLimit).Delete("/github-app", s.deleteGitHubApp)
			r.With(s.requireSession, s.rateLimit).Post("/github-app:test", s.testGitHubApp)
			r.With(s.requireSession, s.rateLimit).Get("/github-apps", s.listGitHubApps)
			r.With(s.requireSession, s.rateLimit).Get("/github-apps/{id}", s.getGitHubAppByID)
			r.With(s.requireSession, s.rateLimit, s.mutationIdempotency("github_app.delete")).Delete("/github-apps/{id}", s.deleteGitHubAppByID)
			r.With(s.requireSession, s.rateLimit).Post("/github-apps/{id}:test", s.testGitHubAppByID)
		}
		if s.syncRules != nil {
			r.With(s.requireSession, s.rateLimit).Get("/sync-rules", s.listSyncRules)
			r.With(s.requireSession, s.rateLimit, s.mutationIdempotency("sync_rule.create")).Post("/sync-rules", s.createSyncRule)
			r.With(s.requireSession, s.rateLimit).Get("/sync-rules/{id}", s.getSyncRule)
			r.With(s.requireSession, s.rateLimit, s.mutationIdempotency("sync_rule.update")).Put("/sync-rules/{id}", s.updateSyncRule)
			r.With(s.requireSession, s.rateLimit, s.mutationIdempotency("sync_rule.delete")).Delete("/sync-rules/{id}", s.deleteSyncRule)
			if s.syncEngine != nil {
				r.With(s.requireSession, s.rateLimit, s.mutationIdempotency("sync_rule.run")).Post("/sync-rules/{id}:run", s.runSyncRule)
			}
		}
		if s.repositoryOnboarding != nil {
			r.With(s.requireSession, s.rateLimit).Get("/repository-onboarding", s.getRepositoryOnboarding)
			r.With(s.requireSession, s.rateLimit).Get("/github-apps/{id}/repositories", s.listGitHubAppRepositories)
			r.With(s.requireSession, s.rateLimit, s.mutationIdempotency("repository.github_app.create")).Post("/repositories/github-app", s.addGitHubAppRepository)
			r.With(s.requireSession, s.rateLimit, s.mutationIdempotency("repository.public.create")).Post("/repositories/public", s.addPublicRepository)
			r.With(s.requireSession, s.rateLimit, s.mutationIdempotency("repository.delete")).Delete("/repositories/{id}", s.deleteOnboardedRepository)
		}
	})
	return r
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if s.healthChecker == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.healthChecker.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

func captureSocketRemoteAddr(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), socketRemoteAddrContextKey{}, r.RemoteAddr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func jsonResponse(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal := mustPrincipal(r)
		key := r.Method + "|" + r.URL.Path
		if principal.TenantID != "" {
			key = principal.TenantID + "|" + key
		} else {
			key = "anonymous|" + callerSource(r) + "|" + key
		}
		allowed, retryAfter := s.limiter.Allow(r.Context(), key)
		if !allowed {
			writeRateLimited(w, "rate limit exceeded", retryAfter)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func callerSource(r *http.Request) string {
	source := strings.TrimSpace(r.RemoteAddr)
	if socketRemoteAddr, ok := r.Context().Value(socketRemoteAddrContextKey{}).(string); ok {
		source = strings.TrimSpace(socketRemoteAddr)
	}
	if source == "" {
		return "unknown"
	}
	if host, _, err := net.SplitHostPort(source); err == nil {
		source = host
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return "unknown"
	}
	return source
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid authorization")
			return
		}
		principal, err := s.verifier.Verify(r.Context(), parts[1])
		if err != nil {
			if errors.Is(err, auth.ErrTokenExpired) {
				writeError(w, http.StatusUnauthorized, "TOKEN_EXPIRED", "token expired")
				return
			}
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "token verification failed")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
	})
}

func (s *Server) publishTask(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	var body struct {
		RequestID    string    `json:"request_id"`
		Type         string    `json:"type"`
		Title        string    `json:"title"`
		Problem      string    `json:"problem"`
		Constraints  []string  `json:"constraints"`
		Requirements []string  `json:"requirements"`
		Deadline     time.Time `json:"deadline"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.svc.PublishTask(r.Context(), principal, application.PublishTask{
		RequestID:    idempotencyKey,
		Type:         body.Type,
		Title:        body.Title,
		Problem:      body.Problem,
		Constraints:  body.Constraints,
		Requirements: body.Requirements,
		Deadline:     body.Deadline,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "limit is invalid", "limit")
		return
	}
	query := application.ListTasks{
		Limit:                   limit,
		Cursor:                  r.URL.Query().Get("cursor"),
		PublisherAgentVersionID: r.URL.Query().Get("publisher_agent_version_id"),
		Type:                    r.URL.Query().Get("type"),
	}
	if statuses := r.URL.Query()["status"]; len(statuses) > 0 {
		query.Statuses = make([]domain.TaskStatus, 0, len(statuses))
		for _, s := range statuses {
			query.Statuses = append(query.Statuses, domain.TaskStatus(s))
		}
	}

	result, err := s.svc.ListTasks(r.Context(), principal, query)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	result, err := s.svc.GetTask(r.Context(), principal, application.GetTask{TaskID: chi.URLParam(r, "id")})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) claimTask(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	var body struct {
		RequestID string `json:"request_id"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.svc.ClaimTask(r.Context(), principal, application.ClaimTask{
		RequestID: idempotencyKey,
		TaskID:    chi.URLParam(r, "id"),
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) cancelTask(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	var body struct {
		RequestID string `json:"request_id"`
		Reason    string `json:"reason"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.svc.CancelTask(r.Context(), principal, application.CancelTask{
		RequestID: idempotencyKey,
		TaskID:    chi.URLParam(r, "id"),
		Reason:    body.Reason,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getExecution(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	result, err := s.svc.GetExecution(r.Context(), principal, application.GetExecution{ExecutionID: chi.URLParam(r, "id")})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) startExecution(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	var body struct {
		RequestID       string   `json:"request_id"`
		LeaseGeneration int64    `json:"lease_generation"`
		Stage           *string  `json:"stage,omitempty"`
		Progress        *float64 `json:"progress,omitempty"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.svc.StartExecution(r.Context(), principal, application.StartExecution{
		RequestID:       idempotencyKey,
		ExecutionID:     chi.URLParam(r, "id"),
		LeaseGeneration: body.LeaseGeneration,
		Stage:           body.Stage,
		Progress:        body.Progress,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) heartbeatExecution(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	var body struct {
		RequestID       string   `json:"request_id"`
		LeaseGeneration int64    `json:"lease_generation"`
		Stage           *string  `json:"stage,omitempty"`
		Progress        *float64 `json:"progress,omitempty"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.svc.HeartbeatExecution(r.Context(), principal, application.HeartbeatExecution{
		RequestID:       idempotencyKey,
		ExecutionID:     chi.URLParam(r, "id"),
		LeaseGeneration: body.LeaseGeneration,
		Stage:           body.Stage,
		Progress:        body.Progress,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// resolveIdempotencyKey 优先使用 Idempotency-Key header，其次使用 body 中的 request_id。
// REST 规范以 header 为首选，保留 body 字段用于兼容测试与旧客户端。
func resolveIdempotencyKey(r *http.Request, bodyRequestID string, w http.ResponseWriter) (string, bool) {
	if header := r.Header.Get("Idempotency-Key"); header != "" {
		return header, true
	}
	if bodyRequestID != "" {
		return bodyRequestID, true
	}
	writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "idempotency_key is required", "idempotency_key")
	return "", false
}

func parseLimit(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeEnvelope[T any](w http.ResponseWriter, status int, data T) {
	writeJSON(w, status, identityapp.Envelope[T]{Data: data, Meta: identityapp.Meta{ServerTime: time.Now()}})
}

func mustPrincipal(r *http.Request) auth.Principal {
	p, _ := auth.PrincipalFrom(r.Context())
	return p
}
