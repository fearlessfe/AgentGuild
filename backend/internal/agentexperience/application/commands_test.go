package application_test

import (
	"context"
	"testing"

	"agentguild.dev/agentguild/backend/internal/agentexperience/application"
	"agentguild.dev/agentguild/backend/internal/agentexperience/domain"
	"agentguild.dev/agentguild/backend/internal/agentexperience/postgres"
	avapplication "agentguild.dev/agentguild/backend/internal/agentversion/application"
	avpostgres "agentguild.dev/agentguild/backend/internal/agentversion/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestExtractCandidateFromAcceptedSubmission(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	svc, _, _ := newCandidateService(t, db)
	insertAgent(t, db, "t1", "a1", "owner")

	resp, err := svc.ExtractCandidate(ctx, application.ExtractCandidate{
		TenantID:      "t1",
		AgentID:       "a1",
		SubmissionID:  "sub-1",
		EvidenceBytes: []byte("sha256:evidence"),
		CreatedBy:     "owner",
	})
	require.NoError(t, err)
	require.Equal(t, "sub-1", resp.Candidate.SourceSubmissionID)
	require.Equal(t, []string{"code"}, resp.Candidate.ApplicableCapabilities)
	require.Equal(t, domain.StatusPendingReview, resp.Candidate.Status)
}

func TestExtractCandidateForbiddenAutoRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	svc, _, _ := newCandidateService(t, db)
	insertAgent(t, db, "t1", "a1", "owner")

	resp, err := svc.ExtractCandidate(ctx, application.ExtractCandidate{
		TenantID:      "t1",
		AgentID:       "a1",
		SubmissionID:  "sub-1",
		EvidenceBytes: []byte("password = super-secret"),
		CreatedBy:     "owner",
	})
	require.NoError(t, err)
	require.Equal(t, domain.StatusRejected, resp.Candidate.Status)
	require.NotEmpty(t, resp.Candidate.PolicyReason)
}

func TestReviewCandidate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	svc, _, _ := newCandidateService(t, db)
	insertAgent(t, db, "t1", "a1", "owner")

	resp, err := svc.ExtractCandidate(ctx, application.ExtractCandidate{
		TenantID:      "t1",
		AgentID:       "a1",
		SubmissionID:  "sub-1",
		EvidenceBytes: []byte("sha256:evidence"),
		CreatedBy:     "owner",
	})
	require.NoError(t, err)

	err = svc.ReviewCandidate(ctx, application.ReviewCandidate{
		TenantID:    "t1",
		AgentID:     "a1",
		CandidateID: resp.Candidate.ID,
		Action:      "approve",
		ReviewerID:  "owner",
	})
	require.NoError(t, err)

	summary, err := svc.GetCandidate(ctx, "t1", "a1", resp.Candidate.ID)
	require.NoError(t, err)
	require.Equal(t, string(domain.StatusApproved), summary.Status)
}

func TestApprovedCandidateBoundToNewDraft(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	xpSvc, xpRepo, _ := newCandidateService(t, db)
	_, avRepo, avStore := newVersionService(t, db)
	insertAgent(t, db, "t1", "a1", "owner")

	resp, err := xpSvc.ExtractCandidate(ctx, application.ExtractCandidate{
		TenantID:      "t1",
		AgentID:       "a1",
		SubmissionID:  "sub-1",
		EvidenceBytes: []byte("sha256:experience-memory"),
		CreatedBy:     "owner",
	})
	require.NoError(t, err)
	require.NoError(t, xpSvc.ReviewCandidate(ctx, application.ReviewCandidate{
		TenantID:    "t1",
		AgentID:     "a1",
		CandidateID: resp.Candidate.ID,
		Action:      "approve",
		ReviewerID:  "owner",
	}))

	avSvcWithXP, err := avapplication.NewVersionService(
		avStore, avRepo, fakeEvalProvider{},
		&xpProvider{repo: xpRepo},
		avapplication.NewPolicy(avRepo),
		avapplication.VersionOptions{},
	)
	require.NoError(t, err)

	draftResp, err := avSvcWithXP.CreateDraft(ctx, avapplication.CreateDraft{
		TenantID:              "t1",
		AgentID:               "a1",
		CreatedBy:             "owner",
		Runtime:               "python",
		Model:                 "gpt-4",
		PromptRef:             "sha256:prompt",
		MemoryRef:             "sha256:base-memory",
		ApprovedExperienceIDs: []string{resp.Candidate.ID},
	})
	require.NoError(t, err)
	require.Contains(t, draftResp.Version.MemoryRef, "sha256:base-memory")
	require.Contains(t, draftResp.Version.MemoryRef, resp.Candidate.EvidenceRef)
}

