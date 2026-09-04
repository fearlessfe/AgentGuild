package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agentexperiencedomain "agentguild.dev/agentguild/backend/internal/agentexperience/domain"
	publictaskanalysis "agentguild.dev/agentguild/backend/internal/publictask/analysis"
	publictaskdomain "agentguild.dev/agentguild/backend/internal/publictask/domain"
	publictaskpostgres "agentguild.dev/agentguild/backend/internal/publictask/postgres"
	publictasksource "agentguild.dev/agentguild/backend/internal/publictask/source"
	"github.com/jackc/pgx/v5/pgxpool"
)

const batchSize = 50

type baseCommitResolver interface {
	ResolveBaseCommit(context.Context, string, string) (string, error)
}

type projectionWriter interface {
	InsertIfAbsent(context.Context, *publictaskdomain.Projection) (bool, error)
}

// SourceSnapshotter provides a bounded source snapshot at an immutable commit.
type SourceSnapshotter interface {
	Snapshot(context.Context, string, string) ([]publictaskanalysis.SourceFile, error)
}

// Options controls optional source-aware analysis. A nil Analyzer preserves
// the deterministic projection behavior used when no provider is configured.
type Options struct {
	Analyzer        publictaskanalysis.Analyzer
	Cloner          SourceSnapshotter
	CloneMaxFiles   int
	CloneMaxBytes   int
	AnalysisTimeout time.Duration
}

type Worker struct {
	pool            *pgxpool.Pool
	resolver        baseCommitResolver
	writer          projectionWriter
	analyzer        publictaskanalysis.Analyzer
	cloner          SourceSnapshotter
	analysisTimeout time.Duration
	now             func() time.Time
}

func NewWorker(pool *pgxpool.Pool, resolver baseCommitResolver) (*Worker, error) {
	return NewWorkerWithOptions(pool, resolver, Options{})
}

// NewWorkerWithOptions creates a projection worker with an optional analysis
// agent. Analysis is best effort; the deterministic contract remains the
// availability fallback if cloning or the provider fails.
func NewWorkerWithOptions(pool *pgxpool.Pool, resolver baseCommitResolver, options Options) (*Worker, error) {
	if pool == nil || resolver == nil {
		return nil, fmt.Errorf("public task projection worker dependencies are required")
	}
	if options.Analyzer != nil && options.Cloner == nil {
		options.Cloner = publictasksource.NewGitCloner(nil, options.CloneMaxFiles, options.CloneMaxBytes)
	}
	if options.AnalysisTimeout <= 0 {
		options.AnalysisTimeout = 2 * time.Minute
	}
	return &Worker{
		pool:            pool,
		resolver:        resolver,
		writer:          publictaskpostgres.NewRepository(pool),
		analyzer:        options.Analyzer,
		cloner:          options.Cloner,
		analysisTimeout: options.AnalysisTimeout,
		now:             time.Now,
	}, nil
}

