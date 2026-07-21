package domain

import "time"

type AgentVersion struct {
	ID                string
	TenantID          string
	AgentID           string
	VersionNumber     int
	Runtime           string
	Model             string
	Capabilities      []string
	ConfigFingerprint string
	CreatedAt         time.Time
}

func NewAgentVersion(
	id, tenantID, agentID string,
	versionNumber int,
	runtime, model string,
	capabilities []string,
	configFingerprint string,
	now time.Time,
) (*AgentVersion, error) {
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
	}
	version, err := NewGlobalAgentVersion(id, agentID, versionNumber, runtime, model, capabilities, configFingerprint, now)
	if err != nil {
		return nil, err
	}
	version.TenantID = tenantID
	return version, nil
}

// NewGlobalAgentVersion creates an immutable version for a platform-global
// Agent identity. TenantID is deliberately left empty; organization and task
// authorization are modeled separately.
func NewGlobalAgentVersion(
	id, agentID string,
	versionNumber int,
	runtime, model string,
	capabilities []string,
	configFingerprint string,
	now time.Time,
) (*AgentVersion, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if agentID == "" {
		return nil, invalidArgument("agent_id")
	}
	if versionNumber < 1 {
		return nil, invalidArgument("version_number")
	}
	if runtime == "" {
		return nil, invalidArgument("runtime")
	}
	if model == "" {
		return nil, invalidArgument("model")
	}
	return &AgentVersion{
		ID:                id,
		AgentID:           agentID,
		VersionNumber:     versionNumber,
		Runtime:           runtime,
		Model:             model,
		Capabilities:      capabilities,
		ConfigFingerprint: configFingerprint,
		CreatedAt:         now,
	}, nil
}
