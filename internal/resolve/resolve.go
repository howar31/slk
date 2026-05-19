// Package resolve maps Slack IDs to human-readable names with a local cache.
package resolve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Lookup fetches the display name for an ID (user or channel).
type Lookup interface {
	Name(id string) (string, error)
}

// Resolver caches ID->name lookups on disk.
type Resolver struct {
	path   string
	lookup Lookup
	mu     sync.Mutex
	cache  map[string]string
}

// New returns a Resolver backed by cachePath and the given Lookup.
func New(cachePath string, lookup Lookup) *Resolver {
	r := &Resolver{path: cachePath, lookup: lookup, cache: map[string]string{}}
	if data, err := os.ReadFile(cachePath); err == nil {
		json.Unmarshal(data, &r.cache)
	}
	return r
}

// Resolve returns the name for id, or id itself if lookup fails.
func (r *Resolver) Resolve(id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name, ok := r.cache[id]; ok {
		return name
	}
	name, err := r.lookup.Name(id)
	if err != nil {
		return id // graceful degradation
	}
	r.cache[id] = name
	r.flush()
	return name
}

func (r *Resolver) flush() {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return
	}
	data, err := json.Marshal(r.cache)
	if err != nil {
		return
	}
	os.WriteFile(r.path, data, 0o600)
}
