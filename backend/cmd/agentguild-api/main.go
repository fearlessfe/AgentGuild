package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/config"
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
	service, err := application.NewService(postgres.NewStore(pool), application.Options{CursorSecret: []byte(cfg.CursorSecret)})
	if err != nil {
		return err
	}
	verifier := auth.NewJWKSVerifier(cfg.OAuthIssuer, cfg.OAuthAudience, cfg.OAuthJWKSURL, nil)
	restHandler := resttransport.NewServer(service, verifier).Router()
	mcpHandler := mcptransport.NewServer(service, verifier).Handler()
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
		wg.Wait()
		slog.Info("workers stopped, shutting down server")
		return server.Shutdown(shutdown)
	case err := <-errCh:
		cancelWorkers()
		wg.Wait()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func costProvider(enabled bool, cfg config.Config) telemetry.TraceCostProvider {
	if !enabled {
		return disabledCostProvider{}
	}
	return telemetry.NewLangfuseProvider(telemetry.LangfuseConfig{BaseURL: cfg.LangfuseBaseURL, PublicKey: cfg.LangfusePublicKey, SecretKey: cfg.LangfuseSecretKey, Mode: cfg.LangfuseMode, SupportsCost: cfg.LangfuseSupportsCost, MetricsPath: cfg.LangfuseMetricsPath, CompleteCoverageTag: cfg.LangfuseCompleteTag}, &http.Client{Timeout: 10 * time.Second})
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

func repeat(ctx context.Context, interval time.Duration, name string, fn func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := fn(ctx); err != nil && ctx.Err() == nil {
			slog.Error(name+" iteration failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
