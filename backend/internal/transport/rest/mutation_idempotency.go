package rest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	canonicaljson "github.com/gibson042/canonicaljson-go"
)

const (
	mutationIdempotencyTTL       = 24 * time.Hour
	mutationCompletionTimeout    = 5 * time.Second
	mutationHeartbeatInterval    = time.Minute
	mutationIdempotencyIOTimeout = 5 * time.Second
)

type mutationIdempotencyStore interface {
	AcquireMutationIdempotency(context.Context, application.IdempotencyKey, [32]byte, time.Duration) (*application.IdempotencyRecord, error)
	RenewIdempotency(context.Context, application.IdempotencyKey, string) error
	CompleteIdempotency(context.Context, application.IdempotencyKey, string, int, []byte) error
}

func (s *Server) mutationIdempotency(operation string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := mustPrincipal(r)
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
			requestID := r.Header.Get("Idempotency-Key")
			if requestID == "" {
				requestID = mutationBodyRequestID(body)
			}
			if requestID == "" {
				writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "idempotency_key is required", "idempotency_key")
				return
			}
			key := application.IdempotencyKey{
				TenantID: principal.TenantID, ActorID: principal.OwnerID,
				Operation: operation, RequestID: requestID,
			}
			record, err := s.idempotencyStore.AcquireMutationIdempotency(r.Context(), key, CanonicalMutationRequestHash(r.Method, r.URL.Path, body), mutationIdempotencyTTL)
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

			handlerContext, cancelHandler := context.WithCancel(r.Context())
			heartbeat := startMutationHeartbeat(
				r.Context(), s.idempotencyStore, key, record.OwnerToken,
				s.idempotencyHeartbeat, s.idempotencyIOTimeout, cancelHandler,
			)
			defer func() {
				cancelHandler()
				_ = heartbeat.stopAndWait()
			}()
			recorder := newBufferedResponse()
			next.ServeHTTP(recorder, r.WithContext(handlerContext))
			heartbeatErr := heartbeat.stopAndWait()
			cancelHandler()
			if heartbeatErr != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
				return
			}
			if recorder.status < http.StatusInternalServerError {
				completionTimeout := mutationCompletionTimeout
				if s.idempotencyIOTimeout < completionTimeout {
					completionTimeout = s.idempotencyIOTimeout
				}
				completionContext, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), completionTimeout)
				err := s.idempotencyStore.CompleteIdempotency(completionContext, key, record.OwnerToken, recorder.status, recorder.body.Bytes())
				cancel()
				if err != nil {
					// The heartbeat has stopped, so a failed completion remains
					// pending only until the last renewal ages past the takeover
					// lease (or the overall retention expires).
					writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
					return
				}
			}
			recorder.copyTo(w)
		})
	}
}

func mutationBodyRequestID(body []byte) string {
	if len(bytes.TrimSpace(body)) == 0 {
		return ""
	}
	var payload struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return payload.RequestID
}

type mutationHeartbeat struct {
	stopOnce sync.Once
	stop     context.CancelFunc
	done     chan struct{}
	err      error
}

func startMutationHeartbeat(
	requestContext context.Context,
	store mutationIdempotencyStore,
	key application.IdempotencyKey,
	ownerToken string,
	interval time.Duration,
	ioTimeout time.Duration,
	cancelHandler context.CancelFunc,
) *mutationHeartbeat {
	lifecycle, stop := context.WithCancel(context.WithoutCancel(requestContext))
	heartbeat := &mutationHeartbeat{stop: stop, done: make(chan struct{})}
	go func() {
		defer close(heartbeat.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		heartbeat.err = runMutationHeartbeat(lifecycle, store, key, ownerToken, ticker.C, ioTimeout, cancelHandler)
	}()
	return heartbeat
}

func runMutationHeartbeat(
	lifecycle context.Context,
	store mutationIdempotencyStore,
	key application.IdempotencyKey,
	ownerToken string,
	ticks <-chan time.Time,
	ioTimeout time.Duration,
	cancelHandler context.CancelFunc,
) error {
	for {
		select {
		case <-lifecycle.Done():
			return nil
		case <-ticks:
			err := renewMutationHeartbeat(lifecycle, store, key, ownerToken, ioTimeout)
			if err != nil {
				cancelHandler()
				return err
			}
		}
	}
}

func renewMutationHeartbeat(
	lifecycle context.Context,
	store mutationIdempotencyStore,
	key application.IdempotencyKey,
	ownerToken string,
	ioTimeout time.Duration,
) error {
	// Stop and tick may become ready together. Recheck immediately before
	// starting I/O so normal shutdown wins that race.
	if lifecycle.Err() != nil {
		return nil
	}
	renewContext, cancel := context.WithTimeout(lifecycle, ioTimeout)
	err := store.RenewIdempotency(renewContext, key, ownerToken)
	cancel()
	if err == nil {
		return nil
	}
	// Only cancellation produced by normal heartbeat shutdown is benign.
	// Storage and owner-fencing errors must remain failures even when stop
	// happens concurrently.
	if lifecycle.Err() != nil && errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (h *mutationHeartbeat) stopAndWait() error {
	h.stopOnce.Do(h.stop)
	<-h.done
	return h.err
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
