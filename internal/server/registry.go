package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

const MaxRegisteredSources = 8

// Registry stores normalized, read-only documents for the lifetime of the
// local daemon. Persistent indexing replaces this in Milestone 2 without
// changing the browser's stable source IDs.
type Registry struct {
	mu        sync.RWMutex
	documents map[string]Document
	paths     map[string]string
	order     []string
}

func NewRegistry() *Registry {
	return &Registry{documents: make(map[string]Document), paths: make(map[string]string)}
}

// SourceID is stable for a resolved source path across daemon restarts.
func SourceID(path string) string {
	digest := sha256.Sum256([]byte(path))
	return hex.EncodeToString(digest[:10])
}

func (registry *Registry) Put(path string, document Document) string {
	return registry.PutWithIdentity(path, path, document)
}

// PutWithIdentity keeps the user-facing source path separate from the stable
// identity. Adapter-backed views include the adapter in identity so opening a
// second interpretation of one source cannot replace the first tab's data.
func (registry *Registry) PutWithIdentity(identity, path string, document Document) string {
	id := SourceID(identity)
	registry.mu.Lock()
	registry.removeFromOrder(id)
	registry.documents[id] = document
	registry.paths[id] = path
	registry.order = append(registry.order, id)
	for len(registry.order) > MaxRegisteredSources {
		evicted := registry.order[0]
		registry.order = registry.order[1:]
		delete(registry.documents, evicted)
		delete(registry.paths, evicted)
	}
	registry.mu.Unlock()
	return id
}

func (registry *Registry) Get(id string) (Document, bool) {
	registry.mu.Lock()
	document, ok := registry.documents[id]
	if ok {
		registry.removeFromOrder(id)
		registry.order = append(registry.order, id)
	}
	registry.mu.Unlock()
	return document, ok
}

func (registry *Registry) removeFromOrder(id string) {
	for index, existing := range registry.order {
		if existing == id {
			registry.order = append(registry.order[:index], registry.order[index+1:]...)
			return
		}
	}
}

func (registry *Registry) Path(id string) (string, bool) {
	registry.mu.RLock()
	path, ok := registry.paths[id]
	registry.mu.RUnlock()
	return path, ok
}

func (registry *Registry) Only() (Document, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if len(registry.documents) != 1 {
		return Document{}, false
	}
	for _, document := range registry.documents {
		return document, true
	}
	return Document{}, false
}

func (registry *Registry) Count() int {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return len(registry.documents)
}

func (registry *Registry) Require(id string) (Document, error) {
	if id == "" {
		if document, ok := registry.Only(); ok {
			return document, nil
		}
		return Document{}, fmt.Errorf("trajectory query parameter is required")
	}
	document, ok := registry.Get(id)
	if !ok {
		return Document{}, fmt.Errorf("trajectory %q is not registered", id)
	}
	return document, nil
}
