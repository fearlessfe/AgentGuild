package main

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
	evaluationapp "agentguild.dev/agentguild/backend/internal/evaluation/application"
	evaluationpostgres "agentguild.dev/agentguild/backend/internal/evaluation/postgres"
	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	gitpostgres "agentguild.dev/agentguild/backend/internal/git/postgres"
	"agentguild.dev/agentguild/backend/internal/git/validation"
	gitworker "agentguild.dev/agentguild/backend/internal/git/worker"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	identitypostgres "agentguild.dev/agentguild/backend/internal/identity/postgres"
	"agentguild.dev/agentguild/backend/internal/postgres"
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
	service, err := application.NewService(postgres.NewStore(pool), application.Options{
		CursorSecret: []byte(cfg.CursorSecret),
	})
	if err != nil {
		return err
	}
	verifier, identityService, oidcProvider, err := buildIdentityRuntime(cfg, pool)
	if err != nil {
		return err
	}

	versionService, evaluationService, experienceService, err := buildVersionExperienceRuntime(pool)
	if err != nil {
		return err
	}

	gitRuntime, gitAppManager, err := buildGitRuntime(cfg, pool, service)
	if err != nil {
		return err
	}

	var ruleService *syncapp.RuleService
	var syncEngine *syncapp.Engine
	var manifestService *gitapp.ManifestService
	if gitAppManager != nil {
		ruleRepo := syncpostgres.NewRuleRepository(pool)
		mapRepo := syncpostgres.NewMapRepository(pool)
		ruleService, err = syncapp.NewRuleService(ruleRepo, syncapp.RuleServiceOptions{})
		if err != nil {
			return err
		}
		syncEngine = syncapp.NewEngine(ruleRepo, mapRepo, syncTaskSink{service: service, pool: pool}, gitAppManager, syncapp.EngineOptions{
			DefaultDeadline: cfg.SyncDefaultDeadline,
		})
		if cfg.WebEnabled {
			manifestService = gitapp.NewManifestService(gitAppManager, gitapp.ManifestOptions{
				PublicBaseURL: cfg.GitHubAppPublicBaseURL,
				StateSecret:   []byte(cfg.GitHubAppManifestStateSecret),
			})
		}
	}

	restOptions := make([]resttransport.Option, 0, 13)
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

	reviewSvc, err := reviewapp.NewService(postgres.NewStore(pool), reviewapp.SyntheticDiffProvider{}, reviewapp.AlwaysPassValidationProvider{}, reviewapp.Options{})
	if err != nil {
		return err
	}
	reputationSvc := application.NewReputationQueryService(postgres.NewStore(pool))

	restOptions = append(restOptions,
		resttransport.WithReviewService(reviewSvc),
		resttransport.WithRubricService(reviewSvc),
		resttransport.WithReputationService(reputationSvc),
	)
	if gitRuntime != nil {
		restOptions = append(restOptions,
			resttransport.WithSubmissionService(gitRuntime.submissionService),
			resttransport.WithCredentialService(gitRuntime.credentialService),
		)
	}
	restHandler := resttransport.NewServer(service, verifier, restOptions...).Router()

	mcpOptions := make([]mcptransport.Option, 0, 7)
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
		if err := reviewpostgres.SeedReviewDefaults(ctx, pool, cfg.ReviewSeedTenantID); err != nil {
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

func buildVersionExperienceRuntime(pool *pgxpool.Pool) (*agentversionapp.VersionService, *evaluationapp.EvaluationService, *agentexperienceapp.CandidateService, error) {
	avStore := agentversionpostgres.NewStore(pool)
	versionRepo := agentversionpostgres.NewVersionRepository(pool)
	evalProvider := agentversionpostgres.NewEvaluationRunProvider(pool)
	xpProvider := agentversionpostgres.NewExperienceCandidateProvider(pool)
	avPolicy := agentversionapp.NewPolicy(versionRepo)
	versionService, err := agentversionapp.NewVersionService(avStore, versionRepo, evalProvider, xpProvider, avPolicy, agentversionapp.VersionOptions{})
	if err != nil {
		return nil, nil, nil, err
	}

	evStore := evaluationpostgres.NewStore(pool)
	bsRepo := evaluationpostgres.NewBenchmarkSetRepository(pool)
	runRepo := evaluationpostgres.NewEvaluationRunRepository(pool)
	versionLifecycle := agentversionpostgres.NewVersionLifecycleAdapter(pool)
	executor := evaluationapp.NewFixedBenchmarkExecutor()
	evPolicy := evaluationapp.NewPolicy(versionRepo)
	evaluationService, err := evaluationapp.NewEvaluationService(evStore, bsRepo, runRepo, versionLifecycle, executor, evPolicy, evaluationapp.EvaluationOptions{})
	if err != nil {
		return nil, nil, nil, err
	}

	axStore := agentexperiencepostgres.NewStore(pool)
	candidateRepo := agentexperiencepostgres.NewExperienceCandidateRepository(pool)
	submissionStore := agentexperienceapp.NewFixedSubmissionStore(nil)
	executionStore := agentexperienceapp.NewFixedExecutionStore(nil)
	classifier := agentexperiencedomain.NewRuleBasedSensitivityPolicy()
	axPolicy := agentexperienceapp.NewPolicy(versionRepo)
	experienceService, err := agentexperienceapp.NewCandidateService(axStore, candidateRepo, submissionStore, executionStore, axPolicy, classifier, agentexperienceapp.CandidateOptions{})
	if err != nil {
		return nil, nil, nil, err
	}

	return versionService, evaluationService, experienceService, nil
}

func costProvider(enabled bool, cfg config.Config) telemetry.TraceCostProvider {
	if !enabled {
		return disabledCostProvider{}
	}
	return telemetry.NewLangfuseProvider(telemetry.LangfuseConfig{BaseURL: cfg.LangfuseBaseURL, PublicKey: cfg.LangfusePublicKey, SecretKey: cfg.LangfuseSecretKey, Mode: cfg.LangfuseMode, SupportsCost: cfg.LangfuseSupportsCost, MetricsPath: cfg.LangfuseMetricsPath, CompleteCoverageTag: cfg.LangfuseCompleteTag}, &http.Client{Timeout: 10 * time.Second})
}

func buildIdentityRuntime(cfg config.Config, pool *pgxpool.Pool) (auth.TokenVerifier, *identityapp.IdentityService, *auth.OIDCProvider, error) {
	externalVerifier := auth.NewJWKSVerifier(cfg.OAuthIssuer, cfg.OAuthAudience, cfg.OAuthJWKSURL, nil)
	if !cfg.WebEnabled {
		return externalVerifier, nil, nil, nil
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
	localVerifier := auth.NewRS256Verifier(&privateKey.PublicKey, auth.TokenVerifierConfig{
		Issuer:   cfg.OAuthIssuer,
		Audience: cfg.OAuthAudience,
	})
	return chainedTokenVerifier{verifiers: []auth.TokenVerifier{localVerifier, externalVerifier}}, identityService, oidcProvider, nil
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
	credentialService *gitapp.CredentialService
	submissionService *gitapp.SubmissionService
	validationWorker  *gitworker.ValidationWorker
}

func buildGitRuntime(cfg config.Config, pool *pgxpool.Pool, service *application.Service) (*gitRuntime, gitapp.GitHubAppManager, error) {
	gitStore := gitpostgres.NewStore(pool)
	gitAppRepo := gitpostgres.NewGitHubAppRepository(pool)
	gitAppManager, err := gitapp.NewGitHubAppManager(gitAppRepo)
	if err != nil {
		return nil, nil, fmt.Errorf("build github app service: %w", err)
	}

	ctx := context.Background()
	if cfg.GitHub.AppID != 0 && cfg.GitHub.PrivateKey != "" && cfg.GitHub.InstallationID != 0 {
		seedTenant := cfg.OIDCTenantID
		if seedTenant == "" {
			seedTenant = cfg.LocalAdmin.TenantID
		}
		if seedTenant != "" {
			if _, err := gitAppRepo.GetByTenant(ctx, seedTenant); err != nil {
				if errors.Is(err, git.ErrGitHubAppNotConfigured) {
					if upsertErr := gitAppManager.Upsert(ctx, gitapp.UpsertGitHubApp{
						TenantID:       seedTenant,
						Provider:       "github",
						AppID:          cfg.GitHub.AppID,
						InstallationID: cfg.GitHub.InstallationID,
						PrivateKey:     cfg.GitHub.PrivateKey,
						BaseURL:        cfg.GitHub.BaseURL,
					}); upsertErr != nil {
						return nil, nil, fmt.Errorf("seed github app configuration: %w", upsertErr)
					}
					slog.Info("seeded github app configuration from environment", "tenant", seedTenant)
				} else {
					return nil, nil, fmt.Errorf("check existing github app configuration: %w", err)
				}
			}
		}
	}

	credentialService, err := gitapp.NewCredentialService(gitStore, gitAppManager, gitapp.Options{
		Provider: "github",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("build credential service: %w", err)
	}

	verifier := gitapp.NewCommitVerifier(gitAppManager, &submissionRepoAdapter{store: gitStore})
	notifier := application.NewCoreExecutionNotifier(service)
	submissionService, err := gitapp.NewSubmissionService(gitStore, verifier, notifier, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build submission service: %w", err)
	}

	runner := validation.NewRunner(validation.DefaultRegistry(), &validation.TempWorkspaceFactory{}, nil)
	validationWorker := gitworker.NewValidationWorker(
		gitStore,
		"validation-worker",
		cfg.ValidationLease,
		cfg.ValidationMaxAttempts,
		runner,
		notifier,
	)

	return &gitRuntime{
		credentialService: credentialService,
		submissionService: submissionService,
		validationWorker:  validationWorker,
	}, gitAppManager, nil
}

func runValidationWorker(ctx context.Context, w *gitworker.ValidationWorker, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT tenant_id
		FROM validation_jobs
		WHERE status IN ('pending','running')
		LIMIT 100`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var lastErr error
	for rows.Next() {
		var tenantID string
		if err := rows.Scan(&tenantID); err != nil {
			lastErr = err
			continue
		}
		if _, err := w.RunOnce(ctx, tenantID); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			lastErr = err
		}
	}
	if err := rows.Err(); err != nil {
		lastErr = err
	}
	return lastErr
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