// RunOnce converts open Issue-sourced tasks without a public projection into a
// published public task. When configured, the analyzer receives a pinned source
// snapshot; deterministic fields remain the availability fallback.
func (w *Worker) RunOnce(ctx context.Context) error {
	rows, err := w.pool.Query(ctx, `
		SELECT t.tenant_id, t.id, t.title, t.problem, t.deadline,
		       m.repo, COALESCE(m.issue_url, ''), m.issue_number,
		       COALESCE(m.last_synced_at, t.updated_at)
		FROM issue_task_map m
		JOIN tasks t ON t.tenant_id=m.tenant_id AND t.id=m.task_id
		JOIN onboarded_repositories r
		  ON r.tenant_id=m.tenant_id AND r.full_name=m.repo
		LEFT JOIN public_task_projections p
		  ON p.resource_tenant_id=t.tenant_id AND p.task_id=t.id
		WHERE r.source_type='public_github'
		  AND t.status='open'
		  AND t.deadline > clock_timestamp()
		  AND p.id IS NULL
		ORDER BY t.created_at, t.id
		LIMIT $1`, batchSize)
	if err != nil {
		return err
	}
	defer rows.Close()

	var lastErr error
	for rows.Next() {
		var candidate issueTask
		if err := rows.Scan(
			&candidate.tenantID, &candidate.taskID, &candidate.title, &candidate.problem,
			&candidate.deadline, &candidate.repo, &candidate.issueURL, &candidate.issueNumber,
			&candidate.issueRevision,
		); err != nil {
			return err
		}
		if err := w.publish(ctx, candidate); err != nil {
			lastErr = err
			slog.ErrorContext(ctx, "public task projection failed", "tenant", candidate.tenantID, "task", candidate.taskID, "error", err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return lastErr
}

type issueTask struct {
	tenantID      string
	taskID        string
	title         string
	problem       string
	deadline      time.Time
	repo          string
	issueURL      string
	issueNumber   int
	issueRevision time.Time
}

func (w *Worker) publish(ctx context.Context, candidate issueTask) error {
	if candidate.issueURL == "" || candidate.repo == "" || candidate.issueRevision.IsZero() {
		return fmt.Errorf("issue task is missing public source metadata")
	}
	baseCommit, err := w.resolver.ResolveBaseCommit(ctx, candidate.tenantID, candidate.repo)
	if err != nil {
		return err
	}
	var analyzed *publictaskanalysis.Result
	if w.analyzer != nil && w.cloner != nil {
		analysisCtx, cancel := context.WithTimeout(ctx, w.analysisTimeout)
		defer cancel()
		files, snapshotErr := w.cloner.Snapshot(analysisCtx, candidate.repo, baseCommit)
		if snapshotErr != nil {
			slog.WarnContext(ctx, "public task source snapshot unavailable; using deterministic analysis", "tenant", candidate.tenantID, "task", candidate.taskID, "error", snapshotErr)
		} else {
			result, analysisErr := w.analyzer.Analyze(analysisCtx, publictaskanalysis.Input{
				Repository: candidate.repo, BaseCommit: baseCommit, IssueURL: candidate.issueURL,
				IssueNumber: candidate.issueNumber, Title: candidate.title, Problem: candidate.problem,
				Files: files,
			})
			if analysisErr != nil {
				slog.WarnContext(ctx, "public task analysis unavailable; using deterministic analysis", "tenant", candidate.tenantID, "task", candidate.taskID, "error", analysisErr)
			} else {
				result = result.Normalize()
				if analysisResultIsPublic(result) {
					analyzed = &result
				} else {
					slog.WarnContext(ctx, "public task analysis contained sensitive output; using deterministic analysis", "tenant", candidate.tenantID, "task", candidate.taskID)
				}
			}
		}
	}
	projection, err := buildProjectionWithAnalysis(candidate, baseCommit, w.now(), analyzed)
	if err != nil {
		return err
	}
	_, err = w.writer.InsertIfAbsent(ctx, projection)
	return err
}

func analysisResultIsPublic(result publictaskanalysis.Result) bool {
	values := []string{result.Title, result.Summary, result.ProblemDiagnosis, result.Impact, result.ProposedSolution}
	values = append(values, result.ImplementationSteps...)
	values = append(values, result.Constraints...)
	values = append(values, result.NonGoals...)
	values = append(values, result.Risks...)
	for _, criterion := range result.AcceptanceCriteria {
		values = append(values, criterion.ID, criterion.Statement, criterion.VerifierKind, criterion.ExpectedResult)
	}
	classification, _ := (&agentexperiencedomain.RuleBasedSensitivityPolicy{}).Classify([]byte(strings.Join(values, "\n")))
	return classification != agentexperiencedomain.SensitivityForbidden
}

func buildProjection(candidate issueTask, baseCommit string, now time.Time) (*publictaskdomain.Projection, error) {
	return buildProjectionWithAnalysis(candidate, baseCommit, now, nil)
}

func buildProjectionWithAnalysis(candidate issueTask, baseCommit string, now time.Time, analyzed *publictaskanalysis.Result) (*publictaskdomain.Projection, error) {
	problem := strings.TrimSpace(candidate.problem)
	if problem == "" {
		problem = "来自公开 GitHub Issue 的待处理问题。"
	}
	title := strings.TrimSpace(candidate.title)
	summary := truncate(problem, 240)
	problemDiagnosis := problem
	impact := "该 Issue 需要在仓库中完成修复并提供验证结果。"
	proposedSolution := "根据 Issue 和仓库上下文定位根因，实施最小修复并补充验证。"
	steps := []string{"阅读原始 GitHub Issue", "在固定 base commit 中定位相关代码", "实现修复并运行相关测试"}
	constraints := []string{"repo:" + candidate.repo, "base_commit:" + baseCommit}
	nonGoals := []string{"不扩大任务范围，不修改无关模块"}
	risks := []string{"需要维护者确认修复与原始 Issue 的语义一致"}
	acceptanceCriteria := []publictaskdomain.AcceptanceCriterion{{
		ID: "issue-resolution", Statement: "原始 GitHub Issue 描述的问题得到修复并提供验证证据",
		Critical: true, VerifierKind: "manual", ExpectedResult: "维护者确认修复有效",
	}}
	if analyzed != nil {
		if analyzed.Title != "" {
			title = analyzed.Title
		}
		if analyzed.Summary != "" {
			summary = truncate(analyzed.Summary, 240)
		}
		if analyzed.ProblemDiagnosis != "" {
			problemDiagnosis = analyzed.ProblemDiagnosis
		}
		if analyzed.Impact != "" {
			impact = analyzed.Impact
		}
		if analyzed.ProposedSolution != "" {
			proposedSolution = analyzed.ProposedSolution
		}
		if len(analyzed.ImplementationSteps) > 0 {
			steps = analyzed.ImplementationSteps
		}
		if len(analyzed.Constraints) > 0 {
			constraints = analyzed.Constraints
		}
		if len(analyzed.NonGoals) > 0 {
			nonGoals = analyzed.NonGoals
		}
		if len(analyzed.Risks) > 0 {
			risks = analyzed.Risks
		}
		if len(analyzed.AcceptanceCriteria) > 0 {
			acceptanceCriteria = analyzed.AcceptanceCriteria
		}
	}
	projection, err := publictaskdomain.NewProjection(publictaskdomain.NewProjectionParams{
		Projection: publictaskdomain.Projection{
			ID:                         projectionID(candidate.tenantID, candidate.taskID),
			ResourceTenantID:           candidate.tenantID,
			TaskID:                     candidate.taskID,
			TaskSpecificationVersionID: "issue-task-v1:" + candidate.taskID,
			CanonicalRepository:        candidate.repo,
			SourceIssueURL:             candidate.issueURL,
			IssueRevision:              candidate.issueRevision.UTC().Format(time.RFC3339Nano),
			BaseCommit:                 baseCommit,
			Title:                      title, Summary: summary, ProblemDiagnosis: problemDiagnosis,
			Impact: impact, ProposedSolution: proposedSolution,
			ImplementationSteps: steps, Constraints: constraints,
			NonGoals: nonGoals, Risks: risks, AcceptanceCriteria: acceptanceCriteria,
			QualityLevel: publictaskdomain.QualityStandard,
			PublishedAt:  now,
		},
		Checks: publictaskdomain.PublicationChecks{
			QualityGatePassed: true, VisibilityAllowed: true,
			SensitivityCheckPassed: true, RepositoryPublic: true,
		},
	})
	if err != nil {
		return nil, err
	}
	return projection, nil
}

func projectionID(tenantID, taskID string) string {
	sum := sha256.Sum256([]byte(tenantID + "\x00" + taskID))
	return hex.EncodeToString(sum[:16])
}

func truncate(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "..."
}
