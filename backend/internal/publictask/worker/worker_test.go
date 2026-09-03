package worker

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	publictaskapp "agentguild.dev/agentguild/backend/internal/publictask/application"
	publictaskdomain "agentguild.dev/agentguild/backend/internal/publictask/domain"
	publictaskpostgres "agentguild.dev/agentguild/backend/internal/publictask/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
)

func TestBuildProjectionCreatesClaimableTaskSpecification(t *testing.T) {
	now := time.Date(2026, 9, 4, 1, 2, 3, 0, time.UTC)
	projection, err := buildProjection(issueTask{
		tenantID: "tenant-1", taskID: "task-1", title: "Fix timeout handling",
		problem: "Requests time out under load.", deadline: now.Add(24 * time.Hour),
		repo: "acme/service", issueURL: "https://github.com/acme/service/issues/1",
		issueNumber: 1, issueRevision: now,
	}, "0123456789abcdef0123456789abcdef01234567", now)

	if err != nil {
		t.Fatalf("buildProjection() error = %v", err)
	}
	if projection.Status != publictaskdomain.StatusPublished {
		t.Fatalf("projection status = %q, want published", projection.Status)
	}
	if projection.TaskSpecificationVersionID != "issue-task-v1:task-1" {
		t.Fatalf("spec version = %q", projection.TaskSpecificationVersionID)
	}
	if len(projection.AcceptanceCriteria) != 1 {
		t.Fatalf("acceptance criteria = %d, want 1", len(projection.AcceptanceCriteria))
	}
	if projection.BaseCommit != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("base commit = %q", projection.BaseCommit)
	}
}

func TestProjectionIDIsStablePerTenantAndTask(t *testing.T) {
	first := projectionID("tenant-1", "task-1")
	second := projectionID("tenant-1", "task-1")
	otherTenant := projectionID("tenant-2", "task-1")

	if first != second {
		t.Fatalf("projection ID is not stable: %q != %q", first, second)
	}
	if first == otherTenant {
		t.Fatalf("projection ID ignores tenant: %q", first)
	}
}

func TestWorkerRunOncePublishesMappedPublicIssue(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 4, 1, 2, 3, 0, time.UTC)
	_, err := db.Exec(ctx, `
		INSERT INTO tasks (
			tenant_id, id, publisher_agent_version_id, type, title, problem, deadline, status
		) VALUES (
			'tenant-public', 'task-public-1', 'publisher-version', 'bug',
			'Fix public timeout', 'Requests time out under load.', $1, 'open'
		)`, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
	_, err = db.Exec(ctx, `
		INSERT INTO onboarded_repositories (
			tenant_id, id, source_type, full_name, default_branch, visibility
		) VALUES ('tenant-public', 'repo-1', 'public_github', 'acme/service', 'main', 'public')`)
	if err != nil {
		t.Fatalf("insert repository: %v", err)
	}
	_, err = db.Exec(ctx, `
		INSERT INTO issue_task_map (
			tenant_id, repo, issue_number, task_id, issue_state, issue_url, last_synced_at
		) VALUES ('tenant-public', 'acme/service', 7, 'task-public-1', 'open',
			'https://github.com/acme/service/issues/7', $1)`, now)
	if err != nil {
		t.Fatalf("insert issue mapping: %v", err)
	}

	resolver := &recordingBaseCommitResolver{commit: "0123456789abcdef0123456789abcdef01234567"}
	worker, err := NewWorker(db, resolver)
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	worker.now = func() time.Time { return now }
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("second RunOnce() error = %v", err)
	}
	if resolver.calls != 1 {
		t.Fatalf("ResolveBaseCommit calls = %d, want 1", resolver.calls)
	}

	projection, err := publictaskpostgres.NewRepository(db).GetByID(ctx, projectionID("tenant-public", "task-public-1"))
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if projection.Status != publictaskdomain.StatusPublished || projection.BaseCommit != resolver.commit {
		t.Fatalf("projection = %#v, want published projection at %s", projection, resolver.commit)
	}
}

