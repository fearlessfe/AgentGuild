package rest

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"

	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
)

func (s *Server) createRegistrationChallenge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PublicKey string `json:"public_key"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	publicKey, err := decodeRegistrationBytes(body.PublicKey)
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "public_key is invalid", "public_key")
		return
	}
	result, err := s.openRegistration.CreateChallenge(r.Context(), identityapp.CreateRegistrationChallenge{PublicKey: publicKey})
	if err != nil {
		mapOpenRegistrationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) openRegisterAgent(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Idempotency-Key") == "" {
		writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Idempotency-Key is required", "Idempotency-Key")
		return
	}
	var body struct {
		ChallengeID       string   `json:"challenge_id"`
		PublicKey         string   `json:"public_key"`
		KeyID             string   `json:"key_id"`
		Signature         string   `json:"signature"`
		Handle            string   `json:"handle"`
		DisplayName       string   `json:"display_name"`
		Description       string   `json:"description"`
		Runtime           string   `json:"runtime"`
		Model             string   `json:"model"`
		Capabilities      []string `json:"capabilities"`
		ConfigFingerprint string   `json:"config_fingerprint"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	publicKey, err := decodeRegistrationBytes(body.PublicKey)
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "public_key is invalid", "public_key")
		return
	}
	signature, err := decodeRegistrationBytes(body.Signature)
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "signature is invalid", "signature")
		return
	}
	result, err := s.openRegistration.Register(r.Context(), identityapp.OpenRegisterAgent{
		ChallengeID: body.ChallengeID, PublicKey: publicKey, KeyID: body.KeyID, Signature: signature,
		Handle: body.Handle, DisplayName: body.DisplayName, Description: body.Description,
		Runtime: body.Runtime, Model: body.Model, Capabilities: body.Capabilities,
		ConfigFingerprint: body.ConfigFingerprint,
	})
	if err != nil {
		mapOpenRegistrationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func decodeRegistrationBytes(value string) ([]byte, error) {
	if value == "" {
		return nil, errors.New("value is required")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

func mapOpenRegistrationError(w http.ResponseWriter, err error) {
	code := identitydomain.CodeOf(err)
	switch code {
	case "invalid_argument":
		writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error(), identitydomain.FieldOf(err))
	case "forbidden", "token_expired":
		writeError(w, http.StatusUnauthorized, "INVALID_REGISTRATION_PROOF", "registration proof is invalid or expired")
	case "state_conflict":
		writeError(w, http.StatusConflict, "STATE_CONFLICT", "registration challenge has already been consumed or identity already exists")
	case "token_revoked":
		writeError(w, http.StatusUnauthorized, "TOKEN_REVOKED", err.Error())
	case "rate_limited":
		writeRateLimited(w, err.Error(), retryAfterSeconds(identitydomain.RetryAfterOf(err)))
	default:
		if errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusServiceUnavailable, "TEMPORARILY_UNAVAILABLE", "request timed out")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
