package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	avapplication "agentguild.dev/agentguild/backend/internal/agentversion/application"
	"agentguild.dev/agentguild/backend/internal/evaluation/application"
	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// benchmarkSetRepository persists benchmark sets and their task references.
type benchmarkSetRepository struct {
	q queryer
}

// evaluationRunRepository persists evaluation runs and their per-task results.
type evaluationRunRepository struct {
	q queryer
}

// NewBenchmarkSetRepository returns a BenchmarkSetRepository backed by pool.
func NewBenchmarkSetRepository(pool *pgxpool.Pool) application.BenchmarkSetRepository {
	return &benchmarkSetRepository{q: pool}
}

// NewEvaluationRunRepository returns an EvaluationRunRepository backed by pool.
func NewEvaluationRunRepository(pool *pgxpool.Pool) application.EvaluationRunRepository {
	return &evaluationRunRepository{q: pool}
}

// BenchmarkSetRepository methods

func (r *benchmarkSetRepository) Create(ctx context.Context, tx application.Tx, bs *domain.BenchmarkSet) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO benchmark_sets (
			id, tenant_id, version_number, name, description,
			is_active, created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		bs.ID(), bs.TenantID(), bs.VersionNumber(), bs.Name(), bs.Description(),
		bs.IsActive(), bs.CreatedBy(), bs.CreatedAt(),
	)
	if err != nil {
		return err
	}
	for _, task := range bs.Tasks() {
		_, err := tx.Exec(ctx, `
			INSERT INTO benchmark_set_tasks (
				tenant_id, benchmark_set_id, task_ref, ordering
			) VALUES ($1, $2, $3, $4)`,
			bs.TenantID(), bs.ID(), task.TaskRef, task.Ordering,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *benchmarkSetRepository) GetByID(ctx context.Context, tenantID, id string) (*domain.BenchmarkSet, error) {
	row := r.q.QueryRow(ctx, `
		SELECT id, tenant_id, version_number, name, description,
		       is_active, created_by, created_at
		FROM benchmark_sets
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, id,
	)
	bs, err := scanBenchmarkSet(row)
	if err != nil {
		return nil, err
	}
	tasks, err := r.listTasks(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return bs.WithTasks(tasks), nil
}

func (r *benchmarkSetRepository) ListByTenant(ctx context.Context, tenantID string) ([]domain.BenchmarkSet, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, tenant_id, version_number, name, description,
		       is_active, created_by, created_at
		FROM benchmark_sets
		WHERE tenant_id=$1
		ORDER BY version_number DESC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sets []domain.BenchmarkSet
	for rows.Next() {
		bs, err := scanBenchmarkSet(rows)
		if err != nil {
			return nil, err
		}
		sets = append(sets, *bs)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range sets {
		tasks, err := r.listTasks(ctx, tenantID, sets[i].ID())
		if err != nil {
			return nil, err
		}
		sets[i] = *sets[i].WithTasks(tasks)
	}
	return sets, nil
}

func (r *benchmarkSetRepository) listTasks(ctx context.Context, tenantID, benchmarkSetID string) ([]domain.BenchmarkTask, error) {
	rows, err := r.q.Query(ctx, `
		SELECT task_ref, ordering
		FROM benchmark_set_tasks
		WHERE tenant_id=$1 AND benchmark_set_id=$2
		ORDER BY ordering ASC, task_ref ASC`,
		tenantID, benchmarkSetID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []domain.BenchmarkTask
	for rows.Next() {
		var task domain.BenchmarkTask
		if err := rows.Scan(&task.TaskRef, &task.Ordering); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (r *benchmarkSetRepository) SetActive(ctx context.Context, tx application.Tx, tenantID, id string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE benchmark_sets
		SET is_active=true
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *benchmarkSetRepository) SetInactiveAll(ctx context.Context, tx application.Tx, tenantID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE benchmark_sets
		SET is_active=false
		WHERE tenant_id=$1`,
		tenantID,
	)
	return err
}

func (r *benchmarkSetRepository) GetActiveByTenant(ctx context.Context, tenantID string) (*domain.BenchmarkSet, error) {
	row := r.q.QueryRow(ctx, `
		SELECT id, tenant_id, version_number, name, description,
		       is_active, created_by, created_at
		FROM benchmark_sets
		WHERE tenant_id=$1 AND is_active=true
		ORDER BY version_number DESC
		LIMIT 1`,
		tenantID,
	)
	bs, err := scanBenchmarkSet(row)
	if err != nil {
		return nil, err
	}
	tasks, err := r.listTasks(ctx, tenantID, bs.ID())
	if err != nil {
		return nil, err
	}
	return bs.WithTasks(tasks), nil
}

func (r *benchmarkSetRepository) NextVersionNumber(ctx context.Context, tx application.Tx, tenantID string) (int, error) {
	var versionNumber int
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version_number), 0) + 1
		FROM benchmark_sets
		WHERE tenant_id=$1`,
		tenantID,
	).Scan(&versionNumber)
	return versionNumber, err
}

var _ application.BenchmarkSetRepository = (*benchmarkSetRepository)(nil)

// NewEvaluationRunProvider returns an agentversion.EvaluationRunProvider backed
// by pool. It is a separate constructor so the agentversion module can depend
// on evaluation without importing the concrete repository type.
func NewEvaluationRunProvider(pool *pgxpool.Pool) avapplication.EvaluationRunProvider {
	return &evaluationRunRepository{q: pool}
}

// EvaluationRunRepository methods

func (r *evaluationRunRepository) Create(ctx context.Context, tx application.Tx, run *domain.EvaluationRun) error {
	thresholdJSON, err := json.Marshal(run.ThresholdResults())
	if err != nil {
		return err
	}
	summaryJSON, err := json.Marshal(run.Summary())
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO evaluation_runs (
			id, tenant_id, agent_version_id, benchmark_set_id, status,
			environment_digest, scoring_rule_version, threshold_results, summary,
			started_at, completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		run.ID(), run.TenantID(), run.AgentVersionID(), run.BenchmarkSetID(), string(run.Status()),
		run.EnvironmentDigest(), run.ScoringRuleVersion(), thresholdJSON, summaryJSON,
		run.StartedAt(), nil,
	)
	return err
}

func (r *evaluationRunRepository) GetByID(ctx context.Context, tenantID, id string) (*domain.EvaluationRun, error) {
	row := r.q.QueryRow(ctx, `
		SELECT id, tenant_id, agent_version_id, benchmark_set_id, status,
		       environment_digest, scoring_rule_version, threshold_results, summary,
		       started_at, completed_at
		FROM evaluation_runs
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, id,
	)
	return scanEvaluationRun(row)
}

