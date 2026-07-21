package application

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	participationdomain "agentguild.dev/agentguild/backend/internal/participation/domain"
	"agentguild.dev/agentguild/backend/internal/publictask/domain"
)

const (
	publicCursorVersion   = 1
	publicCursorNamespace = "public_tasks"
)

type Options struct {
	CursorSecret []byte
	CursorTTL    time.Duration
	Now          func() time.Time
	ClaimStore   ClaimStore
	GrantTTL     time.Duration
	NewID        func() string
}

type Service struct {
	repository Repository
	claims     ClaimStore
	secret     []byte
	ttl        time.Duration
	now        func() time.Time
	grantTTL   time.Duration
	newID      func() string
}

func NewService(repository Repository, options Options) (*Service, error) {
	if repository == nil {
		return nil, invalid("repository")
	}
	if len(options.CursorSecret) < 32 {
		return nil, invalid("cursor_secret")
	}
	if options.CursorTTL <= 0 {
		options.CursorTTL = 15 * time.Minute
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.ClaimStore == nil {
		options.ClaimStore, _ = repository.(ClaimStore)
	}
	if options.GrantTTL <= 0 {
		options.GrantTTL = 24 * time.Hour
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	return &Service{
		repository: repository, claims: options.ClaimStore,
		secret: append([]byte(nil), options.CursorSecret...), ttl: options.CursorTTL,
		now: options.Now, grantTTL: options.GrantTTL, newID: options.NewID,
	}, nil
}

type ListPublicTasks struct {
	Limit              int
	Cursor             string
	AuthenticatedAgent bool
}

type GetPublicTask struct {
	ID                 string
	AuthenticatedAgent bool
}

type Meta struct {
	ServerTime      time.Time `json:"server_time"`
	ResourceVersion int64     `json:"resource_version"`
	NextCursor      string    `json:"next_cursor,omitempty"`
}

type Envelope[T any] struct {
	Data T    `json:"data"`
	Meta Meta `json:"meta"`
}

type TaskSummary struct {
	ID                         string    `json:"id"`
	TaskSpecificationVersionID string    `json:"task_specification_version_id"`
	CanonicalRepository        string    `json:"canonical_repository"`
	SourceIssueURL             string    `json:"source_issue_url"`
	Title                      string    `json:"title"`
	Summary                    string    `json:"summary"`
	QualityLevel               string    `json:"quality_level"`
	PublishedAt                time.Time `json:"published_at"`
	CanClaim                   bool      `json:"can_claim"`
}

type TaskPage struct {
	Items []TaskSummary `json:"items"`
}

type TaskDetail struct {
	TaskSummary
	IssueRevision       string                       `json:"issue_revision"`
	BaseCommit          string                       `json:"base_commit"`
	ProblemDiagnosis    string                       `json:"problem_diagnosis"`
	Impact              string                       `json:"impact"`
	ProposedSolution    string                       `json:"proposed_solution"`
	ImplementationSteps []string                     `json:"implementation_steps"`
	Constraints         []string                     `json:"constraints"`
	NonGoals            []string                     `json:"non_goals"`
	Risks               []string                     `json:"risks"`
	AcceptanceCriteria  []domain.AcceptanceCriterion `json:"acceptance_criteria"`
	EvidenceRefs        []domain.EvidenceRef         `json:"evidence_refs"`
}

type ClaimPublicTask struct {
	RequestID    string
	PublicTaskID string
}

type PublicClaimView struct {
	PublicTaskID               string                   `json:"public_task_id"`
	ExecutionID                string                   `json:"execution_id"`
	AgentID                    string                   `json:"agent_id"`
	AgentVersionID             string                   `json:"agent_version_id"`
	TaskSpecificationVersionID string                   `json:"task_specification_version_id"`
	Status                     string                   `json:"status"`
	LeaseGeneration            int64                    `json:"lease_generation"`
	LeaseSoftExpiresAt         time.Time                `json:"lease_soft_expires_at"`
	LeaseHardExpiresAt         time.Time                `json:"lease_hard_expires_at"`
	ClaimedAt                  time.Time                `json:"claimed_at"`
	Grant                      PublicParticipationGrant `json:"grant"`
}

type PublicParticipationGrant struct {
	ID        string                      `json:"id"`
	Scopes    []participationdomain.Scope `json:"scopes"`
	ExpiresAt time.Time                   `json:"expires_at"`
}

var publicClaimScopes = []participationdomain.Scope{
	participationdomain.ScopeTaskRead,
	participationdomain.ScopeExecutionRead,
	participationdomain.ScopeExecutionWrite,
	participationdomain.ScopeSubmissionCreate,
	participationdomain.ScopeReviewRead,
	participationdomain.ScopeGitWrite,
}

// Claim atomically claims the sponsor-owned Task behind a public projection.
// A platform-global identity is necessary but not sufficient: the store also
// verifies the current Agent and Version status in the same transaction.
func (s *Service) Claim(ctx context.Context, principal auth.Principal, command ClaimPublicTask) (Envelope[PublicClaimView], error) {
	if command.RequestID == "" {
		return Envelope[PublicClaimView]{}, invalid("request_id")
	}
	if command.PublicTaskID == "" {
		return Envelope[PublicClaimView]{}, invalid("id")
	}
	if !principal.IsGlobalAgent() {
		return Envelope[PublicClaimView]{}, domain.ErrForbidden
	}
	if err := (auth.ScopePolicy{}).Require(principal, "tasks:claim"); err != nil {
		return Envelope[PublicClaimView]{}, domain.ErrForbidden
	}
	if s.claims == nil {
		return Envelope[PublicClaimView]{}, domain.ErrForbidden
	}
	payload, err := json.Marshal(struct {
		PublicTaskID   string `json:"public_task_id"`
		AgentID        string `json:"agent_id"`
		AgentVersionID string `json:"agent_version_id"`
	}{command.PublicTaskID, principal.AgentID, principal.AgentVersionID})
	if err != nil {
		return Envelope[PublicClaimView]{}, err
	}
	return s.claims.Claim(ctx, PublicClaimRequest{
		PublicTaskID: command.PublicTaskID, AgentID: principal.AgentID,
		AgentVersionID: principal.AgentVersionID, RequestID: command.RequestID,
		RequestHash: sha256.Sum256(payload), ExecutionID: s.newID(),
		GrantID: s.newID(), OutboxEventID: s.newID(),
		GrantScopes: append([]participationdomain.Scope(nil), publicClaimScopes...),
		GrantTTL:    s.grantTTL,
	})
}

func (s *Service) List(ctx context.Context, query ListPublicTasks) (Envelope[TaskPage], error) {
	limit := query.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return Envelope[TaskPage]{}, invalid("limit")
	}
	now := s.now()
	audience := cursorAudience(query.AuthenticatedAgent)
	pageQuery := PublishedPageQuery{Limit: limit + 1}
	if query.Cursor != "" {
		cursor, err := s.decodeCursor(query.Cursor, audience, now)
		if err != nil {
			return Envelope[TaskPage]{}, err
		}
		pageQuery.AfterPublishedAt = cursor.PublishedAt
		pageQuery.AfterID = cursor.ID
	}
	projections, err := s.repository.ListPublishedPage(ctx, pageQuery)
	if err != nil {
		return Envelope[TaskPage]{}, err
	}
	hasMore := len(projections) > limit
	if hasMore {
		projections = projections[:limit]
	}
	items := make([]TaskSummary, len(projections))
	for index := range projections {
		items[index] = summary(projections[index], query.AuthenticatedAgent)
	}
	result := Envelope[TaskPage]{Data: TaskPage{Items: items}, Meta: Meta{ServerTime: now}}
	if hasMore {
		last := projections[len(projections)-1]
		result.Meta.NextCursor = s.encodeCursor(publicCursor{
			Version: publicCursorVersion, Namespace: publicCursorNamespace,
			Audience: audience, PublishedAt: last.PublishedAt, ID: last.ID,
			ExpiresAt: now.Add(s.ttl),
		})
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, query GetPublicTask) (Envelope[TaskDetail], error) {
	if query.ID == "" {
		return Envelope[TaskDetail]{}, invalid("id")
	}
	projection, err := s.repository.GetByID(ctx, query.ID)
	if err != nil {
		return Envelope[TaskDetail]{}, err
	}
	if projection.Status != domain.StatusPublished {
		return Envelope[TaskDetail]{}, domain.ErrNotFound
	}
	view := TaskDetail{
		TaskSummary:   summary(*projection, query.AuthenticatedAgent),
		IssueRevision: projection.IssueRevision, BaseCommit: projection.BaseCommit,
		ProblemDiagnosis: projection.ProblemDiagnosis, Impact: projection.Impact,
		ProposedSolution:    projection.ProposedSolution,
		ImplementationSteps: append([]string(nil), projection.ImplementationSteps...),
		Constraints:         append([]string(nil), projection.Constraints...),
		NonGoals:            append([]string(nil), projection.NonGoals...), Risks: append([]string(nil), projection.Risks...),
		AcceptanceCriteria: append([]domain.AcceptanceCriterion(nil), projection.AcceptanceCriteria...),
		EvidenceRefs:       append([]domain.EvidenceRef(nil), projection.EvidenceRefs...),
	}
	return Envelope[TaskDetail]{Data: view, Meta: Meta{ServerTime: s.now()}}, nil
}

func summary(projection domain.Projection, authenticated bool) TaskSummary {
	return TaskSummary{
		ID: projection.ID, TaskSpecificationVersionID: projection.TaskSpecificationVersionID,
		CanonicalRepository: projection.CanonicalRepository, SourceIssueURL: projection.SourceIssueURL,
		Title: projection.Title, Summary: projection.Summary, QualityLevel: projection.QualityLevel,
		PublishedAt: projection.PublishedAt, CanClaim: authenticated,
	}
}

type publicCursor struct {
	Version     int       `json:"v"`
	Namespace   string    `json:"n"`
	Audience    string    `json:"a"`
	PublishedAt time.Time `json:"p"`
	ID          string    `json:"i"`
	ExpiresAt   time.Time `json:"e"`
}

func (s *Service) encodeCursor(payload publicCursor) string {
	body, _ := json.Marshal(payload)
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) decodeCursor(value, audience string, now time.Time) (publicCursor, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return publicCursor{}, invalid("cursor")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return publicCursor{}, invalid("cursor")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return publicCursor{}, invalid("cursor")
	}
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write(body)
	var payload publicCursor
	if !hmac.Equal(signature, mac.Sum(nil)) || json.Unmarshal(body, &payload) != nil ||
		payload.Version != publicCursorVersion || payload.Namespace != publicCursorNamespace ||
		payload.Audience != audience || payload.ID == "" || payload.PublishedAt.IsZero() || !now.Before(payload.ExpiresAt) {
		return publicCursor{}, invalid("cursor")
	}
	return payload, nil
}

func cursorAudience(authenticated bool) string {
	if authenticated {
		return "agent"
	}
	return "anonymous"
}

func invalid(field string) error {
	return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
