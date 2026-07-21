package main

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	agentexperienceapp "agentguild.dev/agentguild/backend/internal/agentexperience/application"
	agentexperiencedomain "agentguild.dev/agentguild/backend/internal/agentexperience/domain"
	agentexperiencepostgres "agentguild.dev/agentguild/backend/internal/agentexperience/postgres"
	agentversionapp "agentguild.dev/agentguild/backend/internal/agentversion/application"
	agentversionpostgres "agentguild.dev/agentguild/backend/internal/agentversion/postgres"
	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/config"
	"agentguild.dev/agentguild/backend/internal/domain"
	evaluationapp "agentguild.dev/agentguild/backend/internal/evaluation/application"
	evaluationpostgres "agentguild.dev/agentguild/backend/internal/evaluation/postgres"
	evaluationworker "agentguild.dev/agentguild/backend/internal/evaluation/worker"
	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	githubapi "agentguild.dev/agentguild/backend/internal/git/github"
	gitpostgres "agentguild.dev/agentguild/backend/internal/git/postgres"
	gitproxy "agentguild.dev/agentguild/backend/internal/git/proxy"
	"agentguild.dev/agentguild/backend/internal/git/validation"
	gitworker "agentguild.dev/agentguild/backend/internal/git/worker"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	identitypostgres "agentguild.dev/agentguild/backend/internal/identity/postgres"
	"agentguild.dev/agentguild/backend/internal/postgres"
	publictaskapp "agentguild.dev/agentguild/backend/internal/publictask/application"
	publictaskpostgres "agentguild.dev/agentguild/backend/internal/publictask/postgres"
	reputationworker "agentguild.dev/agentguild/backend/internal/reputation/worker"
	reviewapp "agentguild.dev/agentguild/backend/internal/review/application"
	reviewpostgres "agentguild.dev/agentguild/backend/internal/review/postgres"
	syncapp "agentguild.dev/agentguild/backend/internal/sync/application"
	syncpostgres "agentguild.dev/agentguild/backend/internal/sync/postgres"
	"agentguild.dev/agentguild/backend/internal/telemetry"
	mcptransport "agentguild.dev/agentguild/backend/internal/transport/mcp"
	resttransport "agentguild.dev/agentguild/backend/internal/transport/rest"
	"agentguild.dev/agentguild/backend/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		slog.Error("agentguild stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return err
	}
	mapRepo := syncpostgres.NewMapRepository(pool)
	store := postgres.NewStore(pool)
	service, err := application.NewService(store, application.Options{
		CursorSecret:      []byte(cfg.CursorSecret),
		IssueSourceLookup: mapRepo,
	})
	if err != nil {
		return err
	}
	verifier, identityService, oidcProvider, err := buildIdentityRuntime(cfg, pool)
	if err != nil {
		return err
	}

	versionService, evaluationService, experienceService, evaluationWorker, err := buildVersionExperienceRuntime(cfg, pool, service)
	if err != nil {
		return err
	}

	gitRuntime, gitAppManager, repositoryOnboarding, err := buildGitRuntime(cfg, pool, service)
	if err != nil {
		return err
	}

	var ruleService *syncapp.RuleService
	var syncEngine *syncapp.Engine
	var manifestService *gitapp.ManifestService
	if gitAppManager != nil {
		ruleRepo := syncpostgres.NewRuleRepository(pool)
		ruleService, err = syncapp.NewRuleService(ruleRepo, syncapp.RuleServiceOptions{})
		if err != nil {
			return err
		}
		syncEngine = syncapp.NewEngine(ruleRepo, mapRepo, syncTaskSink{service: service, pool: pool}, gitRuntime.repositoryResolver, syncapp.EngineOptions{
			DefaultDeadline: cfg.SyncDefaultDeadline,
		})
		if cfg.WebEnabled {
			manifestService, err = gitapp.NewManifestService(gitAppManager, githubManifestOptions(cfg))
			if err != nil {
				return fmt.Errorf("build github manifest service: %w", err)
			}
		}
	}

	restOptions := make([]resttransport.Option, 0, 14)
	restOptions = append(restOptions, resttransport.WithIdempotencyStore(store))
	publicTaskService, err := publictaskapp.NewService(publictaskpostgres.NewRepository(pool), publictaskapp.Options{
		CursorSecret: []byte(cfg.CursorSecret),
	})
	if err != nil {
		return fmt.Errorf("build public task service: %w", err)
	}
	restOptions = append(restOptions, resttransport.WithPublicTaskService(publicTaskService))
	if identityService != nil {
		restOptions = append(restOptions, resttransport.WithIdentityService(identityService), resttransport.WithSession(cfg.SessionCookieSecret, cfg.SessionCookieSecure))
	}
	if oidcProvider != nil {
		restOptions = append(restOptions, resttransport.WithOIDCProvider(oidcProvider))
	}
	if cfg.LocalAdmin.Enabled {
		restOptions = append(restOptions, resttransport.WithLocalAdmin(cfg))
	}
	if gitAppManager != nil {
		restOptions = append(restOptions, resttransport.WithGitHubAppManager(gitAppManager))
	}
	if repositoryOnboarding != nil {
		restOptions = append(restOptions, resttransport.WithRepositoryOnboardingService(repositoryOnboarding))
	}
	if ruleService != nil && syncEngine != nil {
		restOptions = append(restOptions,
			resttransport.WithSyncRuleService(ruleService),
			resttransport.WithSyncEngine(syncEngine),
		)
	}
	if manifestService != nil {
		restOptions = append(restOptions, resttransport.WithGitHubManifest(manifestService))
	}
	if versionService != nil {
		restOptions = append(restOptions, resttransport.WithVersionService(versionService))
	}
	if evaluationService != nil {
		restOptions = append(restOptions, resttransport.WithEvaluationService(evaluationService))
	}
	if experienceService != nil {
		restOptions = append(restOptions, resttransport.WithExperienceService(experienceService))
	}

	submissionRepository := gitpostgres.NewSubmissionRepository(pool)
	diffProvider, err := reviewapp.NewGitDiffProvider(submissionRepository, gitRuntime.repositoryResolver)
	if err != nil {
		return fmt.Errorf("build review diff provider: %w", err)
	}
	validationProvider, err := reviewapp.NewGitValidationProvider(gitpostgres.NewValidationJobRepository(pool))
	if err != nil {
		return fmt.Errorf("build review validation provider: %w", err)
	}
	integrityChecker, err := reviewapp.NewGitSubmissionIntegrityChecker(submissionRepository, gitRuntime.repositoryResolver)
	if err != nil {
		return fmt.Errorf("build review integrity checker: %w", err)
	}
	reviewSvc, err := reviewapp.NewService(postgres.NewStore(pool), submissionRepository, diffProvider, validationProvider, reviewapp.Options{Integrity: integrityChecker})
	if err != nil {
		return err
	}
	reputationSvc := application.NewReputationQueryService(postgres.NewStore(pool))

	restOptions = append(restOptions,
		resttransport.WithHealthChecker(pool),
		resttransport.WithReviewService(reviewSvc),
		resttransport.WithRubricService(reviewSvc),
		resttransport.WithReputationService(reputationSvc),
	)
	if gitRuntime != nil {
		restOptions = append(restOptions,
			resttransport.WithSubmissionService(gitRuntime.submissionService),
			resttransport.WithCredentialService(gitRuntime.credentialService),
			resttransport.WithGitProxy(gitRuntime.gitProxy),
		)
	}
	restHandler := resttransport.NewServer(service, verifier, restOptions...).Router()

	mcpOptions := make([]mcptransport.Option, 0, 8)
	mcpOptions = append(mcpOptions, mcptransport.WithIdempotencyStore(store))
	if versionService != nil {
		mcpOptions = append(mcpOptions, mcptransport.WithVersionService(versionService))
	}
	if evaluationService != nil {
		mcpOptions = append(mcpOptions, mcptransport.WithEvaluationService(evaluationService))
	}
	if experienceService != nil {
		mcpOptions = append(mcpOptions, mcptransport.WithExperienceService(experienceService))
	}
	mcpOptions = append(mcpOptions,
		mcptransport.WithReviewService(reviewSvc),
		mcptransport.WithReputationService(reputationSvc),
	)
	if gitRuntime != nil {
		mcpOptions = append(mcpOptions,
			mcptransport.WithSubmissionService(gitRuntime.submissionService),
			mcptransport.WithCredentialService(gitRuntime.credentialService),
		)
	}
	mcpHandler := mcptransport.NewServer(service, verifier, mcpOptions...).Handler()

	if cfg.ReviewSeedTenantID != "" {
		if err := reviewpostgres.SeedReviewDefaults(ctx, pool, cfg.ReviewSeedTenantID, cfg.ReviewSeedReviewerUserID); err != nil {
			return fmt.Errorf("seed review defaults: %w", err)
		}
	}
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: adapterHandler(cfg.WebEnabled, cfg.MCPEnabled, restHandler, mcpHandler), ReadHeaderTimeout: 5 * time.Second}

	workerCtx, cancelWorkers := context.WithCancel(ctx)
	var wg sync.WaitGroup
	reaper := postgres.NewReaper(pool)
	runWorker(workerCtx, &wg, cfg.ReaperInterval, "reaper", func(ctx context.Context) error { _, err := reaper.RunBatch(ctx, 100); return err })
	provider := costProvider(cfg.LangfuseEnabled, cfg)
	outbox := worker.NewOutbox(pool, provider)
	runWorker(workerCtx, &wg, cfg.OutboxInterval, "outbox", outbox.RunOnce)
	reputation := reputationworker.NewWorker(postgres.NewStore(pool), cfg.ReputationWorkerInterval, 100, slog.Default())
	runWorker(workerCtx, &wg, cfg.ReputationWorkerInterval, "reputation", reputation.RunOnce)
	if evaluationWorker != nil {
		runWorker(workerCtx, &wg, cfg.EvaluationWorkerInterval, "evaluation", evaluationWorker.RunOnce)
	}
	if gitRuntime != nil {
		runWorker(workerCtx, &wg, cfg.ValidationWorkerInterval, "validation", func(ctx context.Context) error {
			return runValidationWorker(ctx, gitRuntime.validationWorker, pool)
		})
	}
	if syncEngine != nil {
		runWorker(workerCtx, &wg, cfg.SyncWorkerInterval, "sync", func(ctx context.Context) error {
			_, err := syncEngine.RunAllEnabled(ctx)
			return err
		})
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("agentguild listening", "addr", cfg.HTTPAddr, "web", cfg.WebEnabled, "mcp", cfg.MCPEnabled, "langfuse", cfg.LangfuseEnabled)
		errCh <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdown, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer shutdownCancel()
		slog.Info("shutting down workers")
		cancelWorkers()
		waitWorkers(&wg, cfg.ShutdownTimeout)
		slog.Info("workers stopped, shutting down server")
		return server.Shutdown(shutdown)
	case err := <-errCh:
		cancelWorkers()
		waitWorkers(&wg, cfg.ShutdownTimeout)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func githubManifestOptions(cfg config.Config) gitapp.ManifestOptions {
	return gitapp.ManifestOptions{
		PublicBaseURL:      cfg.GitHubAppPublicBaseURL,
		StateSecret:        []byte(cfg.GitHubAppManifestStateSecret),
		ConversionsBaseURL: cfg.GitHub.BaseURL,
	}
}

func buildVersionExperienceRuntime(cfg config.Config, pool *pgxpool.Pool, coreService *application.Service) (*agentversionapp.VersionService, *evaluationapp.EvaluationService, *agentexperienceapp.CandidateService, *evaluationworker.Worker, error) {
	avStore := agentversionpostgres.NewStore(pool)
	versionRepo := agentversionpostgres.NewVersionRepository(pool)
	evalProvider := agentversionpostgres.NewEvaluationRunProvider(pool)
	xpProvider := agentversionpostgres.NewExperienceCandidateProvider(pool)
	avPolicy := agentversionapp.NewPolicy(versionRepo)

	evStore := evaluationpostgres.NewStore(pool)
	bsRepo := evaluationpostgres.NewBenchmarkSetRepository(pool)
	runRepo := evaluationpostgres.NewEvaluationRunRepository(pool)
	versionLifecycle := agentversionpostgres.NewVersionLifecycleAdapter(pool)
	// Evaluation is opt-in via EVALUATION_EXECUTOR: unset means evaluation runs
	// fail closed with evaluation_unavailable; "fixed" selects the fixed-pass
	// stub for local development and demos only; "platform" publishes real
	// tasks through the platform's own delivery pipeline and registers the
	// harvest worker that resolves and completes the runs.
	var executor evaluationapp.BenchmarkExecutor
	var harvestWorker *evaluationworker.Worker
	evOptions := evaluationapp.EvaluationOptions{}
	switch cfg.EvaluationExecutor {
	case config.EvaluationExecutorFixed:
		slog.Warn("EVALUATION_EXECUTOR=fixed: benchmark evaluation uses a fixed-pass stub; results are not a real quality gate")
		executor = evaluationapp.NewFixedBenchmarkExecutor()
	case config.EvaluationExecutorPlatform:
		slog.Info("EVALUATION_EXECUTOR=platform: benchmark evaluation publishes real platform tasks")
		executor = evaluationapp.NewPlatformBenchmarkExecutor()
		runTasks := evaluationpostgres.NewEvaluationRunTaskRepository(pool)
		evOptions.TaskPublisher = evaluationTaskPublisher{service: coreService}
		evOptions.RunTasks = runTasks
		evOptions.TaskDeadline = cfg.EvaluationTaskDeadline
		harvestWorker = evaluationworker.NewWorker(
			evStore, runRepo, runTasks, bsRepo, versionLifecycle,
			evaluationpostgres.NewEvaluationTaskObserver(pool),
			cfg.EvaluationWorkerInterval, cfg.EvaluationRunTimeout, 10, slog.Default(),
		)
	default:
		slog.Info("EVALUATION_EXECUTOR unset: evaluation runs are disabled and will fail closed")
		executor = evaluationapp.NewRejectingBenchmarkExecutor()
	}
	evPolicy := evaluationapp.NewPolicy(versionRepo)
	evaluationService, err := evaluationapp.NewEvaluationService(evStore, bsRepo, runRepo, versionLifecycle, executor, evPolicy, evOptions)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// EVALUATION_AUTO: when automatic evaluation is enabled and an executor is
	// configured, every new draft version automatically starts an evaluation
	// run against the tenant's active benchmark set (best-effort post-commit
	// hook). Without an executor the hook stays disconnected and drafts are
	// unaffected.
	avOptions := agentversionapp.VersionOptions{}
	if cfg.EvaluationExecutor != "" {
		avOptions.DraftCreatedHook = evaluationapp.NewAutoEvaluator(
			cfg.EvaluationAuto, evaluationService, bsRepo,
			evaluationpostgres.NewVersionEnvironmentProvider(pool), slog.Default(),
		)
	} else if cfg.EvaluationAuto {
		slog.Warn("EVALUATION_AUTO=true but EVALUATION_EXECUTOR is unset; automatic evaluation on draft creation is disabled")
	}
	versionService, err := agentversionapp.NewVersionService(avStore, versionRepo, evalProvider, xpProvider, avPolicy, avOptions)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	axStore := agentexperiencepostgres.NewStore(pool)
	candidateRepo := agentexperiencepostgres.NewExperienceCandidateRepository(pool)
	submissionStore := agentexperiencepostgres.NewSubmissionStore(pool)
	executionStore := agentexperiencepostgres.NewExecutionStore(pool)
	classifier := agentexperiencedomain.NewRuleBasedSensitivityPolicy()
	axPolicy := agentexperienceapp.NewPolicy(versionRepo)
	experienceService, err := agentexperienceapp.NewCandidateService(axStore, candidateRepo, submissionStore, executionStore, axPolicy, classifier, agentexperienceapp.CandidateOptions{})
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return versionService, evaluationService, experienceService, harvestWorker, nil
}

func costProvider(enabled bool, cfg config.Config) telemetry.TraceCostProvider {
	if !enabled {
		return disabledCostProvider{}
	}
	return telemetry.NewLangfuseProvider(telemetry.LangfuseConfig{BaseURL: cfg.LangfuseBaseURL, PublicKey: cfg.LangfusePublicKey, SecretKey: cfg.LangfuseSecretKey, Mode: cfg.LangfuseMode, SupportsCost: cfg.LangfuseSupportsCost, MetricsPath: cfg.LangfuseMetricsPath, CompleteCoverageTag: cfg.LangfuseCompleteTag}, &http.Client{Timeout: 10 * time.Second})
}

func buildIdentityRuntime(cfg config.Config, pool *pgxpool.Pool) (auth.TokenVerifier, *identityapp.IdentityService, *auth.OIDCProvider, error) {
	if !cfg.WebEnabled {
		return auth.NewJWKSVerifier(cfg.OAuthIssuer, cfg.OAuthAudience, cfg.OAuthJWKSURL, nil), nil, nil, nil
	}

	privateKey, err := loadAgentRSAPrivateKey(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	tokenTTL := 15 * time.Minute
	tokenIssuer, err := auth.NewRS256TokenIssuer(privateKey, auth.TokenIssuerConfig{
		Issuer:   cfg.OAuthIssuer,
		Audience: cfg.OAuthAudience,
		KeyID:    "agentguild-api",
		TTL:      tokenTTL,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	identityService, err := identityapp.NewIdentityService(identitypostgres.NewStore(pool), identityapp.IdentityOptions{
		TokenIssuer: rs256IdentityTokenIssuer{issuer: tokenIssuer, ttl: tokenTTL},
	})
	if err != nil {
		return nil, nil, nil, err
	}
	var oidcProvider *auth.OIDCProvider
	if cfg.OIDCTenantID != "" {
		oidcProvider, err = auth.NewOIDCProvider(auth.OIDCConfig{
			TenantID:     cfg.OIDCTenantID,
			Issuer:       cfg.OIDCIssuer,
			ClientID:     cfg.OIDCClientID,
			ClientSecret: cfg.OIDCClientSecret,
			RedirectURI:  cfg.OIDCRedirectURI,
			AuthURL:      cfg.OIDCAuthURL,
			TokenURL:     cfg.OIDCTokenURL,
			JWKSURL:      cfg.OIDCJWKSURL,
			AdminClaim:   cfg.OIDCAdminClaim,
			AdminEmails:  append([]string(nil), cfg.OIDCAdminEmails...),
		}, nil)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return buildTokenVerifier(cfg, &privateKey.PublicKey), identityService, oidcProvider, nil
}

func buildTokenVerifier(cfg config.Config, publicKey *rsa.PublicKey) auth.TokenVerifier {
	localVerifier := auth.NewRS256Verifier(publicKey, auth.TokenVerifierConfig{
		Issuer:   cfg.OAuthIssuer,
		Audience: cfg.OAuthAudience,
	})
	if cfg.OAuthJWKSURL == "" {
		return localVerifier
	}
	return chainedTokenVerifier{verifiers: []auth.TokenVerifier{
		localVerifier,
		auth.NewJWKSVerifier(cfg.OAuthIssuer, cfg.OAuthAudience, cfg.OAuthJWKSURL, nil),
	}}
}

func loadAgentRSAPrivateKey(cfg config.Config) (*rsa.PrivateKey, error) {
	if cfg.AgentRSAPrivateKeyPEM != "" {
		return parseAgentRSAPrivateKey([]byte(cfg.AgentRSAPrivateKeyPEM))
	}
	if cfg.AgentRSAPrivateKeyPath == "" {
		return nil, errors.New("agent rsa private key is required")
	}
	body, err := os.ReadFile(cfg.AgentRSAPrivateKeyPath)
	if err != nil {
		return nil, err
	}
	return parseAgentRSAPrivateKey(body)
}

func parseAgentRSAPrivateKey(body []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(body)
	if block == nil {
		return nil, errors.New("decode agent rsa private key pem")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse agent rsa private key: %w", err)
	}
	privateKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("agent rsa private key is not rsa")
	}
	return privateKey, nil
}

type rs256IdentityTokenIssuer struct {
	issuer *auth.TokenIssuer
	ttl    time.Duration
}

func (i rs256IdentityTokenIssuer) IssueAccessToken(_ context.Context, agent *identitydomain.Agent, version *identitydomain.AgentVersion, now time.Time) (identityapp.AccessTokenView, error) {
	token, err := i.issuer.Issue(agent, version, now)
	if err != nil {
		return identityapp.AccessTokenView{}, err
	}
	return identityapp.AccessTokenView{
		Token:          token,
		TokenType:      "Bearer",
		ExpiresAt:      now.Add(i.ttl),
		AgentID:        agent.ID,
		AgentVersionID: version.ID,
		Scopes:         append([]string(nil), agent.Scopes...),
		RepoScope:      append([]string(nil), agent.RepoScope...),
	}, nil
}

type chainedTokenVerifier struct {
	verifiers []auth.TokenVerifier
}

func (c chainedTokenVerifier) Verify(ctx context.Context, rawToken string) (auth.Principal, error) {
	var firstErr error
	var sawExpired bool
	for _, verifier := range c.verifiers {
		principal, err := verifier.Verify(ctx, rawToken)
		if err == nil {
			return principal, nil
		}
		if errors.Is(err, auth.ErrTokenExpired) {
			sawExpired = true
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if sawExpired {
		return auth.Principal{}, auth.ErrTokenExpired
	}
	if firstErr == nil {
		firstErr = errors.New("no token verifier configured")
	}
	return auth.Principal{}, firstErr
}

type disabledCostProvider struct{}

func (disabledCostProvider) Observe(context.Context, telemetry.ExecutionRef) (telemetry.CostObservation, error) {
	return telemetry.CostObservation{Coverage: telemetry.CoverageUnavailable, Provider: "disabled", Disabled: true}, nil
}

func adapterHandler(webEnabled, mcpEnabled bool, web, mcp http.Handler) http.Handler {
	mux := http.NewServeMux()
	if mcpEnabled {
		mux.Handle("/mcp", mcp)
	} else {
		mux.HandleFunc("/mcp", http.NotFound)
	}
	if webEnabled {
		mux.Handle("/", web)
	}
	return mux
}

func runWorker(ctx context.Context, wg *sync.WaitGroup, interval time.Duration, name string, fn func(context.Context) error) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		repeat(ctx, interval, name, fn)
	}()
}

func waitWorkers(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		slog.Warn("worker shutdown timed out", "timeout", timeout)
		return false
	}
}

func repeat(ctx context.Context, interval time.Duration, name string, fn func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					if ctx.Err() == nil {
						slog.Error(name+" iteration panicked", "panic", r)
					}
				}
			}()
			if err := fn(ctx); err != nil && ctx.Err() == nil {
				slog.Error(name+" iteration failed", "error", err)
			}
		}()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type syncTaskSink struct {
	service *application.Service
	pool    *pgxpool.Pool
}

// evaluationTaskPublisher adapts the core task service to the evaluation
// module's EvaluationTaskPublisher port. PublishSystemTask opens its own
// transaction, so the evaluation service calls it only after its own
// transaction has committed.
type evaluationTaskPublisher struct {
	service *application.Service
}

func (p evaluationTaskPublisher) PublishEvaluationTask(ctx context.Context, cmd evaluationapp.PublishEvaluationTaskCommand) (string, error) {
	task, err := p.service.PublishSystemTask(ctx, application.PublishSystemTask{
		TenantID:     cmd.TenantID,
		RequestID:    cmd.RequestID,
		Type:         cmd.Type,
		Title:        cmd.Title,
		Problem:      cmd.Problem,
		Constraints:  cmd.Constraints,
		Requirements: cmd.Requirements,
		Deadline:     cmd.Deadline,
	})
	if err != nil {
		return "", err
	}
	return task.ID, nil
}

func (s syncTaskSink) PublishSystemTask(ctx context.Context, in syncapp.PublishSystemTaskInput) (string, error) {
	task, err := s.service.PublishSystemTask(ctx, application.PublishSystemTask{
		TenantID:     in.TenantID,
		RequestID:    in.RequestID,
		Type:         in.Type,
		Title:        in.Title,
		Problem:      in.Problem,
		Constraints:  in.Constraints,
		Requirements: in.Requirements,
		Deadline:     in.Deadline,
	})
	if err != nil {
		return "", err
	}
	return task.ID, nil
}

func (s syncTaskSink) CancelSystemTask(ctx context.Context, tenantID, taskID, reason string) error {
	_, err := s.service.CancelSystemTask(ctx, tenantID, taskID, reason)
	return err
}

func (s syncTaskSink) UpdateSystemTaskContent(ctx context.Context, tenantID, taskID string, in syncapp.ContentInput) (string, error) {
	if err := s.service.UpdateSystemTaskContent(ctx, tenantID, taskID, in.Title, in.Problem, in.Constraints, in.Requirements); err != nil {
		return "", err
	}
	return s.TaskStatus(ctx, tenantID, taskID)
}

func (s syncTaskSink) TaskStatus(ctx context.Context, tenantID, taskID string) (string, error) {
	var status string
	if err := s.pool.QueryRow(ctx, `SELECT status FROM tasks WHERE tenant_id=$1 AND id=$2`, tenantID, taskID).Scan(&status); err != nil {
		return "", err
	}
	return status, nil
}

var _ syncapp.TaskSink = (*syncTaskSink)(nil)

// gitRuntime holds the git delivery and validation services initialized for
// this process. It is nil when GitHub App configuration is not provided.
type gitRuntime struct {
	credentialService  *gitapp.CredentialService
	submissionService  *gitapp.SubmissionService
	validationWorker   *gitworker.ValidationWorker
	repositoryResolver gitapp.RepositoryGitResolver
	gitProxy           http.Handler
}

func buildGitRuntime(cfg config.Config, pool *pgxpool.Pool, service *application.Service) (*gitRuntime, gitapp.GitHubAppManager, *gitapp.RepositoryOnboardingService, error) {
	gitStore := gitpostgres.NewStore(pool)
	gitAppRepo := gitpostgres.NewGitHubAppRepository(pool)
	gitAppManager, err := gitapp.NewGitHubAppManagerWithOptions(gitAppRepo, gitapp.GitHubAppManagerOptions{AllowedHosts: cfg.GitHubAllowedHosts})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build github app service: %w", err)
	}
	onboardedRepositoryStore := gitpostgres.NewOnboardedRepositoryRepository(pool)
	repositoryResolver, err := gitapp.NewRepositoryGitResolver(
		onboardedRepositoryStore,
		gitAppManager,
		githubapi.NewPublicIssueSource("", nil),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build repository git resolver: %w", err)
	}

	ctx := context.Background()
	if cfg.GitHub.AppID != 0 && cfg.GitHub.PrivateKey != "" && cfg.GitHub.InstallationID != 0 {
		seedTenant := cfg.OIDCTenantID
		if seedTenant == "" {
			seedTenant = cfg.LocalAdmin.TenantID
		}
		if seedTenant != "" {
			if _, err := gitAppRepo.GetDefault(ctx, seedTenant); err != nil {
				if errors.Is(err, git.ErrGitHubAppNotConfigured) {
					if upsertErr := gitAppManager.Upsert(ctx, gitapp.UpsertGitHubApp{
						TenantID:       seedTenant,
						Provider:       "github",
						AppID:          cfg.GitHub.AppID,
						InstallationID: cfg.GitHub.InstallationID,
						PrivateKey:     cfg.GitHub.PrivateKey,
						BaseURL:        cfg.GitHub.BaseURL,
					}); upsertErr != nil {
						return nil, nil, nil, fmt.Errorf("seed github app configuration: %w", upsertErr)
					}
					slog.Info("seeded github app configuration from environment", "tenant", seedTenant)
				} else {
					return nil, nil, nil, fmt.Errorf("check existing github app configuration: %w", err)
				}
			}
		}
	}

	credentialService, err := gitapp.NewCredentialService(gitStore, repositoryResolver, gitapp.Options{
		Provider:     "github",
		Authorizer:   service,
		ProxyBaseURL: cfg.GitHubAppPublicBaseURL,
		TokenSecret:  []byte(cfg.CursorSecret),
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build credential service: %w", err)
	}

	verifier := gitapp.NewCommitVerifier(repositoryResolver, &submissionRepoAdapter{store: gitStore})
	notifier := application.NewCoreExecutionNotifier(service)
	submissionService, err := gitapp.NewSubmissionService(gitStore, verifier, notifier, service, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build submission service: %w", err)
	}
	// Enforce the task deadline directly on the submission path, aligned with
	// the heartbeat guard, instead of relying on the reaper alone.
	submissionService.SetTaskDeadlineResolver(gitapp.TaskDeadlineResolverFunc(func(ctx context.Context, tenantID, taskID string) (time.Time, error) {
		var deadline time.Time
		err := service.WithTx(ctx, func(tx application.Tx) error {
			task, err := tx.GetTask(ctx, tenantID, taskID)
			if err != nil {
				return err
			}
			deadline = task.Deadline
			return nil
		})
		return deadline, err
	}))
	gitProxy, err := gitproxy.NewHandler(gitStore, repositoryResolver)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build git proxy: %w", err)
	}

	repositoryOnboarding, err := gitapp.NewRepositoryOnboardingService(
		onboardedRepositoryStore,
		gitAppManager,
		newPublicRepositoryResolver(cfg.GitHub.BaseURL),
		nil,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build repository onboarding service: %w", err)
	}

	cloneHosts := append([]string{"github.com"}, cfg.GitHubAllowedHosts...)
	workspaceFactory, err := validation.NewGitWorkspaceFactory(repositoryResolver, cloneHosts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build validation workspace factory: %w", err)
	}
	runner := validation.NewRunner(validation.DefaultRegistry(), workspaceFactory, validation.NewContainerExecutor(cfg.ValidationSandboxImage))
	validationWorker := gitworker.NewValidationWorker(
		gitStore,
		"validation-worker",
		cfg.ValidationLease,
		cfg.ValidationMaxAttempts,
		runner,
		notifier,
	)

	return &gitRuntime{
		credentialService:  credentialService,
		submissionService:  submissionService,
		validationWorker:   validationWorker,
		repositoryResolver: repositoryResolver,
		gitProxy:           gitProxy,
	}, gitAppManager, repositoryOnboarding, nil
}

