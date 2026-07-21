package application

import (
	"encoding/json"
	"sort"
	"time"

	"agentguild.dev/agentguild/backend/internal/contribution/domain"
)

type ProjectionSet struct {
	AgentLifetime []domain.Projection
	AgentVersions []domain.Projection
}

type Projector struct {
	algorithmVersion string
}

func NewProjector(algorithmVersion string) (*Projector, error) {
	if algorithmVersion == "" {
		return nil, domain.ErrInvalidArgument
	}
	return &Projector{algorithmVersion: algorithmVersion}, nil
}

// Project rebuilds both levels from immutable facts. Each outcome is counted
// at most once per Contribution, so webhook volume cannot inflate reputation.
func (p *Projector) Project(facts []domain.ContributionFacts, calculatedAt time.Time) (ProjectionSet, error) {
	if calculatedAt.IsZero() {
		return ProjectionSet{}, domain.ErrInvalidArgument
	}
	agents := make(map[string]*projectionAccumulator)
	versions := make(map[string]*projectionAccumulator)
	for _, item := range facts {
		if item.Contribution.AttributionStatus != domain.AttributionVerified {
			continue
		}
		if item.Contribution.AgentID == "" || item.Contribution.AgentVersionID == "" {
			return ProjectionSet{}, domain.ErrInvalidArgument
		}
		agent := agents[item.Contribution.AgentID]
		if agent == nil {
			agent = newAccumulator(domain.ProjectionAgentLifetime, item.Contribution.AgentID, "", p.algorithmVersion, calculatedAt)
			agents[item.Contribution.AgentID] = agent
		}
		versionKey := item.Contribution.AgentID + "\x00" + item.Contribution.AgentVersionID
		version := versions[versionKey]
		if version == nil {
			version = newAccumulator(domain.ProjectionAgentVersion, item.Contribution.AgentID, item.Contribution.AgentVersionID, p.algorithmVersion, calculatedAt)
			versions[versionKey] = version
		}
		agent.apply(item)
		version.apply(item)
	}

	set := ProjectionSet{
		AgentLifetime: finalizeAccumulators(agents),
		AgentVersions: finalizeAccumulators(versions),
	}
	return set, nil
}

type projectionAccumulator struct {
	projection     domain.Projection
	taskCounts     map[string]int
	ciObserved     int
	reviewObserved int
	stableMerges   int
}

func newAccumulator(scope domain.ProjectionScope, agentID, versionID, algorithmVersion string, calculatedAt time.Time) *projectionAccumulator {
	return &projectionAccumulator{
		projection: domain.Projection{
			Scope:                  scope,
			AgentID:                agentID,
			AgentVersionID:         versionID,
			AlgorithmVersion:       algorithmVersion,
			RepositoryDistribution: make(map[string]int),
			VersionDistribution:    make(map[string]int),
			CalculatedAt:           calculatedAt,
		},
		taskCounts: make(map[string]int),
	}
}

func (a *projectionAccumulator) apply(item domain.ContributionFacts) {
	c := item.Contribution
	a.projection.OutcomeCounts.Attempts++
	a.projection.RepositoryDistribution[c.CanonicalRepository]++
	a.projection.VersionDistribution[c.AgentVersionID]++
	a.taskCounts[c.TaskID]++
	if c.SelfOwnedRepository {
		a.projection.AntiGaming.SelfOwnedRepositoryCount++
	}

	seen := make(map[domain.Outcome]bool)
	independentFeedback := false
	for _, event := range item.Events {
		seen[event.Outcome] = true
		if event.ID > a.projection.LatestEventID {
			a.projection.LatestEventID = event.ID
		}
		if independentMaintainerFeedback(event.Payload) {
			independentFeedback = true
		}
	}
	counts := &a.projection.OutcomeCounts
	if seen[domain.OutcomeCIPassed] {
		counts.CIPassed++
	}
	if seen[domain.OutcomeCIFailed] {
		counts.CIFailed++
	}
	if seen[domain.OutcomeReviewed] {
		counts.Reviewed++
	}
	if seen[domain.OutcomeChangesRequested] {
		counts.ChangesRequested++
	}
	if seen[domain.OutcomeApproved] {
		counts.Approved++
	}
	if seen[domain.OutcomeMerged] {
		counts.Merged++
	}
	if seen[domain.OutcomeClosed] {
		counts.Closed++
	}
	if seen[domain.OutcomeReverted] {
		counts.Reverted++
	}
	if seen[domain.OutcomeIssueReopened] {
		counts.IssueReopened++
	}
	if seen[domain.OutcomeCIPassed] || seen[domain.OutcomeCIFailed] {
		a.ciObserved++
	}
	if seen[domain.OutcomeReviewed] || seen[domain.OutcomeChangesRequested] || seen[domain.OutcomeApproved] {
		a.reviewObserved++
	}
	if !independentFeedback {
		a.projection.AntiGaming.WithoutIndependentFeedback++
	}
	if seen[domain.OutcomeMerged] && seen[domain.OutcomeReverted] {
		a.projection.AntiGaming.RevertedAfterMerge++
	}
	if seen[domain.OutcomeMerged] && seen[domain.OutcomeIssueReopened] {
		a.projection.AntiGaming.IssueReopenedAfterMerge++
	}
	if seen[domain.OutcomeMerged] && !seen[domain.OutcomeReverted] && !seen[domain.OutcomeIssueReopened] {
		a.stableMerges++
	}
	if seen[domain.OutcomeMerged] || seen[domain.OutcomeClosed] || seen[domain.OutcomeReverted] || seen[domain.OutcomeIssueReopened] {
		a.projection.QualitySampleSize++
	}
}

func (a *projectionAccumulator) finalize() domain.Projection {
	for _, count := range a.taskCounts {
		if count > 1 {
			a.projection.AntiGaming.DuplicateTaskAttempts += count - 1
		}
	}
	attempts := a.projection.OutcomeCounts.Attempts
	if attempts > 0 {
		largest := 0
		for _, count := range a.projection.RepositoryDistribution {
			if count > largest {
				largest = count
			}
		}
		a.projection.AntiGaming.DominantRepositoryShare = float64(largest) / float64(attempts)
	}
	terminal := a.projection.QualitySampleSize
	if terminal > 0 {
		a.projection.StableMergeRate = float64(a.stableMerges) / float64(terminal)
	}
	if a.ciObserved > 0 {
		a.projection.CIPassRate = float64(a.projection.OutcomeCounts.CIPassed) / float64(a.ciObserved)
	}
	if a.reviewObserved > 0 {
		a.projection.ApprovalRate = float64(a.projection.OutcomeCounts.Approved) / float64(a.reviewObserved)
	}
	a.projection.SampleSizeHint = sampleSizeHint(terminal)
	if a.projection.Scope == domain.ProjectionAgentVersion {
		a.projection.VersionDistribution = map[string]int{}
	}
	return a.projection
}

func finalizeAccumulators(groups map[string]*projectionAccumulator) []domain.Projection {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]domain.Projection, 0, len(keys))
	for _, key := range keys {
		result = append(result, groups[key].finalize())
	}
	return result
}

func independentMaintainerFeedback(payload json.RawMessage) bool {
	var value struct {
		IndependentMaintainer bool `json:"independent_maintainer"`
	}
	return json.Unmarshal(payload, &value) == nil && value.IndependentMaintainer
}

func sampleSizeHint(size int) string {
	switch {
	case size == 0:
		return "unverified"
	case size < 5:
		return "low"
	case size < 20:
		return "medium"
	default:
		return "high"
	}
}
