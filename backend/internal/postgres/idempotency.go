package postgres

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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

func (tx *Tx) AcquireIdempotency(
	ctx context.Context,
	key application.IdempotencyKey,
	requestHash [32]byte,
	expiresAt time.Time,
) (*application.IdempotencyRecord, error) {
	ownerToken := tx.acquiredIdempotency[key]
	if ownerToken == "" {
		var err error
		ownerToken, err = newOwnerToken()
		if err != nil {
			return nil, err
		}
	}
	now, err := tx.Now(ctx)
	if err != nil {
		return nil, err
	}
	_, err = tx.tx.Exec(ctx, `
		INSERT INTO idempotency_records (
			tenant_id, actor_id, operation, request_id, request_hash, owner_token,
			expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		ON CONFLICT (tenant_id, actor_id, operation, request_id) DO NOTHING`,
		key.TenantID, key.ActorID, key.Operation, key.RequestID, requestHash[:],
		ownerToken, expiresAt, now,
	)
	if err != nil {
		return nil, err
	}

	record := &application.IdempotencyRecord{Key: key}
	var storedHash []byte
	var storedOwner string
	err = tx.tx.QueryRow(ctx, `
		SELECT request_hash, response_code, response_body, expires_at, owner_token
		FROM idempotency_records
		WHERE tenant_id=$1 AND actor_id=$2 AND operation=$3 AND request_id=$4
		FOR UPDATE`,
		key.TenantID, key.ActorID, key.Operation, key.RequestID,
	).Scan(
		&storedHash, &record.ResponseCode, &record.ResponseBody, &record.ExpiresAt,
		&storedOwner,
	)
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
	record.Completed = record.ResponseCode != nil
	if !record.Completed && storedOwner == ownerToken {
		record.OwnerToken = ownerToken
		record.Acquired = true
		tx.acquiredIdempotency[key] = ownerToken
	}
	return record, nil
}

func (tx *Tx) CompleteIdempotency(
	ctx context.Context,
	key application.IdempotencyKey,
	ownerToken string,
	responseCode int,
	responseBody []byte,
) error {
	now, err := tx.Now(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.tx.Exec(ctx, `
		UPDATE idempotency_records
		SET response_code=$5, response_body=$6, updated_at=$7
		WHERE tenant_id=$1 AND actor_id=$2 AND operation=$3 AND request_id=$4
		  AND owner_token=$8 AND response_code IS NULL`,
		key.TenantID, key.ActorID, key.Operation, key.RequestID,
		responseCode, responseBody, now, ownerToken,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return &domain.Error{
			Code:    "idempotency_not_owner",
			Message: "idempotency record is not pending for this owner",
		}
	}
	delete(tx.acquiredIdempotency, key)
	return nil
}

func newOwnerToken() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}
