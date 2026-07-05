package domain

// VersionStatus is the lifecycle state of an AgentVersion.
type VersionStatus string

const (
	StatusDraft       VersionStatus = "draft"
	StatusEvaluating  VersionStatus = "evaluating"
	StatusEligible    VersionStatus = "eligible"
	StatusActive      VersionStatus = "active"
	StatusRetired     VersionStatus = "retired"
	StatusRejected    VersionStatus = "rejected"
)

// CanRollbackTo reports whether a version in the given state may become the
// current production version through a rollback. Rollback does not mutate the
// state of the version being rolled back to, nor the version being rolled back
// from.
func CanRollbackTo(status VersionStatus) bool {
	switch status {
	case StatusActive, StatusEligible, StatusRetired:
		return true
	default:
		return false
	}
}