type recordingBaseCommitResolver struct {
	commit string
	calls  int
}

func (r *recordingBaseCommitResolver) ResolveBaseCommit(context.Context, string, string) (string, error) {
	r.calls++
	return r.commit, nil
}

func TestWorkerProjectionIsListedAsClaimableAndAcceptedByClaimService(t *testing.T) {
	now := time.Date(2026, 9, 4, 1, 2, 3, 0, time.UTC)
	projection, err := buildProjection(issueTask{
		tenantID: "tenant-1", taskID: "task-1", title: "Fix timeout handling",
		problem: "Requests time out under load.", deadline: now.Add(24 * time.Hour),
		repo: "acme/service", issueURL: "https://github.com/acme/service/issues/1",
		issueNumber: 1, issueRevision: now,
	}, "0123456789abcdef0123456789abcdef01234567", now)
	if err != nil {
		t.Fatalf("buildProjection() error = %v", err)
	}

	claims := &recordingClaimStore{}
	service, err := publictaskapp.NewService(&projectionRepository{projection: projection}, publictaskapp.Options{
		CursorSecret: []byte("0123456789abcdef0123456789abcdef"),
		ClaimStore:   claims,
		Now:          func() time.Time { return now },
		NewID:        sequenceIDs("execution-1", "grant-1", "outbox-1"),
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	listed, err := service.List(context.Background(), publictaskapp.ListPublicTasks{AuthenticatedAgent: true})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed.Data.Items) != 1 || !listed.Data.Items[0].CanClaim {
		t.Fatalf("listed tasks = %#v, want one claimable task", listed.Data.Items)
	}

	principal := auth.Principal{
		SubjectID: "agent:agent-1", IdentityScope: auth.IdentityScopeGlobal,
		Type: auth.PrincipalTypeAgent, AgentID: "agent-1", AgentVersionID: "version-1",
		Scopes: []string{"tasks:claim"},
	}
	claimed, err := service.Claim(context.Background(), principal, publictaskapp.ClaimPublicTask{
		RequestID: "request-1", PublicTaskID: projection.ID,
	})
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if claimed.Data.ExecutionID != "execution-1" || claims.request.PublicTaskID != projection.ID {
		t.Fatalf("claim result = %#v, request = %#v", claimed.Data, claims.request)
	}
}

type projectionRepository struct {
	projection *publictaskdomain.Projection
}

func (r *projectionRepository) Insert(context.Context, *publictaskdomain.Projection) error {
	return nil
}
func (r *projectionRepository) Update(context.Context, *publictaskdomain.Projection) error {
	return nil
}
func (r *projectionRepository) GetByID(_ context.Context, id string) (*publictaskdomain.Projection, error) {
	if r.projection != nil && r.projection.ID == id {
		copy := *r.projection
		return &copy, nil
	}
	return nil, publictaskdomain.ErrNotFound
}
func (r *projectionRepository) ListPublished(context.Context, int) ([]publictaskdomain.Projection, error) {
	return []publictaskdomain.Projection{*r.projection}, nil
}
func (r *projectionRepository) ListPublishedPage(context.Context, publictaskapp.PublishedPageQuery) ([]publictaskdomain.Projection, error) {
	return []publictaskdomain.Projection{*r.projection}, nil
}

type recordingClaimStore struct {
	request publictaskapp.PublicClaimRequest
}

func (s *recordingClaimStore) Claim(_ context.Context, request publictaskapp.PublicClaimRequest) (publictaskapp.Envelope[publictaskapp.PublicClaimView], error) {
	s.request = request
	return publictaskapp.Envelope[publictaskapp.PublicClaimView]{Data: publictaskapp.PublicClaimView{
		PublicTaskID: request.PublicTaskID, ExecutionID: request.ExecutionID,
		AgentID: request.AgentID, AgentVersionID: request.AgentVersionID,
		Status: "leased",
	}}, nil
}

func sequenceIDs(ids ...string) func() string {
	index := 0
	return func() string {
		value := ids[index]
		index++
		return value
	}
}
