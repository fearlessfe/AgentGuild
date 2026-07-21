package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"agentguild.dev/agentguild/backend/internal/contribution/application"
	"agentguild.dev/agentguild/backend/internal/contribution/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProjectionRepository struct {
	pool *pgxpool.Pool
}

func NewProjectionRepository(pool *pgxpool.Pool) application.ProjectionRepository {
	return &ProjectionRepository{pool: pool}
}

func (r *ProjectionRepository) ReplaceAlgorithm(ctx context.Context, algorithmVersion string, set application.ProjectionSet) error {
	if algorithmVersion == "" {
		return domain.ErrInvalidArgument
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM agent_version_performance_projections WHERE algorithm_version=$1`, algorithmVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM agent_contribution_projections WHERE algorithm_version=$1`, algorithmVersion); err != nil {
		return err
	}
	for _, projection := range set.AgentLifetime {
		if projection.Scope != domain.ProjectionAgentLifetime || projection.AlgorithmVersion != algorithmVersion || projection.AgentID == "" || projection.AgentVersionID != "" {
			return domain.ErrInvalidArgument
		}
		if err := insertAgentProjection(ctx, tx, projection); err != nil {
			return err
		}
	}
	for _, projection := range set.AgentVersions {
		if projection.Scope != domain.ProjectionAgentVersion || projection.AlgorithmVersion != algorithmVersion || projection.AgentID == "" || projection.AgentVersionID == "" {
			return domain.ErrInvalidArgument
		}
		if err := insertVersionProjection(ctx, tx, projection); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *ProjectionRepository) GetAgentLifetime(ctx context.Context, agentID, algorithmVersion string) (*domain.Projection, error) {
	projection, err := scanAgentProjection(r.pool.QueryRow(ctx, `
		SELECT agent_id, algorithm_version,
		       attempt_count, ci_passed_count, ci_failed_count, reviewed_count,
		       changes_requested_count, approved_count, merged_count, closed_count,
		       reverted_count, issue_reopened_count, quality_sample_size,
		       sample_size_hint, stable_merge_rate, ci_pass_rate, approval_rate,
		       repository_distribution, version_distribution,
		       duplicate_task_attempts, self_owned_repository_count, without_independent_feedback,
		       reverted_after_merge, issue_reopened_after_merge,
		       dominant_repository_share, latest_event_id, calculated_at
		FROM agent_contribution_projections
		WHERE agent_id=$1 AND algorithm_version=$2`, agentID, algorithmVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return projection, err
}

func (r *ProjectionRepository) ListAgentVersions(ctx context.Context, agentID, algorithmVersion string) ([]domain.Projection, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT agent_id, agent_version_id, algorithm_version,
		       attempt_count, ci_passed_count, ci_failed_count, reviewed_count,
		       changes_requested_count, approved_count, merged_count, closed_count,
		       reverted_count, issue_reopened_count, quality_sample_size,
		       sample_size_hint, stable_merge_rate, ci_pass_rate, approval_rate,
		       repository_distribution,
		       duplicate_task_attempts, self_owned_repository_count, without_independent_feedback,
		       reverted_after_merge, issue_reopened_after_merge,
		       dominant_repository_share, latest_event_id, calculated_at
		FROM agent_version_performance_projections
		WHERE agent_id=$1 AND algorithm_version=$2
		ORDER BY agent_version_id`, agentID, algorithmVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projections := make([]domain.Projection, 0)
	for rows.Next() {
		projection, err := scanVersionProjection(rows)
		if err != nil {
			return nil, err
		}
		projections = append(projections, *projection)
	}
	return projections, rows.Err()
}

func insertAgentProjection(ctx context.Context, tx pgx.Tx, p domain.Projection) error {
	repositories, err := json.Marshal(p.RepositoryDistribution)
	if err != nil {
		return err
	}
	versions, err := json.Marshal(p.VersionDistribution)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO agent_contribution_projections (
			agent_id, algorithm_version, attempt_count, ci_passed_count,
			ci_failed_count, reviewed_count, changes_requested_count,
			approved_count, merged_count, closed_count, reverted_count,
			issue_reopened_count, quality_sample_size, sample_size_hint,
			stable_merge_rate, ci_pass_rate, approval_rate,
			repository_distribution, version_distribution,
			duplicate_task_attempts, self_owned_repository_count, without_independent_feedback,
			reverted_after_merge, issue_reopened_after_merge,
			dominant_repository_share, latest_event_id, calculated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,
			$18,$19,$20,$21,$22,$23,$24,$25,$26,$27
		)`, projectionValues(p, repositories, versions)...)
	return err
}

func insertVersionProjection(ctx context.Context, tx pgx.Tx, p domain.Projection) error {
	repositories, err := json.Marshal(p.RepositoryDistribution)
	if err != nil {
		return err
	}
	values := projectionValues(p, repositories, nil)
	values = append(values[:1], append([]any{p.AgentVersionID}, values[1:]...)...)
	_, err = tx.Exec(ctx, `
		INSERT INTO agent_version_performance_projections (
			agent_id, agent_version_id, algorithm_version, attempt_count,
			ci_passed_count, ci_failed_count, reviewed_count,
			changes_requested_count, approved_count, merged_count, closed_count,
			reverted_count, issue_reopened_count, quality_sample_size,
			sample_size_hint, stable_merge_rate, ci_pass_rate, approval_rate,
			repository_distribution, duplicate_task_attempts,
			self_owned_repository_count, without_independent_feedback, reverted_after_merge,
			issue_reopened_after_merge, dominant_repository_share,
			latest_event_id, calculated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,
			$18,$19,$20,$21,$22,$23,$24,$25,$26,$27
		)`, values...)
	return err
}

func projectionValues(p domain.Projection, repositories, versions []byte) []any {
	c := p.OutcomeCounts
	a := p.AntiGaming
	values := []any{
		p.AgentID, p.AlgorithmVersion, c.Attempts, c.CIPassed, c.CIFailed,
		c.Reviewed, c.ChangesRequested, c.Approved, c.Merged, c.Closed,
		c.Reverted, c.IssueReopened, p.QualitySampleSize, p.SampleSizeHint,
		p.StableMergeRate, p.CIPassRate, p.ApprovalRate, repositories,
	}
	if versions != nil {
		values = append(values, versions)
	}
	return append(values,
		a.DuplicateTaskAttempts, a.SelfOwnedRepositoryCount, a.WithoutIndependentFeedback,
		a.RevertedAfterMerge, a.IssueReopenedAfterMerge,
		a.DominantRepositoryShare, p.LatestEventID, p.CalculatedAt,
	)
}

type projectionScanner interface {
	Scan(...any) error
}

func scanAgentProjection(row projectionScanner) (*domain.Projection, error) {
	p := domain.Projection{Scope: domain.ProjectionAgentLifetime}
	var repositories, versions []byte
	err := row.Scan(
		&p.AgentID, &p.AlgorithmVersion,
		&p.OutcomeCounts.Attempts, &p.OutcomeCounts.CIPassed, &p.OutcomeCounts.CIFailed,
		&p.OutcomeCounts.Reviewed, &p.OutcomeCounts.ChangesRequested,
		&p.OutcomeCounts.Approved, &p.OutcomeCounts.Merged, &p.OutcomeCounts.Closed,
		&p.OutcomeCounts.Reverted, &p.OutcomeCounts.IssueReopened,
		&p.QualitySampleSize, &p.SampleSizeHint, &p.StableMergeRate,
		&p.CIPassRate, &p.ApprovalRate, &repositories, &versions,
		&p.AntiGaming.DuplicateTaskAttempts, &p.AntiGaming.SelfOwnedRepositoryCount, &p.AntiGaming.WithoutIndependentFeedback,
		&p.AntiGaming.RevertedAfterMerge, &p.AntiGaming.IssueReopenedAfterMerge,
		&p.AntiGaming.DominantRepositoryShare, &p.LatestEventID, &p.CalculatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(repositories, &p.RepositoryDistribution); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(versions, &p.VersionDistribution); err != nil {
		return nil, err
	}
	return &p, nil
}

func scanVersionProjection(row projectionScanner) (*domain.Projection, error) {
	p := domain.Projection{Scope: domain.ProjectionAgentVersion, VersionDistribution: map[string]int{}}
	var repositories []byte
	err := row.Scan(
		&p.AgentID, &p.AgentVersionID, &p.AlgorithmVersion,
		&p.OutcomeCounts.Attempts, &p.OutcomeCounts.CIPassed, &p.OutcomeCounts.CIFailed,
		&p.OutcomeCounts.Reviewed, &p.OutcomeCounts.ChangesRequested,
		&p.OutcomeCounts.Approved, &p.OutcomeCounts.Merged, &p.OutcomeCounts.Closed,
		&p.OutcomeCounts.Reverted, &p.OutcomeCounts.IssueReopened,
		&p.QualitySampleSize, &p.SampleSizeHint, &p.StableMergeRate,
		&p.CIPassRate, &p.ApprovalRate, &repositories,
		&p.AntiGaming.DuplicateTaskAttempts, &p.AntiGaming.SelfOwnedRepositoryCount, &p.AntiGaming.WithoutIndependentFeedback,
		&p.AntiGaming.RevertedAfterMerge, &p.AntiGaming.IssueReopenedAfterMerge,
		&p.AntiGaming.DominantRepositoryShare, &p.LatestEventID, &p.CalculatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(repositories, &p.RepositoryDistribution); err != nil {
		return nil, err
	}
	return &p, nil
}

var _ application.ProjectionRepository = (*ProjectionRepository)(nil)
