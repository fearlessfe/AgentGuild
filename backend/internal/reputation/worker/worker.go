// Package worker implements the asynchronous reputation projection worker.
package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
)

// Worker scans submitted reviews and recomputes reputation projections.
type Worker struct {
	store     application.Store
	projector *reputationapp.Projector
	interval  time.Duration
	batchSize int
	logger    *slog.Logger
}

// NewWorker creates a reputation worker.
func NewWorker(store application.Store, interval time.Duration, batchSize int, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	if batchSize <= 0 {
		batchSize = 10
	}
	return &Worker{
		store:     store,
		projector: reputationapp.NewProjector(),
		interval:  interval,
		batchSize: batchSize,
		logger:    logger,
	}
}

// Run starts the worker loop. It stops when ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				w.logger.Error("reputation worker batch failed", "error", err)
			}
		}
	}
}

// RunOnce processes a single batch of unprojected reviews.
func (w *Worker) RunOnce(ctx context.Context) error {
	return w.store.WithTx(ctx, func(tx application.Tx) error {
		records, err := tx.Reviews().ListUnprojected(ctx, w.batchSize)
		if err != nil {
			return err
		}
		if len(records) == 0 {
			return nil
		}

		byTenant := make(map[string][]reputationdomain.ReviewSignal)
		for _, rec := range records {
			byTenant[rec.TenantID] = append(byTenant[rec.TenantID], reputationdomain.ReviewSignal{
				Decision:       rec.Decision,
				CostCents:      rec.CostCents,
				LatencyMs:      rec.LatencyMs,
				Capability:     rec.Capability,
				TaskType:       rec.TaskType,
				AgentVersionID: rec.AgentVersionID,
			})
		}

		for tenantID, signals := range byTenant {
			projections, err := w.projector.Project(ctx, signals)
			if err != nil {
				return err
			}
			for _, p := range projections {
				if err := tx.UpsertReputationProjection(ctx, reputationapp.ProjectionRecord{
					TenantID:   tenantID,
					Projection: p,
				}); err != nil {
					return err
				}
			}
		}

		for _, rec := range records {
			if err := tx.Reviews().MarkProjected(ctx, rec.TenantID, rec.ReviewID); err != nil {
				return err
			}
		}
		return nil
	})
}