func TestRollbackLeavesOldMemoryRef(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	xpSvc, xpRepo, _ := newCandidateService(t, db)
	avSvc, avRepo, avStore := newVersionService(t, db)
	insertAgent(t, db, "t1", "a1", "owner")

	base, err := avSvc.CreateDraft(ctx, avapplication.CreateDraft{
		TenantID:  "t1",
		AgentID:   "a1",
		CreatedBy: "owner",
		Runtime:   "python",
		Model:     "gpt-4",
		PromptRef: "sha256:prompt",
		MemoryRef: "sha256:base-memory",
	})
	require.NoError(t, err)

	// Promote base to active so it is a valid rollback target later.
	baseEligible, err := avRepo.GetByID(ctx, "t1", "a1", base.Version.ID)
	require.NoError(t, err)
	require.NoError(t, baseEligible.StartEvaluation())
	require.NoError(t, baseEligible.MarkEligible())
	require.NoError(t, avStore.WithTx(ctx, func(tx avapplication.Tx) error {
		return avRepo.UpdateStatus(ctx, tx, baseEligible)
	}))
	require.NoError(t, avSvc.Promote(ctx, avapplication.Promote{
		TenantID:  "t1",
		AgentID:   "a1",
		VersionID: base.Version.ID,
		ActorID:   "owner",
	}))

	resp, err := xpSvc.ExtractCandidate(ctx, application.ExtractCandidate{
		TenantID:      "t1",
		AgentID:       "a1",
		SubmissionID:  "sub-1",
		EvidenceBytes: []byte("sha256:experience-memory"),
		CreatedBy:     "owner",
	})
	require.NoError(t, err)
	require.NoError(t, xpSvc.ReviewCandidate(ctx, application.ReviewCandidate{
		TenantID:    "t1",
		AgentID:     "a1",
		CandidateID: resp.Candidate.ID,
		Action:      "approve",
		ReviewerID:  "owner",
	}))

	avSvcWithXP, err := avapplication.NewVersionService(
		avStore, avRepo, fakeEvalProvider{},
		&xpProvider{repo: xpRepo},
		avapplication.NewPolicy(avRepo),
		avapplication.VersionOptions{},
	)
	require.NoError(t, err)

	newDraft, err := avSvcWithXP.CreateDraft(ctx, avapplication.CreateDraft{
		TenantID:              "t1",
		AgentID:               "a1",
		CreatedBy:             "owner",
		Runtime:               "python",
		Model:                 "gpt-4",
		PromptRef:             "sha256:prompt",
		MemoryRef:             "sha256:base-memory",
		ApprovedExperienceIDs: []string{resp.Candidate.ID},
	})
	require.NoError(t, err)

	// Promote the new draft so the base version becomes retired and rollback is meaningful.
	newEligible, err := avRepo.GetByID(ctx, "t1", "a1", newDraft.Version.ID)
	require.NoError(t, err)
	require.NoError(t, newEligible.StartEvaluation())
	require.NoError(t, newEligible.MarkEligible())
	require.NoError(t, avStore.WithTx(ctx, func(tx avapplication.Tx) error {
		return avRepo.UpdateStatus(ctx, tx, newEligible)
	}))
	require.NoError(t, avSvc.Promote(ctx, avapplication.Promote{
		TenantID:  "t1",
		AgentID:   "a1",
		VersionID: newDraft.Version.ID,
		ActorID:   "owner",
	}))

	require.NoError(t, avSvc.Rollback(ctx, avapplication.Rollback{
		TenantID:  "t1",
		AgentID:   "a1",
		VersionID: base.Version.ID,
		ActorID:   "owner",
	}))

	rolledBack, err := avSvc.GetVersion(ctx, "t1", "a1", base.Version.ID)
	require.NoError(t, err)
	require.Equal(t, "sha256:base-memory", rolledBack.MemoryRef)
	require.NotContains(t, rolledBack.MemoryRef, resp.Candidate.EvidenceRef)
}

