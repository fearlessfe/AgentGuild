package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// ContentStore is a content-addressed object store used by MCP tools to persist
// large artefacts such as agent version snapshots or experience evidence. The
// initial implementation is in-memory; production deployments can replace it
// with an object-storage adapter.
type ContentStore interface {
	// Get returns the bytes for the given content reference.
	// References use the form "sha256:<hex>".
	Get(ref string) ([]byte, error)
	// Put stores content and returns a content reference of the form
	// "sha256:<hex>".
	Put(content []byte) (string, error)
}

// MemoryContentStore is an in-memory ContentStore for tests and local
// development. It is not safe for use across processes.
type MemoryContentStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// NewMemoryContentStore creates an empty MemoryContentStore.
func NewMemoryContentStore() *MemoryContentStore {
	return &MemoryContentStore{data: make(map[string][]byte)}
}

// Get returns content by reference.
func (s *MemoryContentStore) Get(ref string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	content, ok := s.data[ref]
	if !ok {
		return nil, fmt.Errorf("content not found: %s", ref)
	}
	return content, nil
}

// Put stores content and returns its sha256 reference.
func (s *MemoryContentStore) Put(content []byte) (string, error) {
	sum := sha256.Sum256(content)
	ref := "sha256:" + hex.EncodeToString(sum[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[ref] = content
	return ref, nil
}

var _ ContentStore = (*MemoryContentStore)(nil)
