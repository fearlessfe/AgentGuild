package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	canonicaljson "github.com/gibson042/canonicaljson-go"
)

func CanonicalHash(value any) ([32]byte, error) {
	payload, err := canonicaljson.Marshal(value)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(payload), nil
}

func (tx *Tx) LockIdempotency(
	ctx context.Context,
	key application.IdempotencyKey,
	requestHash [32]byte,
	expiresAt time.Time,
) (*application.IdempotencyRecord, error) {
	_, err := tx.tx.Exec(ctx, `
		INSERT INTO idempotency_records (
			tenant_id, actor_id, operation, request_id, request_hash, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id, actor_id, operation, request_id) DO NOTHING`,
		key.TenantID, key.ActorID, key.Operation, key.RequestID, requestHash[:], expiresAt,
	)
	if err != nil {
		return nil, err
	}

	record := &application.IdempotencyRecord{Key: key}
	var storedHash []byte
	err = tx.tx.QueryRow(ctx, `
		SELECT request_hash, response_code, response_body, expires_at
		FROM idempotency_records
		WHERE tenant_id=$1 AND actor_id=$2 AND operation=$3 AND request_id=$4
		FOR UPDATE`,
		key.TenantID, key.ActorID, key.Operation, key.RequestID,
	).Scan(&storedHash, &record.ResponseCode, &record.ResponseBody, &record.ExpiresAt)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(storedHash, requestHash[:]) {
		return nil, &domain.Error{
			Code:    "idempotency_mismatch",
			Message: "idempotency key was already used with a different request",
		}
	}
	copy(record.RequestHash[:], storedHash)
	return record, nil
}

func (tx *Tx) SaveIdempotencyResponse(
	ctx context.Context,
	key application.IdempotencyKey,
	responseCode int,
	responseBody []byte,
	now time.Time,
) error {
	tag, err := tx.tx.Exec(ctx, `
		UPDATE idempotency_records
		SET response_code=$5, response_body=$6, updated_at=$7
		WHERE tenant_id=$1 AND actor_id=$2 AND operation=$3 AND request_id=$4`,
		key.TenantID, key.ActorID, key.Operation, key.RequestID,
		responseCode, responseBody, now,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return notFound("idempotency record")
	}
	return nil
}