func runValidationWorker(ctx context.Context, w *gitworker.ValidationWorker, pool *pgxpool.Pool) error {
	tenantIDs, err := listValidationWorkerTenants(ctx, pool)
	if err != nil {
		return err
	}

	var lastErr error
	for _, tenantID := range tenantIDs {
		if _, err := w.RunOnce(ctx, tenantID); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			lastErr = err
		}
	}
	return lastErr
}

func listValidationWorkerTenants(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT tenant_id
		FROM validation_jobs
		WHERE status IN ('pending','running')
		   OR (status IN ('succeeded','failed') AND execution_state_synced=FALSE)
		ORDER BY tenant_id
		LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tenantIDs := make([]string, 0)
	for rows.Next() {
		var tenantID string
		if err := rows.Scan(&tenantID); err != nil {
			return nil, err
		}
		tenantIDs = append(tenantIDs, tenantID)
	}
	return tenantIDs, rows.Err()
}

// submissionRepoAdapter exposes git/application.SubmissionRepository by
// opening a short transaction on the git store. It is used by CommitVerifier
// for duplicate-submission detection.
type submissionRepoAdapter struct {
	store gitapp.Store
}

func (a *submissionRepoAdapter) Save(ctx context.Context, sub *gitdomain.Submission) error {
	return a.store.WithTx(ctx, func(tx gitapp.Tx) error {
		return tx.Submissions().Save(ctx, sub)
	})
}

