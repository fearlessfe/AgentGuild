package application

import (
	"context"
	"sort"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
)

// Allocator selects a reviewer profile for a new review assignment.
type Allocator struct{}

// Allocate picks the active reviewer with the lowest current load that covers
// all requested capabilities. Ties are broken by creation time (older first),
// then by reviewer ID for deterministic ordering.
func (Allocator) Allocate(ctx context.Context, tx application.Tx, tenantID string, caps []string) (string, error) {
	reviewers, err := tx.Reviewers().ListActive(ctx, tenantID, 100)
	if err != nil {
		return "", err
	}

	var candidates []reviewdomain.ReviewerProfile
	for _, r := range reviewers {
		if !r.IsActive {
			continue
		}
		if hasCapabilities(r.Capabilities, caps) {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return "", &domain.Error{Code: "no_reviewer_available", Message: "no active reviewer matches the required capabilities"}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].CurrentLoad != candidates[j].CurrentLoad {
			return candidates[i].CurrentLoad < candidates[j].CurrentLoad
		}
		if candidates[i].CreatedAt.Equal(candidates[j].CreatedAt) {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
	})

	return candidates[0].ID, nil
}

func hasCapabilities(have, want []string) bool {
	if len(want) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(have))
	for _, c := range have {
		set[c] = struct{}{}
	}
	for _, c := range want {
		if _, ok := set[c]; !ok {
			return false
		}
	}
	return true
}