func (r *evaluationRunRepository) ListByAgentVersion(ctx context.Context, tenantID, agentVersionID string) ([]domain.EvaluationRun, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, tenant_id, agent_version_id, benchmark_set_id, status,
		       environment_digest, scoring_rule_version, threshold_results, summary,
		       started_at, completed_at
		FROM evaluation_runs
		WHERE tenant_id=$1 AND agent_version_id=$2
		ORDER BY started_at DESC`,
		tenantID, agentVersionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []domain.EvaluationRun
	for rows.Next() {
		run, err := scanEvaluationRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *run)
	}
	return runs, rows.Err()
}

func (r *evaluationRunRepository) Complete(ctx context.Context, tx application.Tx, run *domain.EvaluationRun) error {
	thresholdJSON, err := json.Marshal(run.ThresholdResults())
	if err != nil {
		return err
	}
	summaryJSON, err := json.Marshal(run.Summary())
	if err != nil {
		return err
	}
	completedAt := run.CompletedAt()
	if completedAt == nil {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		completedAt = &now
	}
	tag, err := tx.Exec(ctx, `
		UPDATE evaluation_runs
		SET status=$4,
		    threshold_results=$5,
		    summary=$6,
		    completed_at=$7
		WHERE tenant_id=$1 AND id=$2 AND status=$3`,
		run.TenantID(), run.ID(), string(domain.StatusRunning), string(run.Status()),
		thresholdJSON, summaryJSON, *completedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrStateConflict
	}
	return nil
}

func (r *evaluationRunRepository) CreateResult(ctx context.Context, tx application.Tx, result *domain.EvaluationRunResult) error {
	detailsJSON, err := json.Marshal(result.Details)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO evaluation_run_results (
			tenant_id, evaluation_run_id, task_ref, score, passed, details
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		result.TenantID, result.EvaluationRunID, result.TaskRef, result.Score, result.Passed, detailsJSON,
	)
	return err
}

func (r *evaluationRunRepository) ListResults(ctx context.Context, tenantID, evaluationRunID string) ([]domain.EvaluationRunResult, error) {
	rows, err := r.q.Query(ctx, `
		SELECT tenant_id, evaluation_run_id, task_ref, score, passed, details
		FROM evaluation_run_results
		WHERE tenant_id=$1 AND evaluation_run_id=$2
		ORDER BY task_ref ASC`,
		tenantID, evaluationRunID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.EvaluationRunResult
	for rows.Next() {
		var result domain.EvaluationRunResult
		var detailsJSON []byte
		if err := rows.Scan(
			&result.TenantID, &result.EvaluationRunID, &result.TaskRef,
			&result.Score, &result.Passed, &detailsJSON,
		); err != nil {
			return nil, err
		}
		if len(detailsJSON) > 0 {
			_ = json.Unmarshal(detailsJSON, &result.Details)
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

// GetLatestPassed implements the agentversion EvaluationRunProvider interface.
func (r *evaluationRunRepository) GetLatestPassed(
	ctx context.Context,
	tx avapplication.Tx,
	tenantID, versionID string,
) (*avapplication.EvaluationRunInfo, error) {
	var id, status string
	err := tx.QueryRow(ctx, `
		SELECT id, status
		FROM evaluation_runs
		WHERE tenant_id=$1 AND agent_version_id=$2 AND status='passed'
		ORDER BY started_at DESC
		LIMIT 1`,
		tenantID, versionID,
	).Scan(&id, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &avapplication.EvaluationRunInfo{ID: id, Status: status}, nil
}

var _ application.EvaluationRunRepository = (*evaluationRunRepository)(nil)
var _ avapplication.EvaluationRunProvider = (*evaluationRunRepository)(nil)

// Benchmark-set helpers

type benchmarkSetScanner interface {
	Scan(...any) error
}

func scanBenchmarkSet(row benchmarkSetScanner) (*domain.BenchmarkSet, error) {
	var id, tenantID, name, description, createdBy string
	var versionNumber int
	var isActive bool
	var createdAt time.Time
	if err := row.Scan(
		&id, &tenantID, &versionNumber, &name, &description,
		&isActive, &createdBy, &createdAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	bs, err := domain.NewBenchmarkSetWithTasks(
		id, tenantID, createdBy, versionNumber,
		name, description, nil, createdAt,
	)
	if err != nil {
		return nil, err
	}
	if isActive {
		bs.SetActive()
	}
	return bs, nil
}

// Evaluation-run helpers

type evaluationRunScanner interface {
	Scan(...any) error
}

func scanEvaluationRun(row evaluationRunScanner) (*domain.EvaluationRun, error) {
	var id, tenantID, agentVersionID, benchmarkSetID, status, envDigest, scoringRule string
	var thresholdJSON, summaryJSON []byte
	var startedAt time.Time
	var completedAt *time.Time
	if err := row.Scan(
		&id, &tenantID, &agentVersionID, &benchmarkSetID, &status,
		&envDigest, &scoringRule, &thresholdJSON, &summaryJSON,
		&startedAt, &completedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	var thresholdResults []domain.ThresholdResult
	if len(thresholdJSON) > 0 {
		if err := json.Unmarshal(thresholdJSON, &thresholdResults); err != nil {
			return nil, err
		}
	}
	var summary domain.EvaluationSummary
	if len(summaryJSON) > 0 {
		if err := json.Unmarshal(summaryJSON, &summary); err != nil {
			return nil, err
		}
	}

	run, err := domain.NewEvaluationRun(
		id, tenantID, agentVersionID, benchmarkSetID,
		envDigest, scoringRule, startedAt,
	)
	if err != nil {
		return nil, err
	}
	if status == string(domain.StatusPassed) || status == string(domain.StatusFailed) {
		if err := run.CompleteAt(thresholdResults, summary, *completedAt); err != nil {
			return nil, err
		}
	}
	return run, nil
}
