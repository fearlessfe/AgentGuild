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

// RunOnce processes a single batch of unprojected reviews, merging new signals
// into any existing reputation projections for the same key.
func (w *Worker) RunOnce(ctx context.Context) error {
	return w.store.WithTx(ctx, func(tx application.Tx) error {
		records, err := tx.Reviews().ListUnprojected(ctx, w.batchSize)
		if err != nil {
			return err
		}
		if len(records) == 0 {
			return nil
		}

		byTenantAgent := make(map[string]map[string][]application.ReviewSignalRecord)
		for _, rec := range records {
			agents, ok := byTenantAgent[rec.TenantID]
			if !ok {
				agents = make(map[string][]application.ReviewSignalRecord)
				byTenantAgent[rec.TenantID] = agents
			}
			agents[rec.AgentVersionID] = append(agents[rec.AgentVersionID], rec)
		}

		for tenantID, agents := range byTenantAgent {
			projections := make(map[reputationdomain.ProjectionKey]*reputationdomain.Projection)
			for agentVersionID := range agents {
				existing, err := tx.ListReputationProjectionsByAgentVersion(ctx, tenantID, agentVersionID)
				if err != nil {
					return err
				}
				for _, rec := range existing {
					p := rec.Projection
					projections[p.Key] = &p
				}
			}

			touched := make(map[reputationdomain.ProjectionKey]struct{})
			for _, recs := range agents {
				for _, rec := range recs {
					key := reputationdomain.ProjectionKey{
						AgentVersionID: rec.AgentVersionID,
						Capability:     rec.Capability,
						TaskType:       rec.TaskType,
					}
					p, ok := projections[key]
					if !ok {
						p = reputationdomain.NewProjection(rec.AgentVersionID, rec.Capability, rec.TaskType)
						projections[key] = p
					}
					p.Apply(reputationdomain.ReviewSignal{
						Decision:       rec.Decision,
						CostCents:      rec.CostCents,
						LatencyMs:      rec.LatencyMs,
						Capability:     rec.Capability,
						TaskType:       rec.TaskType,
						AgentVersionID: rec.AgentVersionID,
					})
					touched[key] = struct{}{}
				}
			}

			for key := range touched {
				if err := tx.UpsertReputationProjection(ctx, reputationapp.ProjectionRecord{
					TenantID:   tenantID,
					Projection: *projections[key],
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
