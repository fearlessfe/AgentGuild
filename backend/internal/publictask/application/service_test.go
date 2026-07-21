package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	participationdomain "agentguild.dev/agentguild/backend/internal/participation/domain"
	"agentguild.dev/agentguild/backend/internal/publictask/domain"
	"github.com/stretchr/testify/require"
)

func TestServicePublicCursorIsSignedNamespacedAndAudienceBound(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	repository := &memoryRepository{items: []domain.Projection{
		projection("public-1", now.Add(-time.Minute)),
		projection("public-2", now.Add(-2*time.Minute)),
		projection("public-3", now.Add(-3*time.Minute)),
	}}
	service, err := NewService(repository, Options{
		CursorSecret: []byte("01234567890123456789012345678901"),
		CursorTTL:    time.Hour, Now: func() time.Time { return now },
	})
	require.NoError(t, err)

	first, err := service.List(context.Background(), ListPublicTasks{Limit: 2, AuthenticatedAgent: true})
	require.NoError(t, err)
	require.Len(t, first.Data.Items, 2)
	require.True(t, first.Data.Items[0].CanClaim)
	require.NotEmpty(t, first.Meta.NextCursor)

	second, err := service.List(context.Background(), ListPublicTasks{Limit: 2, Cursor: first.Meta.NextCursor, AuthenticatedAgent: true})
	require.NoError(t, err)
	require.Len(t, second.Data.Items, 1)
	require.Equal(t, "public-3", second.Data.Items[0].ID)

	_, err = service.List(context.Background(), ListPublicTasks{Limit: 2, Cursor: first.Meta.NextCursor})
	require.ErrorIs(t, err, domain.ErrInvalidArgument, "agent cursor must not become an anonymous cursor")

	tenantCursor := signedCursor(t, service.secret, map[string]any{
		"v": 1, "n": "tenant_tasks", "a": "agent", "p": now.Add(-time.Minute),
		"i": "task-1", "e": now.Add(time.Hour),
	})
	_, err = service.List(context.Background(), ListPublicTasks{Cursor: tenantCursor, AuthenticatedAgent: true})
	require.ErrorIs(t, err, domain.ErrInvalidArgument)
}

func TestServiceViewsNeverExposeResourceTenant(t *testing.T) {
	now := time.Now().UTC()
	repository := &memoryRepository{items: []domain.Projection{projection("public-1", now)}}
	service, err := NewService(repository, Options{CursorSecret: []byte("01234567890123456789012345678901"), Now: func() time.Time { return now }})
	require.NoError(t, err)

	list, err := service.List(context.Background(), ListPublicTasks{})
	require.NoError(t, err)
	body, err := json.Marshal(list)
	require.NoError(t, err)
	require.NotContains(t, string(body), "tenant-sponsor")
	require.NotContains(t, string(body), "resource_tenant")
	require.False(t, list.Data.Items[0].CanClaim)

	detail, err := service.Get(context.Background(), GetPublicTask{ID: "public-1"})
	require.NoError(t, err)
	body, err = json.Marshal(detail)
	require.NoError(t, err)
	require.NotContains(t, string(body), "tenant-sponsor")
}

