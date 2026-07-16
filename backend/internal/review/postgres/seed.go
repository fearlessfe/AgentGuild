package postgres

import (
	"context"
	"time"

	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedReviewDefaults inserts a default active rubric version and reviewer profile
// for the supplied tenant if none exist. It is safe to call multiple times.
func SeedReviewDefaults(ctx context.Context, pool *pgxpool.Pool, tenantID, reviewerUserID string) error {
	repo := NewRubricRepository(pool)
	if _, err := repo.GetActive(ctx, tenantID); err != nil {
		version, err := reviewdomain.NewRubricVersion(
			"default-rubric", tenantID, "Default Rubric", 1,
			[]reviewdomain.RubricDimension{{ID: "quality", Name: "Quality"}},
			map[string]float64{"quality": 1.0},
			"2026-07-04-v1", time.Now(),
		)
		if err != nil {
			return err
		}
		if err := repo.CreateVersion(ctx, version); err != nil {
			return err
		}
	}

	reviewerRepo := NewReviewerRepository(pool)
	profiles, err := reviewerRepo.ListActive(ctx, tenantID, 1)
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		if reviewerUserID == "" {
			reviewerUserID = "default-reviewer"
		}
		profile, err := reviewdomain.NewReviewerProfile(
			"default-reviewer", tenantID, reviewerUserID, []string{"default", "code-review"}, time.Now(),
		)
		if err != nil {
			return err
		}
		if err := reviewerRepo.Insert(ctx, profile); err != nil {
			return err
		}
	}
	return nil
}
