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

	agentversionapp "agentguild.dev/agentguild/backend/internal/agentversion/application"
	agentversionpostgres "agentguild.dev/agentguild/backend/internal/agentversion/postgres"
	agentexperienceapp "agentguild.dev/agentguild/backend/internal/agentexperience/application"
	agentexperiencedomain "agentguild.dev/agentguild/backend/internal/agentexperience/domain"
	agentexperiencepostgres "agentguild.dev/agentguild/backend/internal/agentexperience/postgres"
	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/config"
	evaluationapp "agentguild.dev/agentguild/backend/internal/evaluation/application"
	evaluationpostgres "agentguild.dev/agentguild/backend/internal/evaluation/postgres"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	identitypostgres "agentguild.dev/agentguild/backend/internal/identity/postgres"
	"agentguild.dev/agentguild/backend/internal/postgres"
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

	restOptions := make([]resttransport.Option, 0, 6)
	if identityService != nil {
		restOptions = append(restOptions, resttransport.WithIdentityService(identityService), resttransport.WithSession(cfg.SessionCookieSecret, cfg.SessionCookieSecure))
	}
	if oidcProvider != nil {
		restOptions = append(restOptions, resttransport.WithOIDCProvider(oidcProvider))
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
	restHandler := resttransport.NewServer(service, verifier, restOptions...).Router()
	mcpOptions := make([]mcptransport.Option, 0, 3)
	if versionService != nil {
		mcpOptions = append(mcpOptions, mcptransport.WithVersionService(versionService))
	}
	if evaluationService != nil {
		mcpOptions = append(mcpOptions, mcptransport.WithEvaluationService(evaluationService))
	}
	if experienceService != nil {
		mcpOptions = append(mcpOptions, mcptransport.WithExperienceService(experienceService))
	}
	mcpHandler := mcptransport.NewServer(service, verifier, mcpOptions...).Handler()
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: adapterHandler(cfg.WebEnabled, cfg.MCPEnabled, restHandler, mcpHandler), ReadHeaderTimeout: 5 * time.Second}

	workerCtx, cancelWorkers := context.WithCancel(ctx)
	var wg sync.WaitGroup
	reaper := postgres.NewReaper(pool)
	runWorker(workerCtx, &wg, cfg.ReaperInterval, "reaper", func(ctx context.Context) error { _, err := reaper.RunBatch(ctx, 100); return err })
	provider := costProvider(cfg.LangfuseEnabled, cfg)
	outbox := worker.NewOutbox(pool, provider)
	runWorker(workerCtx, &wg, cfg.OutboxInterval, "outbox", outbox.RunOnce)

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
	oidcProvider, err := auth.NewOIDCProvider(auth.OIDCConfig{
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
