package application

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
)

type Options struct {
	CursorSecret []byte
	CursorTTL    time.Duration
	NewID        func() string
}

type Service struct {
	store        Store
	policy       auth.ScopePolicy
	cursorSecret []byte
	cursorTTL    time.Duration
	newID        func() string
}

func NewService(store Store, options Options) *Service {
	if options.CursorTTL <= 0 {
		options.CursorTTL = 15 * time.Minute
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	return &Service{store: store, cursorSecret: append([]byte(nil), options.CursorSecret...), cursorTTL: options.CursorTTL, newID: options.NewID}
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
