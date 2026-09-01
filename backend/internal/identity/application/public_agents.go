package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/domain"
)

type PublicAgentRepository interface {
	ListPublic(context.Context, string, string, int) ([]PublicAgentView, error)
	GetPublic(context.Context, string, string) (PublicAgentView, error)
}

type PublicAgentService struct {
	repository     PublicAgentRepository
	organizationID string
}

type ListPublicAgents struct {
	Status string
	Limit  int
}

type GetPublicAgent struct {
	AgentID string
}

type PublicAgentPage struct {
	Items []PublicAgentView `json:"items"`
}

type PublicAgentView struct {
	AgentID        string     `json:"agent_id"`
	AgentVersionID string     `json:"agent_version_id"`
	Handle         string     `json:"handle"`
	DisplayName    string     `json:"display_name"`
	Description    string     `json:"description,omitempty"`
	Status         string     `json:"status"`
	OrganizationID string     `json:"organization_id"`
	Runtime        string     `json:"runtime,omitempty"`
	Model          string     `json:"model,omitempty"`
	Capabilities   []string   `json:"capabilities,omitempty"`
	LastSeenAt     *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func NewPublicAgentService(repository PublicAgentRepository, organizationID string) (*PublicAgentService, error) {
	if repository == nil {
		return nil, domain.ErrInvalidArgument
	}
	if organizationID == "" {
		organizationID = "public"
	}
	return &PublicAgentService{repository: repository, organizationID: organizationID}, nil
}

func (s *PublicAgentService) List(ctx context.Context, query ListPublicAgents) (Envelope[PublicAgentPage], error) {
	if query.Limit == 0 {
		query.Limit = 24
	}
	if query.Limit < 1 || query.Limit > 100 {
		return Envelope[PublicAgentPage]{}, invalid("limit")
	}
	items, err := s.repository.ListPublic(ctx, s.organizationID, query.Status, query.Limit)
	if err != nil {
		return Envelope[PublicAgentPage]{}, err
	}
	if items == nil {
		items = []PublicAgentView{}
	}
	return Envelope[PublicAgentPage]{Data: PublicAgentPage{Items: items}, Meta: Meta{ServerTime: time.Now()}}, nil
}

func (s *PublicAgentService) Get(ctx context.Context, query GetPublicAgent) (Envelope[PublicAgentView], error) {
	if query.AgentID == "" {
		return Envelope[PublicAgentView]{}, invalid("agent_id")
	}
	item, err := s.repository.GetPublic(ctx, s.organizationID, query.AgentID)
	if err != nil {
		return Envelope[PublicAgentView]{}, err
	}
	return Envelope[PublicAgentView]{Data: item, Meta: Meta{ServerTime: time.Now()}}, nil
}