func (a *submissionRepoAdapter) GetByID(ctx context.Context, tenantID, id string) (*gitdomain.Submission, error) {
	var result *gitdomain.Submission
	err := a.store.WithTx(ctx, func(tx gitapp.Tx) error {
		var err error
		result, err = tx.Submissions().GetByID(ctx, tenantID, id)
		return err
	})
	return result, err
}

func (a *submissionRepoAdapter) GetByExecutionID(ctx context.Context, tenantID, executionID string) ([]*gitdomain.Submission, error) {
	var result []*gitdomain.Submission
	err := a.store.WithTx(ctx, func(tx gitapp.Tx) error {
		var err error
		result, err = tx.Submissions().GetByExecutionID(ctx, tenantID, executionID)
		return err
	})
	return result, err
}

type publicRepositoryResolver struct {
	baseURL string
	client  *http.Client
}

func newPublicRepositoryResolver(baseURL string) *publicRepositoryResolver {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	return &publicRepositoryResolver{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (r *publicRepositoryResolver) ResolvePublicRepository(ctx context.Context, fullName string) (git.Repository, error) {
	owner, name, ok := strings.Cut(fullName, "/")
	if !ok || owner == "" || name == "" {
		return git.Repository{}, &domain.Error{Code: "invalid_argument", Message: "repo is invalid", Field: "repo"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name), nil)
	if err != nil {
		return git.Repository{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := r.client.Do(req)
	if err != nil {
		return git.Repository{}, fmt.Errorf("request github repository: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return git.Repository{}, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return git.Repository{}, &domain.Error{Code: "not_found", Message: "repository not found"}
	}
	if isGitHubRateLimit(resp, body) {
		return git.Repository{}, &domain.Error{Code: "rate_limited", Message: "github rate limit exceeded", RetryAfter: retryAfterDuration(resp.Header)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return git.Repository{}, fmt.Errorf("github repository lookup failed: status %d", resp.StatusCode)
	}
	var payload struct {
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
		Visibility    string `json:"visibility"`
		Private       bool   `json:"private"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return git.Repository{}, fmt.Errorf("decode github repository: %w", err)
	}
	if payload.FullName == "" {
		payload.FullName = fullName
	}
	if payload.DefaultBranch == "" {
		payload.DefaultBranch = "main"
	}
	if payload.Visibility == "" {
		if payload.Private {
			payload.Visibility = "private"
		} else {
			payload.Visibility = "public"
		}
	}
	return git.Repository{
		FullName:      payload.FullName,
		DefaultBranch: payload.DefaultBranch,
		Visibility:    payload.Visibility,
	}, nil
}

func isGitHubRateLimit(resp *http.Response, body []byte) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	return strings.Contains(strings.ToLower(string(body)), "rate limit")
}

func retryAfterDuration(header http.Header) time.Duration {
	value := strings.TrimSpace(header.Get("Retry-After"))
	if value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
		if when, err := http.ParseTime(value); err == nil {
			if wait := time.Until(when); wait > 0 {
				return wait
			}
		}
	}
	reset := strings.TrimSpace(header.Get("X-RateLimit-Reset"))
	if reset == "" {
		return 0
	}
	seconds, err := strconv.ParseInt(reset, 10, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	if wait := time.Until(time.Unix(seconds, 0)); wait > 0 {
		return wait
	}
	return 0
}
