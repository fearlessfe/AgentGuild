package application_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/identity/domain"
	"github.com/stretchr/testify/require"
)

func TestNonOwnerCannotSuspendOthersAgent(t *testing.T) {
	fixture := newServiceFixture(t)
	other := fixture.seedActiveAgent(t, "tenant-1", "agent-owned-by-other", "owner-2")

	_, err := fixture.svc.SuspendAgent(context.Background(), application.Principal{
		TenantID: "tenant-1",
		OwnerID:  "owner-1",
	}, application.SuspendAgent{AgentID: other.ID})

	require.ErrorIs(t, err, domain.ErrForbidden)
	require.Equal(t, domain.AgentActive, fixture.store.agents[other.ID].Status)
	require.Empty(t, fixture.store.audits.eventsFor(other.ID, "suspend"))
}

func TestAdminCanSuspendWithinTenantAndWritesAudit(t *testing.T) {
	fixture := newServiceFixture(t)
	agent := fixture.seedActiveAgent(t, "tenant-1", "agent-1", "owner-1")

	got, err := fixture.svc.SuspendAgent(context.Background(), application.Principal{
		TenantID: "tenant-1",
		OwnerID:  "admin-1",
		IsAdmin:  true,
	}, application.SuspendAgent{AgentID: agent.ID, Reason: "maintenance"})

	require.NoError(t, err)
	require.Equal(t, domain.AgentSuspended, got.Data.Status)
	require.Equal(t, domain.AgentSuspended, fixture.store.agents[agent.ID].Status)
	events := fixture.store.audits.eventsFor(agent.ID, "suspend")
	require.Len(t, events, 1)
	require.Equal(t, domain.ActorHuman, events[0].ActorType)
	require.Equal(t, "admin-1", events[0].ActorID)
}

func TestSuspendedAgentCannotRefreshToken(t *testing.T) {
	fixture := newServiceFixture(t)
	agent := fixture.seedActiveAgent(t, "tenant-1", "agent-1", "owner-1")
	_, err := fixture.svc.SuspendAgent(context.Background(), fixture.owner(agent), application.SuspendAgent{AgentID: agent.ID})
	require.NoError(t, err)

	_, err = fixture.svc.IssueAccessToken(context.Background(), fixture.agentPrincipal(agent), application.IssueAccessToken{})

	require.ErrorIs(t, err, domain.ErrStateConflict)
	require.Empty(t, fixture.issuer.issued)
}