func TestServiceClaimRequiresGlobalAgentAndTasksClaimScope(t *testing.T) {
	now := time.Now().UTC()
	claims := &recordingClaimStore{result: Envelope[PublicClaimView]{
		Data: PublicClaimView{PublicTaskID: "public-1", ExecutionID: "execution-1"},
		Meta: Meta{ServerTime: now},
	}}
	ids := []string{"execution-1", "grant-1", "outbox-1"}
	service, err := NewService(&memoryRepository{}, Options{
		CursorSecret: []byte("01234567890123456789012345678901"),
		ClaimStore:   claims, GrantTTL: time.Hour,
		NewID: func() string {
			id := ids[0]
			ids = ids[1:]
			return id
		},
	})
	require.NoError(t, err)

	tenantPrincipal := auth.Principal{
		Type: auth.PrincipalTypeAgent, TenantID: "tenant-1",
		AgentID: "agent-1", AgentVersionID: "version-1",
		Scopes: []string{"tasks:claim"},
	}
	_, err = service.Claim(context.Background(), tenantPrincipal, ClaimPublicTask{RequestID: "request-1", PublicTaskID: "public-1"})
	require.ErrorIs(t, err, domain.ErrForbidden)

	globalPrincipal := auth.Principal{
		SubjectID: auth.AgentSubject("agent-1"), IdentityScope: auth.IdentityScopeGlobal,
		Type: auth.PrincipalTypeAgent, AgentID: "agent-1", AgentVersionID: "version-1",
	}
	_, err = service.Claim(context.Background(), globalPrincipal, ClaimPublicTask{RequestID: "request-1", PublicTaskID: "public-1"})
	require.ErrorIs(t, err, domain.ErrForbidden)

	globalPrincipal.Scopes = []string{"tasks:claim"}
	result, err := service.Claim(context.Background(), globalPrincipal, ClaimPublicTask{RequestID: "request-1", PublicTaskID: "public-1"})
	require.NoError(t, err)
	require.Equal(t, "execution-1", result.Data.ExecutionID)
	require.Equal(t, "agent-1", claims.request.AgentID)
	require.Equal(t, "version-1", claims.request.AgentVersionID)
	require.Equal(t, []participationdomain.Scope{
		participationdomain.ScopeTaskRead,
		participationdomain.ScopeExecutionRead,
		participationdomain.ScopeExecutionWrite,
		participationdomain.ScopeSubmissionCreate,
		participationdomain.ScopeReviewRead,
		participationdomain.ScopeGitWrite,
	}, claims.request.GrantScopes)
}

type recordingClaimStore struct {
	request PublicClaimRequest
	result  Envelope[PublicClaimView]
}

func (s *recordingClaimStore) Claim(_ context.Context, request PublicClaimRequest) (Envelope[PublicClaimView], error) {
	s.request = request
	return s.result, nil
}

type memoryRepository struct{ items []domain.Projection }

func (r *memoryRepository) Insert(context.Context, *domain.Projection) error { return nil }
func (r *memoryRepository) Update(context.Context, *domain.Projection) error { return nil }
func (r *memoryRepository) GetByID(_ context.Context, id string) (*domain.Projection, error) {
	for _, item := range r.items {
		if item.ID == id {
			copy := item
			return &copy, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *memoryRepository) ListPublished(ctx context.Context, limit int) ([]domain.Projection, error) {
	return r.ListPublishedPage(ctx, PublishedPageQuery{Limit: limit})
}
func (r *memoryRepository) ListPublishedPage(_ context.Context, query PublishedPageQuery) ([]domain.Projection, error) {
	start := 0
	if query.AfterID != "" {
		for index, item := range r.items {
			if item.ID == query.AfterID && item.PublishedAt.Equal(query.AfterPublishedAt) {
				start = index + 1
				break
			}
		}
	}
	end := start + query.Limit
	if end > len(r.items) {
		end = len(r.items)
	}
	return append([]domain.Projection(nil), r.items[start:end]...), nil
}

func projection(id string, publishedAt time.Time) domain.Projection {
	return domain.Projection{
		ID: id, ResourceTenantID: "tenant-sponsor", TaskID: "task-1",
		TaskSpecificationVersionID: "spec-1", CanonicalRepository: "acme/widgets",
		SourceIssueURL: "https://github.com/acme/widgets/issues/1", IssueRevision: "r1",
		BaseCommit: "1111111111111111111111111111111111111111",
		Title:      "Fix widget", Summary: "Public summary", ProblemDiagnosis: "Diagnosis",
		Impact: "Impact", ProposedSolution: "Solution", Status: domain.StatusPublished,
		QualityLevel: domain.QualityStandard, PublishedAt: publishedAt,
	}
}

func signedCursor(t *testing.T, secret []byte, payload any) string {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
