package rest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	canonicaljson "github.com/gibson042/canonicaljson-go"
)

const (
	mutationIdempotencyTTL    = 24 * time.Hour
	mutationCompletionTimeout = 5 * time.Second
)

type mutationIdempotencyStore interface {
	AcquireIdempotency(context.Context, application.IdempotencyKey, [32]byte, time.Time) (*application.IdempotencyRecord, error)
	CompleteIdempotency(context.Context, application.IdempotencyKey, string, int, []byte) error
}

func (s *Server) mutationIdempotency(operation string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := mustPrincipal(r)
			requestID := r.Header.Get("Idempotency-Key")
			if requestID == "" {
				writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "idempotency_key is required", "idempotency_key")
				return
			}
			if s.idempotencyStore == nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
				return
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			key := application.IdempotencyKey{
				TenantID: principal.TenantID, ActorID: principal.OwnerID,
				Operation: operation, RequestID: requestID,
			}
			record, err := s.idempotencyStore.AcquireIdempotency(r.Context(), key, CanonicalMutationRequestHash(r.Method, r.URL.Path, body), time.Now().Add(mutationIdempotencyTTL))
			if err != nil {
				mapDomainError(w, err, principal)
				return
			}
			if record.Completed {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(*record.ResponseCode)
				_, _ = w.Write(record.ResponseBody)
				return
			}
			if !record.Acquired {
				mapDomainError(w, &domain.Error{Code: "idempotency_in_progress", Message: "an idempotent request is already in progress"}, principal)
				return
			}

			recorder := newBufferedResponse()
			next.ServeHTTP(recorder, r)
			if recorder.status < http.StatusInternalServerError {
				completionContext, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), mutationCompletionTimeout)
				err := s.idempotencyStore.CompleteIdempotency(completionContext, key, record.OwnerToken, recorder.status, recorder.body.Bytes())
				cancel()
				if err != nil {
					writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
					return
				}
			}
			recorder.copyTo(w)
		})
	}
}

// CanonicalMutationRequestHash makes insignificant JSON whitespace and key
// ordering irrelevant while binding a key to the exact method and resource.
func CanonicalMutationRequestHash(method, path string, body []byte) [32]byte {
	canonicalBody := body
	if len(bytes.TrimSpace(body)) == 0 {
		canonicalBody = []byte("null")
	} else {
		var value any
		if json.Unmarshal(body, &value) == nil {
			if encoded, err := canonicaljson.Marshal(value); err == nil {
				canonicalBody = encoded
			}
		}
	}
	payload := append([]byte(method+"\n"+path+"\n"), canonicalBody...)
	return sha256.Sum256(payload)
}

type bufferedResponse struct {
	header http.Header
	status int
	wrote  bool
	body   bytes.Buffer
}

func newBufferedResponse() *bufferedResponse {
	return &bufferedResponse{header: make(http.Header), status: http.StatusOK}
}

func (r *bufferedResponse) Header() http.Header { return r.header }
func (r *bufferedResponse) WriteHeader(status int) {
	if r.wrote {
		return
	}
	r.status = status
	r.wrote = true
}
func (r *bufferedResponse) Write(body []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	return r.body.Write(body)
}
func (r *bufferedResponse) copyTo(w http.ResponseWriter) {
	for key, values := range r.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(r.status)
	_, _ = w.Write(r.body.Bytes())
}
