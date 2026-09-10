package postgres

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/reward/domain"
	"github.com/jackc/pgx/v5"
)

type destinationRepository struct{ q queryer }

const destinationColumns = `
	id, agent_id, chain, address, recipient_ref, status,
	created_at, verified_at, revoked_at`

func (r *destinationRepository) InsertChallenge(ctx context.Context, challenge *domain.Challenge) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO payout_destination_challenges (
			nonce, agent_id, chain, address, issued_at, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6)`,
		challenge.Nonce, challenge.AgentID, challenge.Chain, challenge.Address,
		challenge.IssuedAt, challenge.ExpiresAt,
	)
	return writeError(err)
}

// ConsumeChallenge 一次性消费 nonce。
//
// 消费在**单条条件 UPDATE** 里完成：`consumed_at IS NULL` 是原子的，
// 先读后写会给并发重放留出窗口。
func (r *destinationRepository) ConsumeChallenge(ctx context.Context, nonce, agentID, chain, address string, now time.Time) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE payout_destination_challenges
		SET consumed_at=$6
		WHERE nonce=$1 AND agent_id=$2 AND chain=$3 AND address=$4
		  AND consumed_at IS NULL AND expires_at > $5`,
		nonce, agentID, chain, address, now, now,
	)
	if err != nil {
		return writeError(err)
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrStateConflict
	}
	return nil
}

func (r *destinationRepository) Insert(ctx context.Context, destination *domain.Destination) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO payout_destinations (`+destinationColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		destination.ID, destination.AgentID, destination.Chain, destination.Address,
		destination.RecipientRef, string(destination.Status), destination.CreatedAt,
		destination.VerifiedAt, destination.RevokedAt,
	)
	return writeError(err)
}

func (r *destinationRepository) GetActive(ctx context.Context, agentID string) (*domain.Destination, error) {
	rows, err := r.q.Query(ctx, `
		SELECT`+destinationColumns+`
		FROM payout_destinations
		WHERE agent_id=$1 AND status='verified'`, agentID)
	if err != nil {
		return nil, err
	}
	destinations, err := scanDestinations(rows)
	if err != nil {
		return nil, err
	}
	if len(destinations) == 0 {
		return nil, domain.ErrNotFound
	}
	return &destinations[0], nil
}

func (r *destinationRepository) ListByAgent(ctx context.Context, agentID string) ([]domain.Destination, error) {
	rows, err := r.q.Query(ctx, `
		SELECT`+destinationColumns+`
		FROM payout_destinations
		WHERE agent_id=$1
		ORDER BY created_at DESC, id`, agentID)
	if err != nil {
		return nil, err
	}
	return scanDestinations(rows)
}

func (r *destinationRepository) Save(ctx context.Context, destination *domain.Destination) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE payout_destinations
		SET status=$2, verified_at=$3, revoked_at=$4
		WHERE id=$1`,
		destination.ID, string(destination.Status),
		destination.VerifiedAt, destination.RevokedAt,
	)
	if err != nil {
		return writeError(err)
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}

func scanDestinations(rows pgx.Rows) ([]domain.Destination, error) {
	defer rows.Close()
	var destinations []domain.Destination
	for rows.Next() {
		var destination domain.Destination
		var status string
		if err := rows.Scan(
			&destination.ID, &destination.AgentID, &destination.Chain,
			&destination.Address, &destination.RecipientRef, &status,
			&destination.CreatedAt, &destination.VerifiedAt, &destination.RevokedAt,
		); err != nil {
			return nil, err
		}
		destination.Status = domain.DestinationStatus(status)
		destinations = append(destinations, destination)
	}
	return destinations, rows.Err()
}