func TestTenantBoundaryHidesForeignAgent(t *testing.T) {
	fixture := newServiceFixture(t)
	agent := fixture.seedActiveAgent(t, "tenant-1", "agent-1", "owner-1")

	_, err := fixture.svc.GetAgent(context.Background(), application.Principal{
		TenantID: "tenant-2",
		OwnerID:  agent.OwnerID,
		IsAdmin:  true,
	}, application.GetAgent{AgentID: agent.ID})

	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestAgentSelfCanReadItselfButNotPeer(t *testing.T) {
	fixture := newServiceFixture(t)
	self := fixture.seedActiveAgent(t, "tenant-1", "agent-1", "owner-1")
	peer := fixture.seedActiveAgent(t, "tenant-1", "agent-2", "owner-1")

	got, err := fixture.svc.GetAgent(context.Background(), fixture.agentPrincipal(self), application.GetAgent{AgentID: self.ID})
	require.NoError(t, err)
	require.Equal(t, self.ID, got.Data.ID)

	_, err = fixture.svc.GetAgent(context.Background(), fixture.agentPrincipal(self), application.GetAgent{AgentID: peer.ID})
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestPolicyRequiresScopeRepoAndAllowedStatus(t *testing.T) {
	policy := application.Policy{}
	principal := application.Principal{
		TenantID:  "tenant-1",
		AgentID:   "agent-1",
		Scopes:    []string{"tasks:read"},
		RepoScope: []string{"acme/*", "exact/repo"},
	}

	require.NoError(t, policy.Require(principal, "tasks:read", application.Resource{TenantID: "tenant-1", Repo: "acme/api"}))
	require.NoError(t, policy.Require(principal, "tasks:read", application.Resource{TenantID: "tenant-1", Repo: "exact/repo"}))
	require.ErrorIs(t, policy.Require(principal, "tasks:execute", application.Resource{TenantID: "tenant-1", Repo: "acme/api"}), domain.ErrForbidden)
	require.ErrorIs(t, policy.Require(principal, "tasks:read", application.Resource{TenantID: "tenant-2", Repo: "acme/api"}), domain.ErrForbidden)
	require.ErrorIs(t, policy.Require(principal, "tasks:read", application.Resource{TenantID: "tenant-1", Repo: "other/repo"}), domain.ErrForbidden)

	agent := &domain.Agent{TenantID: "tenant-1", ID: "agent-1", Status: domain.AgentSuspended}
	require.NoError(t, policy.RequireAgentStatus(agent, domain.AgentSuspended))
	require.ErrorIs(t, policy.RequireAgentStatus(agent, domain.AgentActive), domain.ErrStateConflict)
}

type serviceFixture struct {
	svc    *application.IdentityService
	store  *memoryStore
	issuer *recordingIssuer
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	store := newMemoryStore(time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC))
	issuer := &recordingIssuer{}
	svc, err := application.NewIdentityService(store, application.IdentityOptions{
		NewID:       sequenceIDs("generated-agent", "generated-version"),
		TokenIssuer: issuer,
	})
	require.NoError(t, err)
	return &serviceFixture{svc: svc, store: store, issuer: issuer}
}

func (f *serviceFixture) seedActiveAgent(t *testing.T, tenantID, agentID, ownerID string) *domain.Agent {
	t.Helper()
	description := "seeded"
	agent, err := domain.NewAgent(agentID, tenantID, ownerID, ownerID+"@example.com", "team-a", []string{"tasks:read"}, &description)
	require.NoError(t, err)
	agent.Name = agentID
	agent.RepoScope = []string{"acme/*"}
	version, err := domain.NewAgentVersion(agentID+".v1", tenantID, agentID, 1, "go1.26", "gpt-5", []string{"shell"}, "cfg", f.store.now)
	require.NoError(t, err)
	credential, token, err := domain.NewActivationCredential(agent.ID, agent.TenantID, time.Hour)
	require.NoError(t, err)
	require.NoError(t, agent.Activate(credential, version, token, f.store.now))
	agent.Events = nil
	f.store.agents[agent.ID] = cloneAgent(agent)
	f.store.versions[version.ID] = cloneVersion(version)
	return cloneAgent(agent)
}

func (f *serviceFixture) owner(agent *domain.Agent) application.Principal {
	return application.Principal{TenantID: agent.TenantID, OwnerID: agent.OwnerID, OwnerEmail: agent.OwnerEmail}
}

func (f *serviceFixture) agentPrincipal(agent *domain.Agent) application.Principal {
	return application.Principal{TenantID: agent.TenantID, AgentID: agent.ID, AgentVersionID: agent.CurrentVersionID, Scopes: agent.Scopes, RepoScope: agent.RepoScope}
}

type recordingIssuer struct {
	issued []issuedToken
}

func (r *recordingIssuer) IssueAccessToken(_ context.Context, agent *domain.Agent, version *domain.AgentVersion, now time.Time) (application.AccessTokenView, error) {
	view := application.AccessTokenView{
		Token:          "token-for-" + agent.ID,
		TokenType:      "Bearer",
		ExpiresAt:      now.Add(15 * time.Minute),
		AgentID:        agent.ID,
		AgentVersionID: version.ID,
		Scopes:         slices.Clone(agent.Scopes),
		RepoScope:      slices.Clone(agent.RepoScope),
	}
	r.issued = append(r.issued, issuedToken{agentID: agent.ID, versionID: version.ID})
	return view, nil
}

type issuedToken struct {
	agentID   string
	versionID string
}

type memoryStore struct {
	now         time.Time
	agents      map[string]*domain.Agent
	versions    map[string]*domain.AgentVersion
	credentials map[string]*domain.ActivationCredential
	audits      *memoryAuditRepository
}

func newMemoryStore(now time.Time) *memoryStore {
	return &memoryStore{
		now:         now,
		agents:      map[string]*domain.Agent{},
		versions:    map[string]*domain.AgentVersion{},
		credentials: map[string]*domain.ActivationCredential{},
		audits:      &memoryAuditRepository{},
	}
}

func (s *memoryStore) WithTx(ctx context.Context, fn func(application.Tx) error) error {
	return fn(memoryTx{ctx: ctx, store: s})
}

type memoryTx struct {
	ctx   context.Context
	store *memoryStore
}

func (tx memoryTx) Agents() application.AgentRepository {
	return memoryAgentRepository{store: tx.store}
}
func (tx memoryTx) Versions() application.VersionRepository {
	return memoryVersionRepository{store: tx.store}
}
func (tx memoryTx) Credentials() application.CredentialRepository {
	return memoryCredentialRepository{store: tx.store}
}
func (tx memoryTx) Audits() application.AuditRepository    { return tx.store.audits }
func (tx memoryTx) Now(context.Context) (time.Time, error) { return tx.store.now, nil }

type memoryAgentRepository struct{ store *memoryStore }

func (r memoryAgentRepository) Insert(_ context.Context, agent *domain.Agent) error {
	r.store.agents[agent.ID] = cloneAgent(agent)
	return nil
}

func (r memoryAgentRepository) GetByID(_ context.Context, tenantID, agentID string) (*domain.Agent, error) {
	agent := r.store.agents[agentID]
	if agent == nil || agent.TenantID != tenantID {
		return nil, domain.ErrNotFound
	}
	return cloneAgent(agent), nil
}

func (r memoryAgentRepository) Update(_ context.Context, agent *domain.Agent) error {
	if existing := r.store.agents[agent.ID]; existing == nil || existing.TenantID != agent.TenantID {
		return domain.ErrNotFound
	}
	r.store.agents[agent.ID] = cloneAgent(agent)
	return nil
}

func (r memoryAgentRepository) List(_ context.Context, query application.AgentListQuery) ([]domain.Agent, error) {
	var agents []domain.Agent
	for _, agent := range r.store.agents {
		if agent.TenantID != query.TenantID {
			continue
		}
		if query.OwnerID != "" && agent.OwnerID != query.OwnerID {
			continue
		}
		agents = append(agents, *cloneAgent(agent))
	}
	return agents, nil
}

type memoryVersionRepository struct{ store *memoryStore }

func (r memoryVersionRepository) Insert(_ context.Context, version *domain.AgentVersion) error {
	r.store.versions[version.ID] = cloneVersion(version)
	return nil
}

func (r memoryVersionRepository) GetByID(_ context.Context, tenantID, versionID string) (*domain.AgentVersion, error) {
	version := r.store.versions[versionID]
	if version == nil || version.TenantID != tenantID {
		return nil, domain.ErrNotFound
	}
	return cloneVersion(version), nil
}

func (r memoryVersionRepository) ListByAgent(_ context.Context, tenantID, agentID string) ([]domain.AgentVersion, error) {
	var versions []domain.AgentVersion
	for _, version := range r.store.versions {
		if version.TenantID == tenantID && version.AgentID == agentID {
			versions = append(versions, *cloneVersion(version))
		}
	}
	return versions, nil
}

type memoryCredentialRepository struct{ store *memoryStore }

func (r memoryCredentialRepository) Insert(_ context.Context, credential *domain.ActivationCredential) error {
	r.store.credentials[credential.ID] = cloneCredential(credential)
	return nil
}

func (r memoryCredentialRepository) GetPending(_ context.Context, tenantID, agentID string) (*domain.ActivationCredential, error) {
	for _, credential := range r.store.credentials {
		if credential.TenantID == tenantID && credential.AgentID == agentID && credential.Status == domain.ActivationCredentialPending {
			return cloneCredential(credential), nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r memoryCredentialRepository) GetByID(_ context.Context, tenantID, credID string) (*domain.ActivationCredential, error) {
	credential := r.store.credentials[credID]
	if credential == nil || credential.TenantID != tenantID {
		return nil, domain.ErrNotFound
	}
	return cloneCredential(credential), nil
}

func (r memoryCredentialRepository) Save(_ context.Context, credential *domain.ActivationCredential) error {
	if existing := r.store.credentials[credential.ID]; existing == nil || existing.TenantID != credential.TenantID {
		return domain.ErrNotFound
	}
	r.store.credentials[credential.ID] = cloneCredential(credential)
	return nil
}

func (r memoryCredentialRepository) GetPendingByPlaintext(_ context.Context, token string) (*domain.ActivationCredential, error) {
	for _, credential := range r.store.credentials {
		candidate := cloneCredential(credential)
		if candidate.Status == domain.ActivationCredentialPending && candidate.Consume(token, r.store.now) == nil {
			return cloneCredential(credential), nil
		}
	}
	return nil, domain.ErrTokenExpired
}

type memoryAuditRepository struct {
	events []domain.IdentityEvent
}

func (r *memoryAuditRepository) Append(_ context.Context, event domain.IdentityEvent) error {
	r.events = append(r.events, event)
	return nil
}

func (r *memoryAuditRepository) ListByAgent(_ context.Context, tenantID, agentID string, limit int) ([]domain.IdentityEvent, error) {
	var events []domain.IdentityEvent
	for _, event := range r.events {
		if event.TenantID == tenantID && event.AgentID == agentID {
			events = append(events, event)
			if limit > 0 && len(events) == limit {
				break
			}
		}
	}
	return events, nil
}

func (r *memoryAuditRepository) eventsFor(agentID, intent string) []domain.IdentityEvent {
	var events []domain.IdentityEvent
	for _, event := range r.events {
		if event.AgentID == agentID && event.Intent == intent {
			events = append(events, event)
		}
	}
	return events
}

func sequenceIDs(values ...string) func() string {
	next := 0
	return func() string {
		if next >= len(values) {
			return values[len(values)-1]
		}
		value := values[next]
		next++
		return value
	}
}

func cloneAgent(agent *domain.Agent) *domain.Agent {
	if agent == nil {
		return nil
	}
	clone := *agent
	clone.Scopes = slices.Clone(agent.Scopes)
	clone.RepoScope = slices.Clone(agent.RepoScope)
	clone.Events = slices.Clone(agent.Events)
	return &clone
}

func cloneVersion(version *domain.AgentVersion) *domain.AgentVersion {
	if version == nil {
		return nil
	}
	clone := *version
	clone.Capabilities = slices.Clone(version.Capabilities)
	return &clone
}

func cloneCredential(credential *domain.ActivationCredential) *domain.ActivationCredential {
	if credential == nil {
		return nil
	}
	clone := *credential
	clone.Hash = slices.Clone(credential.Hash)
	return &clone
}
