package domain

import (
	"time"
)

const (
	MinRubricScore = 0
	MaxRubricScore = 100
)

// RubricDimension defines a single dimension evaluated by a rubric.
type RubricDimension struct {
	ID   string
	Name string
}

// RubricVersion is an immutable version of a review rubric.
type RubricVersion struct {
	ID               string
	TenantID         string
	VersionNumber    int
	Name             string
	Dimensions       []RubricDimension
	Weights          map[string]float64
	AlgorithmVersion string
	IsActive         bool
	CreatedAt        time.Time
}

// NewRubricVersion creates a new rubric version after validating its contents.
func NewRubricVersion(
	id, tenantID, name string,
	versionNumber int,
	dimensions []RubricDimension,
	weights map[string]float64,
	algorithmVersion string,
	now time.Time,
) (*RubricVersion, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
	}
	if name == "" {
		return nil, invalidArgument("name")
	}
	if versionNumber < 1 {
		return nil, invalidArgument("version_number")
	}
	if len(dimensions) == 0 {
		return nil, invalidArgument("dimensions")
	}
	seen := make(map[string]struct{}, len(dimensions))
	for _, d := range dimensions {
		if d.ID == "" {
			return nil, invalidArgument("dimension.id")
		}
		if d.Name == "" {
			return nil, invalidArgument("dimension.name")
		}
		if _, ok := seen[d.ID]; ok {
			return nil, invalidArgument("dimensions")
		}
		seen[d.ID] = struct{}{}
	}
	if len(weights) != len(dimensions) {
		return nil, invalidArgument("weights")
	}
	for _, d := range dimensions {
		w, ok := weights[d.ID]
		if !ok {
			return nil, invalidArgument("weights")
		}
		if w <= 0 {
			return nil, invalidArgument("weights")
		}
	}
	if algorithmVersion == "" {
		return nil, invalidArgument("algorithm_version")
	}
	if now.IsZero() {
		return nil, invalidArgument("created_at")
	}
	return &RubricVersion{
		ID:               id,
		TenantID:         tenantID,
		VersionNumber:    versionNumber,
		Name:             name,
		Dimensions:       dimensions,
		Weights:          weights,
		AlgorithmVersion: algorithmVersion,
		IsActive:         true,
		CreatedAt:        now,
	}, nil
}

// Complete reports whether the supplied scores fully cover all dimensions
// of the rubric with valid score values.
func (rv *RubricVersion) Complete(scores []RubricScore) bool {
	if len(scores) != len(rv.Dimensions) {
		return false
	}
	scored := make(map[string]int, len(scores))
	for _, s := range scores {
		if s.Dimension == "" {
			return false
		}
		if s.Score < MinRubricScore || s.Score > MaxRubricScore {
			return false
		}
		scored[s.Dimension] = s.Score
	}
	for _, d := range rv.Dimensions {
		if _, ok := scored[d.ID]; !ok {
			return false
		}
	}
	return true
}
