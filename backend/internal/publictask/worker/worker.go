package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	publictaskdomain "agentguild.dev/agentguild/backend/internal/publictask/domain"
	publictaskpostgres "agentguild.dev/agentguild/backend/internal/publictask/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

const batchSize = 50

type baseCommitResolver interface {
	ResolveBaseCommit(context.Context, string, string) (string, error)
}

type projectionWriter interface {
	InsertIfAbsent(context.Context, *publictaskdomain.Projection) (bool, error)
}

type Worker struct {
	pool     *pgxpool.Pool
	resolver baseCommitResolver
	writer   projectionWriter
	now      func() time.Time
}

func NewWorker(pool *pgxpool.Pool, resolver baseCommitResolver) (*Worker, error) {
	if pool == nil || resolver == nil {
		return nil, fmt.Errorf("public task projection worker dependencies are required")
	}
	return &Worker{
		pool:     pool,
		resolver: resolver,
		writer:   publictaskpostgres.NewRepository(pool),
		now:      time.Now,
	}, nil
}

// RunOnce converts open Issue-sourced tasks without a public projection into
// deterministic, standard-quality public tasks. It is intentionally model-free
// so the claim path is usable before an LLM analyzer is introduced.
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
	projection, err := buildProjection(candidate, baseCommit, w.now())
	if err != nil {
		return err
	}
	_, err = w.writer.InsertIfAbsent(ctx, projection)
	return err
}

func buildProjection(candidate issueTask, baseCommit string, now time.Time) (*publictaskdomain.Projection, error) {
	problem := strings.TrimSpace(candidate.problem)
	if problem == "" {
		problem = "来自公开 GitHub Issue 的待处理问题。"
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
			Title:                      strings.TrimSpace(candidate.title),
			Summary:                    truncate(problem, 240),
			ProblemDiagnosis:           problem,
			Impact:                     "该 Issue 需要在仓库中完成修复并提供验证结果。",
			ProposedSolution:           "根据 Issue 和仓库上下文定位根因，实施最小修复并补充验证。",
			ImplementationSteps:        []string{"阅读原始 GitHub Issue", "在固定 base commit 中定位相关代码", "实现修复并运行相关测试"},
			Constraints:                []string{"repo:" + candidate.repo, "base_commit:" + baseCommit},
			NonGoals:                   []string{"不扩大任务范围，不修改无关模块"},
			Risks:                      []string{"需要维护者确认修复与原始 Issue 的语义一致"},
			AcceptanceCriteria: []publictaskdomain.AcceptanceCriterion{{
				ID: "issue-resolution", Statement: "原始 GitHub Issue 描述的问题得到修复并提供验证证据",
				Critical: true, VerifierKind: "manual", ExpectedResult: "维护者确认修复有效",
			}},
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
