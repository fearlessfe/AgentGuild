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
) (*AgentVersion, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
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
		TenantID:          tenantID,
		AgentID:           agentID,
		VersionNumber:     versionNumber,
		Runtime:           runtime,
		Model:             model,
		Capabilities:      capabilities,
		ConfigFingerprint: configFingerprint,
		CreatedAt:         time.Now(),
	}, nil
}
