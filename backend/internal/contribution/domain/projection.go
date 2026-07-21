package domain

import "time"

const DefaultAlgorithmVersion = "2026-07-21-v1"

type ProjectionScope string

const (
	ProjectionAgentLifetime ProjectionScope = "agent_lifetime"
	ProjectionAgentVersion  ProjectionScope = "agent_version"
)

type OutcomeCounts struct {
	Attempts         int
	CIPassed         int
	CIFailed         int
	Reviewed         int
	ChangesRequested int
	Approved         int
	Merged           int
	Closed           int
	Reverted         int
	IssueReopened    int
}

type AntiGamingSignals struct {
	DuplicateTaskAttempts      int
	SelfOwnedRepositoryCount   int
	WithoutIndependentFeedback int
	RevertedAfterMerge         int
	IssueReopenedAfterMerge    int
	DominantRepositoryShare    float64
}

// Projection contains explainable counts and rates derived exclusively from
// verified Contribution facts. It deliberately has no commit/LOC/claim score.
type Projection struct {
	Scope                  ProjectionScope
	AgentID                string
	AgentVersionID         string
	AlgorithmVersion       string
	OutcomeCounts          OutcomeCounts
	QualitySampleSize      int
	SampleSizeHint         string
	StableMergeRate        float64
	CIPassRate             float64
	ApprovalRate           float64
	RepositoryDistribution map[string]int
	VersionDistribution    map[string]int
	AntiGaming             AntiGamingSignals
	LatestEventID          int64
	CalculatedAt           time.Time
}

type ContributionFacts struct {
	Contribution Contribution
	Events       []ContributionEvent
}
