package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func generateID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// rand.Read only fails in exceptional circumstances; panic to satisfy the
		// domain constructor contract and surface the failure immediately.
		panic(fmt.Sprintf("failed to generate id: %v", err))
	}
	return prefix + "_" + hex.EncodeToString(b)
}