type fakeSubmissionStore struct {
	submission *application.Submission
}

func (f *fakeSubmissionStore) GetAcceptedSubmission(ctx context.Context, tenantID, submissionID string) (*application.Submission, error) {
	return f.submission, nil
}

type fakeExecutionStore struct {
	execution *application.Execution
}

func (f *fakeExecutionStore) GetExecution(ctx context.Context, tenantID, executionID string) (*application.Execution, error) {
	return f.execution, nil
}

type fakeAgentOwnerProvider struct{}

func (fakeAgentOwnerProvider) GetAgentOwner(ctx context.Context, tenantID, agentID string) (string, error) {
	return "owner", nil
}

type fakeEvalProvider struct{}

func (fakeEvalProvider) GetLatestPassed(ctx context.Context, tx avapplication.Tx, tenantID, versionID string) (*avapplication.EvaluationRunInfo, error) {
	return &avapplication.EvaluationRunInfo{ID: "run-1", Status: "passed"}, nil
}

type xpProvider struct {
	repo application.ExperienceCandidateRepository
}

func (p *xpProvider) ListApprovedByAgent(ctx context.Context, tenantID, agentID string) ([]avapplication.ExperienceCandidateRef, error) {
	return p.mapRefs(func() ([]domain.ExperienceCandidate, error) {
		return p.repo.ListApprovedByAgent(ctx, tenantID, agentID)
	})
}

func (p *xpProvider) ListApprovedByAgentTx(ctx context.Context, tx avapplication.Tx, tenantID, agentID string) ([]avapplication.ExperienceCandidateRef, error) {
	return p.mapRefs(func() ([]domain.ExperienceCandidate, error) {
		return p.repo.ListApprovedByAgentTx(ctx, tx, tenantID, agentID)
	})
}

func (p *xpProvider) mapRefs(fn func() ([]domain.ExperienceCandidate, error)) ([]avapplication.ExperienceCandidateRef, error) {
	candidates, err := fn()
	if err != nil {
		return nil, err
	}
	refs := make([]avapplication.ExperienceCandidateRef, 0, len(candidates))
	for _, c := range candidates {
		refs = append(refs, avapplication.ExperienceCandidateRef{
			ID:          c.ID,
			EvidenceRef: c.EvidenceRef,
		})
	}
	return refs, nil
}

func newCandidateService(t *testing.T, db *pgxpool.Pool) (*application.CandidateService, application.ExperienceCandidateRepository, *postgres.Store) {
	t.Helper()
	repo := postgres.NewExperienceCandidateRepository(db)
	store := postgres.NewStore(db)
	submission := &application.Submission{
		ID: "sub-1", TenantID: "t1", AgentID: "a1",
		TaskID: "task-1", ReviewID: "rev-1", ExecutionID: "exec-1",
		Status: "accepted",
	}
	execution := &application.Execution{
		ID: "exec-1", TenantID: "t1", AgentID: "a1",
		AgentVersionID: "v1", TaskID: "task-1", Capabilities: []string{"code"},
	}
	svc, err := application.NewCandidateService(
		store, repo,
		&fakeSubmissionStore{submission: submission},
		&fakeExecutionStore{execution: execution},
		application.NewPolicy(fakeAgentOwnerProvider{}),
		domain.NewRuleBasedSensitivityPolicy(),
		application.CandidateOptions{},
	)
	require.NoError(t, err)
	return svc, repo, store
}

func newVersionService(t *testing.T, db *pgxpool.Pool) (*avapplication.VersionService, avapplication.VersionRepository, *avpostgres.Store) {
	t.Helper()
	repo := avpostgres.NewVersionRepository(db)
	store := avpostgres.NewStore(db)
	svc, err := avapplication.NewVersionService(
		store, repo, fakeEvalProvider{}, nil,
		avapplication.NewPolicy(repo),
		avapplication.VersionOptions{},
	)
	require.NoError(t, err)
	return svc, repo, store
}

func insertAgent(t *testing.T, db *pgxpool.Pool, tenantID, agentID, ownerID string) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO agents (tenant_id, id, name, owner_id, owner_email, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'active', clock_timestamp(), clock_timestamp())
		ON CONFLICT (tenant_id, id) DO NOTHING`,
		tenantID, agentID, agentID, ownerID, ownerID+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert agent: %v", err)
	}
}
